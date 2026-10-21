package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func channelByNumber(t *testing.T, st *Store, number string) Channel {
	t.Helper()
	all, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range all {
		if ch.GuideNumber == number {
			return ch
		}
	}
	t.Fatalf("no channel %s in %+v", number, all)
	return Channel{}
}

// A backup restored onto a new install brings back channels, recordings,
// paired devices, and teams, with every id that moved followed.
func TestRestoreOntoANewServerBringsBackTheCatalog(t *testing.T) {
	ctx := context.Background()
	old := openTestStore(t)
	dev := hdhr.Device{DeviceID: "D1", FriendlyName: "DUO", BaseURL: "http://127.0.0.1", TunerCount: 2}
	if err := old.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "FOUR"}, {GuideNumber: "5.1", GuideName: "FIVE"}, {GuideNumber: "7.1", GuideName: "SEVEN"},
	}); err != nil {
		t.Fatal(err)
	}
	five := channelByNumber(t, old, "5.1")
	yes, name, no := true, "Five Local", false
	if _, err := old.PatchChannel(ctx, five.ID, ChannelPatch{Favorite: &yes, CustomName: &name}); err != nil {
		t.Fatal(err)
	}
	four := channelByNumber(t, old, "4.1")
	if _, err := old.PatchChannel(ctx, four.ID, ChannelPatch{Hidden: &yes, Enabled: &no}); err != nil {
		t.Fatal(err)
	}
	if err := old.AddPass(ctx, "News", five.ID, 1, 2); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	recID, err := old.CreateRecording(ctx, Recording{ChannelID: five.ID, GuideNumber: "5.1", Title: "Evening", Path: "/rec/evening.ts", Status: "complete", StartedAt: start})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.CreateRecording(ctx, Recording{ChannelID: five.ID, GuideNumber: "5.1", Title: "Late", Path: "/rec/late.ts", Status: "recording", StartedAt: start.Add(2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := old.AddMarker(ctx, recID, 60, 120); err != nil {
		t.Fatal(err)
	}
	if err := old.SaveProgress(ctx, recID, 300); err != nil {
		t.Fatal(err)
	}
	if _, _, err := old.InsertClient(ctx, "Living room", "tv", []string{"watch"}, start); err != nil {
		t.Fatal(err)
	}
	if err := old.FollowTeam(ctx, TeamFollow{Name: "Harbor FC", League: "mls"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.db")
	if err := old.BackupTo(ctx, path); err != nil {
		t.Fatal(err)
	}

	// The new server found the tuner in a different order and has one
	// recording and one paired device of its own.
	st := openTestStore(t)
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{{GuideNumber: "5.1", GuideName: "FIVE"}, {GuideNumber: "4.1", GuideName: "FOUR"}}); err != nil {
		t.Fatal(err)
	}
	mine, err := st.CreateRecording(ctx, Recording{GuideNumber: "4.1", Title: "Mine", Path: "/rec/mine.ts", Status: "complete", StartedAt: start})
	if err != nil {
		t.Fatal(err)
	}
	if mine != recID {
		t.Fatalf("test wants the ids to collide: %d vs %d", mine, recID)
	}
	if _, _, err := st.InsertClient(ctx, "Admin laptop", "web", []string{"admin"}, start); err != nil {
		t.Fatal(err)
	}
	if err := st.RestoreFrom(ctx, path); err != nil {
		t.Fatal(err)
	}

	newFive := channelByNumber(t, st, "5.1")
	if !newFive.Favorite || newFive.DisplayName != "Five Local" {
		t.Fatalf("5.1 after restore: %+v", newFive)
	}
	newFour := channelByNumber(t, st, "4.1")
	if !newFour.Hidden || newFour.Enabled {
		t.Fatalf("4.1 after restore: %+v", newFour)
	}
	if seven := channelByNumber(t, st, "7.1"); seven.Present {
		t.Fatalf("7.1 is not on the tuner yet: %+v", seven)
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || passes[0].ChannelID != newFive.ID {
		t.Fatalf("pass channel: %+v, want %d", passes, newFive.ID)
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]Recording{}
	for _, rec := range recs {
		byTitle[rec.Title] = rec
	}
	if len(recs) != 3 || byTitle["Mine"].ID != mine {
		t.Fatalf("recordings: %+v", recs)
	}
	evening := byTitle["Evening"]
	if evening.ID == recID || evening.ChannelID != newFive.ID {
		t.Fatalf("evening: %+v", evening)
	}
	if late := byTitle["Late"]; late.Status != "interrupted" || late.ID == 0 {
		t.Fatalf("late: %+v", late)
	}
	markers, err := st.Markers(ctx, evening.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 1 || markers[0].Start != 60 {
		t.Fatalf("markers: %+v", markers)
	}
	if pos, _ := st.Progress(ctx, evening.ID); pos != 300 {
		t.Fatalf("progress %v", pos)
	}
	if pos, _ := st.Progress(ctx, mine); pos != 0 {
		t.Fatalf("the new server's recording took the backup's playhead: %v", pos)
	}
	clients, err := st.Clients(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 2 {
		t.Fatalf("clients: %+v", clients)
	}
	teams, err := st.TeamFollows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].Name != "Harbor FC" {
		t.Fatalf("teams: %+v", teams)
	}
}

// Restoring on the same server keeps a recording made since, and one the
// catalog still has keeps its own state.
func TestRestoreKeepsRecordingsTheCatalogHas(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	start := time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	kept, err := st.CreateRecording(ctx, Recording{Title: "Kept", Path: "/rec/kept.ts", Status: "complete", StartedAt: start})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.db")
	if err := st.BackupTo(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(ctx, kept, 42); err != nil {
		t.Fatal(err)
	}
	later, err := st.CreateRecording(ctx, Recording{Title: "Later", Path: "/rec/later.ts", Status: "complete", StartedAt: start.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RestoreFrom(ctx, path); err != nil {
		t.Fatal(err)
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("recordings: %+v", recs)
	}
	if _, err := st.Recording(ctx, later); err != nil {
		t.Fatal(err)
	}
	if pos, _ := st.Progress(ctx, kept); pos != 42 {
		t.Fatalf("progress %v", pos)
	}
}
