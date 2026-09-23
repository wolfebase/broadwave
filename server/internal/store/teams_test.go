package store

import (
	"path/filepath"
	"testing"

	"waveguide/internal/hdhr"
)

func TestFollowTeamRecordsEveryGame(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	if err := s.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "DUO", BaseURL: "http://127.0.0.1", TunerCount: 1}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.FollowTeam(ctx, TeamFollow{Name: "Kansas City Chiefs", Short: "Chiefs", Abbr: "KC", League: "nfl", Record: true}); err != nil {
		t.Fatal(err)
	}
	teams, err := s.TeamFollows(ctx)
	if err != nil || len(teams) != 1 || !teams[0].Record || teams[0].Short != "Chiefs" {
		t.Fatal(err, teams)
	}
	passes, err := s.Passes(ctx)
	if err != nil || len(passes) != 1 || passes[0].Kind != "team" || passes[0].Title != "Chiefs" {
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
