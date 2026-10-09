package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkRecordingByID is one lookup in a library of 2,000 recordings.
func BenchmarkRecordingByID(b *testing.B) {
	st, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	ctx := context.Background()
	const n = 2000
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var id int64
	for i := range n {
		got, err := st.CreateRecording(ctx, Recording{
			Title: fmt.Sprintf("Show %d", i), Status: "complete",
			StartedAt: start.Add(time.Duration(i) * time.Minute),
			Path:      fmt.Sprintf("rec/%d.ts", i),
		})
		if err != nil {
			b.Fatal(err)
		}
		if i == n/2 {
			id = got
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rec, err := st.Recording(ctx, id)
		if err != nil || rec.ID != id || rec.Title != fmt.Sprintf("Show %d", n/2) {
			b.Fatal(err, rec.ID, rec.Title)
		}
	}
}
