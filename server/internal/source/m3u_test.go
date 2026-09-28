package source

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestInstallLeavesUnnamedCodecsEmpty(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := t.Context()
	entries := []Entry{
		{Number: "801", Name: "Link", URL: "http://example/live.ts"},
		{Number: "802", Name: "Named", URL: "http://example/named.ts", Video: "mpeg2video", Audio: "ac3"},
	}
	if err := Install(ctx, st, 1, "Link", "Link", entries); err != nil {
		t.Fatal(err)
	}
	codecs := func() (map[string][2]string, map[string]int64) {
		channels, err := st.Channels(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][2]string{}
		ids := map[string]int64{}
		for _, ch := range channels {
			out[ch.GuideNumber] = [2]string{ch.VideoCodec, ch.AudioCodec}
			ids[ch.GuideNumber] = ch.ID
		}
		return out, ids
	}
	got, ids := codecs()
	if got["801"] != [2]string{"", ""} {
		t.Fatalf("unnamed codecs %v", got["801"])
	}
	if got["802"] != [2]string{"mpeg2video", "ac3"} {
		t.Fatalf("named codecs %v", got["802"])
	}
	// A refresh of the same playlist keeps what a tune learned.
	if err := st.SetChannelCodecs(ctx, ids["801"], "MPEG2", "AC3"); err != nil {
		t.Fatal(err)
	}
	if err := Install(ctx, st, 1, "Link", "Link", entries); err != nil {
		t.Fatal(err)
	}
	if got, _ := codecs(); got["801"] != [2]string{"MPEG2", "AC3"} {
		t.Fatalf("refresh codecs %v", got["801"])
	}
}

func TestParseM3U(t *testing.T) {
	raw := `#EXTM3U
#EXTINF:-1 tvg-name="News",Local News
http://example/news.ts
http://example/extra.ts
`
	got := ParseM3U(strings.NewReader(raw))
	if len(got) != 2 || got[0].Name != "Local News" || got[0].URL != "http://example/news.ts" {
		t.Fatalf("%+v", got)
	}
	if got[1].Name != "Stream 2" {
		t.Fatalf("unnamed: %+v", got[1])
	}
}

func TestParseM3UKeepsChannelAttributes(t *testing.T) {
	raw := `#EXTM3U url-tvg="http://example/guide.xml"
#EXTINF:-1 tvg-id="kbwv" tvg-chno="4.1" tvg-logo="http://example/a.png" group-title="Local",ABC
http://example/abc.ts
`
	got := ParseM3U(strings.NewReader(raw))
	if len(got) != 1 {
		t.Fatal(len(got))
	}
	e := got[0]
	if e.ID != "kbwv" || e.Number != "4.1" || e.Logo != "http://example/a.png" || e.Group != "Local" || e.GuideURL != "http://example/guide.xml" || e.Name != "ABC" {
		t.Fatalf("%+v", e)
	}
}

func TestParseM3UKeepsStreamOptions(t *testing.T) {
	raw := `#EXTM3U
#EXTINF:-1 channel-id="kbwv" tvg-shift="0" catchup="default" catchup-source="http://example/{utc}" catchup-days="3" tvc-guide-title="News" tvc-guide-description="At 6" tvc-guide-art="http://example/art.jpg" tvc-guide-tags="news" tvc-guide-genres="News" tvc-stream-vcodec="MPEG2" tvc-stream-acodec="AC3",ABC
#EXTVLCOPT:http-user-agent=Broadwave
#KODIPROP:http-referrer=http://example/
http://example/abc.ts
`
	e := ParseM3U(strings.NewReader(raw))[0]
	if e.ID != "kbwv" || e.Shift != "0" || e.Catchup != "default" || e.CatchupDays != "3" || e.GuideTitle != "News" || e.Video != "MPEG2" || e.Audio != "AC3" || e.UserAgent != "Broadwave" || e.Referrer != "http://example/" {
		t.Fatalf("%+v", e)
	}
}

