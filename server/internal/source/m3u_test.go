package source

import (
	"strings"
	"testing"
)

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
