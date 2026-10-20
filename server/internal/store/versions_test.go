package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestDataVersionsCountWhatClientsSee(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	read := func() Versions {
		t.Helper()
		v, err := st.DataVersions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	dev := hdhr.Device{DeviceID: "D1", FriendlyName: "DUO", BaseURL: "http://127.0.0.1:1", TunerCount: 2}
	lineup := []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV"}, {GuideNumber: "5.1", GuideName: "KRVT"}}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	start := read()

	// The five-minute tuner refresh rewrites the same lineup and last-seen time.
	if err := st.RefreshDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.Channels != start.Channels {
		t.Fatalf("an unchanged refresh moved channels %d -> %d", start.Channels, got.Channels)
	}
	fav := true
	if _, err := st.PatchChannel(ctx, 1, ChannelPatch{Favorite: &fav}); err != nil {
		t.Fatal(err)
	}
	after := read()
	if after.Channels == start.Channels {
		t.Fatal("a favorite did not move channels")
	}
	if after.Guide != start.Guide || after.Recordings != start.Recordings {
		t.Fatalf("a favorite moved other data: %+v -> %+v", start, after)
	}

	at := time.Now().UTC().Truncate(time.Minute)
	if err := st.InsertAirings(ctx, []Airing{{ChannelID: 1, Title: "News", Start: at, End: at.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if read().Guide == after.Guide {
		t.Fatal("new listings did not move the guide")
	}
	guide := read().Guide
	// Linking the same game again changes nothing a client sees.
	if err := st.SetAiringGames(ctx, at.Add(-time.Hour), at.Add(2*time.Hour), map[int64]string{}); err != nil {
		t.Fatal(err)
	}
	if read().Guide != guide {
		t.Fatal("a no-op game link moved the guide")
	}

	if _, err := st.CreateRecording(ctx, Recording{ChannelID: 1, Title: "News", Status: "recording", StartedAt: at, Path: "/x.ts"}); err != nil {
		t.Fatal(err)
	}
	if read().Recordings == after.Recordings {
		t.Fatal("a recording did not move recordings")
	}
	sched := read().Schedule
	if err := st.AddPass(ctx, "News", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if read().Schedule == sched {
		t.Fatal("a pass did not move the schedule")
	}
	set := read().Settings
	if err := st.PutSettings(ctx, map[string]string{"layout": "tv"}); err != nil {
		t.Fatal(err)
	}
	if read().Settings == set {
		t.Fatal("a setting did not move settings")
	}
}

func TestSettingsStayInMemoryAndFollowWrites(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.PutSettings(ctx, map[string]string{"layout": "tv"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got["layout"] = "changed by a caller"
	again, _ := st.Settings(ctx)
	if again["layout"] != "tv" {
		t.Fatalf("a caller's change leaked into the kept settings: %q", again["layout"])
	}
	if on, _ := st.DeviceAuthOn(ctx); on {
		t.Fatal("sign-in on before it was turned on")
	}
	if _, err := st.EnableDeviceAuth(ctx, map[string]string{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if on, _ := st.DeviceAuthOn(ctx); !on {
		t.Fatal("sign-in still off after it was turned on")
	}
	if err := st.PutSettings(ctx, map[string]string{"deviceAuth": "0"}); err != nil {
		t.Fatal(err)
	}
	if on, _ := st.DeviceAuthOn(ctx); on {
		t.Fatal("sign-in still on after it was turned off")
	}
}

func TestPruneAiringsKeepsRecentHistory(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	if err := st.InsertAirings(ctx, []Airing{
		{ChannelID: 1, Title: "Old", Start: now.Add(-10 * 24 * time.Hour), End: now.Add(-10*24*time.Hour + time.Hour)},
		{ChannelID: 1, Title: "Yesterday", Start: now.Add(-24 * time.Hour), End: now.Add(-23 * time.Hour)},
		{ChannelID: 1, Title: "Now", Start: now, End: now.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	n, err := st.PruneAirings(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	rows, _ := st.Airings(ctx, now.Add(-30*24*time.Hour), now.Add(24*time.Hour))
	if len(rows) != 2 {
		t.Fatalf("left %d listings, want 2", len(rows))
	}
}

// Writers that read before they write used to fail at once with "database
// is locked" when another connection committed in between: busy_timeout
// does not cover a deferred transaction's upgrade to a writer.
func TestConcurrentReadThenWriteTransactionsWait(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				tx, err := st.db.BeginTx(ctx, nil)
				if err != nil {
					errs <- err
					return
				}
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings`).Scan(&n); err != nil {
					_ = tx.Rollback()
					errs <- err
					return
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)`, fmt.Sprintf("k%d-%d", w, i), fmt.Sprint(n)); err != nil {
					_ = tx.Rollback()
					errs <- err
					return
				}
				if err := tx.Commit(); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	var all []error
	for err := range errs {
		all = append(all, err)
	}
	if err := errors.Join(all...); err != nil {
		t.Fatal(err)
	}
}

// A copy is written under a temporary name and renamed into place. An older
// copy under the name stays whole until the new one is complete, and no
// half-written file is left behind.
func TestBackupToReplacesAtomically(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "copy.db")
	if err := os.WriteFile(path, []byte("older copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.BackupTo(ctx, path); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 16)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Read(head)
	f.Close()
	if string(head) != "SQLite format 3\x00" {
		t.Fatalf("copy is not a catalog: %q", head)
	}
	if err := st.BackupTo(ctx, filepath.Join(dir, "missing", "copy.db")); err == nil {
		t.Fatal("a copy into a missing folder succeeded")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".partial") {
			t.Fatalf("left %s behind", e.Name())
		}
	}
}
