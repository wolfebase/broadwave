package live

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

// quick fails when the hub lock, which every playlist reload takes, is held
// for long while slow is running.
func quick(t *testing.T, h *Hub, what string, slow func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		slow()
		close(done)
	}()
	time.Sleep(300 * time.Millisecond)
	began := time.Now()
	h.Status()
	if took := time.Since(began); took > 300*time.Millisecond {
		t.Errorf("the hub lock waited %v on %s", took, what)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not return", what)
	}
}

// A stop held the hub lock while the copy finished its file and the row and
// sidecar were written: every viewer's playlist stalled at each recording's
// end on a slow share.
func TestStoppingARecordingDoesNotHoldTheHub(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bin := filepath.Join(dir, "slow-ffmpeg")
	script := "#!/bin/sh\nfor a; do out=$a; done\ncat > \"$out\"\nsleep 1.5\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h, st, city := labTune(t, dir, bin)
	rec, err := h.RecordMeta(ctx, 60, store.Recording{ChannelID: city, Title: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	quick(t, h, "a recording's stop", func() { h.StopRecord(rec.ID) })
	if got, _ := st.Recording(ctx, rec.ID); got.Status != "complete" {
		t.Fatalf("status %s once the stop returned", got.Status)
	}
}

// A start made the recording's folder and file and its row under the hub lock.
func TestStartingARecordingDoesNotHoldTheHub(t *testing.T) {
	h, st, city, _ := rawLabTune(t)
	prev := createRecording
	createRecording = func(path string) (io.WriteCloser, error) {
		time.Sleep(time.Second)
		return prev(path)
	}
	t.Cleanup(func() { createRecording = prev })
	var rec store.Recording
	quick(t, h, "a recording's start", func() {
		var err error
		if rec, err = h.RecordMeta(context.Background(), 60, store.Recording{ChannelID: city, Title: "Evening"}); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() { h.StopRecord(rec.ID) })
	if got, _ := st.Recording(context.Background(), rec.ID); got.Status != "recording" {
		t.Fatalf("status %s", got.Status)
	}
}

// A shutdown while a recording starts outside the hub lock waits for the
// start, and the row ends interrupted, so the next start records the rest.
func TestAShutdownDuringARecordingStartWaitsForIt(t *testing.T) {
	h, st, city, _ := rawLabTune(t)
	prev := createRecording
	createRecording = func(path string) (io.WriteCloser, error) {
		time.Sleep(time.Second)
		return prev(path)
	}
	t.Cleanup(func() { createRecording = prev })
	started := make(chan error, 1)
	go func() {
		_, err := h.RecordMeta(context.Background(), 60, store.Recording{ChannelID: city, Title: "Evening"})
		started <- err
	}()
	time.Sleep(300 * time.Millisecond)
	h.Shutdown()
	// Read as the shutdown returns: it waited for the row.
	recs, err := st.Recordings(context.Background())
	if err != nil || len(recs) != 1 || recs[0].Status != "interrupted" {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	select {
	case err := <-started:
		if err == nil {
			t.Fatal("a start the shutdown ended reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("the start did not return")
	}
}
