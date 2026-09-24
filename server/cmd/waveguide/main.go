package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"waveguide/internal/discovery"
	"waveguide/internal/dvr"
	"waveguide/internal/guide"
	"waveguide/internal/httpapi"
	"waveguide/internal/live"
	"waveguide/internal/realtime"
	"waveguide/internal/source"
	"waveguide/internal/sports"
	"waveguide/internal/store"
)

//go:embed all:assets
var embedded embed.FS

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	addr := flag.String("addr", ":8477", "listen address")
	configDir := flag.String("config", "data", "directory for the catalog database")
	dev := flag.Bool("dev", false, "allow a local Vite dev server to call the API")
	hdhrHost := flag.String("hdhr", os.Getenv("HDHR_HOST"), "tuner address when the container cannot hear broadcast discovery")
	bonjour := flag.Bool("bonjour", true, "advertise this server to the apps over Bonjour")
	healthcheck := flag.Bool("healthcheck", false, "check a running server on -addr and exit (for container health checks)")
	flag.Parse()
	if *healthcheck {
		os.Exit(checkHealth(*addr))
	}

	st, err := store.Open(*configDir)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	assets, err := fs.Sub(embedded, "assets/web")
	if err != nil {
		log.Fatal(err)
	}
	work := filepath.Join(*configDir, "work")
	ffmpegPath, _ := execLook("ffmpeg")
	encoder := live.DetectEncoder(ffmpegPath)
	live.Reap(work)
	hub := live.New(st, work, ffmpegPath, encoder)
	if err := dvr.Recover(context.Background(), st, time.Now(), func(rec store.Recording, left time.Duration) error {
		minutes := int(left / time.Minute)
		if minutes < 1 {
			return nil
		}
		_, err := hub.RecordMeta(context.Background(), minutes, store.Recording{
			ChannelID: rec.ChannelID, Title: rec.Title, Subtitle: rec.Subtitle,
			Description: rec.Description, Category: rec.Category, ProgramID: rec.ProgramID, GameID: rec.GameID,
		})
		if err != nil {
			log.Printf("recording: resume %s: %v", rec.Title, err)
		}
		return nil
	}); err != nil {
		log.Printf("recording: %v", err)
	}
	hub.OnSaved = func(rec store.Recording) {
		dvr.OnSaved(context.Background(), st, hub, rec)
	}
	log.Printf("encoder: %s deint: %s smooth: %s blend: %v", encoder, hub.DeintBroadcast, hub.DeintSmooth, hub.Blend)
	bus := realtime.NewBus()
	st.OnEvent = func(ev store.Event) { bus.Publish("activity", ev) }
	hub.OnChange = debounce(500*time.Millisecond, func() { bus.Publish("live.changed", nil) })
	api := &httpapi.Server{Store: st, Assets: assets, Dev: *dev, Hub: hub, Version: version, Bus: bus, Sports: sports.NewCache(sports.NewESPN())}
	handler := api.Handler()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		n, err := source.Sync(ctx, st, nil, *hdhrHost)
		if err != nil {
			log.Printf("discovery: %v", err)
			return
		}
		log.Printf("discovery: %d device(s)", n)
		bus.Publish("sources.found", map[string]int{"found": n})
		// SiliconDust asks for a random 20-28 h gap after each successful pull.
		// A restart waits out whatever nextGuidePull was already stored.
		for {
			if wait := api.GuideDelay(time.Now()); wait > 0 {
				time.Sleep(wait)
				continue
			}
			refreshGuide(api)
		}
	}()
	go func() {
		tick := time.NewTicker(5 * time.Minute)
		defer tick.Stop()
		for range tick.C {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			n, err := source.Sync(ctx, st, nil, *hdhrHost)
			cancel()
			if err != nil {
				log.Printf("discovery: %v", err)
				continue
			}
			bus.Publish("sources.found", map[string]int{"found": n})
		}
	}()
	go func() {
		time.Sleep(20 * time.Second)
		tick := time.NewTicker(10 * time.Minute)
		defer tick.Stop()
		api.LinkGames(context.Background())
		api.NoteTeams(context.Background())
		for range tick.C {
			api.LinkGames(context.Background())
			api.NoteTeams(context.Background())
		}
	}()
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for range tick.C {
			dvr.Tick(context.Background(), st, hub)
			api.ExtendRecordings(context.Background())
			if hub != nil {
				hub.ReleaseAbandoned(45 * time.Second)
				httpapi.SyncEmulator(st, hub)
			}
		}
	}()

	if *bonjour {
		if id, err := st.Identity(context.Background(), httpapi.DefaultServerName()); err == nil {
			if advert, err := discovery.Announce(discovery.Advert{ID: id.ID, Name: id.Name, Version: version, Port: portOf(*addr)}); err != nil {
				log.Printf("bonjour: %v", err)
			} else {
				defer advert.Shutdown()
			}
		}
	}
	log.Printf("Waveguide listening on %s", *addr)
	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		hub.Shutdown()
		shut, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = server.Shutdown(shut)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func refreshGuide(api *httpapi.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := api.RefreshGuide(ctx); err != nil {
		log.Printf("guide: %v", err)
		api.DeferGuide(ctx, guide.RetryAfterError)
	}
}

func checkHealth(addr string) int {
	client := http.Client{Timeout: 4 * time.Second}
	res, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", portOf(addr)))
	if err != nil {
		return 1
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func portOf(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 8477
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return 8477
	}
	return n
}

// debounce collapses bursts of calls into one call after d of quiet.
func debounce(d time.Duration, fn func()) func() {
	var mu sync.Mutex
	var t *time.Timer
	return func() {
		mu.Lock()
		defer mu.Unlock()
		if t != nil {
			t.Stop()
		}
		t = time.AfterFunc(d, fn)
	}
}

func execLook(name string) (string, error) {
	return exec.LookPath(name)
}
