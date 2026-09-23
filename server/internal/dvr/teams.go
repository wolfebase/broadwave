package dvr

import (
	"fmt"
	"strconv"
	"time"

	"waveguide/internal/sports"
	"waveguide/internal/store"
)

// TeamNotice is the next game for a followed team that has not been announced.
func TeamNotice(team store.TeamFollow, airings []store.Airing, now time.Time) (store.Airing, string, bool) {
	var best store.Airing
	found := false
	for _, airing := range airings {
		if !airing.Start.After(now) || airing.Start.After(now.Add(36*time.Hour)) {
			continue
		}
		if !teamOn(team, airing) {
			continue
		}
		if !found || airing.Start.Before(best.Start) {
			best = airing
			found = true
		}
	}
	if !found || team.LastNotice == strconv.FormatInt(best.ID, 10) {
		return store.Airing{}, "", false
	}
	when := best.Start.In(time.Local)
	label := when.Format("3:04 PM")
	if when.YearDay() != now.In(time.Local).YearDay() || when.Year() != now.Year() {
		label = when.Format("Mon 3:04 PM")
	}
	name := team.Short
	if name == "" {
		name = team.Name
	}
	return best, fmt.Sprintf("%s is on at %s.", name, label), true
}

func teamOn(team store.TeamFollow, airing store.Airing) bool {
	text := airing.Title + " " + airing.Subtitle
	if team.Short != "" && sports.Mentions(text, team.Short) {
		return true
	}
	if team.Name != "" && sports.Mentions(text, team.Name) {
		return true
	}
	return len([]rune(team.Abbr)) >= 3 && sports.Mentions(text, team.Abbr)
}
