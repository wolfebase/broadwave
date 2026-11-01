package live

import (
	"strings"
	"testing"

	"broadwave/internal/store"
)

// A 3.0 station sent its Spanish commentary on the PID its PMT called
// English, and English on the one it called Spanish. With the channel's
// SoundSwapped choice, the English sound plays as the main one and both
// tracks carry the language they really are.
func TestSwappedSoundsPlayTheOtherTrackAsMain(t *testing.T) {
	read := []AudioTrack{
		{PID: 50, Language: "eng", Role: "main", Codec: "ac4", Label: "English", Channels: 2, Measured: true},
		{PID: 51, Language: "spa", Role: "language", Codec: "ac4", Label: "Spanish", Channels: 2, Measured: true},
	}
	got := swapSounds(read)
	if got[0].PID != 51 || got[0].Role != "main" || got[0].Language != "eng" || got[0].Label != "English" {
		t.Fatalf("main = %+v, want PID 51 as English main", got[0])
	}
	if got[1].PID != 50 || got[1].Role != "language" || got[1].Language != "spa" {
		t.Fatalf("second = %+v, want PID 50 as Spanish", got[1])
	}
	if read[0].PID != 50 {
		t.Fatal("swapSounds changed its input")
	}
	if one := swapSounds(read[:1]); one[0].PID != 50 {
		t.Fatalf("a lone main moved: %+v", one)
	}

	f := &feed{channel: store.SourceChannel{Channel: store.Channel{SoundSwapped: true}}, tracks: got}
	src := f.sourceFor(Rendition{Video: "copy", Audio: "aac2"})
	if src.AudioPID != 51 || src.AudioLang != "eng" {
		t.Fatalf("source = PID %d lang %q, want 51 eng", src.AudioPID, src.AudioLang)
	}
	src.Extras = got[1:]
	args := strings.Join(RenditionArgs(0, src, Rendition{Video: "copy", Audio: "aac2"}, "libx264", ""), " ")
	for _, want := range []string{"-map 0:i:51 -map 0:i:50", "-metadata:s:a:0 language=eng -metadata:s:a:1 language=spa"} {
		if !strings.Contains(args, want) {
			t.Errorf("args miss %q:\n%s", want, args)
		}
	}

	plain := &feed{tracks: read}
	if src := plain.sourceFor(Rendition{Video: "copy", Audio: "aac2"}); src.AudioPID != 50 || src.AudioLang != "" {
		t.Fatalf("an unswapped channel = PID %d lang %q", src.AudioPID, src.AudioLang)
	}
	if args := strings.Join(RenditionArgs(0, Source{VideoCodec: "H264", AudioCodec: "AC3", AudioPID: 50}, Rendition{Video: "copy", Audio: "aac2"}, "libx264", ""), " "); strings.Contains(args, "-metadata:s:a") {
		t.Fatalf("an unswapped channel relabels: %s", args)
	}
}
