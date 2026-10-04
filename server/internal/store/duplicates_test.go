package store

import (
	"context"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func openTwoTuners(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s := openTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"AAA1", "BBB2"} {
		dev := hdhr.Device{DeviceID: id, BaseURL: "http://" + id, LineupURL: "http://" + id + "/lineup.json", TunerCount: 2}
		if err := s.UpsertDevice(ctx, dev, []hdhr.Channel{
			{GuideNumber: "5.1", GuideName: "WTSTDT1", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://" + id + ":5004/auto/v5.1"},
			{GuideNumber: "5.2", GuideName: "Rivers", VideoCodec: "MPEG2", AudioCodec: "AC3", StreamURL: "http://" + id + ":5004/auto/v5.2"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A third tuner on another antenna has its own 5.2.
	other := hdhr.Device{DeviceID: "CCC3", BaseURL: "http://CCC3", LineupURL: "http://CCC3/lineup.json", TunerCount: 2}
	if err := s.UpsertDevice(ctx, other, []hdhr.Channel{
		{GuideNumber: "5.2", GuideName: "Mountains", VideoCodec: "MPEG2", AudioCodec: "AC3", StreamURL: "http://CCC3:5004/auto/v5.2"},
	}); err != nil {
		t.Fatal(err)
	}
	return s, ctx
}

func rowFor(t *testing.T, chs []Channel, device, number string) Channel {
	t.Helper()
	for _, ch := range chs {
		if ch.DeviceID == device && ch.GuideNumber == number {
			return ch
		}
	}
	t.Fatalf("no %s on %s", number, device)
	return Channel{}
}

func guideNumbers(t *testing.T, s *Store, ctx context.Context) map[string][]Channel {
	t.Helper()
	chs, err := s.Channels(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]Channel{}
	for _, ch := range chs {
		out[ch.GuideNumber] = append(out[ch.GuideNumber], ch)
	}
	return out
}

func TestTwoTunersOnOneAntennaListEachChannelOnce(t *testing.T) {
	s, ctx := openTwoTuners(t)
	all, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	a51, b51 := rowFor(t, all, "AAA1", "5.1"), rowFor(t, all, "BBB2", "5.1")

	guide := guideNumbers(t, s, ctx)
	if len(guide["5.1"]) != 1 || guide["5.1"][0].ID != a51.ID {
		t.Fatalf("5.1 on the guide = %+v, want only AAA1's row", guide["5.1"])
	}
	// Same number, another station: both stay.
	if len(guide["5.2"]) != 2 {
		t.Fatalf("5.2 on the guide = %d rows, want Rivers and Mountains", len(guide["5.2"]))
	}
	if b := rowFor(t, all, "BBB2", "5.1"); b.SameAs != a51.ID {
		t.Fatalf("BBB2 5.1 sameAs = %d, want %d", b.SameAs, a51.ID)
	}
	// The hidden row still resolves, so a watch or a pass on it still plays.
	if got, err := s.Channel(ctx, b51.ID); err != nil || got.ID != b51.ID {
		t.Fatalf("Channel(%d) = %+v, %v", b51.ID, got, err)
	}

	// The row with listings wins a tie.
	now := time.Now().UTC()
	if err := s.InsertAirings(ctx, []Airing{{ChannelID: b51.ID, Title: "News", Start: now.Add(-time.Hour), End: now.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if got := guideNumbers(t, s, ctx)["5.1"]; len(got) != 1 || got[0].ID != b51.ID {
		t.Fatalf("with listings on BBB2, 5.1 = %+v", got)
	}
	// The tuner tried first wins over listings.
	if _, err := s.db.Exec(`UPDATE devices SET priority = 1 WHERE device_id = 'BBB2'`); err != nil {
		t.Fatal(err)
	}
	if got := guideNumbers(t, s, ctx)["5.1"]; len(got) != 1 || got[0].ID != a51.ID {
		t.Fatalf("with BBB2 at priority 1, 5.1 = %+v", got)
	}
	// A tuner that stopped answering loses to one that answers, whatever its priority.
	if _, err := s.db.Exec(`UPDATE devices SET last_seen = ? WHERE device_id = 'AAA1'`, now.Add(-2*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if got := guideNumbers(t, s, ctx)["5.1"]; len(got) != 1 || got[0].ID != b51.ID {
		t.Fatalf("with AAA1 gone for 2 h, 5.1 = %+v", got)
	}
}

func TestAChangeToADuplicatedChannelReachesEveryTuner(t *testing.T) {
	s, ctx := openTwoTuners(t)
	all, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	a51, b51 := rowFor(t, all, "AAA1", "5.1"), rowFor(t, all, "BBB2", "5.1")
	yes, no := true, false

	if _, err := s.PatchChannel(ctx, a51.ID, ChannelPatch{Hidden: &yes}); err != nil {
		t.Fatal(err)
	}
	if got := guideNumbers(t, s, ctx)["5.1"]; len(got) != 0 {
		t.Fatalf("hidden 5.1 still on the guide: %+v", got)
	}
	if _, err := s.PatchChannel(ctx, a51.ID, ChannelPatch{Hidden: &no}); err != nil {
		t.Fatal(err)
	}

	name := "Channel Five"
	if _, err := s.PatchChannel(ctx, b51.ID, ChannelPatch{Favorite: &yes, CustomName: &name}); err != nil {
		t.Fatal(err)
	}
	got := guideNumbers(t, s, ctx)["5.1"]
	if len(got) != 1 || !got[0].Favorite || got[0].DisplayName != name {
		t.Fatalf("5.1 after a favorite and a name on the other row = %+v", got)
	}
	if _, err := s.PatchChannel(ctx, got[0].ID, ChannelPatch{Favorite: &no}); err != nil {
		t.Fatal(err)
	}
	if got := guideNumbers(t, s, ctx)["5.1"]; got[0].Favorite {
		t.Fatal("5.1 is still a favorite after it was turned off")
	}
	// Mountains is another station and keeps its own flags.
	mountains := rowFor(t, all, "CCC3", "5.2")
	if _, err := s.PatchChannel(ctx, rowFor(t, all, "AAA1", "5.2").ID, ChannelPatch{Hidden: &yes}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Channel(ctx, mountains.ID); got.Hidden {
		t.Fatal("hiding Rivers hid Mountains")
	}
}

func TestAViewerHiddenDuplicateLeavesTheOtherOnTheGuide(t *testing.T) {
	s, ctx := openTwoTuners(t)
	all, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	a51, b51 := rowFor(t, all, "AAA1", "5.1"), rowFor(t, all, "BBB2", "5.1")
	// Stored before rows were grouped: the viewer hid one tuner's copy.
	if _, err := s.db.Exec(`UPDATE channels SET hidden = 1 WHERE id = ?`, a51.ID); err != nil {
		t.Fatal(err)
	}
	if got := guideNumbers(t, s, ctx)["5.1"]; len(got) != 1 || got[0].ID != b51.ID {
		t.Fatalf("5.1 = %+v, want BBB2's row", got)
	}
}
