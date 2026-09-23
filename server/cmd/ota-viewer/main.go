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
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"ota-viewer/internal/discovery"
	"ota-viewer/internal/dvr"
	"ota-viewer/internal/httpapi"
	"ota-viewer/internal/live"
	"ota-viewer/internal/realtime"
	"ota-viewer/internal/source"
	"ota-viewer/internal/store"
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
	hub := live.New(st, work, ffmpegPath, encoder)
	hub.OnSaved = func(rec store.Recording) {
		dvr.OnSaved(context.Background(), st, hub, rec)
	}
	log.Printf("encoder: %s deint: %s smooth: %s blend: %v", encoder, hub.DeintBroadcast, hub.DeintSmooth, hub.Blend)
	bus := realtime.NewBus()
	st.OnEvent = func(ev store.Event) { bus.Publish("activity", ev) }
	hub.OnChange = debounce(500*time.Millisecond, func() { bus.Publish("live.changed", nil) })
	api := &httpapi.Server{Store: st, Assets: assets, Dev: *dev, Hub: hub, Version: version, Bus: bus}
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
		refreshGuide(api)
		tick := time.NewTicker(guideRefreshEvery)
		defer tick.Stop()
		for range tick.C {
			refreshGuide(api)
		}
	}()
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for range tick.C {
			dvr.Tick(context.Background(), st, hub)
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
	log.Printf("OTA Viewer listening on %s", *addr)
	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// Listings shift through the day (late news, sports overruns), so the guide is re-read regularly.
const guideRefreshEvery = 4 * time.Hour

func refreshGuide(api *httpapi.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	count, err := api.RefreshGuide(ctx)
	if err != nil {
		log.Printf("guide: %v", err)
		return
	}
	log.Printf("guide: %d airings", count)
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
