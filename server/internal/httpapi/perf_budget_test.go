package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// TestPerfBudget is the CI p95 ceiling for cheap reads.
// 50ms is too tight for CI noise, so the ceiling is 200ms.
// The handler is in-process: no listener, no tuner, no LAN.
func TestPerfBudget(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "FAKEHDHR", FriendlyName: "DUO", ModelNumber: "HDHR5-2US",
		BaseURL: "http://127.0.0.1:5004", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true},
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(time.Second)
	if err := st.ReplaceAirings(ctx, []store.Airing{
		{ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	h := (&Server{Store: st, Version: "dev"}).Handler()
	paths := []string{"/api/v1/server", "/api/v1/channels", "/api/v1/schedule"}
	want := map[string]string{
		"/api/v1/server":   `"apiVersion"`,
		"/api/v1/channels": `"4.1"`,
		"/api/v1/schedule": `"items"`,
	}
	const iterations = 50
	const budget = 200 * time.Millisecond
	samples := make([]time.Duration, 0, iterations*len(paths))
	for range iterations {
		for _, path := range paths {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			began := time.Now()
			h.ServeHTTP(rec, req)
			elapsed := time.Since(began)
			if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(want[path])) {
				t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
			}
			samples = append(samples, elapsed)
		}
	}
	got := percentile(samples, 95)
	fmt.Printf("api p95 %s (50 iterations, %d requests, budget %s)\n", got, len(samples), budget)
	t.Logf("api p95 %s (50 iterations, %d requests, budget %s)", got, len(samples), budget)
	if got > budget {
		t.Fatalf("api p95 %s exceeds %s", got, budget)
	}
}

// percentile is the nearest-rank value: ceil(p/100 * n), 1-based.
func percentile(samples []time.Duration, p int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	rank := (n*p + 99) / 100
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}
