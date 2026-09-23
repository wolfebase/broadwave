package dvr

import (
	"strings"
	"testing"
	"time"

	"waveguide/internal/store"
)

func TestTeamPassMatchesAcrossChannels(t *testing.T) {
	start := time.Date(2026, 9, 25, 0, 15, 0, 0, time.UTC)
	pass := store.Pass{Title: "Chiefs", Kind: "team", MatchKind: "team"}
	game := store.Airing{Title: "NFL Football", Subtitle: "Chiefs at Bills", ChannelID: 4, Start: start, End: start.Add(3 * time.Hour)}
	news := store.Airing{Title: "News", ChannelID: 4, Start: start, End: start.Add(time.Hour)}
	if _, ok := matchPass([]store.Pass{pass}, game); !ok {
		t.Fatal("a team pass should match the game on any channel")
	}
	if _, ok := matchPass([]store.Pass{pass}, news); ok {
		t.Fatal("a team pass should not match the news")
	}
}

func TestTeamNoticeAnnouncesOnce(t *testing.T) {
	start := time.Date(2026, 9, 25, 0, 15, 0, 0, time.UTC)
	now := start.Add(-6 * time.Hour)
	team := store.TeamFollow{ID: 1, Name: "Kansas City Chiefs", Short: "Chiefs", League: "nfl"}
	airings := []store.Airing{{ID: 9, Title: "Chiefs at Bills", Start: start, End: start.Add(3 * time.Hour)}}
	airing, note, ok := TeamNotice(team, airings, now)
	if !ok || airing.ID != 9 || !strings.Contains(note, "Chiefs is on at") {
		t.Fatalf("%v %q %+v", ok, note, airing)
	}
	team.LastNotice = "9"
	if _, _, ok := TeamNotice(team, airings, now); ok {
		t.Fatal("the same game should be announced once")
	}
}
