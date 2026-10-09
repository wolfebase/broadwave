package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

// BenchmarkSearch is a guide of 500 channels across two weeks, half-hour
// listings. Every description says bulletin. One listing is titled Cedar Quiz,
// and a hidden channel repeats that title.
func BenchmarkSearch(b *testing.B) {
	const channels = 500
	const days = 14
	st, ids := loadSearchBench(b, channels, days)
	ctx := context.Background()
	from := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	hits, _, err := st.Search(ctx, "cedar quiz", from, 20)
	if err != nil {
		b.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Title != "Cedar Quiz" || hits[0].ChannelID != ids[6] {
		b.Fatalf("title hit %+v", hits)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.Run("Title", func(b *testing.B) {
		for b.Loop() {
			hits, _, err := st.Search(ctx, "cedar quiz", from, 20)
			if err != nil || len(hits) != 1 {
				b.Fatal(err, len(hits))
			}
		}
	})
	b.Run("Word", func(b *testing.B) {
		for b.Loop() {
			hits, _, err := st.Search(ctx, "bulletin", from, 20)
			if err != nil || len(hits) != 20 {
				b.Fatal(err, len(hits))
			}
		}
	})
	until := from.Add(14 * 24 * time.Hour)
	b.Run("Listed", func(b *testing.B) {
		for b.Loop() {
			listed, err := st.ChannelsListedBetween(ctx, from, until)
			if err != nil || len(listed) != channels {
				b.Fatal(err, len(listed))
			}
		}
	})
	b.Run("Depth", func(b *testing.B) {
		for b.Loop() {
			depth, err := st.ListingDepth(ctx, from)
			if err != nil || len(depth) != channels || depth[ids[6]] != 12 {
				b.Fatal(err, len(depth), depth[ids[6]])
			}
		}
	})
	b.Run("Window", func(b *testing.B) {
		for b.Loop() {
			total, _, rows, err := st.GuideWindow(ctx, from, until)
			if err != nil || len(rows) != channels || total != channels*648+2 {
				b.Fatal(err, len(rows), total)
			}
		}
	})
	b.Run("Channel", func(b *testing.B) {
		for b.Loop() {
			rows, err := st.QueryAirings(ctx, AiringQuery{From: from, To: until, Channels: []int64{ids[6]}})
			if err != nil || len(rows) != 649 {
				b.Fatal(err, len(rows))
			}
		}
	})
}

func loadSearchBench(b testing.TB, channels, days int) (*Store, []int64) {
	b.Helper()
	st, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	ctx := context.Background()
	lineup := make([]hdhr.Channel, channels)
	for i := range channels {
		lineup[i] = hdhr.Channel{GuideNumber: fmt.Sprintf("%d.1", i+1), GuideName: fmt.Sprintf("Harbor %d", i+1)}
	}
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "Duo", BaseURL: "http://127.0.0.1", TunerCount: 1}, lineup); err != nil {
		b.Fatal(err)
	}
	all, err := st.Channels(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	ids := make([]int64, channels)
	var hidden int64
	for _, ch := range all {
		var n int
		if _, err := fmt.Sscanf(ch.GuideNumber, "%d.1", &n); err != nil || n < 1 || n > channels {
			b.Fatalf("channel %s", ch.GuideNumber)
		}
		ids[n-1] = ch.ID
		if n == channels {
			hidden = ch.ID
		}
	}
	hide := true
	if _, err := st.PatchChannel(ctx, hidden, ChannelPatch{Hidden: &hide}); err != nil {
		b.Fatal(err)
	}
	start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	slots := days * 48
	rows := make([]Airing, 0, channels*slots+1)
	for c := range channels {
		id := ids[c]
		for slot := range slots {
			at := start.Add(time.Duration(slot) * 30 * time.Minute)
			rows = append(rows, Airing{
				ChannelID: id, Title: "Evening News", Description: "The evening bulletin",
				Start: at, End: at.Add(30 * time.Minute),
			})
		}
	}
	quiz := start.Add(7 * 24 * time.Hour)
	rows = append(rows, Airing{ChannelID: ids[6], Title: "Cedar Quiz", Description: "A quiet evening", Start: quiz, End: quiz.Add(30 * time.Minute)})
	rows = append(rows, Airing{ChannelID: hidden, Title: "Cedar Quiz", Description: "A quiet evening", Start: quiz, End: quiz.Add(30 * time.Minute)})
	if err := st.InsertAirings(ctx, rows); err != nil {
		b.Fatal(err)
	}
	return st, ids
}