func TestFilterGroupsAndGuideURL(t *testing.T) {
	entries := []Entry{
		{Name: "News", Group: "Local"},
		{Name: "Shop", Group: "Shopping"},
		{Name: "Game", Group: "Sports", GuideURL: "http://example/guide.xml"},
	}
	got := FilterGroups(entries, "Local, Sports, -Shopping")
	if len(got) != 2 || got[0].Name != "News" || got[1].Name != "Game" {
		t.Fatalf("%+v", got)
	}
	if GuideFromPlaylist("", entries) != "http://example/guide.xml" {
		t.Fatal(GuideFromPlaylist("", entries))
	}
	if GuideFromPlaylist("http://mine/guide.xml", entries) != "http://mine/guide.xml" {
		t.Fatal("typed guide should win")
	}
}

func TestRenumberAndReadAGzipFile(t *testing.T) {
	entries := []Entry{{Number: "4.1", Name: "ABC"}, {Number: "5.1", Name: "NBC"}}
	got := Renumber(entries, 100)
	if got[0].Number != "100" || got[1].Number != "101" || entries[0].Number != "4.1" {
		t.Fatalf("%+v %+v", got, entries)
	}
	if Renumber(entries, 0)[0].Number != "4.1" {
		t.Fatal("zero keeps playlist numbers")
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("#EXTM3U\n#EXTINF:-1,News\nhttp://example/news.ts\n"))
	_ = zw.Close()
	path := t.TempDir() + "/pl.m3u.gz"
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := ReadPlaylist(t.Context(), path)
	if err != nil || !strings.Contains(string(body), "News") {
		t.Fatal(err, string(body))
	}
}

func TestBigPlaylistAsksForGroups(t *testing.T) {
	entries := make([]Entry, 301)
	for i := range entries {
		entries[i] = Entry{Name: "Ch", Group: "Local"}
	}
	entries[0].Group = "Sports"
	msg := BigPlaylistMessage(entries)
	if !strings.Contains(msg, "301") || !strings.Contains(msg, "Sports") || !strings.Contains(msg, "Local") {
		t.Fatal(msg)
	}
	if BigPlaylistMessage(entries[:10]) != "" {
		t.Fatal("a short playlist should add without a picker")
	}
}

func TestFilterKeep(t *testing.T) {
	entries := []Entry{{Name: "News", ID: "kbwv"}, {Name: "Shop", ID: "shop"}}
	got := FilterKeep(entries, "kbwv")
	if len(got) != 1 || got[0].Name != "News" {
		t.Fatalf("%+v", got)
	}
	if len(FilterKeep(entries, "")) != 2 {
		t.Fatal("an empty keep list should leave the playlist alone")
	}
}

func TestProbeFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "ts") {
			_, _ = w.Write([]byte{0x47, 0x00})
			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,News\n"))
	}))
	defer srv.Close()
	if ProbeFormat(t.Context(), srv.URL+"/live.m3u8") != "hls" {
		t.Fatal("m3u8 address")
	}
	if ProbeFormat(t.Context(), srv.URL+"/guide") != "hls" {
		t.Fatal("playlist body")
	}
	if ProbeFormat(t.Context(), srv.URL+"/ch.ts") != "mpegts" {
		t.Fatal("sync byte")
	}
}

func TestParseM3UHandlesALargePlaylist(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&b, "#EXTINF:-1 tvg-id=\"id%d\" tvg-chno=\"%d\" tvc-guide-stationid=\"s%d\",Name %d\nhttp://example/%d.ts\n", i, i, i, i, i)
	}
	start := time.Now()
	got := ParseM3U(strings.NewReader(b.String()))
	if len(got) != 5000 || got[4999].ID != "id4999" || got[4999].Station != "s4999" {
		t.Fatalf("len %d last %+v", len(got), got[len(got)-1])
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("import took %s", time.Since(start))
	}
}
