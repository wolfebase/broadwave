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
	// An unplugged device does not answer at all.
	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
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
	other.DeviceID, other.BaseURL, other.FriendlyName = "B0000001", gone.URL, "Attic"
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
	// With nothing tuned, the tuner list is the first device that answers;
	// the unplugged one sorts first and is skipped.
	h.mu.Lock()
	for _, f := range h.feedsLocked() {
		h.stopFeedLocked(f)
	}
	h.mu.Unlock()
	list, err := h.Tuners(ctx)
	if err != nil || len(list) != 2 || asked.Load() != 1 {
		t.Fatalf("tuners %v err %v, unplugged asked %d", list, err, asked.Load())
	}
	// Once its skip runs out it is asked again, and still the list comes
	// from the other device within a second, as the tune check needs.
	h.mu.Lock()
	clear(h.statusDown)
	h.mu.Unlock()
	short, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	began := time.Now()
	list, err = h.Tuners(short)
	if err != nil || len(list) != 2 || time.Since(began) > time.Second {
		t.Fatalf("tuners %v err %v after %v", list, err, time.Since(began))
	}
	h.mu.Lock()
	down := time.Now().Before(h.statusDown[gone.URL])
	h.mu.Unlock()
	if !down {
		t.Fatal("a device that did not answer was not set aside")
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
