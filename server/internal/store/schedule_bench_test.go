package store

import (
	"context"
	"testing"
	"time"
)

// BenchmarkScheduleAirings is one day of 3,000 channels. A category pass still
// reads that day. A series pass reads the one listing it names.
func BenchmarkScheduleAirings(b *testing.B) {
	st, from, to := loadGuideBench(b)
	start := from.Add(3 * time.Hour)
	if err := st.InsertAirings(context.Background(), []Airing{{
		ChannelID: 7, Title: "Cedar Quiz", Category: "Series",
		Start: start, End: start.Add(time.Hour),
	}}); err != nil {
		b.Fatal(err)
	}
	window := to.Add(13 * 24 * time.Hour)
	wide := []Pass{{ID: 1, Title: "Sports", Kind: "series", MatchKind: "category"}}
	series := []Pass{{ID: 2, Title: "Cedar Quiz", Kind: "series", MatchKind: "title"}}
	b.Run("Window", func(b *testing.B) {
		var n int
		for i := 0; i < b.N; i++ {
			rows, err := st.RecordingAiringsFor(context.Background(), from, window, wide)
			if err != nil {
				b.Fatal(err)
			}
			n = len(rows)
		}
		if n < 3000*16 {
			b.Fatalf("rows %d", n)
		}
	})
	b.Run("Series", func(b *testing.B) {
		var n int
		for i := 0; i < b.N; i++ {
			rows, err := st.RecordingAiringsFor(context.Background(), from, window, series)
			if err != nil {
				b.Fatal(err)
			}
			n = len(rows)
		}
		if n != 1 {
			b.Fatalf("rows %d", n)
		}
	})
}
