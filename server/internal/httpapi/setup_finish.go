package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/source"
	"broadwave/internal/store"
)

type finishStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type finishStatus struct {
	Running   bool         `json:"running"`
	Ready     string       `json:"ready,omitempty"`
	ChannelID int64        `json:"channelId,omitempty"`
	Steps     []finishStep `json:"steps"`
}

func (st finishStatus) clone() finishStatus {
	out := st
	out.Steps = append([]finishStep(nil), st.Steps...)
	return out
}

func newFinishStatus() finishStatus {
	return finishStatus{
		Running: true,
		Steps: []finishStep{
			{ID: "scan", Title: "Channels", State: "waiting"},
			{ID: "guide", Title: "Guide", State: "waiting"},
			{ID: "folder", Title: "Recordings", State: "waiting"},
			{ID: "favorites", Title: "Favorites", State: "waiting"},
			{ID: "encoder", Title: "Picture", State: "waiting"},
			{ID: "signal", Title: "Signal", State: "waiting"},
		},
	}
}

func (s *Server) postSetupFinish(w http.ResponseWriter, r *http.Request) {
	if !jsonRequest(r) {
		httpError(w, "Send setup as JSON.", http.StatusUnsupportedMediaType)
		return
	}
	s.finishMu.Lock()
	if s.finishOn {
		snap := s.finishRun.clone()
		s.finishMu.Unlock()
		writeJSON(w, http.StatusOK, snap)
		return
	}
	run := newFinishStatus()
	s.finishRun = &run
	s.finishOn = true
	snap := run.clone()
	s.finishMu.Unlock()
	go s.runSetupFinish()
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) getSetupFinish(w http.ResponseWriter, r *http.Request) {
	s.finishMu.Lock()
	defer s.finishMu.Unlock()
	if s.finishRun == nil {
		idle := newFinishStatus()
		idle.Running = false
		writeJSON(w, http.StatusOK, idle)
		return
	}
	snap := s.finishRun.clone()
	if !s.finishOn {
		snap.Running = false
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) runSetupFinish() {
	defer func() {
		s.finishMu.Lock()
		s.finishOn = false
		if s.finishRun != nil {
			s.finishRun.Running = false
		}
		s.finishMu.Unlock()
	}()
	ctx := context.Background()
	s.stepScan(ctx)
	s.stepGuide(ctx)
	s.stepFolder(ctx)
	s.stepFavorites(ctx)
	s.stepEncoder(ctx)
	s.stepSignal(ctx)
	ready, channelID := s.readyLine(ctx)
	s.finishMu.Lock()
	if s.finishRun != nil {
		s.finishRun.Ready = ready
		s.finishRun.ChannelID = channelID
		s.finishRun.Running = false
	}
	s.finishMu.Unlock()
	slog.Info(fmt.Sprintf("setup: %s", ready))
	// The boot harvest is a single pass. If setup was using the tuner then,
	// that pass returned. Start it again now that the signal check is done.
	// Playback still wins: the harvest stops when a tuner is no longer idle.
	if !s.Staging && s.Hub != nil {
		go s.BroadcastScan(context.Background())
	}
}

func (s *Server) setFinish(id, state, detail string) {
	s.finishMu.Lock()
	defer s.finishMu.Unlock()
	if s.finishRun == nil {
		return
	}
	for i := range s.finishRun.Steps {
		if s.finishRun.Steps[i].ID == id {
			s.finishRun.Steps[i].State = state
			s.finishRun.Steps[i].Detail = detail
			return
		}
	}
}

const setupScanLimit = 40 * time.Second

func (s *Server) setupScanWait() time.Duration {
	if s != nil && s.ScanWait > 0 {
		return s.ScanWait
	}
	return setupScanLimit
}

// The caller's context may already be done. Abort on its own deadline so the tuner still hears it.
func (s *Server) stopSetupScan(client *hdhr.Client, baseURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.AbortScan(ctx, baseURL); err != nil {
		slog.Error(fmt.Sprintf("setup: scan abort: %v", err))
	}
}

func (s *Server) stepScan(ctx context.Context) {
	s.setFinish("scan", "running", "Looking at the lineup.")
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		s.setFinish("scan", "done", "The lineup could not be read.")
		return
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		s.setFinish("scan", "done", "The lineup could not be read.")
		return
	}
	var tuner *store.Device
	for i := range devices {
		if devices[i].TunerCount > 0 {
			tuner = &devices[i]
			break
		}
	}
	visible := visibleChannels(channels)
	if tuner == nil {
		if len(visible) == 0 {
			s.setFinish("scan", "skipped", "No tuner yet.")
			return
		}
		s.setFinish("scan", "done", countLine(len(visible), "channel", "channels")+" in the lineup.")
		return
	}
	mine := 0
	for _, ch := range channels {
		if ch.DeviceID == tuner.DeviceID && ch.Present {
			mine++
		}
	}
	if mine > 0 {
		s.setFinish("scan", "done", countLine(len(visible), "channel", "channels")+" already in the lineup.")
		return
	}
	if strings.TrimSpace(tuner.BaseURL) == "" {
		s.setFinish("scan", "done", "This tuner has no address to scan.")
		return
	}
	if s.Staging {
		s.setFinish("scan", "done", "A test server does not scan the antenna.")
		return
	}
	client := s.HDHR
	if client == nil {
		client = &hdhr.Client{}
	}
	if err := client.StartScan(ctx, tuner.BaseURL); err != nil {
		s.setFinish("scan", "done", "The scan did not start.")
		return
	}
	// scanning stays true until a status read says the tuner is done. A progress
	// error or a cancelled wait never counts as finished.
	scanning := true
	deadline := time.Now().Add(s.setupScanWait())
	for {
		prog, err := client.ScanProgress(ctx, tuner.BaseURL)
		if err != nil {
			if ctx.Err() != nil {
				s.stopSetupScan(client, tuner.BaseURL)
				s.setFinish("scan", "done", "The scan stopped.")
				return
			}
			break
		}
		scanning = prog.Scan
		if prog.Scan {
			s.setFinish("scan", "running", fmt.Sprintf("Scanning for channels. %d found.", prog.Found))
		}
		if !prog.Scan || !time.Now().Before(deadline) {
			break
		}
		wait := time.Second
		if left := time.Until(deadline); left < wait {
			wait = left
		}
		if wait <= 0 {
			break
		}
		select {
		case <-ctx.Done():
			s.stopSetupScan(client, tuner.BaseURL)
			s.setFinish("scan", "done", "The scan stopped.")
			return
		case <-time.After(wait):
		}
	}
	if scanning {
		s.stopSetupScan(client, tuner.BaseURL)
	}
	host := tuner.BaseURL
	if u, err := url.Parse(tuner.BaseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	if _, err := source.Sync(ctx, s.Store, client, host); err != nil {
		slog.Error(fmt.Sprintf("setup: lineup: %v", err))
	}
	channels, _ = s.Store.Channels(ctx, false)
	n := len(visibleChannels(channels))
	if n == 0 {
		s.setFinish("scan", "done", "No channels yet.")
		return
	}
	s.setFinish("scan", "done", countLine(n, "channel", "channels")+".")
}

func (s *Server) stepGuide(ctx context.Context) {
	s.setFinish("guide", "running", "Loading listings.")
	listed := s.listedChannels(ctx)
	if listed == 0 && s.Staging {
		s.setFinish("guide", "done", "A test server does not pull the guide.")
		return
	}
	if listed == 0 {
		pullCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		n, err := s.RefreshGuide(pullCtx)
		cancel()
		if err != nil {
			slog.Error(fmt.Sprintf("setup: guide: %v", err))
			s.setFinish("guide", "done", "Listings did not load. More listings arrive from the broadcast.")
			return
		}
		listed = s.listedChannels(ctx)
		if listed == 0 {
			s.setFinish("guide", "done", fmt.Sprintf("Loaded %d listings. More listings arrive from the broadcast.", n))
			return
		}
	}
	s.setFinish("guide", "done", fmt.Sprintf("Listings for %s. More listings arrive from the broadcast.", countLine(listed, "channel", "channels")))
}

func (s *Server) stepFolder(ctx context.Context) {
	s.setFinish("folder", "running", "Checking the recordings folder.")
	devices, _ := s.Store.Devices(ctx)
	for _, note := range s.doctorNotes(devices) {
		if note.ID == "disk" || note.ID == "volume" {
			s.setFinish("folder", "done", note.Message)
			return
		}
	}
	if s.Hub != nil && s.Hub.Dir != "" {
		dir := filepath.Join(s.Hub.Dir, "recordings")
		if space, err := disk.Stat(dir); err == nil && space.Free > 0 {
			s.setFinish("folder", "done", formatFree(space.Free))
			return
		}
	}
	s.setFinish("folder", "done", "Recordings save in the folder mapped for this server.")
}

func (s *Server) stepFavorites(ctx context.Context) {
	s.setFinish("favorites", "running", "Picking ABC, CBS, FOX, and NBC.")
	starred, err := s.starBigFour(ctx)
	if err != nil {
		s.setFinish("favorites", "done", "Favorites could not be saved.")
		return
	}
	s.setFinish("favorites", "done", favoriteLine(starred))
}

func (s *Server) stepEncoder(ctx context.Context) {
	s.setFinish("encoder", "running", "Checking the picture encoder.")
	encoder, ffmpeg := "libx264", "ffmpeg"
	if s.Hub != nil {
		if s.Hub.Encoder != "" {
			encoder = s.Hub.Encoder
		}
		if s.Hub.FFmpeg != "" {
			ffmpeg = s.Hub.FFmpeg
		}
	}
	bench := s.SetupBench
	if bench == nil {
		bench = live.BenchEncoder
	}
	benchCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	speed, err := bench(benchCtx, ffmpeg, encoder)
	cancel()
	if err != nil {
		slog.Error(fmt.Sprintf("setup: encoder: %v", err))
	}
	s.setFinish("encoder", "done", live.FormatEncoderLine(encoder, speed, err == nil))
}

func (s *Server) stepSignal(ctx context.Context) {
	s.setFinish("signal", "running", "Checking the antenna.")
	if s.SetupSignal != nil {
		great, ok, weak, lost, err := s.SetupSignal(ctx)
		if err != nil {
			s.setFinish("signal", "done", err.Error())
			return
		}
		s.setFinish("signal", "done", signalSummary(great, ok, weak, lost))
		return
	}
	if s.Staging {
		s.setFinish("signal", "done", s.storedSignalSummary(ctx))
		return
	}
	s.setFinish("signal", "done", s.measureSetupSignals(ctx))
}

func (s *Server) measureSetupSignals(ctx context.Context) string {
	if s.Hub == nil {
		return "No tuner to check."
	}
	if s.recordingSoon(ctx) {
		return "A recording is coming up, so the signal check can wait."
	}
	if !s.Hub.Idle() {
		return "Tuners are busy, so the signal check can wait."
	}
	if !s.startSignalScan() {
		return "Already checking channels."
	}
	defer s.finishSignalScan()
	// A fresh lineup has no stored frequency. Tuning one channel learns the
	// whole mux, so the next pass skips the channels that share it.
	scanCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	tried := map[int64]bool{}
	seen := map[int]bool{}
	for scanCtx.Err() == nil {
		if !s.Hub.Idle() || s.recordingSoon(scanCtx) {
			break
		}
		rows, err := s.Store.ChannelSignals(scanCtx)
		if err != nil {
			return "The signal check did not start."
		}
		next, ok := nextSignalChannel(rows, tried, seen)
		if !ok {
			if len(rows) == 0 {
				return "No antenna channels to check."
			}
			break
		}
		tried[next] = true
		lock, err := s.Hub.Measure(scanCtx, next)
		if err != nil {
			slog.Error(fmt.Sprintf("setup: signal %d: %v", next, err))
			if scanCtx.Err() != nil {
				break
			}
			continue
		}
		full, err := s.Store.SourceChannel(scanCtx, next)
		if err != nil || full.FrequencyHz <= 0 {
			continue
		}
		seen[full.FrequencyHz] = true
		if err := s.Store.SaveFrequencySignal(scanCtx, full.FrequencyHz, lock.Locked, lock.Strength, lock.Quality, lock.Symbol, time.Now()); err != nil {
			slog.Error(fmt.Sprintf("setup: signal save %d: %v", full.FrequencyHz, err))
		}
	}
	return s.storedSignalSummary(ctx)
}

// nextSignalChannel picks one channel that still needs a reading. Channels
// whose frequency was already checked are skipped, including ones that had no
// frequency until a tune filled it in.
func nextSignalChannel(rows []store.ChannelSignal, tried map[int64]bool, seen map[int]bool) (int64, bool) {
	for _, row := range rows {
		if tried[row.ChannelID] {
			continue
		}
		if row.FrequencyHz > 0 && seen[row.FrequencyHz] {
			continue
		}
		return row.ChannelID, true
	}
	return 0, false
}

func (s *Server) storedSignalSummary(ctx context.Context) string {
	rows, err := s.Store.ChannelSignals(ctx)
	if err != nil {
		return "No signal reading."
	}
	var great, ok, weak, lost int
	for _, row := range rows {
		if !row.HasReading {
			continue
		}
		verdict, _ := (hdhr.Lock{Strength: row.Strength, Quality: row.Quality, Symbol: row.Symbol, Locked: row.Locked}).Verdict()
		switch verdict {
		case "Great":
			great++
		case "OK":
			ok++
		case "Weak":
			weak++
		default:
			lost++
		}
	}
	return signalSummary(great, ok, weak, lost)
}

func (s *Server) readyLine(ctx context.Context) (string, int64) {
	channels, err := s.Store.Channels(ctx, true)
	if err != nil {
		return "Ready.", 0
	}
	channels = withNetworks(channels)
	listed := map[int64]bool{}
	now := s.now()
	if airings, err := s.Store.Airings(ctx, now, now.Add(14*24*time.Hour)); err == nil {
		for _, row := range airings {
			listed[row.ChannelID] = true
		}
	}
	withGuide := 0
	var channelID int64
	for _, ch := range channels {
		if listed[ch.ID] {
			withGuide++
		}
		if channelID == 0 && ch.Favorite {
			channelID = ch.ID
		}
	}
	if channelID == 0 && len(channels) > 0 {
		channelID = channels[0].ID
	}
	tuners := 0
	if devices, err := s.Store.Devices(ctx); err == nil {
		for _, device := range devices {
			if device.TunerCount > 0 {
				tuners += device.TunerCount
			}
		}
	}
	encoder := "libx264"
	if s.Hub != nil && s.Hub.Encoder != "" {
		encoder = s.Hub.Encoder
	}
	return readyLine(len(channels), withGuide, tuners, live.EncoderName(encoder)), channelID
}

func (s *Server) listedChannels(ctx context.Context) int {
	channels, err := s.Store.Channels(ctx, true)
	if err != nil {
		return 0
	}
	now := s.now()
	airings, err := s.Store.Airings(ctx, now, now.Add(14*24*time.Hour))
	if err != nil {
		return 0
	}
	have := map[int64]bool{}
	for _, ch := range channels {
		have[ch.ID] = true
	}
	listed := map[int64]bool{}
	for _, row := range airings {
		if have[row.ChannelID] {
			listed[row.ChannelID] = true
		}
	}
	return len(listed)
}

func (s *Server) starBigFour(ctx context.Context) ([]map[string]any, error) {
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return nil, err
	}
	var starred []map[string]any
	for _, ch := range withNetworks(channels) {
		if ch.Hidden || !bigFour(ch.Network) {
			continue
		}
		if !ch.Favorite {
			on := true
			if _, err := s.Store.PatchChannel(ctx, ch.ID, store.ChannelPatch{Favorite: &on}); err != nil {
				return nil, err
			}
		}
		starred = append(starred, map[string]any{
			"id": ch.ID, "guideName": ch.GuideName, "displayNumber": ch.DisplayNumber, "network": ch.Network,
		})
	}
	if starred == nil {
		starred = []map[string]any{}
	}
	return starred, nil
}

