package store

import (
	"context"
	"path/filepath"
	"testing"

	"broadwave/internal/hdhr"
)

func TestLineupKeepsLocalEditsOnRefresh(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "10611B4C", FriendlyName: "DUO", ModelNumber: "HDHR5-2US", BaseURL: "http://192.168.1.252", LineupURL: "http://192.168.1.252/lineup.json", TunerCount: 2, FirmwareVersion: "20250815"}
	channels := []hdhr.Channel{
		{GuideNumber: "14.10", GuideName: "ZLiving", VideoCodec: "H264", AudioCodec: "AC3", StreamURL: "http://192.168.1.252:5004/auto/v14.10"},
		{GuideNumber: "4.1", GuideName: "WDAF-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, Favorite: true, StreamURL: "http://192.168.1.252:5004/auto/v4.1"},
		{GuideNumber: "14.2", GuideName: "Snapshp", VideoCodec: "H264", AudioCodec: "AC3"},
	}
	if err := s.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	got, err := s.Channels(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].GuideNumber != "4.1" || got[1].GuideNumber != "14.2" || got[2].GuideNumber != "14.10" {
		t.Fatalf("order %+v", numbers(got))
	}
	if !got[0].Favorite {
		t.Fatal("device favorite was not imported")
	}
	name := "Fox 4"
	num := "4.1"
	fav := false
	hidden := false
	if _, err := s.PatchChannel(ctx, got[0].ID, ChannelPatch{CustomName: &name, CustomNumber: &num, Favorite: &fav, Hidden: &hidden}); err != nil {
		t.Fatal(err)
	}
	channels[0].GuideName = "Z Living HD"
	channels[1].Favorite = true
	if err := s.UpsertDevice(ctx, dev, channels[:2]); err != nil {
		t.Fatal(err)
	}
	all, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	byNum := map[string]Channel{}
	for _, ch := range all {
		byNum[ch.GuideNumber] = ch
	}
	if byNum["4.1"].DisplayName != "Fox 4" || byNum["4.1"].Favorite {
		t.Fatalf("local edit lost: %+v", byNum["4.1"])
	}
	if byNum["14.10"].GuideName != "Z Living HD" || !byNum["14.10"].Present {
		t.Fatalf("refresh did not update name: %+v", byNum["14.10"])
	}
	if byNum["14.2"].Present {
		t.Fatal("missing channel still present")
	}
	guide, err := s.Channels(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(guide) != 2 {
		t.Fatalf("guide should hide the channel that left the lineup, got %+v", numbers(guide))
	}
	for _, ch := range guide {
		if ch.GuideNumber == "4.1" && ch.DisplayName != "Fox 4" {
			t.Fatalf("guide name %+v", ch)
		}
	}
}

func TestSettingsRejectUnknown(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.PutSettings(context.Background(), map[string]string{"password": "nope"}); err == nil {
		t.Fatal("password must not be a stored setting")
	}
	if err := s.PutSettings(context.Background(), map[string]string{"layout": "tv", "recordingsPath": `D:\DVR`}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got["layout"] != "tv" || got["recordingsPath"] != `D:\DVR` {
		t.Fatalf("%v", got)
	}
}

func numbers(channels []Channel) []string {
	out := make([]string, len(channels))
	for i, ch := range channels {
		out[i] = ch.GuideNumber
	}
	return out
}
