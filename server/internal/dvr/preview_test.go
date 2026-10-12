package dvr

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func previewIDs(items []StoragePlanItem, rule string) []int64 {
	var out []int64
	for _, item := range items {
		if rule == "" || item.Rule == rule {
			out = append(out, item.ID)
		}
	}
	return out
}

func previewTitles(items []StoragePlanItem, rule string) []string {
	var out []string
	for _, item := range items {
		if rule == "" || item.Rule == rule {
			out = append(out, item.Title)
		}
	}
	return out
}

func TestPreviewDeletesNothing(t *testing.T) {
	r := newStorageRig(t, map[string]string{"deleteWatchedDays": "1", "makeRoom": "1"})
	rec := r.add(t, "marked", 1, false, "")
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) { return disk.Space{Free: 1, Total: 100}, nil }
	before := r.left(t)
	plan, err := Preview(context.Background(), r.st, r.hub, time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.left(t); !slices.Equal(got, before) {
		t.Fatalf("preview removed %v, left %v", before, got)
	}
	if _, err := os.Stat(rec.Path); err != nil {
		t.Fatalf("file: %v", err)
	}
	if !slices.Equal(previewTitles(plan.Remove, "watchedDays"), []string{"marked"}) {
		t.Fatalf("remove %+v", plan.Remove)
	}
}

func TestPreviewMatchesTheDaysItWouldDelete(t *testing.T) {
	r := newStorageRig(t, map[string]string{"deleteWatchedDays": "7"})
	r.add(t, "marked", 1, false, "")
	r.add(t, "played", 2, false, "")
	r.add(t, "unwatched", 0, false, "")
	r.add(t, "kept", 1, true, "")
	r.add(t, "stopped short", 3, false, "")
	r.addAs(t, "failed", 1, false, "", "failed")
	r.addAs(t, "on now", 1, false, "", "recording")
	missing := r.add(t, "gone", 1, false, "")
	if err := os.Remove(missing.Path); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(8 * 24 * time.Hour)
	plan, err := Preview(context.Background(), r.st, r.hub, when)
	if err != nil {
		t.Fatal(err)
	}
	Tidy(context.Background(), r.st, r.hub, when)
	left := r.left(t)
	for _, title := range previewTitles(plan.Remove, "") {
		if slices.Contains(left, title) {
			t.Fatalf("preview said %s would go, and it stayed: %v", title, left)
		}
	}
	for _, title := range []string{"unwatched", "kept", "stopped short", "failed", "on now", "gone"} {
		if !slices.Contains(left, title) {
			t.Fatalf("%s was removed: %v (plan %+v)", title, left, plan)
		}
	}
	if !slices.Contains(previewTitles(plan.Held, "kept"), "kept") {
		t.Fatalf("kept %+v", plan.Held)
	}
	if !slices.Contains(previewTitles(plan.Held, "recording"), "on now") {
		t.Fatalf("recording %+v", plan.Held)
	}
	if !slices.Contains(previewTitles(plan.Held, "missing"), "gone") {
		t.Fatalf("missing %+v", plan.Held)
	}
	for _, item := range plan.Remove {
		if item.Title == "on now" || item.Title == "kept" || item.Title == "gone" {
			t.Fatalf("protected recording in remove: %+v", item)
		}
	}
}

func TestPreviewMatchesMakeRoom(t *testing.T) {
	r := newStorageRig(t, map[string]string{"makeRoom": "1", "watermarkGB": "1"})
	r.add(t, "unwatched", 0, false, "")
	r.add(t, "kept", 1, true, "")
	r.add(t, "older", 1, false, "")
	r.add(t, "newer", 1, false, "")
	r.add(t, "newest", 1, false, "")
	r.addAs(t, "on now", 1, false, "", "recording")
	missing := r.add(t, "gone", 1, false, "")
	if err := os.Remove(missing.Path); err != nil {
		t.Fatal(err)
	}
	// The reserve is 1 GB. Free space is 150 bytes under it. Each file is 100.
	// The missing one adds nothing, so two real files go and the newest stays.
	reserve := disk.WatermarkBytes("1")
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) {
		return disk.Space{Free: reserve - 150, Total: reserve * 2}, nil
	}
	plan, err := Preview(context.Background(), r.st, r.hub, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Covers || plan.ShortBytes != 150 || plan.ReserveBytes != reserve {
		t.Fatalf("cover %+v short %d reserve %d", plan.Covers, plan.ShortBytes, plan.ReserveBytes)
	}
	if got := previewTitles(plan.Remove, "makeRoom"); !slices.Equal(got, []string{"older", "newer"}) {
		t.Fatalf("remove %v", got)
	}
	if slices.Contains(previewTitles(plan.Remove, ""), "gone") || slices.Contains(previewTitles(plan.Remove, ""), "on now") || slices.Contains(previewTitles(plan.Remove, ""), "kept") {
		t.Fatalf("protected in remove %+v", plan.Remove)
	}
	MakeRoom(context.Background(), r.st, r.hub, reserve)
	if got := r.left(t); !slices.Equal(got, []string{"gone", "kept", "newest", "on now", "unwatched"}) {
		t.Fatalf("left %v, plan %+v", got, plan.Remove)
	}
}

