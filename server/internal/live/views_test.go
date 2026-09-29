package live

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

const stampedFixture = `#EXTM3U
#EXT-X-VERSION:9
#EXT-X-TARGETDURATION:2
#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,HOLD-BACK=6.000,PART-HOLD-BACK=2.000,CAN-SKIP-UNTIL=12.000
#EXT-X-PART-INF:PART-TARGET=0.500
#EXT-X-MEDIA-SEQUENCE:40
#EXT-X-MAP:URI="init.mp4"
#EXT-X-PROGRAM-DATE-TIME:2026-09-28T20:00:00.000Z
#EXTINF:2.002,
seg00040.m4s
#EXT-X-DISCONTINUITY
#EXT-X-PROGRAM-DATE-TIME:2026-09-28T20:00:02.002Z
#EXTINF:2.002,
seg00041.m4s
#EXT-X-PART:DURATION=0.501,INDEPENDENT=YES,URI="part00000.m4s"
#EXT-X-PART:DURATION=0.501,URI="part00001.m4s"
`

func TestAViewIsTheSamePlaylistWithItsOwnMedia(t *testing.T) {
	reports := renditionReports([]byte(stampedFixture), []string{"video.m3u8", "audio-3.m3u8"})
	got := string(viewPlaylist([]byte(stampedFixture), "a2", reports))
	want := strings.NewReplacer(`"init.mp4"`, `"init.a2.mp4"`, "seg00040.m4s", "seg00040.a2.m4s",
		"seg00041.m4s", "seg00041.a2.m4s", `part00000.m4s"`, `part00000.a2.m4s"`, `part00001.m4s"`, `part00001.a2.m4s"`).Replace(stampedFixture) +
		`#EXT-X-RENDITION-REPORT:URI="video.m3u8",LAST-MSN=42,LAST-PART=1` + "\n" +
		`#EXT-X-RENDITION-REPORT:URI="audio-3.m3u8",LAST-MSN=42,LAST-PART=1` + "\n"
	if got != want {
		t.Fatalf("view:\n%s\nwant:\n%s", got, want)
	}
	closed := strings.Split(stampedFixture, "#EXT-X-PART:")[0]
	if r := renditionReports([]byte(closed), []string{"video.m3u8"}); len(r) != 1 || r[0] != `#EXT-X-RENDITION-REPORT:URI="video.m3u8",LAST-MSN=41` {
		t.Fatalf("no open parts: %v", r)
	}
}

func TestViewNames(t *testing.T) {
	for name, want := range map[string]string{"video.m3u8": "v", "audio-2.m3u8": "a2", "audio-12.m3u8": "a12"} {
		if got, ok := playlistTag(name); !ok || got != want {
			t.Errorf("%s: %q %v", name, got, ok)
		}
	}
	for _, name := range []string{"audio-1.m3u8", "audio-02.m3u8", "audio-x.m3u8", "audio-3.m3u", "index.m3u8", "audio-4294967298.m3u8"} {
		if _, ok := playlistTag(name); ok {
			t.Errorf("%s is not a view", name)
		}
	}
}

func TestScreensShareOneCut(t *testing.T) {
	var c cutCache
	k := cutKey{path: "/x/seg00001.m4s", tag: "a2", size: 10}
	body := []byte("0123456789")
	c.put(k, body)
	got, ok := c.get(k)
	if !ok || &got[0] != &body[0] {
		t.Fatal("a second screen should get the same bytes")
	}
	k.size = 11
	if _, ok := c.get(k); ok {
		t.Fatal("a file rewritten after a restart is a new cut")
	}
	big := make([]byte, cutBudget/2+1)
	c.put(cutKey{path: "a"}, big)
	c.put(cutKey{path: "b"}, big)
	if _, ok := c.get(cutKey{path: "a"}); ok || c.bytes > cutBudget {
		t.Fatalf("the oldest cut should go past the budget (%d bytes)", c.bytes)
	}
}

// viewHub serves dir as rendition 1080.aac2.broadcast of channel 1.
func viewHub(t *testing.T, dir string, extras []AudioTrack) *Hub {
	t.Helper()
	h, m := testHub(t)
	spec, _ := ParseRenditionKey("1080.aac2.broadcast")
	h.mu.Lock()
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1"}, FieldOrder: "progressive"})
	f.renditions[spec.Key()] = &rendition{spec: spec, dir: dir, extras: extras, seen: time.Now()}
	h.mu.Unlock()
	return h
}

func trackIDs(t *testing.T, body []byte, init bool) []uint32 {
	t.Helper()
	var ids []uint32
	if init {
		for id := range trackScales(body) {
			ids = append(ids, id)
		}
	} else {
		for _, moof := range boxesOf(body, "moof") {
			for _, traf := range boxesOf(moof, "traf") {
				r, ok := parseRun(traf)
				if !ok {
					t.Fatal("unreadable traf")
				}
				if !slices.Contains(ids, r.id) {
					ids = append(ids, r.id)
				}
			}
		}
	}
	slices.Sort(ids)
	return ids
}

