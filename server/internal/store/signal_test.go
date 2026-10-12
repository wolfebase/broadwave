package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSaveFrequencySignalWritesEveryChannel(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if _, err := st.db.Exec(`INSERT INTO devices (device_id, friendly_name, base_url, tuner_count) VALUES ('dev', 'Tuner', 'http://127.0.0.1', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`INSERT INTO channels (device_id, guide_number, guide_name, frequency_hz, present) VALUES ('dev', '4.1', 'KBWV', 593000000, 1), ('dev', '4.2', 'KBWV2', 593000000, 1), ('dev', '5.1', 'WTST', 533000000, 1)`); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	if err := st.SaveFrequencySignal(ctx, 593000000, true, 90, 88, 100, when); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ChannelSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, row := range rows {
		if row.HasReading {
			got[row.GuideNumber] = row.Strength == 90 && row.Quality == 88 && row.Symbol == 100
		}
	}
	if !got["4.1"] || !got["4.2"] || got["5.1"] {
		t.Fatalf("%+v", rows)
	}
}

func TestAntennaListSortsAShown3NextToIts1(t *testing.T) {
	s, ctx := openTwins(t)
	rows, err := s.ChannelSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var numbers []string
	for _, row := range rows {
		numbers = append(numbers, row.GuideNumber)
	}
	// Two tuners each have a 4.1. The shown 104.1 sits with them, and 119.1 sits
	// with 19.1. The unpaired 105.1 keeps its own number.
	want := []string{"4.1", "4.1", "104.1", "5.1", "19.1", "119.1", "19.2", "105.1"}
	if strings.Join(numbers, ",") != strings.Join(want, ",") {
		t.Fatalf("antenna order %v, want %v", numbers, want)
	}

	var shown int64
	for _, ch := range mustChannels(t, s, ctx) {
		if ch.GuideNumber == "104.1" {
			shown = ch.ID
		}
	}
	number := "12.1"
	if _, err := s.PatchChannel(ctx, shown, ChannelPatch{CustomNumber: &number}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ChannelSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	numbers = numbers[:0]
	for _, row := range rows {
		numbers = append(numbers, row.GuideNumber)
	}
	// A number the viewer set is where that row sits. The label stays 104.1.
	want = []string{"4.1", "4.1", "5.1", "104.1", "19.1", "119.1", "19.2", "105.1"}
	if strings.Join(numbers, ",") != strings.Join(want, ",") {
		t.Fatalf("renamed antenna order %v, want %v", numbers, want)
	}
}

func mustChannels(t *testing.T, s *Store, ctx context.Context) []Channel {
	t.Helper()
	channels, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	return channels
}