func TestPreviewRefusesMakeRoomWhenTheWatchedFilesAreTooSmall(t *testing.T) {
	r := newStorageRig(t, map[string]string{"makeRoom": "1", "watermarkGB": "1"})
	r.add(t, "watched", 1, false, "")
	// 200 bytes short, and the one watched file is 100. Neither the preview
	// nor the clean-up deletes it.
	reserve := disk.WatermarkBytes("1")
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) {
		return disk.Space{Free: reserve - 200, Total: reserve * 2}, nil
	}
	plan, err := Preview(context.Background(), r.st, r.hub, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Covers || len(plan.Remove) != 0 || plan.ShortBytes != 200 || plan.RoomNote == "" {
		t.Fatalf("plan %+v", plan)
	}
	MakeRoom(context.Background(), r.st, r.hub, reserve)
	if got := r.left(t); !slices.Equal(got, []string{"watched"}) {
		t.Fatalf("left %v", got)
	}
}

func TestPreviewKeepRulesSkipProtectedRecordings(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string) string {
		t.Helper()
		path := filepath.Join(root, name+".ts")
		if err := os.WriteFile(path, make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	older := now.Add(-72 * time.Hour)
	recs := []store.Recording{
		{ID: 1, Title: "News", Status: "complete", Path: write("old"), StartedAt: older},
		{ID: 2, Title: "News", Status: "complete", Path: write("mid"), StartedAt: old},
		{ID: 3, Title: "News", Status: "complete", Path: write("new"), StartedAt: now},
		{ID: 4, Title: "News", Status: "recording", Path: write("live"), StartedAt: now.Add(time.Minute)},
		{ID: 5, Title: "News", Status: "complete", Path: filepath.Join(root, "missing.ts"), StartedAt: now.Add(2 * time.Minute)},
		{ID: 6, Title: "News", Status: "complete", Path: write("kept"), StartedAt: older.Add(-time.Hour), Keep: true},
		// Oldest on purpose: with the rule off it is the one a keep-1 drops.
		{ID: 7, Title: "News", Status: "complete", Path: write("fav"), StartedAt: older.Add(-2 * time.Hour), ChannelID: 9},
	}
	pass := store.Pass{ID: 1, Title: "News", KeepMode: "last", KeepCount: 1}
	plan := buildPreview(previewInput{
		Now: now, ProtectFavorites: true, Root: root,
		Favorites: map[int64]bool{9: true}, Passes: []store.Pass{pass}, Recs: recs,
	})
	// Newest real episode stays. The favorite and the missing file do not
	// take that slot, and the one still recording does not either.
	if got := previewIDs(plan.Remove, "keepLast"); !slices.Equal(got, []int64{2, 1}) {
		t.Fatalf("remove %v (all %+v)", got, plan.Remove)
	}
	for _, item := range plan.Remove {
		if item.When != "next" {
			t.Fatalf("when %+v", item)
		}
	}
	if !slices.Contains(previewIDs(plan.Held, "recording"), int64(4)) ||
		!slices.Contains(previewIDs(plan.Held, "missing"), int64(5)) ||
		!slices.Contains(previewIDs(plan.Held, "kept"), int64(6)) ||
		!slices.Contains(previewIDs(plan.Held, "favorite"), int64(7)) {
		t.Fatalf("held %+v", plan.Held)
	}
	off := buildPreview(previewInput{
		Now: now, Root: root, Passes: []store.Pass{pass}, Recs: recs,
		Favorites: map[int64]bool{9: true},
	})
	if !slices.Contains(previewIDs(off.Remove, "keepLast"), int64(7)) {
		t.Fatalf("favorite should count when the rule is off: %+v", off.Remove)
	}
}

func TestPreviewKeepUnwatchedUsesPlayed(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := func(name string) string {
		t.Helper()
		p := filepath.Join(root, name+".ts")
		if err := os.WriteFile(p, []byte{1}, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	recs := []store.Recording{
		{ID: 1, Title: "News", Status: "complete", Path: path("end"), Position: 1790, Duration: 1800},
		{ID: 2, Title: "News", Status: "complete", Path: path("most"), Position: 1700, Duration: 1800},
		{ID: 3, Title: "News", Status: "complete", Path: path("fresh"), Position: 10, Duration: 1800},
		{ID: 4, Title: "News", Status: "complete", Path: path("unmarked"), Watched: 2, Position: 1800, Duration: 1800},
	}
	plan := buildPreview(previewInput{
		Now: time.Now(), Root: root,
		Passes: []store.Pass{{Title: "News", KeepMode: "unwatched"}}, Recs: recs,
	})
	// Played to the end, and stopped at 90%. A few seconds in, and one marked
	// unwatched, stay. Titles match the pass, so the ids are what differs.
	if got := previewIDs(plan.Remove, "keepUnwatched"); !slices.Equal(got, []int64{1, 2}) {
		t.Fatalf("remove %v (all %+v)", got, plan.Remove)
	}
}

func TestPreviewNamesLostTime(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "show.ts")
	if err := os.WriteFile(path, []byte{1}, 0o644); err != nil {
		t.Fatal(err)
	}
	lost := &store.RecordingHealth{LostSeconds: 12.4, ContinuityErrors: 30, Gaps: 2}
	lost.Judge(1800)
	broke := &store.RecordingHealth{ContinuityErrors: 80}
	broke.Judge(1800)
	clean := &store.RecordingHealth{ContinuityErrors: 2}
	clean.Judge(1800)
	recs := []store.Recording{
		{ID: 1, Title: "Dropped", Status: "complete", Path: path, ProgramID: "SH1", Health: lost},
		{ID: 2, Title: "Broke", Status: "complete", Path: path, Subtitle: "Part", ChannelID: 3, Health: broke},
		{ID: 3, Title: "Clean", Status: "complete", Path: path, ProgramID: "SH2", Health: clean},
		{ID: 4, Title: "Live", Status: "recording", Path: path, ProgramID: "SH1", Health: lost},
		{ID: 5, Title: "Nameless", Status: "complete", Path: path, Health: lost},
		{ID: 6, Title: "Gone", Status: "complete", Path: filepath.Join(root, "nope.ts"), ProgramID: "SH9", Health: lost},
	}
	plan := buildPreview(previewInput{Now: time.Now(), Root: root, Recs: recs})
	got := map[string]string{}
	for _, item := range plan.Again {
		got[item.Title] = item.Reason
	}
	if got["Dropped"] != "Signal dropped for 12 s." || got["Broke"] != "Signal broke up 80 times." {
		t.Fatalf("again %+v", plan.Again)
	}
	if _, ok := got["Clean"]; ok {
		t.Fatal("clean recording offered")
	}
	if _, ok := got["Live"]; ok {
		t.Fatal("recording in progress offered")
	}
	if _, ok := got["Nameless"]; ok {
		t.Fatal("no episode to match")
	}
	if got["Gone"] != "Signal dropped for 12 s." {
		t.Fatalf("missing file should still be offered: %+v", plan.Again)
	}
	if !slices.Contains(previewTitles(plan.Held, "missing"), "Gone") {
		t.Fatalf("held %+v", plan.Held)
	}
}

func TestPreviewLeavesARecentPlay(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "show.ts")
	if err := os.WriteFile(path, make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	recent := now.Add(-time.Hour)
	exact := now.Add(-6 * time.Hour)
	watched := now.Add(-10 * 24 * time.Hour)
	other := filepath.Join(root, "older.ts")
	if err := os.WriteFile(other, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	recs := []store.Recording{
		{
			ID: 1, Title: "News", Status: "complete", Path: path,
			Watched: 1, WatchedAt: &watched, ProgressAt: &recent, Position: 1790, Duration: 1800,
		},
		{
			ID: 2, Title: "Older", Status: "complete", Path: other,
			Watched: 1, WatchedAt: &watched, ProgressAt: &exact, Position: 1790, Duration: 1800,
		},
	}
	// Make-room would take both. The one played an hour ago is shielded.
	// Exactly six hours is not.
	plan := buildPreview(previewInput{
		Now: now, MakeRoom: true, Free: 0, Reserve: 50, Root: root, Recs: recs,
	})
	if got := previewIDs(plan.Remove, "makeRoom"); !slices.Equal(got, []int64{2}) {
		t.Fatalf("remove %v", got)
	}
	if got := previewTitles(plan.Held, "recent"); !slices.Equal(got, []string{"News"}) {
		t.Fatalf("held %+v", plan.Held)
	}
}

// The hourly clean-up deletes past-their-days recordings before it reads
// free space again. The preview has to count those bytes first, or it
// lists extra make-room deletions and calls a recent play the reason a
// recording stayed.
func TestPreviewCountsDaysBeforeMakeRoom(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, n int) string {
		t.Helper()
		path := filepath.Join(root, name+".ts")
		if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	at := func(ago time.Duration) *time.Time {
		t := now.Add(-ago)
		return &t
	}
	watched := func(id int64, title, path string, ago time.Duration) store.Recording {
		when := at(ago)
		return store.Recording{
			ID: id, Title: title, Status: "complete", Path: path, Watched: 1,
			WatchedAt: when, ProgressAt: when, Position: 1790, Duration: 1800,
			StartedAt: *when,
		}
	}
	recs := []store.Recording{
		watched(1, "aged", write("aged", 100), 10*24*time.Hour),
		watched(2, "older", write("older", 100), 4*24*time.Hour),
		watched(3, "newer", write("newer", 100), 3*24*time.Hour),
		watched(4, "newest", write("newest", 100), 2*24*time.Hour),
		watched(5, "hour", write("hour", 100), time.Hour),
	}
	const reserve = 1000
	partial := buildPreview(previewInput{
		Now: now, Days: 7, MakeRoom: true, Reserve: reserve, Free: reserve - 250,
		Root: root, Recs: recs,
	})
	if got := previewIDs(partial.Remove, "watchedDays"); !slices.Equal(got, []int64{1}) {
		t.Fatalf("days %v", got)
	}
	// 250 short, and the days rule frees 100, so 150 remains. Two files cover it.
	if partial.ShortBytes != 150 || !partial.Covers {
		t.Fatalf("short %d covers %v note %q", partial.ShortBytes, partial.Covers, partial.RoomNote)
	}
	if got := previewIDs(partial.Remove, "makeRoom"); !slices.Equal(got, []int64{2, 3}) {
		t.Fatalf("room %v (all %+v)", got, partial.Remove)
	}
	if slices.Contains(previewIDs(partial.Remove, ""), int64(4)) || slices.Contains(previewIDs(partial.Remove, ""), int64(5)) {
		t.Fatalf("kept a recording that should stay: %+v", partial.Remove)
	}
	if !slices.Contains(previewIDs(partial.Held, "recent"), int64(5)) {
		t.Fatalf("recent %+v", partial.Held)
	}

	// The days file alone covers a 100-byte shortfall. Making room does not run,
	// and the recording played an hour ago is not what saved it.
	covered := buildPreview(previewInput{
		Now: now, Days: 7, MakeRoom: true, Reserve: 200, Free: 100, Root: root,
		Recs: []store.Recording{recs[0], recs[4]},
	})
	if got := previewIDs(covered.Remove, ""); !slices.Equal(got, []int64{1}) {
		t.Fatalf("covered remove %v", got)
	}
	if covered.ShortBytes != 0 || !covered.Covers || covered.RoomNote == "" {
		t.Fatalf("covered short %d covers %v note %q", covered.ShortBytes, covered.Covers, covered.RoomNote)
	}
	if len(covered.Held) != 0 {
		t.Fatalf("held %+v", covered.Held)
	}
}

func TestPreviewDoesNotCreditAProtectionWhenMakeRoomDeletesNothing(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "show.ts")
	if err := os.WriteFile(path, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	watched := now.Add(-48 * time.Hour)
	recent := now.Add(-time.Hour)
	base := store.Recording{
		ID: 1, Title: "Kept Show", Status: "complete", Path: path,
		Watched: 1, WatchedAt: &watched, ProgressAt: &watched, Position: 1790, Duration: 1800,
	}
	kept := base
	kept.Keep = true
	played := base
	played.Title = "News"
	played.ProgressAt = &recent
	favorite := base
	favorite.Title = "Favorite"
	favorite.ChannelID = 9
	for _, recs := range [][]store.Recording{{kept}, {played}, {favorite}} {
		plan := buildPreview(previewInput{
			Now: now, MakeRoom: true, Reserve: 200, Free: 0, Root: root, Recs: recs,
			ProtectFavorites: true, Favorites: map[int64]bool{9: true},
		})
		if plan.Covers || len(plan.Remove) != 0 || len(plan.Held) != 0 {
			t.Fatalf("plan %+v", plan)
		}
	}
}

func TestPreviewWorksBeforeTheWorkFolderExists(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cfg")
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	root := t.TempDir()
	missing := filepath.Join(root, "work", "recordings")
	plan, err := Preview(context.Background(), st, &live.Hub{Dir: filepath.Join(root, "work"), RecordingsDir: missing}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.FreeBytes == 0 || len(plan.Remove) != 0 || !plan.Covers {
		t.Fatalf("plan %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(root, "work")); !os.IsNotExist(err) {
		t.Fatalf("preview created the work folder: %v", err)
	}
}

func TestPreviewWorksBeforeTheRecordingsFolderExists(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cfg")
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	work := t.TempDir()
	missing := filepath.Join(work, "recordings")
	plan, err := Preview(context.Background(), st, &live.Hub{Dir: work, RecordingsDir: missing}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.FreeBytes == 0 || len(plan.Remove) != 0 || !plan.Covers {
		t.Fatalf("plan %+v", plan)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("preview created %s: %v", missing, err)
	}
}
