package sports

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Switch is the auto choice for the big tile. KeepManual means the viewer
// picked a tile less than 2 minutes before now, so nothing moves.
type Switch struct {
	KeepManual bool
	GameID     string
	Banner     string
}

// PickFocus chooses the live game that matters most among games.
// previous is the last board and is only used to notice a lead change.
// now is the clock, so tests can freeze it.
func PickFocus(now, manualAt time.Time, games, previous []Game) Switch {
	if manualHolds(now, manualAt) {
		return Switch{KeepManual: true}
	}
	prev := make(map[string]Game, len(previous))
	for _, game := range previous {
		prev[game.ID] = game
	}
	best := kindNone
	var chosen Game
	var why string
	for _, game := range games {
		if !actionable(game) {
			continue
		}
		_, ok := prev[game.ID]
		label, rank := classify(game, prev[game.ID], ok)
		if rank > best {
			best = rank
			chosen = game
			why = label
		}
	}
	if best == kindNone {
		return Switch{}
	}
	return Switch{GameID: chosen.ID, Banner: banner(why, chosen)}
}

type focusKind int

const (
	kindNone focusKind = iota
	kindFinish
	kindLead
	kindChance
)

func manualHolds(now, manualAt time.Time) bool {
	if manualAt.IsZero() {
		return false
	}
	delta := now.Sub(manualAt)
	return delta >= 0 && delta < 2*time.Minute
}

func actionable(g Game) bool {
	return g.Live() && !betweenPeriods(g.Detail)
}

func classify(g, prev Game, hasPrev bool) (string, focusKind) {
	if g.RedZone && sportOf(g.League) == "football" {
		return "Red zone", kindChance
	}
	if g.PowerPlay && sportOf(g.League) == "hockey" {
		return "Power play", kindChance
	}
	if hasPrev && leadChanged(g, prev) {
		return "Lead change", kindLead
	}
	if closeFinish(g) {
		return "Final minutes", kindFinish
	}
	return "", kindNone
}

func banner(why string, g Game) string {
	who := matchup(g)
	if who == "" {
		return why
	}
	return why + ": " + who
}

func matchup(g Game) string {
	home, okH := g.Home()
	away, okA := g.Away()
	if okH && okA {
		return teamLabel(away) + " at " + teamLabel(home)
	}
	if g.ShortName != "" {
		return strings.ReplaceAll(g.ShortName, " @ ", " at ")
	}
	return g.Name
}

func teamLabel(t Team) string {
	if t.Abbr != "" {
		return t.Abbr
	}
	if t.Short != "" {
		return t.Short
	}
	return t.Name
}

func leadChanged(cur, prev Game) bool {
	prevLead, prevOK := leaderKey(prev)
	curLead, curOK := leaderKey(cur)
	if !prevOK || !curOK || prevLead == "" || curLead == "" {
		return false
	}
	return prevLead != curLead
}

func leaderKey(g Game) (string, bool) {
	if len(g.Teams) < 2 {
		return "", false
	}
	best := 0
	key := ""
	tied := false
	seen := 0
	for _, team := range g.Teams {
		n, err := strconv.Atoi(strings.TrimSpace(team.Score))
		if err != nil {
			return "", false
		}
		label := teamLabel(team)
		if seen == 0 || n > best {
			best = n
			key = label
			tied = false
		} else if n == best {
			tied = true
		}
		seen++
	}
	if seen < 2 {
		return "", false
	}
	if tied {
		return "", true
	}
	return key, true
}

func closeFinish(g Game) bool {
	margin, ok := closeMargin(g.League)
	if !ok || !inFinalMinutes(g) {
		return false
	}
	gap, ok := scoreGap(g)
	return ok && gap <= margin
}

func closeMargin(league string) (int, bool) {
	switch sportOf(league) {
	case "football", "basketball":
		return 8, true
	case "hockey", "soccer":
		return 1, true
	default:
		return 0, false
	}
}

func scoreGap(g Game) (int, bool) {
	if len(g.Teams) < 2 {
		return 0, false
	}
	minScore, maxScore := 0, 0
	for i, team := range g.Teams {
		n, err := strconv.Atoi(strings.TrimSpace(team.Score))
		if err != nil {
			return 0, false
		}
		if i == 0 || n < minScore {
			minScore = n
		}
		if i == 0 || n > maxScore {
			maxScore = n
		}
	}
	return maxScore - minScore, true
}

