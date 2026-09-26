package live

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

func TestDeviceVanishHandsTheStreamOff(t *testing.T) {
	st := openStore(t)
	baseA, idA, srvA := saveProfile(t, st, fake.ProfileConnectDuo)
	baseB, idB, srvB := saveProfile(t, st, fake.ProfileFlexDuo)
	res, err := http.Get(baseA + "/tuner0/v4.1")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatal(res.Status)
	}

	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.MoveBudget = time.Second
	m := &mux{
		freq: 593000000, tuner: 0, host: "127.0.0.1", device: idA, body: res.Body,
		feeds: map[string]*feed{}, cancel: func() {},
	}
	f := &feed{channel: store.SourceChannel{
		Channel:     store.Channel{ID: 1, DeviceID: idA, GuideNumber: "4.1"},
		BaseURL:     baseA,
		FrequencyHz: m.freq,
	}}
	m.feeds["4.1"] = f
	h.muxes[m.freq] = m
	h.channels[f.channel.ID] = f
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go h.readLoop(ctx, m)
	t.Cleanup(func() {
		h.mu.Lock()
		h.stopFeedLocked(f)
		h.mu.Unlock()
	})

	started := time.Now()
	srvA.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if guideOn(t, baseB, 0) == "4.1" {
			h.mu.Lock()
			got := m.base
			id := m.device
			h.mu.Unlock()
			if got != baseB || id != idB {
				t.Fatalf("base %s device %s", got, id)
			}
			if !requested(srvB, "/tuner0/ch593000000") {
				t.Fatalf("full mux not opened: %v", srvB.Requests())
			}
			if time.Since(started) >= 2*time.Second {
				t.Fatalf("moved in %s", time.Since(started))
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("stream did not move")
}

func TestATSC3MoveSkipsATunerThatCannot(t *testing.T) {
	st := openStore(t)
	base, _, srv := saveProfile(t, st, fake.ProfileFlex4K)
	// Tuner 0 stays on a 1.0 channel for the whole test. A non-200 body is not a hold.
	mustTune(t, base+"/tuner0/v4.1")
	waitGuide(t, base, 0, "4.1")
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.MoveBudget = time.Second
	m := deadMux(t, "104.1", "HEVC", "AC-4")
	h.muxes[m.freq] = m
	if !h.handOff(context.Background(), m) {
		t.Fatal("3.0 channel did not move")
	}
	waitGuide(t, base, 1, "104.1")
	if guideOn(t, base, 2) != "" || requested(srv, "/tuner2/v104.1") {
		t.Fatalf("3.0 used a 1.0 tuner: %+v %v", fetchGuides(t, base), srv.Requests())
	}
	// The 104.1 stream has to finish releasing before 4.2 can take tuner 1.
	// Closing the body returns before that release, and a 804 response does not hold the tuner.
	if err := m.body.Close(); err != nil {
		t.Fatal(err)
	}
	waitGuide(t, base, 1, "")
	mustTune(t, base+"/tuner1/v4.2")
	waitGuide(t, base, 1, "4.2")
	if guideOn(t, base, 0) != "4.1" {
		t.Fatalf("tuner 0 was released: %+v", fetchGuides(t, base))
	}
	h2 := New(st, t.TempDir(), "ffmpeg", "libx264")
	h2.MoveBudget = time.Second
	again := deadMux(t, "104.1", "HEVC", "AC-4")
	h2.muxes[again.freq] = again
	before := len(srv.Requests())
	if h2.handOff(context.Background(), again) {
		t.Fatal("3.0 moved onto a tuner that cannot do 3.0")
	}
	if requestedSince(srv, before, "104.1") {
		t.Fatalf("opened a 1.0 tuner: %v", srv.Requests()[before:])
	}
	if h2.handOff(context.Background(), again) {
		t.Fatal("a failed move tried again")
	}
	if requestedSince(srv, before, "104.1") {
		t.Fatal("second move opened a tuner")
	}
}

func TestFailedOpenTriesTheNextDevice(t *testing.T) {
	st := openStore(t)
	hits := atomic.Int32{}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{"Resource":"tuner0","VctNumber":"","TargetIP":""}]`)
			return
		}
		hits.Add(1)
		http.Error(w, "806 Tune Failed", http.StatusServiceUnavailable)
	}))
	t.Cleanup(bad.Close)
	_, _, good := saveProfile(t, st, fake.ProfileConnectDuo)
	saveDevice(t, st, hdhr.Device{
		DeviceID: "A0000001", FriendlyName: "Bad", ModelNumber: "HDHR5-2US",
		BaseURL: bad.URL, LineupURL: bad.URL + "/lineup.json", TunerCount: 1,
	}, "4.1")
	// A0000001 sorts before the CONNECT DUO id, so the pool tries the failing device first.
	if "A0000001" > "A3E00002" {
		t.Fatal("device order")
	}

	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.MoveBudget = time.Second
	m := deadMux(t, "4.1", "MPEG2", "AC3")
	h.muxes[m.freq] = m
	if !h.handOff(context.Background(), m) {
		t.Fatal("did not move to the working device")
	}
	if hits.Load() != 1 {
		t.Fatalf("opens %d", hits.Load())
	}
	if !requested(good, "/tuner0/v4.1") {
		t.Fatalf("next device was not tried: %v", good.Requests())
	}
	if h.handOff(context.Background(), m) {
		t.Fatal("a second loss tried again")
	}
	if hits.Load() != 1 {
		t.Fatalf("opens after retry %d", hits.Load())
	}
}

func TestHandOffStopsAtTheDeadline(t *testing.T) {
	st := openStore(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepts atomic.Int32
	done := make(chan struct{})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			select {
			case <-done:
			case <-time.After(30 * time.Second):
			}
			_ = c.Close()
		}
	}()
	t.Cleanup(func() {
		close(done)
		_ = ln.Close()
	})
	base := "http://" + ln.Addr().String()
	saveDevice(t, st, hdhr.Device{
		DeviceID: "HANG0001", FriendlyName: "Hang", ModelNumber: "HDHR5-2US",
		BaseURL: base, LineupURL: base + "/lineup.json", TunerCount: 1,
	}, "4.1")

	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.MoveBudget = 100 * time.Millisecond
	m := deadMux(t, "4.1", "MPEG2", "AC3")
	h.muxes[m.freq] = m
	started := time.Now()
	if h.handOff(context.Background(), m) {
		t.Fatal("hung device counted as a move")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("deadline took %s", elapsed)
	}
	got := accepts.Load()
	if h.handOff(context.Background(), m) {
		t.Fatal("looped")
	}
	if accepts.Load() != got || got > 1 {
		t.Fatalf("accepts %d then %d", got, accepts.Load())
	}
}

func TestHandOffReleasesTheTunerWhenTheViewerLeft(t *testing.T) {
	st := openStore(t)
	base, _, _ := saveProfile(t, st, fake.ProfileConnectDuo)
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.MoveBudget = time.Second
	m := deadMux(t, "4.1", "MPEG2", "AC3")
	if h.handOff(context.Background(), m) {
		t.Fatal("moved onto a mux the viewer already left")
	}
	if guideOn(t, base, 0) != "" || guideOn(t, base, 1) != "" {
		t.Fatalf("tuner left held: %+v", fetchGuides(t, base))
	}
}

func TestFailoverTuneOpensTheOtherDevice(t *testing.T) {
	t.Setenv("HDHR_CONTROL_PORT", "")
	st := openStore(t)
	baseA, idA, srvA := saveProfile(t, st, fake.ProfileConnectDuo)
	_, idB, srvB := saveProfile(t, st, fake.ProfileFlexDuo)
	hold0, err := http.Get(baseA + "/tuner0/v4.2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hold0.Body.Close() })
	hold1, err := http.Get(baseA + "/tuner1/v5.1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hold1.Body.Close() })

	channels, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	for _, c := range channels {
		if c.DeviceID == idA && c.GuideNumber == "4.1" {
			id = c.ID
		}
	}
	if id == 0 {
		t.Fatal("no 4.1")
	}
	ch, err := st.SourceChannel(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.mu.Lock()
	f, err := h.ensureFeedLocked(context.Background(), ch, nil)
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		h.mu.Lock()
		h.stopFeedLocked(f)
		h.mu.Unlock()
	})
	if f.channel.DeviceID != idB {
		t.Fatalf("device %s, want %s", f.channel.DeviceID, idB)
	}
	if !requested(srvB, "/auto/v4.1") && !requested(srvB, "/tuner") {
		t.Fatalf("other device not opened: %v", srvB.Requests())
	}
	for _, path := range srvA.Requests() {
		if contains(path, "4.1") {
			t.Fatalf("busy device was tuned: %v", srvA.Requests())
		}
	}
}

func deadMux(t *testing.T, guide, video, audio string) *mux {
	t.Helper()
	pr, pw := io.Pipe()
	_ = pw.Close()
	return &mux{
		freq: 1, tuner: 0, host: "127.0.0.1", device: "GONE", body: pr,
		feeds: map[string]*feed{
			guide: {channel: store.SourceChannel{Channel: store.Channel{
				DeviceID: "GONE", GuideNumber: guide, VideoCodec: video, AudioCodec: audio,
			}}},
		},
		cancel: func() {},
	}
}

func TestKnownFrequencySkipsTheControlTune(t *testing.T) {
	st := openStore(t)
	srv := &fake.Server{Profile: fake.ProfileConnectDuo}
	base, control, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	ctx := context.Background()
	dev, err := (&hdhr.Client{}).FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	id := idOf(t, st, "4.1")
	if err := st.RememberProgram(ctx, dev.DeviceID, "4.1", 593000000, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFieldOrder(ctx, id, "progressive"); err != nil {
		t.Fatal(err)
	}
	ch, err := st.SourceChannel(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	h.mu.Lock()
	feed, err := h.ensureFeedLocked(ctx, ch, nil)
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		h.mu.Lock()
		h.stopFeedLocked(feed)
		h.mu.Unlock()
	})
	if !requested(srv, "/tuner0/ch593000000") {
		t.Fatalf("mux was not opened: %v", srv.Requests())
	}
	for _, path := range srv.Requests() {
		if strings.Contains(path, "vchannel") {
			t.Fatalf("stored frequency still probed the tuner: %v", srv.Requests())
		}
	}
	sib := idOf(t, st, "4.2")
	deadline := time.Now().Add(2 * time.Second)
	var stored store.SourceChannel
	for {
		stored, err = st.SourceChannel(ctx, sib)
		if err != nil {
			t.Fatal(err)
		}
		if stored.FrequencyHz == 593000000 && stored.ProgramNum > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("4.2 was not recorded from streaminfo: freq %d program %d requests %v", stored.FrequencyHz, stored.ProgramNum, srv.Requests())
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, path := range srv.Requests() {
		if strings.Contains(path, "vchannel") {
			t.Fatalf("sibling discovery probed: %v", srv.Requests())
		}
	}

	fresh := idOf(t, st, "5.1")
	other, err := st.SourceChannel(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if other.FrequencyHz != 0 || other.ProgramNum != 0 {
		t.Fatalf("5.1 should still be undiscovered: %+v", other.FrequencyHz)
	}
	before := len(srv.Requests())
	h.mu.Lock()
	second, err := h.ensureFeedLocked(ctx, other, nil)
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		h.mu.Lock()
		h.stopFeedLocked(second)
		h.mu.Unlock()
	})
	if !requestedSince(srv, before, "/tuner1/vchannel") && !requestedSince(srv, before, "/tuner0/vchannel") {
		t.Fatalf("an unknown frequency did not probe: %v", srv.Requests()[before:])
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func saveProfile(t *testing.T, st *store.Store, profile string) (string, string, *fake.Server) {
	t.Helper()
	srv := &fake.Server{Profile: profile}
	base, _, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	ctx := context.Background()
	dev, err := (&hdhr.Client{}).FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	return base, dev.DeviceID, srv
}

func saveDevice(t *testing.T, st *store.Store, dev hdhr.Device, guide string) {
	t.Helper()
	err := st.UpsertDevice(context.Background(), dev, []hdhr.Channel{{
		GuideNumber: guide, GuideName: guide, VideoCodec: "MPEG2", AudioCodec: "AC3",
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func mustTune(t *testing.T, rawURL string) {
	t.Helper()
	res, err := http.Get(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s: %s", rawURL, res.Status)
	}
}

func waitGuide(t *testing.T, base string, tuner int, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last []string
	for {
		last = fetchGuides(t, base)
		if tuner >= 0 && tuner < len(last) && last[tuner] == want {
			return
		}
		if !time.Now().Before(deadline) {
			got := ""
			if tuner >= 0 && tuner < len(last) {
				got = last[tuner]
			}
			t.Fatalf("tuner %d is %q, want %q (%v)", tuner, got, want, last)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func guideOn(t *testing.T, base string, tuner int) string {
	t.Helper()
	rows := fetchGuides(t, base)
	if tuner < 0 || tuner >= len(rows) {
		return ""
	}
	return rows[tuner]
}

func fetchGuides(t *testing.T, base string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := fetchTunerStatus(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(raw))
	for i, row := range raw {
		out[i] = row.VctNumber
	}
	return out
}

func requested(srv *fake.Server, path string) bool {
	for _, got := range srv.Requests() {
		if len(got) >= len(path) && got[:len(path)] == path || contains(got, path) {
			return true
		}
	}
	return false
}

func requestedSince(srv *fake.Server, from int, path string) bool {
	rows := srv.Requests()
	if from > len(rows) {
		from = len(rows)
	}
	for _, got := range rows[from:] {
		if contains(got, path) {
			return true
		}
	}
	return false
}

func contains(s, part string) bool {
	return len(part) == 0 || (len(s) >= len(part) && (s == part || len(s) > len(part) && (stringIndex(s, part) >= 0)))
}

func stringIndex(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
