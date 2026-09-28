// Package nfo writes the sidecar Plex, Jellyfin, and Kodi read beside a recording.
package nfo

import (
	"encoding/xml"
	"strings"
	"time"

	"broadwave/internal/store"
)

const xmlHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n"

type episodeDetails struct {
	XMLName   xml.Name `xml:"episodedetails"`
	Title     string   `xml:"title"`
	ShowTitle string   `xml:"showtitle"`
	Season    int      `xml:"season,omitempty"`
	Episode   int      `xml:"episode,omitempty"`
	Plot      string   `xml:"plot,omitempty"`
	Aired     string   `xml:"aired,omitempty"`
	Genre     []string `xml:"genre,omitempty"`
}

type movieDetails struct {
	XMLName   xml.Name `xml:"movie"`
	Title     string   `xml:"title"`
	Plot      string   `xml:"plot,omitempty"`
	Premiered string   `xml:"premiered,omitempty"`
	Year      string   `xml:"year,omitempty"`
	Genre     []string `xml:"genre,omitempty"`
}

// Episode is a Kodi episodedetails document. Season and episode are omitted
// when the guide did not know them.
func Episode(rec store.Recording) []byte {
	title := strings.TrimSpace(rec.Subtitle)
	show := strings.TrimSpace(rec.Title)
	if show == "" {
		show = "Recording"
	}
	if title == "" {
		title = show
	}
	return document(episodeDetails{
		Title:     title,
		ShowTitle: show,
		Season:    rec.Season,
		Episode:   rec.Episode,
		Plot:      strings.TrimSpace(rec.Description),
		Aired:     airedOn(rec),
		Genre:     genres(rec.Category),
	})
}

// Movie is a Kodi movie document. A broadcast movie uses the recording title.
func Movie(rec store.Recording) []byte {
	title := strings.TrimSpace(rec.Title)
	if title == "" {
		title = "Recording"
	}
	aired := airedOn(rec)
	year := ""
	if len(aired) >= 4 {
		year = aired[:4]
	}
	return document(movieDetails{
		Title:     title,
		Plot:      strings.TrimSpace(rec.Description),
		Premiered: aired,
		Year:      year,
		Genre:     genres(rec.Category),
	})
}

// Document picks a movie file when the category says so, and an episode otherwise.
func Document(rec store.Recording) []byte {
	if isMovie(rec.Category) {
		return Movie(rec)
	}
	return Episode(rec)
}

func document(v any) []byte {
	body, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil
	}
	out := make([]byte, 0, len(xmlHeader)+len(body)+1)
	out = append(out, xmlHeader...)
	out = append(out, body...)
	out = append(out, '\n')
	return out
}

func airedOn(rec store.Recording) string {
	if len(rec.OriginalAir) == 10 && rec.OriginalAir[4] == '-' && rec.OriginalAir[7] == '-' {
		return rec.OriginalAir
	}
	if rec.StartedAt.IsZero() {
		return ""
	}
	return rec.StartedAt.Local().Format("2006-01-02")
}

func genres(category string) []string {
	if strings.TrimSpace(category) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(category, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isMovie(category string) bool {
	for _, part := range genres(category) {
		switch strings.ToLower(part) {
		case "movie", "movies":
			return true
		}
	}
	return false
}

// WithGuide copies season, episode, and the original air date from the listing
// the recording covers most. A padded recording starts before its show, so the
// start alone would land on the show before it. Empty title fields on the
// recording are filled from that listing. A recording with no start time is
// returned as it is.
func WithGuide(rec store.Recording, airings []store.Airing) store.Recording {
	if rec.StartedAt.IsZero() {
		return rec
	}
	end := rec.StartedAt.Add(time.Minute)
	if rec.EndedAt != nil && rec.EndedAt.After(rec.StartedAt) {
		end = *rec.EndedAt
	} else if rec.EndsAt != nil && rec.EndsAt.After(rec.StartedAt) {
		end = *rec.EndsAt
	}
	var best *store.Airing
	var most time.Duration
	for i := range airings {
		air := &airings[i]
		if rec.ChannelID != 0 && air.ChannelID != 0 && air.ChannelID != rec.ChannelID {
			continue
		}
		if rec.ProgramID != "" && air.ProgramID != "" && air.ProgramID != rec.ProgramID {
			continue
		}
		if (rec.ProgramID == "" || air.ProgramID == "") && rec.Title != "" && !strings.EqualFold(strings.TrimSpace(air.Title), strings.TrimSpace(rec.Title)) {
			continue
		}
		overlap := minTime(end, air.End).Sub(maxTime(rec.StartedAt, air.Start))
		if overlap > most {
			best, most = air, overlap
		}
	}
	if best == nil {
		return rec
	}
	if rec.Subtitle == "" {
		rec.Subtitle = best.Subtitle
	}
	if rec.Description == "" {
		rec.Description = best.Description
	}
	if rec.Category == "" {
		rec.Category = best.Category
	}
	rec.Season = best.Season
	rec.Episode = best.Episode
	rec.OriginalAir = best.OriginalAir
	return rec
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
