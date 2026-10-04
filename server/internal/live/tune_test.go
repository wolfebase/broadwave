package live

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

func tuneStore(t *testing.T, srv *fake.Server) (*store.Store, string) {
	t.Helper()
	st := openStore(t)
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
	lineup, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"4.1", "4.2", "5.1"} {
		if err := st.SetFieldOrder(ctx, idOf(t, st, n), "progressive"); err != nil {
			t.Fatal(err)
		}
	}
	return st, dev.DeviceID
}

// A channel with no signal takes 15 s or more to fail. Its tune must not
// hold the hub, or every other channel's playlist waits for it.
func TestA1Point0TuneLeavesTheHubFree(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Profile: fake.ProfileConnectDuo, TuneDelay: 800 * time.Millisecond}
	st, _ := tuneStore(t, srv)
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	ctx := context.Background()
	want := Rendition{Video: "copy", Audio: "aac2"}
	id := idOf(t, st, "4.1")

	done := make(chan error, 1)
	var session Session
	go func() {
		var err error
		session, err = h.Watch(ctx, id, want, false)
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	began := time.Now()
	h.Touch(id, want.normalized().Key())
	if waited := time.Since(began); waited > 200*time.Millisecond {
		t.Fatalf("the hub was held %v while the tuner locked", waited)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h.Release(id, session.Rendition)
}

// A recording, an export, a guide dwell, and a signal check tune the same
// way a watch does: a channel with no signal must not hold the hub.
func TestEveryTuneLeavesTheHubFree(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	cases := map[string]func(ctx context.Context, h *Hub, id int64) error{
		"record": func(ctx context.Context, h *Hub, id int64) error {
			rec, err := h.Record(ctx, id, 1, "News")
			if err == nil {
				h.StopRecord(rec.ID)
			}
			return err
		},
		"export": func(ctx context.Context, h *Hub, id int64) error {
			ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			return h.Export(ctx, id, io.Discard)
		},
		"dwell": func(ctx context.Context, h *Hub, id int64) error {
			_, err := h.Dwell(ctx, id, 0)
			return err
		},
		"measure": func(ctx context.Context, h *Hub, id int64) error {
			_, err := h.Measure(ctx, id)
			return err
		},
	}
	for name, tune := range cases {
		t.Run(name, func(t *testing.T) {
			srv := &fake.Server{Profile: fake.ProfileConnectDuo, TuneDelay: 800 * time.Millisecond}
			st, _ := tuneStore(t, srv)
			h := New(st, t.TempDir(), "ffmpeg", "libx264")
			t.Cleanup(h.Shutdown)
			ctx := context.Background()
			id := idOf(t, st, "4.1")

			done := make(chan error, 1)
			go func() { done <- tune(ctx, h, id) }()
			time.Sleep(300 * time.Millisecond)
			began := time.Now()
			h.Touch(idOf(t, st, "5.1"), Rendition{Video: "copy", Audio: "aac2"}.normalized().Key())
			if waited := time.Since(began); waited > 200*time.Millisecond {
				t.Fatalf("the hub was held %v while the tuner locked", waited)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Two subchannels of one station opened at once share one tuner.
func TestSiblingsOpenedAtOnceShareATuner(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Profile: fake.ProfileConnectDuo, TuneDelay: 500 * time.Millisecond}
	st, device := tuneStore(t, srv)
	ctx := context.Background()
	for i, n := range []string{"4.1", "4.2"} {
		if err := st.RememberProgram(ctx, device, n, 593000000, i+1); err != nil {
			t.Fatal(err)
		}
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	want := Rendition{Video: "copy", Audio: "aac2"}

	// Both start inside one status read.
	ids := []int64{idOf(t, st, "4.1"), idOf(t, st, "4.2")}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = h.Watch(ctx, id, want, false)
		}()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	opens := 0
	for _, path := range srv.Requests() {
		if strings.Contains(path, "/ch593000000") {
			opens++
		}
	}
	if opens != 1 {
		t.Fatalf("one station took %d tunes: %v", opens, srv.Requests())
	}
	h.mu.Lock()
	tuners := len(h.usedTunersLocked(""))
	h.mu.Unlock()
	if tuners != 1 {
		t.Fatalf("%d tuners in use", tuners)
	}
}

// With both 1.0 tuners of a FLEX 4K busy, a third 1.0 channel takes a 3.0
// tuner, and that tune does not hold the hub either.
func TestA1Point0TuneOnA3Point0TunerLeavesTheHubFree(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Profile: fake.ProfileFlex4K, TuneDelay: 800 * time.Millisecond, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "4.2", Name: "KBWV2", Freq: 593000000},
		{Number: "5.1", Name: "WTST", Freq: 599000000},
		{Number: "7.1", Name: "KTRE", Freq: 605000000},
		{Number: "104.1", Name: "KBWV", Freq: 611000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
	}}
	st, _ := tuneStore(t, srv)
	if err := st.SetFieldOrder(context.Background(), idOf(t, st, "7.1"), "progressive"); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	ctx := context.Background()
	want := Rendition{Video: "copy", Audio: "aac2"}
	for _, n := range []string{"4.1", "5.1"} {
		if _, err := h.Watch(ctx, idOf(t, st, n), want, false); err != nil {
			t.Fatal(err)
		}
	}
	id := idOf(t, st, "7.1")
	done := make(chan error, 1)
	go func() {
		_, err := h.Watch(ctx, id, want, false)
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	began := time.Now()
	h.Touch(id, want.normalized().Key())
	if waited := time.Since(began); waited > 200*time.Millisecond {
		t.Fatalf("the hub was held %v while a 3.0 tuner locked a 1.0 channel", waited)
	}
	if err := <-done; err != nil {
		t.Fatalf("%v (%v)", err, srv.Requests())
	}
}
