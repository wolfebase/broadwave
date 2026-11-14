package main

import (
	"context"
	"testing"
	"time"

	"broadwave/internal/store"
)

// A staging run resumes nothing, but a recording the last run left
// "recording" is still closed: nothing writes it, and it could be neither
// stopped nor deleted.
func TestAStagingRestartClosesARecordingInProgress(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	ends := time.Now().Add(time.Hour)
	id, err := st.CreateRecording(ctx, store.Recording{Title: "News", GuideNumber: "4.1", Status: "recording",
		Path: "/recordings/news.ts", StartedAt: time.Now().Add(-10 * time.Minute), EndsAt: &ends})
	if err != nil {
		t.Fatal(err)
	}
	recoverRecordings(st, nil, true)
	rec, err := st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "interrupted" || !rec.Partial || rec.EndedAt == nil || rec.Path != "/recordings/news.ts" {
		t.Fatalf("after a staging restart: %+v", rec)
	}
	all, err := st.Recordings(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("staging resumed a show: %d rows %v", len(all), err)
	}
}
