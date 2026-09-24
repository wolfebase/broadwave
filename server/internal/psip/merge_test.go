package psip

import (
	"testing"
	"time"
)

func TestFillGapsLeavesCoveredTimeAlone(t *testing.T) {
	start := time.Date(2026, 9, 24, 20, 0, 0, 0, time.UTC)
	have := []Span{{ChannelID: 1, Start: start, End: start.Add(time.Hour)}}
	extra := []Span{
		{ChannelID: 1, Start: start, End: start.Add(30 * time.Minute)},
		{ChannelID: 1, Start: start.Add(time.Hour), End: start.Add(2 * time.Hour)},
		{ChannelID: 2, Start: start, End: start.Add(time.Hour)},
	}
	got := FillGaps(have, extra)
	if len(got) != 2 || got[0].ChannelID != 1 || got[1].ChannelID != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestTitleCase(t *testing.T) {
	if TitleCase("JEOPARDY!") != "Jeopardy!" {
		t.Fatal(TitleCase("JEOPARDY!"))
	}
	if TitleCase("Mork & Mindy") != "Mork & Mindy" {
		t.Fatal(TitleCase("Mork & Mindy"))
	}
}
