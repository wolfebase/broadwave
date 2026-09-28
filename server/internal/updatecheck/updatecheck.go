// Package updatecheck decides whether a container update may restart the
// server now. An updater runs it inside the container before it pulls a new
// image; exit code 75 (EX_TEMPFAIL) tells the updater to skip this round.
package updatecheck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Skip is the exit code that tells the updater to try again next round.
const Skip = 75

// Window is how far ahead a scheduled recording keeps the server up.
const Window = 2 * time.Hour

type Tuners struct {
	Tuners []struct {
		Ours bool `json:"ours"`
	} `json:"tuners"`
}

type Schedule struct {
	Items []struct {
		Airing struct {
			Title string    `json:"title"`
			Start time.Time `json:"start"`
			End   time.Time `json:"end"`
		} `json:"airing"`
		PadBefore int  `json:"padBefore"`
		Skipped   bool `json:"skipped"`
	} `json:"items"`
}

type Recordings struct {
	Recordings []struct {
		Title  string `json:"title"`
		Status string `json:"status"`
	} `json:"recordings"`
}

// Decide reports why an update now would cut something off, or "" when it
// would not: a tuner this server holds (someone watching, recording, or
// exporting), a recording in progress, or one due to start within window.
func Decide(t Tuners, s Schedule, r Recordings, now time.Time, window time.Duration) string {
	for _, tn := range t.Tuners {
		if tn.Ours {
			return "a tuner is in use"
		}
	}
	for _, rec := range r.Recordings {
		if rec.Status == "recording" {
			return "recording " + rec.Title
		}
	}
	for _, it := range s.Items {
		if it.Skipped {
			continue
		}
		start := it.Airing.Start.Add(-time.Duration(it.PadBefore) * time.Minute)
		if it.Airing.End.After(now) && start.Before(now.Add(window)) {
			return fmt.Sprintf("%s records at %s", it.Airing.Title, start.Local().Format("15:04"))
		}
	}
	return ""
}

// Run asks the server on port what it is doing and returns the exit code for
// the updater. A server that does not answer has nothing to cut off.
func Run(server string, now time.Time) (int, string) {
	base := server + "/api/v1"
	client := http.Client{Timeout: 5 * time.Second}
	var t Tuners
	var s Schedule
	var r Recordings
	if err := get(client, base+"/tuners", &t); err != nil {
		return 0, "server not answering: update"
	}
	// A server that answers but cannot list its schedule might be about to
	// record; wait for the next round rather than guess.
	if err := get(client, base+"/schedule", &s); err != nil {
		return Skip, "schedule unavailable"
	}
	if err := get(client, base+"/recordings", &r); err != nil {
		return Skip, "recordings unavailable"
	}
	if why := Decide(t, s, r, now, Window); why != "" {
		return Skip, why
	}
	return 0, "idle: update"
}

func get(client http.Client, url string, v any) error {
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
