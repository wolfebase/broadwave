package dvr

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestOnSavedStoresRecordingHealth(t *testing.T) {
	ctx := context.Background()
	clean := tsPackets(0x100, 0, 1, 2, 3, 4)
	damaged := tsPackets(0x100, 0, 1, 3, 4)

	t.Run("continuity errors", func(t *testing.T) {
		st, rec := finishedRecording(t, damaged)
		OnSaved(ctx, st, nil, nil, rec)
		got := mustRecording(t, st, rec.ID)
		if got.Status != "complete" || got.Error != "" {
			t.Fatalf("status %q error %q", got.Status, got.Error)
		}
		if got.Health == nil {
			t.Fatal("health was not stored")
		}
		if got.Health.ContinuityErrors != 1 || got.Health.TransportErrors != 0 || got.Health.SyncLosses != 0 || got.Health.Packets != 4 {
			t.Fatalf("%+v", got.Health)
		}
	})

	t.Run("clean file", func(t *testing.T) {
		st, rec := finishedRecording(t, clean)
		OnSaved(ctx, st, nil, nil, rec)
		got := mustRecording(t, st, rec.ID)
		if got.Status != "complete" || got.Error != "" {
			t.Fatalf("status %q error %q", got.Status, got.Error)
		}
		if got.Health == nil {
			t.Fatal("a clean file should store zeros")
		}
		if got.Health.ContinuityErrors != 0 || got.Health.TransportErrors != 0 || got.Health.SyncLosses != 0 || got.Health.Packets != 5 {
			t.Fatalf("%+v", got.Health)
		}
	})

	t.Run("deleted mid-read", func(t *testing.T) {
		st, rec := finishedRecording(t, clean)
		orig := healthOpen
		t.Cleanup(func() { healthOpen = orig })
		healthOpen = func(path string) (io.ReadCloser, error) {
			return &dropMidRead{path: path, rest: clean}, nil
		}
		OnSaved(ctx, st, nil, nil, rec)
		got := mustRecording(t, st, rec.ID)
		if got.Status != "complete" || got.Error != "" {
			t.Fatalf("status %q error %q", got.Status, got.Error)
		}
		if got.Health != nil {
			t.Fatalf("stored a partial count: %+v", got.Health)
		}
		if _, err := os.Stat(rec.Path); !os.IsNotExist(err) {
			t.Fatalf("file still present: %v", err)
		}
	})
}

func TestHealthReadKeepsToItsRate(t *testing.T) {
	orig := healthRate
	t.Cleanup(func() { healthRate = orig })
	healthRate = 188 * 20
	path := filepath.Join(t.TempDir(), "a.ts")
	if err := os.WriteFile(path, tsPackets(0x100, 0, 1, 2, 3, 4), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	sum, err := readHealth(path)
	if err != nil || sum.Packets != 5 {
		t.Fatalf("%+v %v", sum, err)
	}
	// Five packets at twenty a second.
	if took := time.Since(start); took < 200*time.Millisecond {
		t.Fatalf("read in %s", took)
	}
}

func finishedRecording(t *testing.T, body []byte) (*store.Store, store.Recording) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	path := filepath.Join(t.TempDir(), "Evening News.ts")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(context.Background(), store.Recording{
		ChannelID: 4, Title: "Evening News", Status: "complete", Path: path,
		StartedAt: time.Date(2026, 9, 28, 22, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Recording(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return st, rec
}

func mustRecording(t *testing.T, st *store.Store, id int64) store.Recording {
	t.Helper()
	rec, err := st.Recording(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// dropMidRead returns one packet, then removes the path, the way a file
// deleted during the read looks to the next chunk.
type dropMidRead struct {
	path string
	rest []byte
	sent bool
}

func (d *dropMidRead) Read(p []byte) (int, error) {
	if d.sent {
		return 0, io.EOF
	}
	n := 188
	if n > len(p) {
		n = len(p)
	}
	if n > len(d.rest) {
		n = len(d.rest)
	}
	copy(p[:n], d.rest[:n])
	d.sent = true
	if err := os.Remove(d.path); err != nil {
		return n, err
	}
	return n, nil
}

func (d *dropMidRead) Close() error { return nil }

func tsPackets(pid uint16, ccs ...int) []byte {
	out := make([]byte, 0, 188*len(ccs))
	for _, cc := range ccs {
		p := make([]byte, 188)
		p[0] = 0x47
		p[1] = byte(pid >> 8)
		p[2] = byte(pid)
		p[3] = 0x10 | byte(cc&0x0f)
		out = append(out, p...)
	}
	return out
}
