package store

import (
	"path/filepath"
	"testing"

	"broadwave/internal/hdhr"
)

func TestFollowTeamRecordsEveryGame(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	if err := s.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "DUO", BaseURL: "http://127.0.0.1", TunerCount: 1}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.FollowTeam(ctx, TeamFollow{Name: "Chicago Bears", Short: "Bears", Abbr: "CHI", League: "nfl", Record: true}); err != nil {
		t.Fatal(err)
	}
	teams, err := s.TeamFollows(ctx)
	if err != nil || len(teams) != 1 || !teams[0].Record || teams[0].Short != "Bears" {
		t.Fatal(err, teams)
	}
	passes, err := s.Passes(ctx)
	if err != nil || len(passes) != 1 || passes[0].Kind != "team" || passes[0].Title != "Bears" {
		t.Fatal(err, passes)
	}
	teams[0].Record = false
	if err := s.FollowTeam(ctx, teams[0]); err != nil {
		t.Fatal(err)
	}
	passes, err = s.Passes(ctx)
	if err != nil || len(passes) != 0 {
		t.Fatal(err, passes)
	}
	if err := s.UnfollowTeam(ctx, teams[0].ID); err != nil {
		t.Fatal(err)
	}
	teams, err = s.TeamFollows(ctx)
	if err != nil || len(teams) != 0 {
		t.Fatal(err, teams)
	}
}

func TestFollowRollsBackWhenThePassCannotBeSaved(t *testing.T) {
	st := openTestStore(t)
	ctx := t.Context()
	if _, err := st.db.Exec(`CREATE TRIGGER reject_pass_insert BEFORE INSERT ON passes BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	err := st.FollowTeam(ctx, TeamFollow{Name: "Harbor City Gulls", Short: "Gulls", League: "nfl", Record: true})
	if err == nil {
		t.Fatal("followed a team whose pass was rejected")
	}
	teams, err := st.TeamFollows(ctx)
	if err != nil || len(teams) != 0 {
		t.Fatalf("teams %+v %v", teams, err)
	}
}

func TestUnfollowKeepsTheTeamWhenThePassCannotBeDeleted(t *testing.T) {
	st := openTestStore(t)
	ctx := t.Context()
	if err := st.FollowTeam(ctx, TeamFollow{Name: "Harbor City Gulls", Short: "Gulls", League: "nfl", Record: true}); err != nil {
		t.Fatal(err)
	}
	teams, err := st.TeamFollows(ctx)
	if err != nil || len(teams) != 1 {
		t.Fatal(err, teams)
	}
	if _, err := st.db.Exec(`CREATE TRIGGER reject_pass_delete BEFORE DELETE ON passes BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if err := st.UnfollowTeam(ctx, teams[0].ID); err == nil {
		t.Fatal("unfollowed a team whose pass could not be deleted")
	}
	teams, err = st.TeamFollows(ctx)
	if err != nil || len(teams) != 1 || teams[0].Name != "Harbor City Gulls" {
		t.Fatalf("teams %+v %v", teams, err)
	}
	passes, err := st.Passes(ctx)
	if err != nil || len(passes) != 1 || passes[0].Kind != "team" || passes[0].Title != "Gulls" {
		t.Fatalf("passes %+v %v", passes, err)
	}
}