func TestViewsCutOneEncode(t *testing.T) {
	dir, _, _ := packThreeTracks(t)
	h := viewHub(t, dir, []AudioTrack{spaExtra})
	cases := []struct {
		name string
		want []uint32
	}{
		{"init.v.mp4", []uint32{1}},
		{"init.a2.mp4", []uint32{2}},
		{"init.a3.mp4", []uint32{3}},
		{"init.mp4", []uint32{1, 2}},
		{"seg00001.v.m4s", []uint32{1}},
		{"seg00001.a3.m4s", []uint32{3}},
		{"seg00001.m4s", []uint32{1, 2}},
	}
	for _, tc := range cases {
		body, ok, err := h.ViewMedia(1, "1080.aac2.broadcast", tc.name)
		if !ok || err != nil {
			t.Fatalf("%s: ok=%v err=%v", tc.name, ok, err)
		}
		if got := trackIDs(t, body, strings.HasPrefix(tc.name, "init")); !slices.Equal(got, tc.want) {
			t.Errorf("%s keeps %v, want %v", tc.name, got, tc.want)
		}
	}
	parts, _ := filepath.Glob(filepath.Join(dir, "part*.m4s"))
	if len(parts) == 0 {
		t.Fatal("the pack left no parts to cut")
	}
	name := strings.TrimSuffix(filepath.Base(parts[0]), ".m4s") + ".a2.m4s"
	if body, ok, err := h.ViewMedia(1, "1080.aac2.broadcast", name); !ok || err != nil || !slices.Equal(trackIDs(t, body, false), []uint32{2}) {
		t.Errorf("%s: ok=%v err=%v", name, ok, err)
	}
	for _, name := range []string{"init.a4.mp4", "seg99999.v.m4s", "init.v.m4s", "seg00001.v.mp4", "seg00001.a4294967298.m4s"} {
		if _, ok, err := h.ViewMedia(1, "1080.aac2.broadcast", name); !ok || err == nil {
			t.Errorf("%s should not be found: ok=%v err=%v", name, ok, err)
		}
	}
	// One sound: the legacy names are the files as written.
	plain := viewHub(t, dir, nil)
	if _, ok, _ := plain.ViewMedia(1, "1080.aac2.broadcast", "seg00001.m4s"); ok {
		t.Error("an encode with one sound is served from disk")
	}
	if _, ok, _ := plain.ViewMedia(1, "1080.aac2.broadcast", "index.m3u8"); ok {
		t.Error("the playlist is not media")
	}
}

func TestViewPlaylistsFollowTheIndex(t *testing.T) {
	dir, _, _ := packThreeTracks(t)
	h := viewHub(t, dir, []AudioTrack{spaExtra})
	index, err := h.Playlist(1, "1080.aac2.broadcast")
	if err != nil {
		t.Fatal(err)
	}
	for name, tag := range map[string]string{"video.m3u8": "v", "audio-2.m3u8": "a2", "audio-3.m3u8": "a3"} {
		body, ok, err := h.ViewPlaylist(1, "1080.aac2.broadcast", name, false)
		if !ok || err != nil {
			t.Fatalf("%s: ok=%v err=%v", name, ok, err)
		}
		var kept []string
		for line := range strings.SplitSeq(string(body), "\n") {
			if !strings.HasPrefix(line, "#EXT-X-RENDITION-REPORT:") {
				kept = append(kept, line)
			}
		}
		back := strings.ReplaceAll(strings.Join(kept, "\n"), "."+tag+".", ".")
		if back != string(index) {
			t.Fatalf("%s differs from index.m3u8 beyond its media names:\n%s\n---\n%s", name, back, index)
		}
		if n := strings.Count(string(body), "#EXT-X-RENDITION-REPORT:"); n != 2 {
			t.Errorf("%s has %d reports, want its two siblings", name, n)
		}
		if !strings.Contains(string(body), "PROGRAM-DATE-TIME") {
			t.Errorf("%s has no dates", name)
		}
	}
	if _, ok, err := h.ViewPlaylist(1, "1080.aac2.broadcast", "audio-4.m3u8", false); !ok || err == nil {
		t.Errorf("audio-4 is not a track of this encode: ok=%v err=%v", ok, err)
	}
	quiet := viewHub(t, dir, nil)
	quiet.mu.Lock()
	quiet.channels[1].renditions["1080.aac2.broadcast"].spec.Audio = "none"
	quiet.mu.Unlock()
	if _, _, err := quiet.ViewPlaylist(1, "1080.aac2.broadcast", "audio-2.m3u8", false); err == nil {
		t.Error("a silent encode has no sound view")
	}
	if _, err := os.Stat(filepath.Join(dir, "index.m3u8")); err != nil {
		t.Fatal(err)
	}
}