// inFinalMinutes is the last period with under 5:00 on the clock.
// A detail line can name that period when the feed leaves the number at zero.
// Soccer counts up, so 85' and later, or extra time, is the same window.
func inFinalMinutes(g Game) bool {
	if !g.Live() || betweenPeriods(g.Detail) {
		return false
	}
	if soccerClockLate(g) {
		return true
	}
	need := finalPeriod(g.League)
	if need == 0 {
		return false
	}
	late := g.Period >= need || detailIsLastPeriod(g)
	if !late {
		return false
	}
	left, ok := remainingClock(g)
	return ok && left < 5*time.Minute
}

func finalPeriod(league string) int {
	if league == "ncaab" {
		return 2
	}
	switch sportOf(league) {
	case "hockey":
		return 3
	case "soccer":
		return 2
	case "football", "basketball":
		return 4
	default:
		return 0
	}
}

func sportOf(league string) string {
	found, ok := FindLeague(league)
	if !ok {
		return ""
	}
	return found.Sport
}

func betweenPeriods(detail string) bool {
	d := strings.ToLower(strings.TrimSpace(detail))
	if d == "" {
		return false
	}
	if d == "ht" || d == "halftime" || d == "half time" {
		return true
	}
	return strings.Contains(d, "halftime") || strings.Contains(d, "half time") || strings.Contains(d, "end of") || strings.Contains(d, "intermission") || strings.Contains(d, "delay") || strings.Contains(d, "suspended")
}

func detailIsLastPeriod(g Game) bool {
	text := g.Detail
	switch {
	case sportOf(g.League) == "hockey":
		return lastHockey.MatchString(text)
	case g.League == "ncaab" || sportOf(g.League) == "soccer":
		return lastHalf.MatchString(text) || overtime.MatchString(text)
	case sportOf(g.League) == "football" || sportOf(g.League) == "basketball":
		return lastQuarter.MatchString(text)
	default:
		return false
	}
}

func soccerClockLate(g Game) bool {
	if sportOf(g.League) != "soccer" {
		return false
	}
	text := g.Detail + " " + g.Clock
	if extraSoccer.MatchString(text) {
		return true
	}
	for _, src := range []string{g.Clock, g.Detail} {
		m := soccerMinute.FindStringSubmatch(src)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err == nil && n >= 85 {
			return true
		}
	}
	return false
}

func remainingClock(g Game) (time.Duration, bool) {
	if left, ok := clockIn(g.Clock); ok {
		return left, true
	}
	return clockIn(g.Detail)
}

func clockIn(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if left, ok := parseClock(raw); ok {
		return left, true
	}
	m := clockRE.FindStringSubmatch(raw)
	if m == nil {
		return 0, false
	}
	return parseClock(m[1] + ":" + m[2])
}

func parseClock(raw string) (time.Duration, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, false
	}
	secText := parts[1]
	if i := strings.IndexAny(secText, ". "); i >= 0 {
		secText = secText[:i]
	}
	min, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	sec, err2 := strconv.Atoi(secText)
	if err1 != nil || err2 != nil || min < 0 || sec < 0 || sec > 59 {
		return 0, false
	}
	return time.Duration(min)*time.Minute + time.Duration(sec)*time.Second, true
}

var (
	clockRE      = regexp.MustCompile(`(?:^|[^0-9])(\d{1,2}):(\d{2})(?:[^0-9]|$)`)
	lastQuarter  = regexp.MustCompile(`(?i)\b(?:4th|fourth|ot|overtime)\b`)
	lastHockey   = regexp.MustCompile(`(?i)\b(?:3rd|third|ot|overtime)\b`)
	lastHalf     = regexp.MustCompile(`(?i)\b(?:2nd half|second half|2h)\b`)
	overtime     = regexp.MustCompile(`(?i)\b(?:ot|overtime|extra time|et)\b`)
	extraSoccer  = regexp.MustCompile(`(?i)\b(?:extra time|stoppage|penalties|aet|et)\b`)
	soccerMinute = regexp.MustCompile(`(\d{1,3})(?:\s*\+\s*\d{1,2})?\s*'`)
)
