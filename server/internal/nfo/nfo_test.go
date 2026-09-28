package nfo

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"broadwave/internal/store"
)

func rawAmp(body []byte) bool {
	s := string(body)
	for {
		i := strings.IndexByte(s, '&')
		if i < 0 {
			return false
		}
		end := strings.IndexByte(s[i:], ';')
		if end <= 1 {
			return true
		}
		ref := s[i : i+end+1]
		switch ref {
		case "&amp;", "&lt;", "&gt;", "&quot;", "&apos;":
		default:
			if !strings.HasPrefix(ref, "&#") {
				return true
			}
		}
		s = s[i+end+1:]
	}
}

func TestEpisodeEscapesAndKeepsText(t *testing.T) {
	started := time.Date(2026, 9, 28, 12, 30, 0, 0, time.UTC)
	rec := store.Recording{
		Title:       `News & Weather "live"`,
		Subtitle:    "A <b>night</b> in 東京",
		Description: `Tom's "plot" & <tag>`,
		Category:    "News, Sports",
		Season:      12,
		Episode:     3,
		StartedAt:   started,
	}
	body := Episode(rec)
	if !utf8.Valid(body) {
		t.Fatalf("not utf-8: %q", body)
	}
	if !bytes.HasPrefix(body, []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)) {
		t.Fatalf("header: %s", body)
	}
	if !bytes.Contains(body, []byte("News &amp; Weather")) || !bytes.Contains(body, []byte("&lt;b&gt;")) || !bytes.Contains(body, []byte("&lt;tag&gt;")) {
		t.Fatalf("escapes missing: %s", body)
	}
	if !bytes.Contains(body, []byte("&#34;")) && !bytes.Contains(body, []byte("&quot;")) {
		t.Fatalf("quote was left raw: %s", body)
	}
	if !bytes.Contains(body, []byte("東京")) {
		t.Fatalf("non-ascii was rewritten: %s", body)
	}
	if rawAmp(body) {
		t.Fatalf("raw amp: %s", body)
	}
	var got episodeDetails
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.ShowTitle != rec.Title || got.Title != rec.Subtitle || got.Plot != rec.Description {
		t.Fatalf("%+v", got)
	}
	if got.Season != 12 || got.Episode != 3 || got.Aired != "2026-09-28" {
		t.Fatalf("%+v", got)
	}
	if strings.Join(got.Genre, "|") != "News|Sports" {
		t.Fatalf("genres %+v", got.Genre)
	}
}

func TestEpisodeOmitsUnknownSeason(t *testing.T) {
	body := Episode(store.Recording{Title: "Evening News", StartedAt: time.Date(2026, 1, 2, 12, 4, 0, 0, time.UTC)})
	if bytes.Contains(body, []byte("<season>")) || bytes.Contains(body, []byte("<episode>")) {
		t.Fatalf("invented a number: %s", body)
	}
	if !bytes.Contains(body, []byte("<showtitle>Evening News</showtitle>")) || !bytes.Contains(body, []byte("<aired>2026-01-02</aired>")) {
		t.Fatalf("%s", body)
	}
}

func TestOriginalAirWins(t *testing.T) {
	body := Episode(store.Recording{
		Title: "Jeopardy", Subtitle: "Final", OriginalAir: "1996-09-23",
		StartedAt: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
	})
	if !bytes.Contains(body, []byte("<aired>1996-09-23</aired>")) {
		t.Fatalf("%s", body)
	}
}

func TestMovieDocument(t *testing.T) {
	rec := store.Recording{
		Title: "The Long Goodbye", Description: "A detective & a cat", Category: "Movies",
		OriginalAir: "1973-03-07",
	}
	body := Document(rec)
	if bytes.Contains(body, []byte("episodedetails")) || !bytes.Contains(body, []byte("<movie>")) {
		t.Fatalf("%s", body)
	}
	var got movieDetails
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != rec.Title || got.Plot != rec.Description || got.Premiered != "1973-03-07" || got.Year != "1973" {
		t.Fatalf("%+v", got)
	}
	episode := Document(store.Recording{Title: "News", Category: "News"})
	if !bytes.Contains(episode, []byte("episodedetails")) {
		t.Fatalf("%s", episode)
	}
}

func TestWithGuideCopiesTheListingOnAtTheStart(t *testing.T) {
	start := time.Date(2026, 9, 28, 22, 0, 0, 0, time.UTC)
	rec := store.Recording{
		ChannelID: 4, Title: "Evening News", ProgramID: "EP9", Status: "complete", StartedAt: start.Add(10 * time.Minute),
	}
	airings := []store.Airing{
		{ChannelID: 4, Title: "Other", ProgramID: "EP1", Start: start, End: start.Add(time.Hour), Season: 1, Episode: 1},
		{ChannelID: 9, Title: "Evening News", ProgramID: "EP9", Start: start, End: start.Add(time.Hour), Season: 8, Episode: 8},
		{
			ChannelID: 4, Title: "Evening News", ProgramID: "EP9", Subtitle: "Local headlines",
			Description: "The evening newscast.", Category: "News",
			Start: start, End: start.Add(time.Hour), Season: 3, Episode: 4, OriginalAir: "2024-11-02",
		},
	}
	got := WithGuide(rec, airings)
	if got.Season != 3 || got.Episode != 4 || got.OriginalAir != "2024-11-02" || got.Subtitle != "Local headlines" {
		t.Fatalf("%+v", got)
	}
	body := Episode(got)
	if !bytes.Contains(body, []byte("<season>3</season>")) || !bytes.Contains(body, []byte("<episode>4</episode>")) || !bytes.Contains(body, []byte("<aired>2024-11-02</aired>")) {
		t.Fatalf("%s", body)
	}
	// A start with no listing stays free of a guessed number.
	missed := WithGuide(store.Recording{ChannelID: 4, Title: "Evening News", StartedAt: start.Add(3 * time.Hour)}, airings)
	if missed.Season != 0 || missed.Subtitle != "" {
		t.Fatalf("guessed %+v", missed)
	}
}

func TestWithGuidePicksTheShowAPaddedRecordingCovers(t *testing.T) {
	start := time.Date(2026, 9, 28, 22, 0, 0, 0, time.UTC)
	airings := []store.Airing{
		{ChannelID: 4, Title: "Jeopardy", Start: start.Add(-30 * time.Minute), End: start, Season: 42, Episode: 10},
		{ChannelID: 4, Title: "Jeopardy", Start: start, End: start.Add(30 * time.Minute), Season: 42, Episode: 11},
	}
	ends := start.Add(32 * time.Minute)
	rec := store.Recording{ChannelID: 4, Title: "Jeopardy", StartedAt: start.Add(-2 * time.Minute), EndsAt: &ends}
	got := WithGuide(rec, airings)
	if got.Season != 42 || got.Episode != 11 {
		t.Fatalf("padded recording matched %+v", got)
	}
}
