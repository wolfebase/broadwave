package guide

import (
	"testing"

	"waveguide/internal/store"
)

func TestParseEpisodeIdentity(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="4.1"><display-name>4.1</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922153000 -0500" channel="4.1">
    <title>Jeopardy!</title>
    <sub-title>Show 9001</sub-title>
    <episode-num system="dd_progid">EP1</episode-num>
    <category>Game show</category>
    <new />
  </programme>
</tv>`)
	got, err := Parse(raw, []store.Channel{{ID: 7, GuideNumber: "4.1", GuideName: "WDAF"}})
	if err != nil || len(got) != 1 {
		t.Fatal(err, got)
	}
	if got[0].ProgramID != "EP1" || !got[0].New || got[0].Subtitle != "Show 9001" || got[0].Category != "Game show" {
		t.Fatalf("%+v", got[0])
	}
}
