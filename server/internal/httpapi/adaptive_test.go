package httpapi

import (
	"context"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
)

func TestAdaptiveSettingChoosesTheLadder(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	dev := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "River 4", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v4.1"},
	}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Hub: &live.Hub{Store: st, Encoder: "libx264"}}
	id := guideID(t, st, "4.1")
	web := &live.Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}
	off, _, err := s.decide(ctx, watchBody{ChannelID: id, Caps: web})
	if err != nil {
		t.Fatal(err)
	}
	if off.Rendition.ABR {
		t.Fatalf("off by default: %+v", off)
	}
	if err := st.PutSettings(ctx, map[string]string{"adaptive": "1"}); err != nil {
		t.Fatal(err)
	}
	on, _, err := s.decide(ctx, watchBody{ChannelID: id, Caps: web})
	if err != nil {
		t.Fatal(err)
	}
	if on.Rendition.Key() != "1080.aac2.broadcast.abr" {
		t.Fatalf("on: %+v %q", on.Rendition, on.Reason)
	}
	named, chosen, err := s.decide(ctx, watchBody{ChannelID: id, Caps: web, Rendition: "720.aac2.broadcast"})
	if err != nil || !chosen || named.Rendition.Key() != "720.aac2.broadcast" {
		t.Fatalf("a named rendition stays as named: chosen=%v %+v %v", chosen, named.Rendition, err)
	}
	if err := st.PutSettings(ctx, map[string]string{"adaptive": "0"}); err != nil {
		t.Fatal(err)
	}
	back, _, err := s.decide(ctx, watchBody{ChannelID: id, Caps: web})
	if err != nil || back.Rendition.ABR {
		t.Fatalf("adaptive 0: %+v %v", back.Rendition, err)
	}
	kept, chosen, err := s.decide(ctx, watchBody{ChannelID: id, Caps: web, Rendition: "1080.aac2.broadcast.abr"})
	if err != nil || chosen || kept.Rendition.ABR {
		t.Fatalf("a ladder key with adaptive 0: chosen=%v %+v %v", chosen, kept.Rendition, err)
	}
	tile, _, err := s.decide(ctx, watchBody{ChannelID: id, Caps: web, Prefs: live.Prefs{Quality: "tile"}})
	if err != nil || tile.Rendition.ABR {
		t.Fatalf("tile: %+v %v", tile.Rendition, err)
	}
}
