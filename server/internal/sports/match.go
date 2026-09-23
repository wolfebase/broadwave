package sports

import (
	"strings"
	"time"
	"unicode"
)

const matchWindow = 90 * time.Minute

// Listing is the part of a guide airing the matcher needs.
type Listing struct {
	ID          int64
	Title       string
	Subtitle    string
	Description string
	Category    string
	Start       time.Time
}

// Link pairs each listing with one game. A listing matches when both teams
// appear in the title or subtitle and the start times are within 90 minutes.
// A broadcaster name in the listing breaks a tie. Racing uses the event name.
func Link(listings []Listing, games []Game) map[int64]string {
	out := map[int64]string{}
	for _, listing := range listings {
		game, ok := matchOne(listing, games)
		if ok {
			out[listing.ID] = game.ID
		}
	}
	return out
}

func matchOne(listing Listing, games []Game) (Game, bool) {
	text := fold(listing.Title + " " + listing.Subtitle + " " + listing.Category)
	extra := fold(listing.Description)
	hint := leagueHint(text)
	var best Game
	var bestScore int
	found := false
	for _, game := range games {
		if listing.Start.Sub(game.Start).Abs() > matchWindow {
			continue
		}
		if hint != "" && hint != game.League {
			continue
		}
		score := scoreGame(text, game)
		if score == 0 {
			continue
		}
		if broadcastIn(text+" "+extra, game) {
			score += 15
		}
		if hint == game.League {
			score += 25
		}
		score += closeness(listing.Start, game.Start)
		if !found || score > bestScore {
			best = game
			bestScore = score
			found = true
		}
	}
	return best, found
}

func scoreGame(text string, game Game) int {
	home, homeOK := game.Home()
	away, awayOK := game.Away()
	if homeOK && awayOK && teamIn(text, home) && teamIn(text, away) {
		return 100
	}
	league, ok := FindLeague(game.League)
	if !ok || !league.ScheduleOnly {
		return 0
	}
	name := fold(game.Name)
	short := fold(game.ShortName)
	if name != "" && phrase(text, name) {
		return 80
	}
	if short != "" && short != name && phrase(text, short) {
		return 80
	}
	return 0
}

func teamIn(text string, team Team) bool {
	name := fold(team.Name)
	short := fold(team.Short)
	abbr := fold(team.Abbr)
	if name != "" && phrase(text, name) {
		return true
	}
	if len(strings.ReplaceAll(short, " ", "")) >= 4 && phrase(text, short) {
		return true
	}
	return abbr != "" && phrase(text, abbr)
}

func broadcastIn(text string, game Game) bool {
	for _, name := range game.Broadcasts {
		folded := fold(name)
		if len(strings.ReplaceAll(folded, " ", "")) >= 3 && phrase(text, folded) {
			return true
		}
	}
	return false
}

func closeness(listing, game time.Time) int {
	minutes := int(listing.Sub(game).Abs() / time.Minute)
	if minutes > 90 {
		return 0
	}
	return 10 - minutes/9
}

func leagueHint(text string) string {
	switch {
	case phrase(text, "wnba"):
		return "wnba"
	case phrase(text, "nba"):
		return "nba"
	case phrase(text, "ncaaf"), phrase(text, "college football"):
		return "ncaaf"
	case phrase(text, "nfl"):
		return "nfl"
	case phrase(text, "nhl"), phrase(text, "hockey"):
		return "nhl"
	case phrase(text, "ncaab"), phrase(text, "college basketball"):
		return "ncaab"
	case phrase(text, "mlb"), phrase(text, "baseball"):
		return "mlb"
	case phrase(text, "nascar"):
		return "nascar"
	case phrase(text, "formula 1"), phrase(text, "formula one"), phrase(text, "f1"):
		return "f1"
	case phrase(text, "premier league"), phrase(text, "epl"):
		return "epl"
	case phrase(text, "nwsl"):
		return "nwsl"
	case phrase(text, "mls"), phrase(text, "major league soccer"):
		return "mls"
	default:
		return ""
	}
}

func fold(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space && b.Len() > 0 {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// Mentions reports whether name appears as its own words in text.
func Mentions(text, name string) bool {
	return phrase(fold(text), fold(name))
}

func phrase(text, part string) bool {
	if part == "" || text == "" {
		return false
	}
	return strings.Contains(" "+text+" ", " "+part+" ")
}
