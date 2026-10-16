package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// benchGuide is a realistic home catalog: two tuners that share ten channels,
// 90 channels in all, and 14 days of listings with real descriptions, plus
// a day of history. Eight series passes give the planner work.
func benchGuide(b *testing.B) (*Server, http.Handler, time.Time) {
	b.Helper()
	st, err := store.Open(filepath.Join(b.TempDir(), "cfg"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	lineup := func(from, to int) []hdhr.Channel {
		var out []hdhr.Channel
		for n := from; n <= to; n++ {
			out = append(out, hdhr.Channel{GuideNumber: fmt.Sprintf("%d.%d", n/4+2, n%4+1), GuideName: fmt.Sprintf("K%03dDT", n), HD: true})
		}
		return out
	}
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "AAAA0001", FriendlyName: "FLEX", BaseURL: "http://127.0.0.1:1", TunerCount: 4}, lineup(0, 49)); err != nil {
		b.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "AAAA0002", FriendlyName: "DUO", BaseURL: "http://127.0.0.1:2", TunerCount: 2}, lineup(40, 89)); err != nil {
		b.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	day0 := now.Truncate(24 * time.Hour).Add(-24 * time.Hour)
	desc := strings.Repeat("A family gathers for the holidays and old secrets come out over dinner. ", 3)
	var rows []store.Airing
	for i, ch := range chs {
		at := day0
		slot := 0
		for at.Before(day0.Add(15 * 24 * time.Hour)) {
			length := 30 * time.Minute
			if (slot+i)%3 == 0 {
				length = time.Hour
			}
			// A show airs on one channel about once a day.
			title := fmt.Sprintf("Show %d-%d", i, slot%40)
			rows = append(rows, store.Airing{
				ChannelID: ch.ID, Title: title, Subtitle: fmt.Sprintf("Episode %d", slot),
				Description: desc, Category: "Series", ProgramID: fmt.Sprintf("EP%06d%04d", (slot+i)%40, slot),
				Start: at, End: at.Add(length), ImageURL: fmt.Sprintf("https://img.example/%d.jpg", (slot+i)%40),
				Season: 2, Episode: slot%22 + 1, GuideSource: "silicondust",
			})
			at = at.Add(length)
			slot++
		}
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		b.Fatal(err)
	}
	for n := 0; n < 8; n++ {
		if err := st.AddPass(ctx, fmt.Sprintf("Show %d-%d", n*11, n*3), 0, 1, 2); err != nil {
			b.Fatal(err)
		}
	}
	s := &Server{Store: st, Version: "bench"}
	return s, s.Handler(), now
}

func BenchmarkDataEndpoints(b *testing.B) {
	_, h, now := benchGuide(b)
	from := now.Truncate(30 * time.Minute)
	cases := []struct{ name, path string }{
		{"Airings48h", "/api/v1/airings"},
		{"Airings4h", "/api/v1/airings?from=" + from.Format(time.RFC3339) + "&to=" + from.Add(4*time.Hour).Format(time.RFC3339)},
		{"Airings4hUnrounded", "/api/v1/airings?from=" + now.Add(-17*time.Second).Format(time.RFC3339) + "&to=" + now.Add(4*time.Hour).Format(time.RFC3339)},
		{"Channels", "/api/v1/channels"},
		{"ChannelsGuide", "/api/v1/channels?guide=1"},
		{"Schedule", "/api/v1/schedule"},
		{"Now", "/api/v1/now"},
		{"Search", "/api/v1/search?q=show"},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			probe := httptest.NewRecorder()
			h.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, c.path, nil))
			if probe.Code != http.StatusOK {
				b.Skipf("%s: %d", c.path, probe.Code)
			}
			b.ReportMetric(float64(probe.Body.Len()), "bytes")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodGet, c.path, nil)
				req.Header.Set("Accept-Encoding", "gzip")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					b.Fatalf("%d", rec.Code)
				}
			}
		})
		b.Run(c.name+"304", func(b *testing.B) {
			probe := httptest.NewRecorder()
			h.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, c.path, nil))
			tag := probe.Header().Get("ETag")
			if probe.Code != http.StatusOK || tag == "" {
				b.Skip("no ETag")
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodGet, c.path, nil)
				req.Header.Set("Accept-Encoding", "gzip")
				req.Header.Set("If-None-Match", tag)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusNotModified {
					b.Fatalf("%d", rec.Code)
				}
			}
		})
	}
}
