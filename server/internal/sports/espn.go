package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ESPN reads the public scoreboard. It is unofficial, so callers cache it and back off.
type ESPN struct {
	Base string
	HTTP *http.Client
}

func NewESPN() *ESPN {
	return &ESPN{Base: "https://site.api.espn.com/apis/site/v2/sports", HTTP: http.DefaultClient}
}

func (e *ESPN) Scoreboard(ctx context.Context, leagueID string, day time.Time) ([]Game, error) {
	league, ok := FindLeague(leagueID)
	if !ok {
		return nil, fmt.Errorf("unknown league %s", leagueID)
	}
	base := e.Base
	if base == "" {
		base = "https://site.api.espn.com/apis/site/v2/sports"
	}
	client := e.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	url := fmt.Sprintf("%s/%s/%s/scoreboard?dates=%s", strings.TrimRight(base, "/"), league.Sport, league.Slug, day.Format("20060102"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// ESPN rejects some user agents. The default Go client is accepted.
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
	return ParseScoreboard(league.ID, body)
}

// ParseScoreboard reads one scoreboard document into games.
func ParseScoreboard(league string, body []byte) ([]Game, error) {
	var doc struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := make([]Game, 0, len(doc.Events))
	for _, raw := range doc.Events {
		game, ok := parseEvent(league, raw)
		if ok {
			out = append(out, game)
		}
	}
	return out, nil
}

func parseEvent(league string, raw json.RawMessage) (Game, bool) {
	var event struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		ShortName string    `json:"shortName"`
		Date      string    `json:"date"`
		Status    statusDoc `json:"status"`
		Comps     []struct {
			Status     statusDoc `json:"status"`
			Broadcasts []struct {
				Names []string `json:"names"`
			} `json:"broadcasts"`
			Competitors []struct {
				HomeAway string `json:"homeAway"`
				Score    string `json:"score"`
				Team     struct {
					DisplayName      string `json:"displayName"`
					ShortDisplayName string `json:"shortDisplayName"`
					Abbreviation     string `json:"abbreviation"`
					Color            string `json:"color"`
					AlternateColor   string `json:"alternateColor"`
					Logo             string `json:"logo"`
				} `json:"team"`
			} `json:"competitors"`
		} `json:"competitions"`
	}
	if err := json.Unmarshal(raw, &event); err != nil || event.ID == "" {
		return Game{}, false
	}
	start, err := parseStart(event.Date)
	if err != nil {
		return Game{}, false
	}
	game := Game{ID: event.ID, League: league, Name: event.Name, ShortName: event.ShortName, Start: start}
	status := event.Status
	var broadcasts []string
	if len(event.Comps) > 0 {
		comp := event.Comps[0]
		if comp.Status.Type.State != "" {
			status = comp.Status
		}
		for _, b := range comp.Broadcasts {
			broadcasts = append(broadcasts, b.Names...)
		}
		for _, c := range comp.Competitors {
			if strings.TrimSpace(c.Team.DisplayName) == "" && strings.TrimSpace(c.Team.Abbreviation) == "" {
				continue
			}
			game.Teams = append(game.Teams, Team{
				Name: c.Team.DisplayName, Short: c.Team.ShortDisplayName, Abbr: c.Team.Abbreviation,
				Score: strings.TrimSpace(c.Score), Home: c.HomeAway == "home",
				Color: hexColor(c.Team.Color), AltColor: hexColor(c.Team.AlternateColor), Logo: c.Team.Logo,
			})
		}
	}
	game.State = status.Type.State
	game.Completed = status.Type.Completed
	game.Detail = status.Type.ShortDetail
	game.Clock = status.DisplayClock
	game.Period = status.Period
	game.Broadcasts = broadcasts
	if game.State == "" {
		game.State = "pre"
	}
	return game, true
}

type statusDoc struct {
	Period       int    `json:"period"`
	DisplayClock string `json:"displayClock"`
	Type         struct {
		State       string `json:"state"`
		Completed   bool   `json:"completed"`
		ShortDetail string `json:"shortDetail"`
	} `json:"type"`
}

func parseStart(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04Z07:00", raw)
}

func hexColor(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) != 6 {
		return ""
	}
	for _, r := range raw {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return ""
		}
	}
	return "#" + raw
}
