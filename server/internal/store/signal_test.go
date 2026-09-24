package store

import (
	"context"
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
	if _, err := st.db.Exec(`INSERT INTO channels (device_id, guide_number, guide_name, frequency_hz, present) VALUES ('dev', '4.1', 'WDAF', 593000000, 1), ('dev', '4.2', 'WDAF2', 593000000, 1), ('dev', '5.1', 'KCTV', 533000000, 1)`); err != nil {
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
