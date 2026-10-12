package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestRunReturnsWithoutACatalog(t *testing.T) {
	returns := func(s *Scheduler) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.Run(context.Background())
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not return")
		}
	}
	var missing *Scheduler
	done := make(chan struct{})
	go func() {
		defer close(done)
		missing.Run(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	returns(&Scheduler{})
	returns(&Scheduler{Dir: t.TempDir()})

	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	returns(&Scheduler{Store: st})
}

func TestNowUsesTheOverrideOrTheClock(t *testing.T) {
	fixed := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	s := &Scheduler{Now: func() time.Time { return fixed }}
	if got := s.now(); !got.Equal(fixed) {
		t.Fatalf("override %s", got)
	}
	s.Now = nil
	if got := s.now(); !closeToNow(got) {
		t.Fatalf("clock %s", got)
	}
	var missing *Scheduler
	if got := missing.now(); !closeToNow(got) {
		t.Fatalf("nil scheduler %s", got)
	}
}

func TestWaitIsUntilTheNextQuietHour(t *testing.T) {
	loc := time.FixedZone("L", -6*60*60)
	dir := t.TempDir()
	s := &Scheduler{Dir: dir}

	early := time.Date(2026, 7, 4, 1, 0, 0, 0, loc)
	today := time.Date(2026, 7, 4, nightlyHour, 0, 0, 0, loc)
	if got, want := s.wait(early), today.Sub(early); got != want || want <= 0 {
		t.Fatalf("before quiet hour: got %s want %s", got, want)
	}
	late := time.Date(2026, 7, 4, 10, 0, 0, 0, loc)
	if got := s.wait(late); got != 0 {
		t.Fatalf("missed window: got %s want 0", got)
	}

	taken := time.Date(2026, 7, 4, 16, 0, 0, 0, time.UTC)
	if err := os.WriteFile(filepath.Join(dir, Filename(taken, KindDaily)), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	due := nextDaily(late, dir)
	tomorrow := time.Date(2026, 7, 5, nightlyHour, 0, 0, 0, loc)
	if !due.Equal(tomorrow) {
		t.Fatalf("next daily %s", due)
	}
	if got, want := s.wait(late), due.Sub(late); got != want || want <= 0 {
		t.Fatalf("already saved today: got %s want %s", got, want)
	}
}

func TestRunTakesTheDailyThatIsDue(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	dir := filepath.Join(root, "backups")
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	s := &Scheduler{
		Store: st,
		Dir:   dir,
		Now:   func() time.Time { return now },
	}
	if s.wait(now) != 0 {
		t.Fatalf("wait %s", s.wait(now))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	deadline := time.Now().Add(15 * time.Second)
	for {
		items, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		// The weekly is copied from the finished daily, so it marks the end of the run.
		if len(names(items, KindWeekly)) > 0 {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("daily and weekly were not saved")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	items, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	var daily *Item
	for i := range items {
		if items[i].Kind != KindDaily {
			continue
		}
		if daily != nil {
			t.Fatalf("dailies %+v", items)
		}
		daily = &items[i]
	}
	if daily == nil || daily.Bytes == 0 {
		t.Fatalf("dailies %+v", items)
	}
}

func TestRunRetriesWhenTheFolderIsAFile(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	dir := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 4, nightlyHour+1, 0, 0, 0, time.UTC)
	s := &Scheduler{
		Store: st,
		Dir:   dir,
		Now:   func() time.Time { return now },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	select {
	case <-done:
		t.Fatal("Run returned before the retry wait")
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("mode %s", info.Mode())
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	const body = "catalog-bytes"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("missing source", func(t *testing.T) {
		dst := filepath.Join(dir, "missing-dst")
		if err := copyFile(filepath.Join(dir, "no-such-file"), dst); err == nil {
			t.Fatal("missing source")
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Fatalf("dest created: %v", err)
		}
	})

	t.Run("copies", func(t *testing.T) {
		dst := filepath.Join(dir, "dst")
		if err := os.WriteFile(dst+".partial", []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := copyFile(src, dst); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Fatalf("copy = %q", got)
		}
		if _, err := os.Stat(dst + ".partial"); !os.IsNotExist(err) {
			t.Fatalf("partial left behind: %v", err)
		}
	})

	t.Run("missing parent", func(t *testing.T) {
		dst := filepath.Join(dir, "missing-dir", "dst")
		if err := copyFile(src, dst); err == nil {
			t.Fatal("missing parent")
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Fatalf("dest created: %v", err)
		}
		if _, err := os.Stat(dst + ".partial"); !os.IsNotExist(err) {
			t.Fatalf("partial left behind: %v", err)
		}
	})
}

func closeToNow(got time.Time) bool {
	now := time.Now()
	return !got.Before(now.Add(-time.Second)) && !got.After(now.Add(time.Second))
}