func visibleChannels(channels []store.Channel) []store.Channel {
	var out []store.Channel
	for _, ch := range channels {
		if ch.Present && !ch.Hidden {
			out = append(out, ch)
		}
	}
	return out
}

func countLine(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func readyLine(channels, listed, tuners int, gpu string) string {
	return fmt.Sprintf("Ready: %s, guide for %d, %s, %s", countLine(channels, "channel", "channels"), listed, countLine(tuners, "tuner", "tuners"), gpu)
}

func signalSummary(great, ok, weak, lost int) string {
	type bucket struct {
		n    int
		word string
	}
	parts := []bucket{{great, "great"}, {ok, "ok"}, {weak, "weak"}, {lost, "lost"}}
	var bits []string
	for _, part := range parts {
		if part.n == 0 {
			continue
		}
		if len(bits) == 0 {
			bits = append(bits, countLine(part.n, "channel", "channels")+" "+part.word)
			continue
		}
		bits = append(bits, fmt.Sprintf("%d %s", part.n, part.word))
	}
	if len(bits) == 0 {
		return "No signal reading."
	}
	return strings.Join(bits, ", ") + "."
}

func favoriteLine(starred []map[string]any) string {
	have := map[string]bool{}
	for _, row := range starred {
		name, _ := row["network"].(string)
		if name != "" {
			have[name] = true
		}
	}
	var names []string
	for _, name := range []string{"ABC", "CBS", "FOX", "NBC"} {
		if have[name] {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "No ABC, CBS, FOX, or NBC in this lineup."
	}
	return joinAnd(names) + "."
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

func formatFree(bytes uint64) string {
	if bytes >= 1_000_000_000_000 {
		return fmt.Sprintf("%.1f TB free.", float64(bytes)/1e12)
	}
	return fmt.Sprintf("%d GB free.", bytes/1_000_000_000)
}
