package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkAirings measures a day of the guide against the narrow queries the
// teams widget uses. The catalog is 3,000 channels, sixteen listings a day,
// plus the day before so the window has rows to skip. One listing names the Wolves.
// The temp directory belongs to this benchmark, so it outlives every sub-benchmark.
func BenchmarkAirings(b *testing.B) {
	st, from, to := loadGuideBench(b)
	b.Run("Day", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rows, err := st.Airings(context.Background(), from, to)
			if err != nil {
				b.Fatal(err)
			}
			if len(rows) != 3000*16 {
				b.Fatalf("rows %d", len(rows))
			}
		}
	})
	b.Run("Team", func(b *testing.B) {
		var n int
		for i := 0; i < b.N; i++ {
			rows, err := st.QueryAirings(context.Background(), AiringQuery{From: from, To: to, Teams: []string{"Wolves"}})
			if err != nil {
				b.Fatal(err)
			}
			n = len(rows)
		}
		if n != 1 {
			b.Fatalf("rows %d", n)
		}
	})
	b.Run("Channel", func(b *testing.B) {
		var n int
		for i := 0; i < b.N; i++ {
			rows, err := st.QueryAirings(context.Background(), AiringQuery{From: from, To: to, Channels: []int64{7, 8, 9, 10}})
			if err != nil {
				b.Fatal(err)
			}
			n = len(rows)
		}
		if n != 4*16 {
			b.Fatalf("rows %d", n)
		}
	})
}

func loadGuideBench(b *testing.B) (*Store, time.Time, time.Time) {
	b.Helper()
	st, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	from := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	const channels = 3000
	const perDay = 16
	rows := make([]Airing, 0, channels*perDay*2)
	for ch := int64(1); ch <= channels; ch++ {
		for day := -1; day <= 0; day++ {
			for slot := 0; slot < perDay; slot++ {
				start := from.Add(time.Duration(day*24+slot) * time.Hour)
				title := fmt.Sprintf("Show %d", slot)
				if ch == 7 && day == 0 && slot == 3 {
					title = "Wolves at Harbor"
				}
				rows = append(rows, Airing{
					ChannelID: ch, Title: title, Category: "Series",
					Start: start, End: start.Add(time.Hour),
				})
			}
		}
	}
	if err := st.InsertAirings(context.Background(), rows); err != nil {
		b.Fatal(err)
	}
	return st, from, from.Add(24 * time.Hour)
}
