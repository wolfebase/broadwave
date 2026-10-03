package httpapi

import (
	"context"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
)

// On an ffmpeg with no AC-4 decoder, a 3.0 channel plays its picture without
// sound and says why. ffmpeg would otherwise exit and nothing would play.
func TestA3Point0ChannelPlaysSilentWithoutAnAC4Decoder(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	dev := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, StreamURL: "http://flex:5004/auto/v104.1"},
	}); err != nil {
		t.Fatal(err)
	}
	web := &live.Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}
	apple := &live.Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3", "eac3"}}
	for _, noAC4 := range []bool{false, true} {
		s := &Server{Store: st, Hub: &live.Hub{Store: st, Encoder: "libx264", NoAC4: noAC4}}
		for _, caps := range []*live.Caps{web, apple} {
			d, _, err := s.decide(ctx, watchBody{ChannelID: guideID(t, st, "104.1"), Caps: caps})
			if err != nil {
				t.Fatal(err)
			}
			silent := d.Rendition.Audio == "none"
			if silent != noAC4 {
				t.Fatalf("noAC4=%v %s: %+v %q", noAC4, caps.Platform, d.Rendition, d.Reason)
			}
			if noAC4 && d.Reason != "No sound: this server's ffmpeg can't decode ATSC 3.0 sound (AC-4)." {
				t.Fatalf("reason %q", d.Reason)
			}
			named, _, err := s.decide(ctx, watchBody{ChannelID: guideID(t, st, "104.1"), Rendition: "copy.ac3"})
			if err != nil || (named.Rendition.Audio == "none") != noAC4 {
				t.Fatalf("noAC4=%v a rendition the player named: %+v %v", noAC4, named.Rendition, err)
			}
			one, _, err := s.decide(ctx, watchBody{ChannelID: guideID(t, st, "4.1"), Caps: caps})
			if err != nil || one.Rendition.Audio == "none" {
				t.Fatalf("a 1.0 channel keeps its sound: %+v %v", one.Rendition, err)
			}
		}
	}
}

// A link declares no codecs, so the watch decides on sound and the hub, which
// reads AC-4 when the link opens, plays it silent. The note says why.
func TestALinkFoundSilentSaysWhy(t *testing.T) {
	want := live.Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}
	silent := want
	silent.Audio = "none"
	ran := live.Session{Rendition: silent.Key(), Stream: live.StreamInfo{Video: "1080", Audio: "none", SourceAudio: "AC4"}}
	if why := ranAs(true, want, ran, false); why != noAC4Reason {
		t.Fatalf("no AC-4 decoder: %q", why)
	}
	if why := ranAs(false, want, ran, false); why != "Playing the 1080p picture already running." {
		t.Fatalf("a running silent picture: %q", why)
	}
	ran.Stream.SourceAudio = "AC3"
	if why := ranAs(true, want, ran, false); why != "Playing the 1080p picture already running." {
		t.Fatalf("a silent picture of AC-3 sound: %q", why)
	}
	same := live.Session{Rendition: want.Key(), Stream: live.StreamInfo{Video: "1080", Audio: "aac2"}}
	if why := ranAs(true, want, same, false); why != "" {
		t.Fatalf("as decided: %q", why)
	}
}
