package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestPruneKeepsSevenDailiesAndFourWeeklies(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dir := filepath.Join(root, "backups")
	start := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)

	var dailies []string
	for i := 0; i < 8; i++ {
		item, err := Take(ctx, st, dir, start.AddDate(0, 0, i), KindDaily)
		if err != nil {
			t.Fatal(err)
		}
		dailies = append(dailies, item.Name)
	}
	var weeklies []string
	for i := 0; i < 5; i++ {
		item, err := Take(ctx, st, dir, start.AddDate(0, 0, i*7).Add(time.Hour), KindWeekly)
		if err != nil {
			t.Fatal(err)
		}
		weeklies = append(weeklies, item.Name)
	}
	for i := 0; i < 9; i++ {
		name := Filename(start.Add(time.Duration(i)*time.Second), KindVersion)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, versionFile), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Prune(dir); err != nil {
		t.Fatal(err)
	}

	items, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotDaily := names(items, KindDaily)
	gotWeekly := names(items, KindWeekly)
	gotVersion := names(items, KindVersion)
	if len(gotDaily) != 7 {
		t.Fatalf("dailies = %d, want 7 (%v)", len(gotDaily), gotDaily)
	}
	if len(gotWeekly) != 4 {
		t.Fatalf("weeklies = %d, want 4 (%v)", len(gotWeekly), gotWeekly)
	}
	if len(gotVersion) != keepVersion {
		t.Fatalf("version copies = %d, want %d", len(gotVersion), keepVersion)
	}
	if contains(gotDaily, dailies[0]) || !contains(gotDaily, dailies[7]) {
		t.Fatalf("oldest daily stayed or newest dropped: %v", gotDaily)
	}
	if contains(gotWeekly, weeklies[0]) || !contains(gotWeekly, weeklies[4]) {
		t.Fatalf("oldest weekly stayed or newest dropped: %v", gotWeekly)
	}
	body, err := os.ReadFile(filepath.Join(dir, versionFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "1.2.3\n" {
		t.Fatalf("version file = %q", body)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AddPass(ctx, "Morning news", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "backups")
	when := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	item, err := Take(ctx, st, dir, when, KindDaily)
	if err != nil {
		t.Fatal(err)
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 {
		t.Fatalf("passes before delete: %+v", passes)
	}
	if err := st.DeletePass(ctx, passes[0].ID); err != nil {
		t.Fatal(err)
	}
	gone, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 0 {
		t.Fatalf("pass still present: %+v", gone)
	}
	media := filepath.Join(root, "evening.ts")
	if err := os.WriteFile(media, []byte("recording-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	recID, err := st.CreateRecording(ctx, store.Recording{
		Title: "Evening", Path: media, Status: "complete",
		StartedAt: time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Restore(ctx, st, dir, item.Name); err != nil {
		t.Fatal(err)
	}
	passes, err = st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || passes[0].Title != "Morning news" {
		t.Fatalf("restored passes: %+v", passes)
	}
	if _, err := st.Recording(ctx, recID); err != nil {
		t.Fatalf("recording row missing after restore: %v", err)
	}
	if _, err := os.Stat(media); err != nil {
		t.Fatalf("recording file removed: %v", err)
	}
}

func TestRestoreKeepsTheSourcePassword(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	raw := "http://lab:s3cret-pass@example.com/playlist.m3u"
	if _, err := st.AddSource(ctx, "m3u", "Lab", raw, ""); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "backups")
	item, err := Take(ctx, st, dir, time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC), KindDaily)
	if err != nil {
		t.Fatal(err)
	}
	if err := Restore(ctx, st, dir, item.Name); err != nil {
		t.Fatal(err)
	}
	sources, err := st.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources: %+v", sources)
	}
	if got := st.FetchURL(ctx, sources[0].ID, sources[0].URL); got != raw {
		t.Fatalf("fetch url = %q", got)
	}
}

func TestNightlyWritesOneWeeklyPerWeek(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dir := filepath.Join(root, "backups")
	monday := time.Date(2026, 6, 1, nightlyHour, 0, 0, 0, time.UTC)
	for monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, 1)
	}
	if err := Nightly(ctx, st, dir, monday); err != nil {
		t.Fatal(err)
	}
	if err := Nightly(ctx, st, dir, monday.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	items, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names(items, KindDaily)) != 2 || len(names(items, KindWeekly)) != 1 {
		t.Fatalf("same week: %+v", items)
	}
	if err := Nightly(ctx, st, dir, monday.AddDate(0, 0, 7)); err != nil {
		t.Fatal(err)
	}
	items, err = List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names(items, KindDaily)) != 3 || len(names(items, KindWeekly)) != 2 {
		t.Fatalf("next week: %+v", items)
	}
}

