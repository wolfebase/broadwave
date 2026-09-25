package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TheSportsDB reads one day of events. It is used only with a key the user
// typed. An empty key does not make a request, and no key is built in.
type TheSportsDB struct {
	Key  string
	Base string
	HTTP *http.Client
}

func NewTheSportsDB(key string) *TheSportsDB {
	return &TheSportsDB{
		Key:  strings.TrimSpace(key),
		Base: "https://www.thesportsdb.com/api/v1/json",
		HTTP: http.DefaultClient,
	}
}

func (p *TheSportsDB) Scoreboard(ctx context.Context, leagueID string, day time.Time) ([]Game, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.Key == "" {
		return nil, fmt.Errorf("a TheSportsDB key is required")
	}
	league, ok := FindLeague(leagueID)
	if !ok {
		return nil, fmt.Errorf("unknown league %s", leagueID)
	}
	sport, ok := theSportsDBSport[league.ID]
	if !ok {
		return nil, fmt.Errorf("unknown league %s", leagueID)
	}
	base := p.Base
	if base == "" {
		base = "https://www.thesportsdb.com/api/v1/json"
	}
	// The key is a path segment. Errors from here do not include it.
	endpoint := fmt.Sprintf("%s/%s/eventsday.php?d=%s&s=%s",
		strings.TrimRight(base, "/"), url.PathEscape(p.Key),
		day.Format("2006-01-02"), url.QueryEscape(sport))
	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scoreboard returned %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return parseTheSportsDB(league, day, body)
}

func parseTheSportsDB(league League, day time.Time, body []byte) ([]Game, error) {
	var doc struct {
		Events []tsdbEvent `json:"events"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	want := strings.ToLower(theSportsDBLeague[league.ID])
	out := make([]Game, 0, len(doc.Events))
	for _, event := range doc.Events {
		if want != "" && !strings.Contains(strings.ToLower(event.League), want) {
			continue
		}
		state, done := tsdbState(event.Status)
		start := day
		if event.Date != "" {
			clock := event.Time
			if clock == "" {
				clock = "00:00:00"
			}
			if parsed, err := time.Parse("2006-01-02 15:04:05", event.Date+" "+clock); err == nil {
				start = parsed.UTC()
			}
		}
		game := Game{
			ID: event.ID, League: league.ID, Name: event.Name, Start: start,
			State: state, Completed: done, Detail: strings.TrimSpace(event.Progress),
		}
		if event.Home != "" || event.HomeScore != "" {
			game.Teams = append(game.Teams, Team{
				Name: event.Home, Score: string(event.HomeScore), Home: true,
			})
		}
		if event.Away != "" || event.AwayScore != "" {
			game.Teams = append(game.Teams, Team{Name: event.Away, Score: string(event.AwayScore)})
		}
		if game.ID == "" && game.Name == "" {
			continue
		}
		out = append(out, game)
	}
	return out, nil
}

func tsdbState(status string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "ns", "not started", "tbd":
		return "pre", false
	case "ft", "finished", "aet", "aot", "final", "full time", "ended":
		return "post", true
	default:
		return "in", false
	}
}

type tsdbEvent struct {
	ID        string     `json:"idEvent"`
	Name      string     `json:"strEvent"`
	League    string     `json:"strLeague"`
	Date      string     `json:"dateEvent"`
	Time      string     `json:"strTime"`
	Status    string     `json:"strStatus"`
	Progress  string     `json:"strProgress"`
	Home      string     `json:"strHomeTeam"`
	Away      string     `json:"strAwayTeam"`
	HomeScore flexString `json:"intHomeScore"`
	AwayScore flexString `json:"intAwayScore"`
}

type flexString string

func (f *flexString) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" || len(raw) == 0 {
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		*f = flexString(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		*f = flexString(number.String())
		return nil
	}
	return fmt.Errorf("score is not a string")
}

// theSportsDBSport is the eventsday sport name. theSportsDBLeague is the
// piece of strLeague that keeps a sport's other competitions out.
var theSportsDBSport = map[string]string{
	"nfl": "American Football", "ncaaf": "American Football",
	"nba": "Basketball", "wnba": "Basketball", "ncaab": "Basketball",
	"mlb": "Baseball", "nhl": "Ice Hockey",
	"mls": "Soccer", "nwsl": "Soccer", "epl": "Soccer",
	"f1": "Motorsport", "nascar": "Motorsport",
}

var theSportsDBLeague = map[string]string{
	"nfl": "NFL", "ncaaf": "NCAA",
	"nba": "NBA", "wnba": "WNBA", "ncaab": "NCAA",
	"mlb": "MLB", "nhl": "NHL",
	"mls": "MLS", "nwsl": "NWSL", "epl": "Premier League",
	"f1": "Formula", "nascar": "NASCAR",
}
