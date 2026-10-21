package guide

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// SDLineup is one lineup Schedules Direct offers for a postal code.
type SDLineup struct {
	ID string `json:"id"`
	// Name is the provider or "Antenna" with the place, as Schedules Direct names it.
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Location  string `json:"location,omitempty"`
}

var (
	usZip    = regexp.MustCompile(`^[0-9]{5}$`)
	caPostal = regexp.MustCompile(`^[A-Z][0-9][A-Z][0-9][A-Z][0-9]$`)
)

// SDPostal cleans a US ZIP or Canadian postal code and names its country
// the way Schedules Direct does (USA or CAN). The second value is empty for
// a code that is neither.
func SDPostal(raw string) (string, string) {
	code := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if len(code) == 10 && code[5] == '-' {
		code = code[:5]
	}
	switch {
	case usZip.MatchString(code):
		return code, "USA"
	case caPostal.MatchString(code):
		return code, "CAN"
	}
	return code, ""
}

// Headends lists the lineups for a postal code: antenna first, then cable,
// satellite, and streaming providers.
func (c *SD) Headends(ctx context.Context, country, postal string) ([]SDLineup, error) {
	q := url.Values{"country": {country}, "postalcode": {postal}}
	raw, err := c.call(ctx, http.MethodGet, "/headends?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var heads []struct {
		Transport string `json:"transport"`
		Location  string `json:"location"`
		Lineups   []struct {
			Name   string `json:"name"`
			Lineup string `json:"lineup"`
		} `json:"lineups"`
	}
	if err := json.Unmarshal(raw, &heads); err != nil {
		return nil, fmt.Errorf("Schedules Direct sent a lineup list that could not be read")
	}
	var out []SDLineup
	seen := map[string]bool{}
	for _, h := range heads {
		for _, l := range h.Lineups {
			if l.Lineup == "" || seen[l.Lineup] {
				continue
			}
			seen[l.Lineup] = true
			out = append(out, SDLineup{ID: l.Lineup, Name: strings.TrimSpace(l.Name), Transport: h.Transport, Location: strings.TrimSpace(h.Location)})
		}
	}
	rank := func(t string) int {
		switch strings.ToLower(t) {
		case "antenna":
			return 0
		case "cable":
			return 1
		case "satellite":
			return 2
		}
		return 3
	}
	slices.SortStableFunc(out, func(a, b SDLineup) int {
		if d := rank(a.Transport) - rank(b.Transport); d != 0 {
			return d
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// SDAccount is what the account has: its lineups and how many it may have.
type SDAccount struct {
	Lineups    []string `json:"lineups"`
	MaxLineups int      `json:"maxLineups"`
	Expires    string   `json:"expires,omitempty"`
}

// Account reads the account's lineups and its limit.
func (c *SD) Account(ctx context.Context) (SDAccount, error) {
	raw, err := c.call(ctx, http.MethodGet, "/status", nil)
	if err != nil {
		return SDAccount{}, err
	}
	var doc struct {
		Account struct {
			Expires    string `json:"expires"`
			MaxLineups int    `json:"maxLineups"`
		} `json:"account"`
		Lineups []struct {
			Lineup string `json:"lineup"`
		} `json:"lineups"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return SDAccount{}, fmt.Errorf("Schedules Direct sent an account status that could not be read")
	}
	out := SDAccount{MaxLineups: doc.Account.MaxLineups, Expires: doc.Account.Expires, Lineups: []string{}}
	for _, l := range doc.Lineups {
		if l.Lineup != "" {
			out.Lineups = append(out.Lineups, l.Lineup)
		}
	}
	return out, nil
}

// AddLineup puts a lineup on the account. One already there is left alone.
// An account at its limit is refused with Schedules Direct's own words.
func (c *SD) AddLineup(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/?#") {
		return fmt.Errorf("pick a lineup")
	}
	acct, err := c.Account(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(acct.Lineups, id) {
		return nil
	}
	if acct.MaxLineups > 0 && len(acct.Lineups) >= acct.MaxLineups {
		return fmt.Errorf("this Schedules Direct account already has %d of its %d lineups. Remove one at schedulesdirect.org, then try again", len(acct.Lineups), acct.MaxLineups)
	}
	_, err = c.call(ctx, http.MethodPut, "/lineups/"+url.PathEscape(id), nil)
	return err
}
