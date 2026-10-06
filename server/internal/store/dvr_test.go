package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestReplaceAiringsForReplacesChannelListings(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	at := func(h, m int) time.Time { return time.Date(2026, 9, 27, h, m, 0, 0, time.UTC) }
	row := func(ch int64, title, source string, start time.Time, mins int) Airing {
		return Airing{ChannelID: ch, Title: title, GuideSource: source, Start: start, End: start.Add(time.Duration(mins) * time.Minute)}
	}
	if err := st.InsertAirings(ctx, []Airing{
		row(1, "Late film", "broadcast", at(9, 0), 60),
		row(1, "Overlapped", "broadcast", at(7, 0), 30),
	}); err != nil {
		t.Fatal(err)
	}
	first := []Airing{row(1, "Jeopardy!", "silicondust", at(7, 0), 30), row(2, "News", "silicondust", at(7, 0), 30)}
	if err := st.ReplaceAiringsFor(ctx, []int64{1, 2}, first); err != nil {
		t.Fatal(err)
	}
	// The next pull overlaps the last one, as the 2-day feed does every day.
	next := []Airing{row(1, "Jeopardy!", "silicondust", at(7, 0), 30), row(1, "Wheel", "silicondust", at(7, 30), 30)}
	if err := st.ReplaceAiringsFor(ctx, []int64{1}, next); err != nil {
		t.Fatal(err)
	}
	got, err := st.Airings(ctx, at(0, 0), at(23, 0))
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, a := range got {
		titles = append(titles, a.Title)
	}
	slices.Sort(titles)
	want := []string{"Jeopardy!", "Late film", "News", "Wheel"}
	if !slices.Equal(titles, want) {
		t.Fatalf("airings = %v, want %v", titles, want)
	}
}

func TestReplaceAiringsForKeepsOneRowPerSlot(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)
	bare := Airing{ChannelID: 1, Title: "Jeopardy!", Start: start, End: start.Add(30 * time.Minute)}
	rich := bare
	rich.Description = "A classic game show."
	if err := st.ReplaceAiringsFor(ctx, []int64{1}, []Airing{bare, rich}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Airings(ctx, start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Description != rich.Description {
		t.Fatalf("airings = %+v, want the one with a description", got)
	}
}

func TestOncePassIsKeptOnceAndDroppedAfterItAirs(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	start := time.Date(2026, 9, 28, 16, 0, 0, 0, time.UTC)
	for range 2 {
		if err := st.AddOncePass(ctx, "Jeopardy", 3, start, 1, 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AddPass(ctx, "News", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 2 || passes[0].Kind != "once" || !passes[0].AiringStart.Equal(start) || !passes[1].AiringStart.IsZero() {
		t.Fatalf("passes %+v", passes)
	}
	if err := st.DeleteEndedOncePasses(ctx, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	passes, _ = st.Passes(ctx)
	if len(passes) != 1 || passes[0].Title != "News" {
		t.Fatalf("after prune %+v", passes)
	}
}

func TestRestoreFromABackupWithoutTheOnceColumn(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.AddPass(ctx, "News", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{"layout": "tv"}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "old.db")
	if err := st.BackupTo(ctx, backup); err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open("sqlite", "file:"+filepath.ToSlash(backup))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.ExecContext(ctx, `ALTER TABLE passes DROP COLUMN airing_start`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	if err := st.AddOncePass(ctx, "Jeopardy", 3, time.Now(), 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{"layout": "phone"}); err != nil {
		t.Fatal(err)
	}
	if err := st.RestoreFrom(ctx, backup); err != nil {
		t.Fatal(err)
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || passes[0].Title != "News" || !passes[0].AiringStart.IsZero() {
		t.Fatalf("passes after restore %+v", passes)
	}
	settings, _ := st.Settings(ctx)
	if settings["layout"] != "tv" {
		t.Fatalf("layout %q", settings["layout"])
	}
}

func TestATeamPassKeepsItsRulesWhenFollowedAgain(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	team := TeamFollow{Name: "Kansas City Chiefs", Short: "Chiefs", Record: true}
	if err := st.FollowTeam(ctx, team); err != nil {
		t.Fatal(err)
	}
	passes, err := st.Passes(ctx)
	if err != nil || len(passes) != 1 {
		t.Fatalf("%+v %v", passes, err)
	}
	p := passes[0]
	p.PadAfter, p.KeepMode, p.KeepCount, p.Priority = 30, "last", 2, 4
	if err := st.UpdatePassRules(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := st.FollowTeam(ctx, team); err != nil {
		t.Fatal(err)
	}
	again, _ := st.Passes(ctx)
	if len(again) != 1 || again[0].ID != p.ID || again[0].PadAfter != 30 || again[0].KeepCount != 2 || again[0].Priority != 4 {
		t.Fatalf("following again reset the pass: %+v", again)
	}
	team.Record = false
	if err := st.FollowTeam(ctx, team); err != nil {
		t.Fatal(err)
	}
	if gone, _ := st.Passes(ctx); len(gone) != 0 {
		t.Fatalf("not recording left %+v", gone)
	}
}

func TestARecordingNamesThePassThatStartedIt(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateRecording(ctx, Recording{Title: "News", Status: "recording", StartedAt: time.Now(), PassID: 12})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Recording(ctx, id)
	if err != nil || rec.PassID != 12 {
		t.Fatalf("%+v %v", rec, err)
	}
}

func TestARecordingKeepsTheEpisodeItCovers(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	show := time.Date(2026, 10, 5, 19, 0, 0, 0, time.UTC)
	if err := st.InsertAirings(ctx, []Airing{
		{ChannelID: 3, Title: "Quiz Hour", Start: show.Add(-30 * time.Minute), End: show, Season: 4, Episode: 11},
		{ChannelID: 3, Title: "Mystery Hour", Start: show, End: show.Add(time.Hour), ProgramID: "EP1", Season: 2, Episode: 5, EpisodeLabel: "S2E5", OriginalAir: "2026-01-08"},
		{ChannelID: 4, Title: "Mystery Hour", Start: show, End: show.Add(time.Hour), ProgramID: "EP1", Season: 9, Episode: 9},
	}); err != nil {
		t.Fatal(err)
	}
	// Padded a minute early, so the start lands on the show before it.
	ends := show.Add(62 * time.Minute)
	if _, err := st.CreateRecording(ctx, Recording{ChannelID: 3, Title: "Mystery Hour", ProgramID: "EP1", Status: "recording", StartedAt: show.Add(-time.Minute), EndsAt: &ends}); err != nil {
		t.Fatal(err)
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := recs[0]; got.Season != 2 || got.Episode != 5 || got.EpisodeLabel != "S2E5" || got.OriginalAir != "2026-01-08" {
		t.Fatalf("episode %d %d %q %q, want 2 5 S2E5 2026-01-08", got.Season, got.Episode, got.EpisodeLabel, got.OriginalAir)
	}
}

// Keep and limit rules judge watched from the store's list, so it must carry the playhead.
func TestTheRecordingListCarriesThePlayhead(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	id, err := st.CreateRecording(ctx, Recording{Title: "News", Status: "complete", StartedAt: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(ctx, id, 1800); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(ctx, id, 1795); err != nil {
		t.Fatal(err)
	}
	rec, err := st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Position != 1795 || rec.ProgressAt == nil || time.Since(*rec.ProgressAt) > time.Minute {
		t.Fatalf("position %v at %v", rec.Position, rec.ProgressAt)
	}
	if !rec.Played() {
		t.Fatal("watched to the end but not played")
	}
}
