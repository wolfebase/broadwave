package live

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/ring"
	"broadwave/internal/store"
)

// fullDisk is a recording's file on a disk that fills once full is set.
type fullDisk struct {
	f    *os.File
	full *atomic.Bool
}

func (d fullDisk) Write(b []byte) (int, error) {
	if d.full.Load() {
		return 0, &os.PathError{Op: "write", Path: d.f.Name(), Err: syscall.ENOSPC}
	}
	return d.f.Write(b)
}

func (d fullDisk) Close() error { return d.f.Close() }

// rawLabTune is labTune on an AC-4 channel, whose recording writes its own
// file, on a disk that fills when the returned flag is set.
func rawLabTune(t *testing.T) (*Hub, *store.Store, int64, *atomic.Bool) {
	t.Helper()
	full := &atomic.Bool{}
	prev := createRecording
	createRecording = func(path string) (io.WriteCloser, error) {
		f, err := prev(path)
		if err != nil {
			return nil, err
		}
		return fullDisk{f: f.(*os.File), full: full}, nil
	}
	t.Cleanup(func() { createRecording = prev })
	dir := t.TempDir()
	h, st, city := labTune(t, dir, filepath.Join(dir, "no-ffmpeg"))
	h.channels[city].channel.AudioCodec = "ac4"
	return h, st, city, full
}

func packets(n int) []byte {
	b := make([]byte, 188*n)
	for i := 0; i < len(b); i += 188 {
		b[i] = 0x47
	}
	return b
}

func feedMux(m *mux, chunk []byte) {
	for _, sub := range m.noteLead(chunk) {
		sub.offer(chunk, m.freq)
	}
}

func waitFailed(t *testing.T, h *Hub, st *store.Store, id, channel int64) {
	t.Helper()
	ctx := context.Background()
	waitUntil(t, 3*time.Second, "the recording to fail", func() bool {
		got, err := st.Recording(ctx, id)
		return err == nil && got.Status != "recording"
	})
	got, _ := st.Recording(ctx, id)
	if want := (&disk.WriteError{Full: true}).Error(); got.Status != "failed" || got.Error != want {
		t.Fatalf("status %s error %q, want failed %q", got.Status, got.Error, want)
	}
	h.mu.Lock()
	tuned := h.channels[channel] != nil
	h.mu.Unlock()
	if tuned {
		t.Fatal("a failed recording kept its tuner")
	}
}

// A raw recording whose write failed stopped writing, stayed recording, held
// its tuner, and ended complete.
func TestARawRecordingThatCantWriteFails(t *testing.T) {
	h, st, city, full := rawLabTune(t)
	m := muxOf(h, h.channels[city])
	rec, err := h.RecordMeta(context.Background(), 60, store.Recording{ChannelID: city, Title: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.StopRecord(rec.ID) })
	feedMux(m, packets(10))
	full.Store(true)
	feedMux(m, packets(10))
	waitFailed(t, h, st, rec.ID, city)
}

// A backfill whose write failed handed the recording to the live fan-out with
// no subscriber and armed its stop: it ended complete, holding its tuner till then.
func TestARawBackfillThatCantWriteFails(t *testing.T) {
	h, st, city, full := rawLabTune(t)
	m := muxOf(h, h.channels[city])
	m.ring = ring.Open(t.TempDir(), ring.Options{Window: func() time.Duration { return time.Hour }})
	t.Cleanup(m.ring.Close)
	showStart := time.Now()
	feedMux(m, packets(100))
	time.Sleep(50 * time.Millisecond)
	full.Store(true)
	rec, err := h.RecordMeta(context.Background(), 60, store.Recording{ChannelID: city, Title: "Evening", StartedAt: showStart.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.StopRecord(rec.ID) })
	waitFailed(t, h, st, rec.ID, city)
}

// A start that failed wrote the failure with the request's context: a client
// that had gone left the row recording until the next boot.
func TestAFailedStartIsSavedAfterTheClientLeaves(t *testing.T) {
	h, st, city, _ := rawLabTune(t)
	ctx, cancel := context.WithCancel(context.Background())
	prev := createRecording
	createRecording = func(string) (io.WriteCloser, error) {
		cancel()
		return nil, &os.PathError{Op: "open", Path: "show.ts", Err: syscall.EIO}
	}
	t.Cleanup(func() { createRecording = prev })
	if _, err := h.RecordMeta(ctx, 60, store.Recording{ChannelID: city, Title: "Evening"}); err == nil {
		t.Fatal("a start that could not make its file succeeded")
	}
	recs, err := st.Recordings(context.Background())
	if err != nil || len(recs) != 1 || recs[0].Status != "failed" {
		t.Fatalf("recordings %+v %v", recs, err)
	}
}

// shareFile is a recording's file on a share whose last flush fails.
type shareFile struct{ *os.File }

func (f shareFile) Close() error {
	_ = f.File.Close()
	return &os.PathError{Op: "close", Path: f.Name(), Err: syscall.EIO}
}

// The error from a raw recording's last close (a failed flush to a share) was
// ignored, and the recording ended complete.
func TestARawRecordingWhoseCloseFailsFails(t *testing.T) {
	h, st, city, _ := rawLabTune(t)
	prev := createRecording
	createRecording = func(path string) (io.WriteCloser, error) {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, err
		}
		return shareFile{f}, nil
	}
	t.Cleanup(func() { createRecording = prev })
	m := muxOf(h, h.channels[city])
	rec, err := h.RecordMeta(context.Background(), 60, store.Recording{ChannelID: city, Title: "Evening"})
	if err != nil {
		t.Fatal(err)
	}
	feedMux(m, packets(10))
	time.Sleep(100 * time.Millisecond)
	h.StopRecord(rec.ID)
	got, _ := st.Recording(context.Background(), rec.ID)
	if got.Status != "failed" || !strings.Contains(got.Error, "input/output error") {
		t.Fatalf("status %s error %q", got.Status, got.Error)
	}
}
