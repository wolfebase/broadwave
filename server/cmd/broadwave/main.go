package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	// The zone database, so TZ works on a host without zone files.
	_ "time/tzdata"

	"broadwave/internal/backup"
	"broadwave/internal/cloudflare"
	"broadwave/internal/discovery"
	"broadwave/internal/doctor"
	"broadwave/internal/dvr"
	"broadwave/internal/guide"
	"broadwave/internal/httpapi"
	"broadwave/internal/live"
	"broadwave/internal/logbuf"
	"broadwave/internal/psip"
	"broadwave/internal/realtime"
	"broadwave/internal/region"
	"broadwave/internal/source"
	"broadwave/internal/sports"
	"broadwave/internal/store"
	"broadwave/internal/tunnel"
	"broadwave/internal/update"
	"broadwave/internal/updatecheck"
)

//go:embed all:assets
var embedded embed.FS

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	addr := flag.String("addr", ":8477", "listen address")
	configDir := flag.String("config", "data", "directory for the catalog database")
	dev := flag.Bool("dev", false, "allow a local Vite dev server to call the API")
	recordings := flag.String("recordings", os.Getenv("BROADWAVE_RECORDINGS"), "folder for recordings (default <config>/work/recordings)")
	hdhrHost := flag.String("hdhr", os.Getenv("HDHR_HOST"), "tuner address when the container cannot hear broadcast discovery")
	bonjour := flag.Bool("bonjour", true, "advertise this server to the apps over Bonjour")
	healthcheck := flag.Bool("healthcheck", false, "check a running server on -addr and exit (for container health checks)")
	updateCheck := flag.Bool("update-check", false, "exit 75 when a running server on -addr is busy (a tuner in use, or a recording within 2 h), 0 when it may update")
	staging := flag.Bool("staging", false, "test copy beside a real server: never record, scan, or pull the guide; tune only when someone watches")
	resetSignin := flag.Bool("reset-signin", envOn(os.Getenv(resetSignInEnv)), "turn device sign-in off at start, when every admin device is lost (or set "+resetSignInEnv+"=1)")
	flag.Parse()
	addrGiven := false
	flag.Visit(func(f *flag.Flag) { addrGiven = addrGiven || f.Name == "addr" })
	if *healthcheck {
		os.Exit(checkHealth(localBase(checkAddr(*addr, addrGiven))))
	}
	if *updateCheck {
		code, why := updatecheck.Run(localBase(checkAddr(*addr, addrGiven)), time.Now())
		fmt.Println(why)
		os.Exit(code)
	}
	logbuf.Install(os.Stderr)
	warnZone(region.Local())
	if err := doctor.ApplyIdentity(*configDir, recordingsDir(*configDir, *recordings)); err != nil {
		slog.Error(fmt.Sprintf("identity: %v", err))
		os.Exit(1)
	}
	// Copy the catalog before Open migrates it, when this build is a new version.
	if err := backup.SnapshotIfVersionChanged(context.Background(), *configDir, version, time.Now()); err != nil {
		slog.Error(fmt.Sprintf("backup: %v", err))
	}

	st, err := store.Open(*configDir)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	defer st.Close()
	if *resetSignin {
		if err := resetSignIn(context.Background(), st); err != nil {
			slog.Error(fmt.Sprintf("reset-signin: %v", err))
			os.Exit(1)
		}
	}

	assets, err := fs.Sub(embedded, "assets/web")
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	work := filepath.Join(*configDir, "work")
	ffmpegPath, _ := execLook("ffmpeg")
	encoder := live.DetectEncoder(ffmpegPath)
	// BROADWAVE_BENCH=0 skips the startup encode. The relay check opens several
	// renditions at once, and a measured tile budget would refuse the extra ones.
	// BROADWAVE_ENCODER=gpu or software overrides the choice below.
	choice := strings.ToLower(strings.TrimSpace(os.Getenv("BROADWAVE_ENCODER")))
	if choice == "software" {
		encoder = "libx264"
	}
	live.UseLowPower(live.ProbeLowPower(ffmpegPath, encoder)...)
	var host live.Host
	if os.Getenv("BROADWAVE_BENCH") != "0" {
		// A home server's GPU is shared: Plex, Jellyfin, and Channels DVR
		// transcode on the same iGPU, and under their load a 1080i channel
		// encoded at 0.75x real time and every screen ran dry. A CPU that
		// holds a live 1080i encode with room to spare takes live TV off it.
		// A fixed-function encoder is not what they share, so it stays.
		if live.LowPower() {
			slog.Info("encoder: live TV runs on the GPU's low-power encoder, which other apps' transcodes do not slow")
		}
		if encoder != "libx264" && choice != "gpu" && !live.LowPower() {
			// Four seconds of picture at 2.5x take under two. A CPU still going
			// at eight is far too slow, and startup does not wait for it.
			swCtx, swCancel := context.WithTimeout(context.Background(), 8*time.Second)
			speed, err := live.BenchLive(swCtx, ffmpegPath)
			swCancel()
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				slog.Info("encoder: live TV stays on the GPU (the CPU is too slow for a live 1080i channel)")
			case err != nil:
				slog.Warn("encoder: live TV stays on the GPU; the CPU check failed: " + err.Error())
			case live.PreferSoftware(speed):
				slog.Info(fmt.Sprintf("encoder: live TV runs on the CPU (a 1080i channel at %.1fx) so other apps sharing the GPU cannot starve it", speed))
				encoder = "libx264"
			default:
				slog.Info(fmt.Sprintf("encoder: live TV stays on the GPU (the CPU runs a 1080i channel at %.1fx)", speed))
			}
		}
		measureCtx, measureCancel := context.WithTimeout(context.Background(), 20*time.Second)
		host = live.MeasureHost(measureCtx, ffmpegPath, encoder)
		measureCancel()
		slog.Info("encoder: " + host.Line())
	}
	// The e2e harness pins the measured speed, so a fast machine gets a slow one's budget.
	if speed, err := strconv.ParseFloat(os.Getenv("BROADWAVE_SPEED"), 64); err == nil && speed > 0 && os.Getenv("BROADWAVE_E2E") == "1" {
		pinned := live.Budget(host.Class, speed)
		pinned.Encoder = host.Encoder
		host = pinned
		slog.Info("encoder: pinned for e2e: " + host.Line())
	}
	live.Reap(work)
	hub := live.New(st, work, ffmpegPath, encoder)
	hub.RecordingsDir = recordingsDir(*configDir, *recordings)
	// Only a lineup with ATSC 3.0 sound needs the decoder; the first such
	// channel played says so too.
	if hub.NoAC4 = live.MissingAC4(ffmpegPath); hub.NoAC4 && hasAC4Channel(st) {
		slog.Warn("ffmpeg has no AC-4 decoder: ATSC 3.0 channels play without sound (the Docker image's ffmpeg has one)")
	}
	hub.Host = host
	hub.Buffer = time.Hour
	// Other sound tracks ride in each full-size encode, so a player can pick
	// them from master.m3u8 without a new encode. 0 keeps one sound per encode.
	hub.Alternates = os.Getenv("BROADWAVE_ALTERNATES") != "0"
	if *staging {
		slog.Info("staging: scheduled recordings and guide pulls, background tunes, and the tuner emulator are off; a recording or guide refresh you start still runs")
	} else if err := dvr.Recover(context.Background(), st, time.Now(), func(rec store.Recording, left time.Duration) error {
		// The pass, game, and episode carry over; a failed start here is
		// tried again by the pass tick for the time still left.
		if _, err := dvr.Resume(context.Background(), st, hub, rec, left); err != nil {
			slog.Error(fmt.Sprintf("recording: resume %s: %v", rec.Title, err))
		}
		return nil
	}); err != nil {
		slog.Error(fmt.Sprintf("recording: %v", err))
	}
	breakQueue := dvr.NewBreakQueue(st, hub)
	go breakQueue.Run(context.Background())
	hub.OnSaved = func(rec store.Recording) {
		dvr.OnSaved(context.Background(), st, hub, breakQueue, rec)
	}
	// A test harness shares one server across specs; nothing there deletes.
	if !*staging && os.Getenv("BROADWAVE_E2E") != "1" {
		hub.MakeRoom = func(ctx context.Context, need uint64) uint64 { return dvr.MakeRoom(ctx, st, hub, need) }
		go func() {
			time.Sleep(time.Minute)
			for {
				dvr.Refind(context.Background(), st, hub)
				dvr.Tidy(context.Background(), st, hub, time.Now())
				time.Sleep(time.Hour)
			}
		}()
	}
	slog.Info("encoder: " + encoderLine(encoder, hub.DeintBroadcast, hub.DeintSmooth))
	if err := hub.RecordingsReady(context.Background()); err != nil {
		slog.Warn(fmt.Sprintf("recordings: %s (%s)", err, hub.Recordings()))
	}
	bus := realtime.NewBus()
	bus.MediaStart = hub.EarliestMedia
	bus.LongHold = hub.LongGroups
	hub.OnLongGroups = bus.LongGroupsFound
	st.OnEvent = func(ev store.Event) { bus.Publish("activity", ev) }
	hub.OnChange = debounce(500*time.Millisecond, func() { bus.Publish("live.changed", nil) })
	hub.OnMedia = func(channelID int64) {
		if earliest, ok := hub.EarliestMedia(channelID); ok {
			for _, id := range hub.Twins(channelID) {
				bus.Settle(id, earliest)
			}
		}
	}
	api := &httpapi.Server{Store: st, Assets: assets, Dev: *dev, Hub: hub, Version: version, Bus: bus, Sports: sports.NewCache(sports.NewESPN()), Staging: *staging, BackupDir: filepath.Join(*configDir, "backups")}
	api.Updates = releaseCheck(st, version)
	api.Hosts = strings.Split(os.Getenv("BROADWAVE_HOSTS"), ",")
	api.CastAppID = os.Getenv("BROADWAVE_CAST_APP_ID")
	tunnels := tunnel.New()
	tunnels.OnChange = api.RemoteChanged
	api.Tunnel = tunnels
	api.Vault = cloudflare.NewVault(*configDir)
	api.Origin = httpapi.LoopbackOrigin(*addr)
	api.UseCloudflare(cloudflare.NewManager(nil))
	api.StartTunnel(context.Background())
	hub.OnPSIP = func(_ int, g psip.Guide) {
		n, err := api.ApplyBroadcast(context.Background(), g)
		if err != nil {
			slog.Error(fmt.Sprintf("guide: broadcast: %v", err))
			return
		}
		if n > 0 {
			slog.Debug(fmt.Sprintf("guide: broadcast filled %d listings", n))
		}
	}
	handler := api.Handler()
	bus.Versions = api.DataVersions
	go api.WatchData(context.Background(), 2*time.Second)
	// sources.found goes out when discovery found a different set, not every
	// five minutes: each one sent every app to refetch its tuners.
	found := foundOnce(st)

	go bootGuide(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		n, err := source.Auto(ctx, st, nil, *hdhrHost)
		if err != nil {
			// A tuner that is off at boot (a power cut) is found again later;
			// the guide keeps its own schedule meanwhile.
			slog.Error(fmt.Sprintf("discovery: %v", err))
			return
		}
		slog.Info(fmt.Sprintf("discovery: %d device(s)", n))
		if found(n) {
			bus.Publish("sources.found", map[string]int{"found": n})
		}
	}, func() {
		// Each listing source keeps its own schedule (SiliconDust asks for a
		// random 20-28 h gap). A restart waits out what was already stored.
		// The loop looks again at least every guideWake, so a source set up
		// while it slept is not left waiting a day.
		for !*staging {
			if wait := api.GuideDelay(time.Now()); wait > 0 {
				time.Sleep(min(wait, guideWake))
				continue
			}
			time.Sleep(refreshGuide(api))
		}
	})
	go func() {
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for range tick.C {
			// Old listings go even when the guide pull keeps failing, and the
			// query planner's statistics follow the catalog's shape.
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			_, _ = st.PruneAirings(ctx, time.Now())
			_ = st.Optimize(ctx)
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
			n, err := api.RefreshSources(ctx, time.Now())
			cancel()
			if err != nil {
				slog.Error(fmt.Sprintf("playlist refresh: %v", err))
				continue
			}
			if n > 0 {
				slog.Info(fmt.Sprintf("playlist refresh: %d", n))
			}
		}
	}()
	go func() {
		tick := time.NewTicker(5 * time.Minute)
		defer tick.Stop()
		for range tick.C {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			n, err := source.Auto(ctx, st, nil, *hdhrHost)
			cancel()
			if err != nil {
				slog.Error(fmt.Sprintf("discovery: %v", err))
				continue
			}
			if found(n) {
				bus.Publish("sources.found", map[string]int{"found": n})
			}
		}
	}()
	go api.WatchHome(context.Background())
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
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for range tick.C {
			api.GameAlerts(context.Background())
		}
	}()
	go func() {
		time.Sleep(20 * time.Second)
		if !*staging {
			api.BroadcastScan(context.Background())
		}
	}()
	// Idle tuners keep every channel's still recent (BROADWAVE_STILL_SWEEP=0, or the stillSweep setting, turns it off).
	if !*staging && os.Getenv("BROADWAVE_E2E") != "1" {
		hub.RecordingSoon = api.RecordingWithin
		go func() {
			time.Sleep(2 * time.Minute)
			hub.RunStillSweep(context.Background())
		}()
	}
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for range tick.C {
			if !*staging {
				dvr.Tick(context.Background(), st, hub)
				api.ExtendRecordings(context.Background())
			}
			if hub != nil {
				hub.ReleaseAbandoned(45 * time.Second)
				if !*staging {
					httpapi.SyncEmulator(st, hub, api.Hosts)
				}
			}
		}
	}()

	discKey, keyErr := discovery.LoadKey(filepath.Join(*configDir, "discovery.key"))
	if keyErr != nil {
		slog.Error(fmt.Sprintf("discovery key: %v", keyErr))
		discKey = nil
	} else {
		api.DiscoveryKey = discovery.PublicKeyString(discKey)
	}
	if id, err := st.Identity(context.Background(), httpapi.DefaultServerName()); err != nil {
		slog.Error(fmt.Sprintf("identity: %v", err))
	} else {
		if finder, err := discovery.ListenFinder(portOf(*addr), id.ID, id.Name, discKey); err != nil {
			slog.Error(fmt.Sprintf("discovery: %v", err))
		} else {
			defer finder.Close()
		}
		if *bonjour {
			if advert, err := discovery.Announce(discovery.Advert{ID: id.ID, Name: id.Name, Version: version, Port: portOf(*addr)}); err != nil {
				slog.Error(fmt.Sprintf("bonjour: %v", err))
			} else {
				defer advert.Shutdown()
			}
		}
	}
	slog.Info(fmt.Sprintf("Broadwave listening on %s", *addr))
	_ = os.WriteFile(addrFile(), []byte(*addr), 0o644)
	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	secure := startDirect(ctx, st, api, *configDir, *addr, handler)
	if api.Updates != nil {
		go api.Updates.Run(ctx)
	}
	go (&backup.Scheduler{Store: st, Dir: api.BackupDir}).Run(ctx)
	go func() {
		<-ctx.Done()
		hub.Shutdown()
		tunnels.Stop()
		shut, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if secure != nil {
			_ = secure.Shutdown(shut)
		}
		_ = server.Shutdown(shut)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func releaseCheck(st *store.Store, version string) *update.Checker {
	c := update.New(version)
	c.Enabled = func(ctx context.Context) bool {
		on, err := st.UpdatesEnabled(ctx)
		if err != nil {
			slog.Error(fmt.Sprintf("update: %v", err))
			return false
		}
		return on
	}
	c.Load = func(ctx context.Context) (*update.Notice, time.Time) {
		ver, notes, message, at, err := st.SavedUpdate(ctx)
		if err != nil {
			slog.Error(fmt.Sprintf("update: %v", err))
			return nil, time.Time{}
		}
		if ver == "" || message == "" {
			return nil, at
		}
		return &update.Notice{Version: ver, NotesURL: notes, Message: message}, at
	}
	c.Save = func(ctx context.Context, n *update.Notice, at time.Time) {
		ver, notes, message := "", "", ""
		if n != nil {
			ver, notes, message = n.Version, n.NotesURL, n.Message
		}
		if err := st.SaveUpdate(ctx, ver, notes, message, at); err != nil {
			slog.Error(fmt.Sprintf("update: %v", err))
		}
	}
	return c
}

// encoderLine names the encoder and the deinterlacer each picture mode uses.
// Without a working GPU deinterlacer both use bwdif on the CPU.
func encoderLine(encoder, broadcast, smooth string) string {
	if broadcast == "" {
		broadcast = "bwdif"
	}
	if smooth == "" {
		smooth = broadcast
	}
	line := encoder + ", deinterlace " + broadcast
	if smooth != broadcast {
		line += " (smooth: " + smooth + ")"
	}
	return line
}

// bootGuide looks for tuners once, then runs the guide loop whatever the
// look found: a tuner unreachable at boot must not stop listings for good.
func bootGuide(discover, loop func()) {
	discover()
	loop()
}

// guideWake is the longest the guide loop sleeps before it looks again.
const guideWake = 10 * time.Minute

// refreshGuide pulls the sources that are due and returns how long to wait
// before looking again. A source that failed has its own next try already;
// a pull that could not start at all (no tuner yet, a catalog error) waits
// half an hour.
func refreshGuide(api *httpapi.Server) time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, err := api.RefreshGuide(ctx)
	var sourceErr *httpapi.GuideSourceError
	switch {
	case err == nil:
	case errors.As(err, &sourceErr):
		slog.Error(fmt.Sprintf("guide: %v", err))
	case errors.Is(err, httpapi.ErrNoTuner):
		slog.Info("guide: no tuner yet; listings load once one is added")
		api.DeferGuide(ctx, guide.RetryAfterError)
		return guide.RetryAfterError
	default:
		slog.Error(fmt.Sprintf("guide: %v", err))
		api.DeferGuide(ctx, guide.RetryAfterError)
		return guide.RetryAfterError
	}
	// Never twice in one minute, whatever the schedule says.
	return time.Minute
}

