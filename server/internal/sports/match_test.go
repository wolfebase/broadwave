package sports

import (
	"testing"
	"time"
)

func chiefsBills(start time.Time) Game {
	return Game{
		ID: "401772971", League: "nfl", Name: "Kansas City Chiefs at Buffalo Bills", ShortName: "KC @ BUF", Start: start,
		Broadcasts: []string{"Prime Video"},
		Teams: []Team{
			{Name: "Buffalo Bills", Short: "Bills", Abbr: "BUF", Home: true},
			{Name: "Kansas City Chiefs", Short: "Chiefs", Abbr: "KC"},
		},
	}
}

func TestLinkMatchesBothTeamsInsideTheWindow(t *testing.T) {
	start := time.Date(2026, 9, 25, 0, 15, 0, 0, time.UTC)
	games := []Game{chiefsBills(start)}
	links := Link([]Listing{{
		ID: 7, Title: "NFL Football", Subtitle: "Chiefs at Bills", Start: start.Add(10 * time.Minute),
	}}, games)
	if links[7] != "401772971" {
		t.Fatalf("%v", links)
	}
	late := Link([]Listing{{ID: 8, Title: "Chiefs at Bills", Start: start.Add(2 * time.Hour)}}, games)
	if len(late) != 0 {
		t.Fatalf("a start two hours off should not match: %v", late)
	}
	one := Link([]Listing{{ID: 9, Title: "The Bills", Start: start}}, games)
	if len(one) != 0 {
		t.Fatalf("one team is not a game: %v", one)
	}
}

func TestLinkUsesTheLeagueAndTheCloserStart(t *testing.T) {
	start := time.Date(2026, 9, 25, 0, 15, 0, 0, time.UTC)
	nfl := chiefsBills(start)
	college := nfl
	college.ID = "college"
	college.League = "ncaaf"
	college.Start = start.Add(30 * time.Minute)
	later := nfl
	later.ID = "later"
	later.Start = start.Add(40 * time.Minute)
	links := Link([]Listing{{
		ID: 1, Title: "College Football: Chiefs at Bills", Start: start,
	}}, []Game{nfl, college})
	if links[1] != "college" {
		t.Fatalf("%v", links)
	}
	closer := Link([]Listing{{ID: 2, Title: "Chiefs at Bills", Start: start}}, []Game{nfl, later})
	if closer[2] != nfl.ID {
		t.Fatalf("%v", closer)
	}
}

func TestLinkReadsAbbreviationsAndRaceNames(t *testing.T) {
	start := time.Date(2026, 9, 27, 19, 0, 0, 0, time.UTC)
	games := []Game{
		chiefsBills(start),
		{ID: "sp400", League: "nascar", Name: "South Point 400", ShortName: "South Point 400", Start: start},
	}
	abbr := Link([]Listing{{ID: 3, Title: "KC at BUF", Start: start}}, games)
	if abbr[3] != "401772971" {
		t.Fatalf("%v", abbr)
	}
	race := Link([]Listing{{ID: 4, Title: "NASCAR Cup Series: South Point 400", Category: "Sports", Start: start.Add(5 * time.Minute)}}, games)
	if race[4] != "sp400" {
		t.Fatalf("%v", race)
	}
}
