package live

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestExtendRecordingLeavesTheTimerItReplaced(t *testing.T) {
	ctx := context.Background()
	h, st, id := cityRecording(t)

	if err := h.ExtendRecording(ctx, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	baseline := runtime.NumGoroutine()
	h.mu.Lock()
	locked := true
	unlock := func() {
		if locked {
			h.mu.Unlock()
			locked = false
		}
	}
	defer unlock()

	armed := h.channels[id].recording.timer
	if armed == nil {
		t.Fatal("no stop timer")
	}

	// Queue the extend before the callback so it is the head waiter.
	// Both then sleep in the mutex queue; Unlock cannot be barged by the callback.
	errc := make(chan error, 1)
	go func() {
		errc <- h.ExtendRecording(ctx, id, time.Now().Add(time.Hour))
	}()
	waitHubMutex(t, 1)

	deadline := time.Now().Add(2 * time.Second)
	for armed.Stop() {
		armed.Reset(0)
		if time.Now().After(deadline) {
			t.Fatal("stop callback did not start")
		}
		time.Sleep(time.Millisecond)
	}
	waitHubMutex(t, 2)
	unlock()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("extend did not return")
	}
	settle := time.Now().Add(2 * time.Second)
	for {
		got, err := st.Recording(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "recording" {
			t.Fatalf("replaced stop timer finished the row as %s", got.Status)
		}
		if runtime.NumGoroutine() <= baseline {
			if got.EndsAt == nil || got.EndsAt.Before(time.Now().Add(30*time.Minute)) {
				t.Fatalf("end was not extended: %v", got.EndsAt)
			}
			break
		}
		if time.Now().After(settle) {
			t.Fatal("replaced callback did not return")
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.mu.Lock()
	held := h.channels[id] != nil && len(h.muxes) == 1
	timer := h.channels[id].recording.timer
	waiting := timer != nil && timer.Reset(30*time.Millisecond)
	h.mu.Unlock()
	if !held {
		t.Fatal("replaced stop timer dropped the tune")
	}
	if !waiting {
		t.Fatal("replacement timer was not waiting")
	}

	waitUntil(t, 2*time.Second, "the unreplaced timer to finish", func() bool {
		got, err := st.Recording(ctx, id)
		return err == nil && got.Status == "complete"
	})
	h.mu.Lock()
	tuned := h.channels[id] != nil
	left := len(h.muxes)
	h.mu.Unlock()
	if tuned || left != 0 {
		t.Fatalf("unreplaced timer left the tune up: channel %v muxes %d", tuned, left)
	}
}

// cityRecording is City 8.1 on a mux this process never dials. Elsewhere is
// only a guide row. tuner -1 keeps stopFeedLocked off the control port.
func cityRecording(t *testing.T) (*Hub, *store.Store, int64) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "lab", FriendlyName: "Lab", BaseURL: "http://tuner.example", TunerCount: 1,
	}, []hdhr.Channel{
		{GuideNumber: "8.1", GuideName: "City"},
		{GuideNumber: "8.2", GuideName: "Elsewhere"},
	}); err != nil {
		t.Fatal(err)
	}
	city := idOf(t, st, "8.1")
	elsewhere := idOf(t, st, "8.2")
	when := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: city, Title: "Evening", Start: when, End: when.Add(time.Hour)},
		{ChannelID: elsewhere, Title: "Later", Start: when, End: when.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	ends := time.Now().Add(time.Minute)
	id, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: city, GuideNumber: "8.1", Title: "Evening", Status: "recording",
		StartedAt: when, EndsAt: &ends,
	})
	if err != nil {
		t.Fatal(err)
	}

	const freq = 210000000
	h := &Hub{Store: st, Dir: t.TempDir(), channels: map[int64]*feed{}, muxes: map[int]*mux{}}
	m := &mux{freq: freq, tuner: -1, feeds: map[string]*feed{}, cancel: func() {}}
	f := &feed{
		channel: store.SourceChannel{
			Channel:     store.Channel{ID: city, GuideNumber: "8.1", GuideName: "City", DisplayName: "City"},
			FrequencyHz: freq,
		},
		renditions: map[string]*rendition{},
	}
	m.feeds["8.1"] = f
	h.muxes[freq] = m
	h.channels[city] = f
	f.recording = &recording{id: id, ends: ends}
	t.Cleanup(func() {
		h.mu.Lock()
		if live := h.channels[city]; live != nil && live.recording != nil {
			stopTimer(&live.recording.timer)
		}
		h.mu.Unlock()
	})
	return h, st, id
}

// waitHubMutex waits until exactly want Hub methods are blocked on a mutex,
// one of them ExtendRecording. The callback queues second, so want goes 1 then 2.
func waitHubMutex(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		extend, n, dump := hubMutexWaiters()
		if extend && n == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("mutex waiters = %d, extend parked %v; want %d\n%s", n, extend, want, dump)
		}
		time.Sleep(time.Millisecond)
	}
}

func hubMutexWaiters() (extend bool, n int, dump string) {
	buf := make([]byte, 1<<20)
	dump = string(buf[:runtime.Stack(buf, true)])
	for g := range strings.SplitSeq(dump, "\ngoroutine ") {
		header, body, _ := strings.Cut(g, "\n")
		if !strings.Contains(header, "[sync.Mutex.Lock]") {
			continue
		}
		if !strings.Contains(body, "broadwave/internal/live.(*Hub)") {
			continue
		}
		n++
		if strings.Contains(body, "(*Hub).ExtendRecording(") {
			extend = true
		}
	}
	return extend, n, dump
}