func TestBackupBeforeVersionChange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	when := time.Date(2026, 4, 2, 15, 4, 5, 0, time.UTC)
	if err := SnapshotIfVersionChanged(ctx, root, "1.0.0", when); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "backups")
	items, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("copied an empty catalog: %+v", items)
	}
	got, err := readVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.0.0" {
		t.Fatalf("version %q", got)
	}

	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Kept", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if err := SnapshotIfVersionChanged(ctx, root, "1.0.0", when.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err = List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("same version snapshotted: %+v", items)
	}
	if err := SnapshotIfVersionChanged(ctx, root, "1.1.0", when.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err = List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != KindVersion {
		t.Fatalf("version change: %+v", items)
	}
	if err := SnapshotIfVersionChanged(ctx, root, "1.1.0", when.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err = List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("repeated version change: %+v", items)
	}
	st, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || passes[0].Title != "Kept" {
		t.Fatalf("pass after snapshot: %+v", passes)
	}
}

func TestNextDailyUsesLocalDay(t *testing.T) {
	loc := time.FixedZone("L", -6*60*60)
	dir := t.TempDir()
	early := time.Date(2026, 7, 4, 1, 0, 0, 0, loc)
	due := nextDaily(early, dir)
	want := time.Date(2026, 7, 4, nightlyHour, 0, 0, 0, loc)
	if !due.Equal(want) {
		t.Fatalf("before quiet hour: got %s want %s", due, want)
	}
	late := time.Date(2026, 7, 4, 10, 0, 0, 0, loc)
	due = nextDaily(late, dir)
	if !due.Equal(late) {
		t.Fatalf("missed window: got %s want %s", due, late)
	}
	taken := time.Date(2026, 7, 4, 16, 0, 0, 0, time.UTC)
	if err := os.WriteFile(filepath.Join(dir, Filename(taken, KindDaily)), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	due = nextDaily(late, dir)
	want = time.Date(2026, 7, 5, nightlyHour, 0, 0, 0, loc)
	if !due.Equal(want) {
		t.Fatalf("already saved today: got %s want %s", due, want)
	}
	nextMorning := time.Date(2026, 7, 5, 1, 0, 0, 0, loc)
	due = nextDaily(nextMorning, dir)
	if !due.Equal(want) {
		t.Fatalf("next morning: got %s want %s", due, want)
	}
}

func TestResolveRejectsEscape(t *testing.T) {
	items, err := List("")
	if err != nil || len(items) != 0 {
		t.Fatalf("empty folder: %v %v", err, items)
	}
	dir := t.TempDir()
	name := Filename(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), KindDaily)
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve(dir, name); err != nil {
		t.Fatal(err)
	}
	bad := []string{"../" + name, name + "/x", "version", "broadwave-backup.db", "..", ""}
	for _, name := range bad {
		if _, _, err := Resolve(dir, name); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%q: %v", name, err)
		}
	}
}

func names(items []Item, kind string) []string {
	var out []string
	for _, item := range items {
		if item.Kind == kind {
			out = append(out, item.Name)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
