package live

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// A second device with the same channels is failover. Its status is not read
// while the channel's own device has a tuner, and a device whose status read
// failed is skipped for a while instead of costing every tune its timeout.
func TestATuneReadsOnlyTheDevicesItNeeds(t *testing.T) {
	st := openStore(t)
	srv := &fake.Server{Profile: fake.ProfileConnectDuo}
	base, control, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	var asked atomic.Int32
	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		time.Sleep(300 * time.Millisecond)
		http.Error(w, "unplugged", http.StatusBadGateway)
	}))
	t.Cleanup(gone.Close)
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
	other := dev
	other.DeviceID, other.BaseURL, other.FriendlyName = "B0000001", gone.URL, "Unplugged"
	if err := st.UpsertDevice(ctx, other, lineup); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	tune := func(number, device string) {
		t.Helper()
		ch := rowOn(t, st, number, device)
		h.mu.Lock()
		f, err := h.ensureFeedLocked(ctx, ch, nil)
		h.mu.Unlock()
		if err != nil {
			t.Fatalf("%s on %s: %v", number, device, err)
		}
		t.Cleanup(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.stopFeedLocked(f)
		})
	}

	tune("4.1", dev.DeviceID)
	if n := asked.Load(); n != 0 {
		t.Fatalf("a tune with a free tuner read the failover device %d times", n)
	}
	// The unplugged device's own row fails over, once at its timeout.
	tune("5.1", other.DeviceID)
	if n := asked.Load(); n != 1 {
		t.Fatalf("unplugged device asked %d times, want 1", n)
	}
	h.mu.Lock()
	for _, f := range h.feedsLocked() {
		h.stopFeedLocked(f)
	}
	h.mu.Unlock()
	tune("5.1", other.DeviceID)
	if n := asked.Load(); n != 1 {
		t.Fatalf("a device that just failed was asked again (%d)", n)
	}
}

func rowOn(t *testing.T, st *store.Store, number, device string) store.SourceChannel {
	t.Helper()
	chs, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chs {
		if c.GuideNumber == number && c.DeviceID == device {
			ch, err := st.SourceChannel(context.Background(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			return ch
		}
	}
	t.Fatalf("no %s on %s", number, device)
	return store.SourceChannel{}
}
