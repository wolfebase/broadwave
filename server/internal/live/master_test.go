package live

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCodecsReadThePackedEncode(t *testing.T) {
	dir, _, _ := packThreeTracks(t)
	init, err := os.ReadFile(filepath.Join(dir, "init.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	codecs := initCodecs(init)
	// libx264's default is High profile: avc1.64 then compatibility and level.
	if v := codecs[1]; !strings.HasPrefix(v.codec, "avc1.64") || len(v.codec) != 11 || v.width != 640 || v.height != 360 {
		t.Fatalf("video %+v", v)
	}
	// Stereo and mono, read from each track's dac3.
	if codecs[2] != (trackCodec{codec: "ac-3", channels: 2}) || codecs[3] != (trackCodec{codec: "ac-3", channels: 1}) {
		t.Fatalf("sound %+v %+v", codecs[2], codecs[3])
	}
	for _, cut := range []int{0, 8, 16, 40, 100} {
		if cut < len(init) {
			_ = initCodecs(init[:cut])
		}
	}
}

func TestHEVCCodecString(t *testing.T) {
	// Main profile, compatible with Main and Main 10, main tier, level 3.1,
	// progressive and frame-only constraint flags.
	hvcC := []byte{1, 0x01, 0x60, 0, 0, 0, 0xB0, 0, 0, 0, 0, 0, 93}
	if got := hevcCodec("hvc1", hvcC); got != "hvc1.1.6.L93.B0" {
		t.Fatalf("got %s", got)
	}
	hvcC[1] = 0x22 // high tier, Main 10
	hvcC[2] = 0x20
	if got := hevcCodec("hvc1", hvcC); got != "hvc1.2.4.H93.B0" {
		t.Fatalf("got %s", got)
	}
}

func TestMasterListsEachSoundTrack(t *testing.T) {
	codecs := map[uint32]trackCodec{
		1: {codec: "avc1.640028", width: 1920, height: 1080},
		2: {codec: "mp4a.40.2"}, 3: {codec: "mp4a.40.2"}, 4: {codec: "mp4a.40.2"},
	}
	described := AudioTrack{PID: 0x103, Language: "eng", Role: "described", Codec: "ac3", Label: "Described video", Channels: 2, Measured: true}
	spec, _ := ParseRenditionKey("1080.aac6.broadcast")
	body, err := masterPlaylist(spec, codecs, []AudioTrack{engMain, spaExtra, described}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,CHANNELS="6",URI="audio-2.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Spanish",LANGUAGE="es",DEFAULT=NO,AUTOSELECT=YES,CHANNELS="2",URI="audio-3.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Described video",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=YES,CHANNELS="2",CHARACTERISTICS="public.accessibility.describes-video",URI="audio-4.m3u8"
` + captionMedia + `#EXT-X-STREAM-INF:BANDWIDTH=14384000,CODECS="avc1.640028,mp4a.40.2",RESOLUTION=1920x1080,AUDIO="aud",SUBTITLES="cc",CLOSED-CAPTIONS=NONE
video.m3u8
`
	if string(body) != want {
		t.Fatalf("master:\n%s\nwant:\n%s", body, want)
	}
}

func TestMasterKeepsOneSoundCodecAndUniqueNames(t *testing.T) {
	codecs := map[uint32]trackCodec{1: {codec: "avc1.640028"}, 2: {codec: "ac-3"}, 3: {codec: "mp4a.40.2"}, 4: {codec: "ac-3"}}
	second := engMain
	second.Channels = 2
	spec, _ := ParseRenditionKey("copy.copy")
	body, err := masterPlaylist(spec, codecs, []AudioTrack{engMain, spaExtra, second}, false)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if strings.Contains(got, "audio-3.m3u8") || !strings.Contains(got, `NAME="English 2",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=NO`) || strings.Count(got, "DEFAULT=YES") != 1 {
		t.Fatalf("master:\n%s", got)
	}
	if strings.Contains(got, "SUBTITLES") || !strings.Contains(got, `CODECS="avc1.640028,ac-3"`) {
		t.Fatalf("master:\n%s", got)
	}
	quiet, _ := ParseRenditionKey("540.none.broadcast")
	if _, err := masterPlaylist(quiet, codecs, []AudioTrack{engMain}, false); err == nil {
		t.Fatal("a silent encode has no master")
	}
}

func TestLanguagesAPlayerCanMatch(t *testing.T) {
	for raw, want := range map[string]string{"eng": "eng", "ENG": "eng", "Spa": "spa", "und": "", "qaa": "", "e\x00\x00": "", "\x00\x00\x00": "", "12a": ""} {
		if got := cleanLanguage([]byte(raw)); got != want {
			t.Errorf("%q: %q, want %q", raw, got, want)
		}
	}
	for code, want := range map[string]string{"eng": "en", "ita": "it", "haw": "haw"} {
		if got := bcp47(code); got != want {
			t.Errorf("%s: %s, want %s", code, got, want)
		}
	}
	if got := quoted("Main \"2\"\n"); got != `"Main 2"` {
		t.Errorf("quoted %s", got)
	}
}

func TestHubMasterNamesThePMT(t *testing.T) {
	dir, _, _ := packThreeTracks(t)
	h := viewHub(t, dir, []AudioTrack{spaExtra})
	h.mu.Lock()
	h.channels[1].tracks = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	body, err := h.MasterPlaylist(1, "1080.aac2.broadcast")
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{`NAME="English",LANGUAGE="en",DEFAULT=YES`, `NAME="Spanish",LANGUAGE="es",DEFAULT=NO`, `URI="audio-3.m3u8"`, `CODECS="avc1.64`, ",ac-3\"", "RESOLUTION=640x360", "\nvideo.m3u8\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestHubMasterNamesTheStoredMainBeforeThePMT(t *testing.T) {
	dir, _, _ := packThreeTracks(t)
	h := viewHub(t, dir, []AudioTrack{spaExtra})
	h.mu.Lock()
	h.channels[1].stored = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	body, err := h.MasterPlaylist(1, "copy.copy")
	if err == nil {
		t.Fatalf("no such rendition:\n%s", body)
	}
	body, err = h.MasterPlaylist(1, "1080.aac2.broadcast")
	if err != nil || !strings.Contains(string(body), `NAME="English",LANGUAGE="en",DEFAULT=YES`) {
		t.Fatalf("%v\n%s", err, body)
	}
}
