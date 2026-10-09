package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseM3UStopsAtTheEntryCap(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxEntries+10; i++ {
		b.WriteString("http://example/a\n")
	}
	got := ParseM3U(strings.NewReader(b.String()))
	if len(got) != maxEntries {
		t.Fatalf("len %d", len(got))
	}
}

func TestReadPlaylistStopsAtTheCap(t *testing.T) {
	prev := maxPlaylist
	maxPlaylist = 32
	t.Cleanup(func() { maxPlaylist = prev })
	dir := t.TempDir()
	small := filepath.Join(dir, "ok.m3u")
	if err := os.WriteFile(small, []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := readLocalPlaylist(small)
	if err != nil || string(body) != "#EXTM3U\n" {
		t.Fatalf("small playlist %q %v", body, err)
	}
	big := filepath.Join(dir, "big.m3u")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 64)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readLocalPlaylist(big); err == nil {
		t.Fatal("a playlist past the cap was read")
	}
}

func FuzzParseM3U(f *testing.F) {
	f.Add([]byte("#EXTM3U\n#EXTINF:-1 tvg-name=\"News\",News\nhttp://example.test/a.ts\n"))
	f.Add([]byte("http://example.test/only.ts\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			data = data[:64<<10]
		}
		_ = ParseM3U(strings.NewReader(string(data)))
	})
}
