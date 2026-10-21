package guide

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestParseEpisodeIdentity(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="4.1"><display-name>4.1</display-name><icon src="https://img.example/kbwv.png" /></channel>
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
	got, art, err := Parse(raw, []store.Channel{{ID: 7, GuideNumber: "4.1", GuideName: "KBWV"}})
	if err != nil || len(got) != 1 {
		t.Fatal(err, got)
	}
	if got[0].ProgramID != "EP1" || !got[0].New || got[0].Subtitle != "Show 9001" || got[0].Category != "Game show" {
		t.Fatalf("%+v", got[0])
	}
	if art[7] != "https://img.example/kbwv.png" || got[0].ImageURL != "https://img.example/jeopardy.jpg" {
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
	if _, err := PullURL(t.Context(), "http://169.254.169.254/latest/meta-data"); err == nil {
		t.Fatal("expected a metadata address to be refused")
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

func TestGuideInflateStopsAtTheCap(t *testing.T) {
	prev := maxGuide
	maxGuide = 64
	t.Cleanup(func() { maxGuide = prev })

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write([]byte(strings.Repeat("x", 200))); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := inflateGuide(gz.Bytes(), "gzip", "guide.xml.gz"); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatal(err)
	}

	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("xz is not installed")
	}
	cmd := exec.Command("xz", "-c")
	cmd.Stdin = strings.NewReader(strings.Repeat("a", 200))
	payload, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inflateXZ(payload); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatal(err)
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

func TestRequestErrorDropsTheDeviceAuth(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://api.hdhomerun.com/api/xmltv?DeviceAuth=SECRET", Err: errors.New("connection refused")}
	got := requestError(err).Error()
	if strings.Contains(got, "SECRET") || !strings.Contains(got, "connection refused") {
		t.Fatal(got)
	}
}

func TestGuideAccessSkipsATunerThatIsGone(t *testing.T) {
	quiet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"OLD"}`))
	}))
	defer quiet.Close()
	flex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"FLEX","DeviceAuth":"abc"}`))
	}))
	defer flex.Close()
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	auth, err := deviceAuth(context.Background(), &hdhr.Client{}, []string{gone.URL, quiet.URL, flex.URL})
	if err != nil || auth != "abc" {
		t.Fatalf("auth %q, %v; want the first tuner that offers one", auth, err)
	}
	if _, err := deviceAuth(context.Background(), &hdhr.Client{}, []string{gone.URL}); err == nil {
		t.Fatal("a tuner that is gone gave guide access")
	}
	if _, err := deviceAuth(context.Background(), &hdhr.Client{}, nil); err == nil {
		t.Fatal("no tuner gave guide access")
	}
}

func TestParseWhenReadsEveryTimeShape(t *testing.T) {
	want := time.Date(2026, 10, 21, 18, 30, 0, 0, time.UTC)
	for _, raw := range []string{
		"20261021123000 -0600",
		"20261021123000-0600",
		"202610211230 -0600",
		"202610211230-06:00",
		"20261021183000",
		"20261021183000 +0000",
		"202610211830",
	} {
		got, err := parseWhen(raw)
		if err != nil || !got.Equal(want) {
			t.Errorf("%q: %v %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "2026", "20261021123000 -06", "20261021123000 +2500", "tomorrow"} {
		if _, err := parseWhen(raw); err == nil {
			t.Errorf("%q read as a time", raw)
		}
	}
}

func TestParseSaysWhenAFeedHasNoPrograms(t *testing.T) {
	lineup := []store.Channel{{ID: 7, GuideNumber: "4.1"}}
	for name, raw := range map[string]string{
		"empty":     `<tv><channel id="4.1"><display-name>4.1</display-name></channel></tv>`,
		"bad times": `<tv><programme start="Tuesday" stop="later" channel="4.1"><title>A</title></programme></tv>`,
	} {
		if _, _, err := Parse([]byte(raw), lineup); !errors.Is(err, ErrNoPrograms) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Programs on channels this lineup does not have are still a working feed.
	other := `<tv><programme start="20261021120000+0000" stop="20261021130000+0000" channel="9.1"><title>A</title></programme></tv>`
	rows, _, err := Parse([]byte(other), lineup)
	if err != nil || len(rows) != 0 {
		t.Fatalf("%v %v", rows, err)
	}
}
