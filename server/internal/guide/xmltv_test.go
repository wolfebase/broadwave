package guide

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"waveguide/internal/store"
)

func TestParseEpisodeIdentity(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="4.1"><display-name>4.1</display-name><icon src="https://img.example/wdaf.png" /></channel>
  <programme start="20260922150000 -0500" stop="20260922153000 -0500" channel="4.1">
    <title>Jeopardy!</title>
    <sub-title>Show 9001</sub-title>
    <episode-num system="dd_progid">EP1</episode-num>
    <episode-num system="onscreen">S12E34</episode-num>
    <episode-num system="xmltv_ns">11.33.</episode-num>
    <date>19960923</date>
    <series-id>SH1</series-id>
    <category>Game show</category>
    <icon src="https://img.example/jeopardy.jpg" />
    <rating><value>TV-G</value></rating>
    <credits><actor>Alex Trebek</actor></credits>
    <live />
    <new />
  </programme>
</tv>`)
	got, art, err := Parse(raw, []store.Channel{{ID: 7, GuideNumber: "4.1", GuideName: "WDAF"}})
	if err != nil || len(got) != 1 {
		t.Fatal(err, got)
	}
	if got[0].ProgramID != "EP1" || !got[0].New || got[0].Subtitle != "Show 9001" || got[0].Category != "Game show" {
		t.Fatalf("%+v", got[0])
	}
	if art[7] != "https://img.example/wdaf.png" || got[0].ImageURL != "https://img.example/jeopardy.jpg" {
		t.Fatalf("art %+v image %s", art, got[0].ImageURL)
	}
	if got[0].Season != 12 || got[0].Episode != 34 || got[0].EpisodeLabel != "S12E34" || got[0].OriginalAir != "1996-09-23" || got[0].SeriesID != "SH1" || !got[0].Live || got[0].Rating != "TV-G" || got[0].Cast != "Alex Trebek" {
		t.Fatalf("%+v", got[0])
	}
}

func TestPullURLReadsXMLTV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<tv><channel id="14.1"><display-name>14.1</display-name></channel></tv>`))
	}))
	defer srv.Close()
	body, err := PullURL(t.Context(), srv.URL)
	if err != nil || !strings.Contains(string(body), "14.1") {
		t.Fatal(err, string(body))
	}
	if _, err := PullURL(t.Context(), "ftp://example.com/guide.xml"); err == nil {
		t.Fatal("expected a non-http address to be refused")
	}
}