func checkHealth(base string) int {
	client := http.Client{Timeout: 4 * time.Second}
	res, err := client.Get(base + "/api/v1/health")
	if err != nil {
		return 1
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// addrFile is where the running server writes its -addr, so -healthcheck and
// -update-check reach it when the container was started on another port.
func addrFile() string { return filepath.Join(os.TempDir(), "broadwave.addr") }

// checkAddr is -addr when it was given, else the address the running server wrote down.
func checkAddr(addr string, given bool) string {
	if given {
		return addr
	}
	if b, err := os.ReadFile(addrFile()); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return addr
}

// localBase is the URL a check on this machine uses for a server listening on addr.
func localBase(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		host, port = "", "8477"
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
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

// recordingsDir is the -recordings folder made absolute, or the one inside
// the config folder.
func recordingsDir(configDir, flagged string) string {
	if strings.TrimSpace(flagged) == "" {
		return filepath.Join(configDir, "work", "recordings")
	}
	if abs, err := filepath.Abs(flagged); err == nil {
		return abs
	}
	return flagged
}

// foundOnce reports a discovery pass that changed something: a different
// device count, or a lineup or tuner change the channels version counted.
func foundOnce(st *store.Store) func(n int) bool {
	var mu sync.Mutex
	lastN, lastV := -1, int64(-1)
	return func(n int) bool {
		v, err := st.DataVersions(context.Background())
		if err != nil {
			return true
		}
		mu.Lock()
		defer mu.Unlock()
		changed := n != lastN || v.Channels != lastV
		lastN, lastV = n, v.Channels
		return changed
	}
}

// warnZone says so at start when TZ is mistyped or missing: the guide, pass
// times, and recordings would quietly run on UTC.
func warnZone(zone region.Zone) {
	switch {
	case !zone.Valid:
		slog.Warn(fmt.Sprintf("time zone: TZ=%q is not a time zone, so this server runs on UTC and records at the wrong hour. Use a name like America/New_York.", zone.Name))
	case !zone.Set():
		slog.Warn("time zone: TZ is not set, so this server runs on UTC. Set TZ to your time zone, like America/New_York.")
	default:
		slog.Info("time zone: " + zone.Name)
	}
}

// hasAC4Channel reports a channel in the lineup with ATSC 3.0 sound.
func hasAC4Channel(st *store.Store) bool {
	channels, err := st.Channels(context.Background(), false)
	if err != nil {
		return false
	}
	for _, ch := range channels {
		if live.IsAC4(ch.AudioCodec) {
			return true
		}
	}
	return false
}
