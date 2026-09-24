package guide

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"broadwave/internal/store"
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

func TestPullURLReadsGzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(`<tv><channel id="4.1"><display-name>4.1</display-name></channel></tv>`))
	_ = zw.Close()
	payload := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	body, err := PullURL(t.Context(), srv.URL+"/guide.xml.gz")
	if err != nil || !strings.Contains(string(body), "4.1") {
		t.Fatal(err, string(body))
	}
}

func TestPullURLReadsXZ(t *testing.T) {
	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("xz is not installed")
	}
	cmd := exec.Command("xz", "-c")
	cmd.Stdin = strings.NewReader(`<tv><channel id="9.1"><display-name>9.1</display-name></channel></tv>`)
	payload, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	body, err := PullURL(t.Context(), srv.URL+"/guide.xml.xz")
	if err != nil || !strings.Contains(string(body), "9.1") {
		t.Fatal(err, string(body))
	}
}
