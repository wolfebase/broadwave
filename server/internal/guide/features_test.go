package guide

import (
	"testing"

	"broadwave/internal/store"
)

func TestXMLTVReadsPictureSoundAndCaptions(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="4.1"><display-name>4.1</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922153000 -0500" channel="4.1">
    <title>News</title>
    <video><quality>HDTV</quality></video>
    <audio><stereo>dolby digital</stereo></audio>
    <subtitles type="teletext" />
  </programme>
  <programme start="20260922153000 -0500" stop="20260922160000 -0500" channel="4.1">
    <title>Movie</title>
    <video><quality>SDTV</quality></video>
    <audio><stereo>mono</stereo></audio>
    <subtitles type="onscreen" />
  </programme>
  <programme start="20260922160000 -0500" stop="20260922163000 -0500" channel="4.1">
    <title>Quiet</title>
  </programme>
</tv>`)
	got, _, err := Parse(raw, []store.Channel{{ID: 7, GuideNumber: "4.1"}})
	if err != nil || len(got) != 3 {
		t.Fatal(err, got)
	}
	if got[0].Video != VideoHD || got[0].Audio != AudioSurround || !got[0].CC {
		t.Fatalf("news %+v", got[0])
	}
	if got[1].Video != VideoSD || got[1].Audio != AudioMono || got[1].CC {
		t.Fatalf("movie (open subtitles are not captions) %+v", got[1])
	}
	if got[2].Video != "" || got[2].Audio != "" || got[2].CC {
		t.Fatalf("a listing that says nothing %+v", got[2])
	}
}

func TestSchedulesDirectReadsPictureSoundAndCaptions(t *testing.T) {
	row := sdAiring(sdScheduled{ProgramID: "EP012345670003", VideoProperties: []string{"hdtv"}, AudioProperties: []string{"cc", "stereo", "dd 5.1"}}, sdProgram{})
	if row.Video != VideoHD || row.Audio != AudioSurround || !row.CC {
		t.Fatalf("%+v", row)
	}
	bare := sdAiring(sdScheduled{ProgramID: "EP012345670003"}, sdProgram{})
	if bare.Video != "" || bare.Audio != "" || bare.CC {
		t.Fatalf("%+v", bare)
	}
}

func TestFeatureWords(t *testing.T) {
	for raw, want := range map[string]string{"HDTV": VideoHD, "UHDTV": VideoUHD, "4K": VideoUHD, "1080i": VideoHD, "SDTV": VideoSD, "": "", "color": ""} {
		if got := VideoQuality(raw); got != want {
			t.Errorf("video %q = %q, want %q", raw, got, want)
		}
	}
	for raw, want := range map[string]string{"stereo": AudioStereo, "joint stereo": AudioStereo, "bilingual": AudioStereo, "dolby": AudioSurround, "DD 5.1": AudioSurround, "mono": AudioMono, "SAP": "", "": ""} {
		if got := AudioLayout(raw); got != want {
			t.Errorf("audio %q = %q, want %q", raw, got, want)
		}
	}
}
