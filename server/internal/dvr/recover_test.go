package dvr

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"waveguide/internal/store"
)

func TestRecoverFailsAFinishedShowAndResumesOneStillOn(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(40 * time.Minute)
	done, err := st.CreateRecording(context.Background(), store.Recording{
		ChannelID: 1, Title: "News", Path: "news.ts", Status: "recording", StartedAt: now.Add(-time.Hour), EndsAt: &past,
	})
	if err != nil {
		t.Fatal(err)
	}
	still, err := st.CreateRecording(context.Background(), store.Recording{
		ChannelID: 2, Title: "Game", Path: "game.ts", Status: "recording", StartedAt: now.Add(-20 * time.Minute), EndsAt: &future,
	})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := st.CreateRecording(context.Background(), store.Recording{
		ChannelID: 3, Title: "Saved", Path: "saved.ts", Status: "complete", StartedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	var resumed []store.Recording
	err = Recover(context.Background(), st, now, func(rec store.Recording, left time.Duration) error {
		if left < 39*time.Minute {
			t.Fatalf("remaining time %s", left)
		}
		resumed = append(resumed, rec)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed) != 1 || resumed[0].ID != still {
		t.Fatalf("resumed %v", resumed)
	}
	for _, id := range []int64{done, still} {
		rec, err := st.Recording(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Status != "failed" || rec.Error != "Stopped when the server restarted." {
			t.Fatalf("recording %d status %q error %q", id, rec.Status, rec.Error)
		}
	}
	rec, err := st.Recording(context.Background(), kept)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "complete" {
		t.Fatalf("a finished recording was changed: %s", rec.Status)
	}
}
