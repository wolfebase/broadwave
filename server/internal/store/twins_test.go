package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestCallKey(t *testing.T) {
	for name, want := range map[string]string{
		"KBWV-DT": "KBWV", "KBWV": "KBWV", "KRVT-1": "KRVT", "KRVT-2": "KRVT", "WTSTDT1": "WTST",
		"KLMN-HD": "KLMN", "KOPQ-TV": "KOPQ", "WGN-TV": "WGN", "KCTV": "KCTV", "ZLiving": "", "beIN": "", "MTRSPR1": "",
	} {
		if got := callKey(name); got != want {
			t.Errorf("callKey(%q) = %q, want %q", name, got, want)
		}
	}
}

func openTwins(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()
	duo := hdhr.Device{DeviceID: "DUO1", BaseURL: "http://duo", LineupURL: "http://duo/lineup.json", TunerCount: 2}
	if err := s.UpsertDevice(ctx, duo, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, Favorite: true, StreamURL: "http://duo:5004/auto/v4.1"},
	}); err != nil {
		t.Fatal(err)
	}
	flex := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := s.UpsertDevice(ctx, flex, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "5.1", GuideName: "WTSTDT1", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v5.1"},
		{GuideNumber: "19.1", GuideName: "KRVT-1", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v19.1"},
		{GuideNumber: "19.2", GuideName: "KRVT-2", VideoCodec: "MPEG2", AudioCodec: "AC3", StreamURL: "http://flex:5004/auto/v19.2"},
		{GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, StreamURL: "http://flex:5004/auto/v104.1"},
		{GuideNumber: "105.1", GuideName: "WTST", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, Protected: true, StreamURL: "http://flex:5004/auto/v105.1"},
		{GuideNumber: "119.1", GuideName: "KRVT", VideoCodec: "HEVC", AudioCodec: "AC-4", HD: true, StreamURL: "http://flex:5004/auto/v119.1"},
	}); err != nil {
		t.Fatal(err)
	}
	return s, ctx
}

func byNumber(t *testing.T, s *Store, ctx context.Context) map[string][]Channel {
	t.Helper()
	chs, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]Channel{}
	for _, ch := range chs {
		out[ch.GuideNumber] = append(out[ch.GuideNumber], ch)
	}
	return out
}

func TestTwinsPairEachClearStationWithItsMainChannel(t *testing.T) {
	s, ctx := openTwins(t)
	chs, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	num := map[int64]string{}
	dev := map[int64]string{}
	for _, ch := range chs {
		num[ch.ID], dev[ch.ID] = ch.GuideNumber, ch.DeviceID
	}
	twins := Twins(chs)
	got := map[string][]string{}
	for c3, ones := range twins {
		for _, id := range ones {
			got[num[c3]] = append(got[num[c3]], num[id]+"@"+dev[id])
		}
	}
	if len(got) != 2 {
		t.Fatalf("pairs %v, want 104.1 and 119.1 only (105.1 is encrypted)", got)
	}
	if g := got["104.1"]; len(g) != 2 || g[0] != "4.1@FLEX1" || g[1] != "4.1@DUO1" {
		t.Fatalf("104.1 pairs with %v, want the same device's 4.1 first", g)
	}
	if g := got["119.1"]; len(g) != 1 || g[0] != "19.1@FLEX1" {
		t.Fatalf("119.1 pairs with %v, want 19.1 (not the 19.2 subchannel)", g)
	}
}

func TestEncryptedChannelIsStoredAndHidden(t *testing.T) {
	s, ctx := openTwins(t)
	ch := byNumber(t, s, ctx)["105.1"][0]
	if !ch.Protected || !ch.Hidden || ch.Standard != TwinATSC3 || ch.TwinID != 0 {
		t.Fatalf("105.1 %+v, want protected, hidden, atsc3, unpaired", ch)
	}
	if c := byNumber(t, s, ctx)["104.1"][0]; c.Protected {
		t.Fatal("a clear 3.0 channel is marked protected")
	}
}

func TestTwinDefaultShowsThe3Point0ChannelOnce(t *testing.T) {
	s, ctx := openTwins(t)
	got := byNumber(t, s, ctx)
	c3 := got["104.1"][0]
	if c3.Hidden || c3.TwinChoice != TwinATSC3 || c3.TwinID == 0 || !c3.Favorite {
		t.Fatalf("104.1 %+v, want shown, choice atsc3, paired, and the favorite carried over", c3)
	}
	for _, c1 := range got["4.1"] {
		if !c1.Hidden || c1.TwinID != c3.ID || c1.TwinChoice != TwinATSC3 {
			t.Fatalf("4.1 on %s %+v, want hidden and paired with 104.1", c1.DeviceID, c1)
		}
	}
	if got["19.2"][0].Hidden || got["19.2"][0].TwinID != 0 {
		t.Fatal("a subchannel was paired or hidden")
	}
	// The user's choice stays through a lineup refresh.
	if err := s.SetTwinChoice(ctx, got["4.1"][0].ID, TwinATSC1); err != nil {
		t.Fatal(err)
	}
	flex := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := s.UpsertDevice(ctx, flex, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v4.1"},
		{GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4", HD: true, StreamURL: "http://flex:5004/auto/v104.1"},
	}); err != nil {
		t.Fatal(err)
	}
	got = byNumber(t, s, ctx)
	if !got["104.1"][0].Hidden || got["104.1"][0].TwinChoice != TwinATSC1 || got["4.1"][0].Hidden || !got["4.1"][0].Favorite {
		t.Fatalf("after choosing 1.0 and a refresh: 104.1 %+v, 4.1 %+v", got["104.1"][0], got["4.1"][0])
	}
	if err := s.SetTwinChoice(ctx, got["104.1"][0].ID, TwinBoth); err != nil {
		t.Fatal(err)
	}
	got = byNumber(t, s, ctx)
	if got["104.1"][0].Hidden || got["4.1"][0].Hidden || got["4.1"][1].Hidden {
		t.Fatal("both shows a hidden channel")
	}
	if err := s.SetTwinChoice(ctx, got["5.1"][0].ID, TwinBoth); err != ErrNoTwin {
		t.Fatalf("5.1 has no twin, got %v", err)
	}
	if err := s.SetTwinChoice(ctx, got["104.1"][0].ID, "4k"); err == nil {
		t.Fatal("an unknown choice was taken")
	}
}

func TestTwinDefaultKeepsATallerPicture(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := s.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "38.1", GuideName: "KOPQ-TV", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://flex:5004/auto/v38.1"},
	}); err != nil {
		t.Fatal(err)
	}
	c1 := byNumber(t, s, ctx)["38.1"][0]
	if err := s.SetChannelPictureHeight(ctx, c1.ID, 1080); err != nil {
		t.Fatal(err)
	}
	// The 3.0 channel arrives later, already tuned once at 720.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO channels (device_id, guide_number, guide_name, stream_url, video_codec, audio_codec, hd, present, picture_height)
VALUES ('FLEX1', '138.1', 'KOPQ', 'http://flex:5004/auto/v138.1', 'HEVC', 'AC4', 1, 1, 720)`); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyTwinDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	got := byNumber(t, s, ctx)
	if !got["138.1"][0].Hidden || got["38.1"][0].Hidden || got["138.1"][0].TwinChoice != TwinATSC1 {
		t.Fatalf("138.1 %+v 38.1 %+v, want the 1080 1.0 channel shown", got["138.1"][0], got["38.1"][0])
	}
}

func TestASimulcastRecordsOnTheShownChannel(t *testing.T) {
	s, ctx := openTwins(t)
	got := byNumber(t, s, ctx)
	c3, c1 := got["104.1"][0].ID, got["4.1"][0].ID
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	if err := s.InsertAirings(ctx, []Airing{
		{ChannelID: c1, Title: "Football", Start: start, End: start.Add(3 * time.Hour)},
		{ChannelID: c3, Title: "Football", Start: start, End: start.Add(3 * time.Hour)},
		{ChannelID: c3, Title: "Only on 3.0", Start: start.Add(3 * time.Hour), End: start.Add(4 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	marks := func() map[string]int64 {
		rows, err := s.RecordingAirings(ctx, start.Add(-time.Hour), start.Add(5*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]int64{}
		for _, r := range rows {
			out[r.Title+"@"+map[int64]string{c1: "1", c3: "3"}[r.ChannelID]] = r.Simulcast
		}
		return out
	}
	m := marks()
	if m["Football@3"] != 0 || m["Football@1"] != c3 || m["Only on 3.0@3"] != 0 {
		t.Fatalf("3.0 shown: %v, want the 1.0 copy marked for 104.1", m)
	}
	if err := s.SetTwinChoice(ctx, c3, TwinBoth); err != nil {
		t.Fatal(err)
	}
	m = marks()
	if m["Football@1"] != 0 || m["Football@3"] != c1 {
		t.Fatalf("both shown: %v, want the 3.0 copy marked for 4.1", m)
	}
}

func TestATitlePassMarksTheSimulcastCopy(t *testing.T) {
	s, ctx := openTwins(t)
	got := byNumber(t, s, ctx)
	c3, c1 := got["104.1"][0].ID, got["4.1"][0].ID
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	if err := s.InsertAirings(ctx, []Airing{
		{ChannelID: c1, Title: "Football", Start: start, End: start.Add(3 * time.Hour)},
		{ChannelID: c3, Title: "Football", Start: start, End: start.Add(3 * time.Hour)},
		{ChannelID: c3, Title: "Only on 3.0", Start: start.Add(3 * time.Hour), End: start.Add(4 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	from, to := start.Add(-time.Hour), start.Add(5*time.Hour)
	wide, err := s.RecordingAirings(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := s.RecordingAiringsFor(ctx, from, to, []Pass{{Title: "Football", Kind: "series", MatchKind: "title"}})
	if err != nil {
		t.Fatal(err)
	}
	mark := map[int64]int64{}
	for _, row := range wide {
		if row.Title == "Football" {
			mark[row.ChannelID] = row.Simulcast
		}
	}
	if len(narrow) != 2 || len(mark) != 2 {
		t.Fatalf("narrow %d wide football %d", len(narrow), len(mark))
	}
	for _, row := range narrow {
		if row.Title != "Football" || row.Simulcast != mark[row.ChannelID] {
			t.Fatalf("narrow %+v, wide %v", row, mark)
		}
	}
	if mark[c1] == 0 && mark[c3] == 0 {
		t.Fatal("neither copy was marked")
	}
}

// The lineup names an encrypted 3.0 channel by its bare call sign and its 1.0
// twin in each of the forms tuners use.
func TestEachEncryptedChannelPlaysItsClearTwin(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	flex := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	lineup := []hdhr.Channel{
		{GuideNumber: "5.1", GuideName: "KQRSDT1"},
		{GuideNumber: "5.2", GuideName: "KQRS365"},
		{GuideNumber: "9.1", GuideName: "KLMN-HD"},
		{GuideNumber: "29.1", GuideName: "KWXY-HD"},
		{GuideNumber: "41.1", GuideName: "KOPQ-TV"},
		{GuideNumber: "62.1", GuideName: "KTUVDT1"},
		{GuideNumber: "105.1", GuideName: "KQRS", Protected: true},
		{GuideNumber: "109.1", GuideName: "KLMN", Protected: true},
		{GuideNumber: "129.1", GuideName: "KWXY", Protected: true},
		{GuideNumber: "141.1", GuideName: "KOPQ", Protected: true},
		{GuideNumber: "162.1", GuideName: "KTUV", Protected: true},
		{GuideNumber: "170.1", GuideName: "KZZZ", Protected: true},
	}
	for i := range lineup {
		lineup[i].VideoCodec, lineup[i].AudioCodec = "MPEG2", "AC3"
		if lineup[i].Protected {
			lineup[i].VideoCodec, lineup[i].AudioCodec = "HEVC", "AC4"
		}
		lineup[i].StreamURL = "http://flex:5004/auto/v" + lineup[i].GuideNumber
	}
	if err := s.UpsertDevice(ctx, flex, lineup); err != nil {
		t.Fatal(err)
	}
	got := byNumber(t, s, ctx)
	for c3, c1 := range map[string]string{"105.1": "5.1", "109.1": "9.1", "129.1": "29.1", "141.1": "41.1", "162.1": "62.1"} {
		ch := got[c3][0]
		if ch.PlaysAs != got[c1][0].ID || !ch.Hidden || ch.TwinID != 0 {
			t.Errorf("%s %+v, want hidden, unpaired, playing as %s", c3, ch, c1)
			continue
		}
		src, err := s.SourceChannel(ctx, ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		if src.ID != ch.ID || src.PlaysAs != got[c1][0].ID || src.StreamURL != "http://flex:5004/auto/v"+c1 || src.Protected || src.VideoCodec != "MPEG2" {
			t.Errorf("%s streams %+v, want %s's stream under its own id", c3, src, c1)
		}
	}
	if ch := got["170.1"][0]; ch.PlaysAs != 0 {
		t.Errorf("170.1 has no 1.0 station but plays as %d", ch.PlaysAs)
	}
	if src, err := s.SourceChannel(ctx, got["170.1"][0].ID); err != nil || src.StreamURL != "http://flex:5004/auto/v170.1" || src.PlaysAs != 0 {
		t.Errorf("170.1 streams %+v (%v), want its own stream", src, err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	enc, clear := got["105.1"][0].ID, got["5.1"][0].ID
	if err := s.InsertAirings(ctx, []Airing{
		{ChannelID: clear, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ChannelID: enc, Title: "News", Start: start, End: start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.RecordingAirings(ctx, start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if want := map[int64]int64{clear: 0, enc: clear}[r.ChannelID]; r.Simulcast != want {
			t.Errorf("airing on %d marked for %d, want %d", r.ChannelID, r.Simulcast, want)
		}
	}
}

func TestRemovingADeadTunerMovesItsFavoritesAndPasses(t *testing.T) {
	s, ctx := openTwins(t)
	duo := hdhr.Device{DeviceID: "DUO1", BaseURL: "http://duo", LineupURL: "http://duo/lineup.json", TunerCount: 2}
	if err := s.UpsertDevice(ctx, duo, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://duo:5004/auto/v4.1"},
		{GuideNumber: "9.1", GuideName: "KLMN-DT", VideoCodec: "MPEG2", AudioCodec: "AC3", HD: true, StreamURL: "http://duo:5004/auto/v9.1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE channels SET favorite=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE channels SET favorite=1, custom_name='Fox 4' WHERE device_id='DUO1' AND guide_number='4.1'`); err != nil {
		t.Fatal(err)
	}
	ch := byNumber(t, s, ctx)
	var old4, old9 int64
	for _, c := range append(ch["4.1"], ch["9.1"]...) {
		if c.DeviceID == "DUO1" && c.GuideNumber == "4.1" {
			old4 = c.ID
		}
		if c.GuideNumber == "9.1" {
			old9 = c.ID
		}
	}
	if old4 == 0 || old9 == 0 {
		t.Fatalf("missing DUO rows: %+v", ch)
	}
	if err := s.AddPass(ctx, "News", old4, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPass(ctx, "Late", old9, 0, 0); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveDevice(ctx, "DUO1"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveDevice(ctx, "DUO1"); err != ErrNoDevice {
		t.Fatalf("second remove = %v", err)
	}
	devices, err := s.Devices(ctx)
	if err != nil || len(devices) != 1 || devices[0].DeviceID != "FLEX1" {
		t.Fatalf("devices %+v %v", devices, err)
	}
	ch = byNumber(t, s, ctx)
	if len(ch["4.1"]) != 1 || len(ch["9.1"]) != 0 {
		t.Fatalf("lineup after remove: %+v", ch)
	}
	flex4 := ch["4.1"][0]
	if flex4.DeviceID != "FLEX1" || !flex4.Favorite || flex4.DisplayName != "Fox 4" {
		t.Fatalf("FLEX 4.1 = %+v", flex4)
	}
	// 4.1 sits behind its 3.0 twin, so the guide's half carries the favorite too.
	if wide := ch["104.1"][0]; !flex4.Hidden || !wide.Favorite {
		t.Fatalf("4.1 hidden %v, 104.1 = %+v", flex4.Hidden, wide)
	}
	passes, err := s.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range passes {
		switch p.Title {
		case "News":
			if p.ChannelID != flex4.ID {
				t.Fatalf("News pass on %d, want %d", p.ChannelID, flex4.ID)
			}
		case "Late":
			t.Fatalf("the pass for 9.1, carried by no other tuner, stayed on %d", p.ChannelID)
		}
	}
	// A device added later reuses the freed ids. Nothing may point at them.
	if err := s.UpsertDevice(ctx, hdhr.Device{DeviceID: "src-9", BaseURL: "source"}, []hdhr.Channel{
		{GuideNumber: "901", GuideName: "New", StreamURL: "http://example/901.ts"},
		{GuideNumber: "902", GuideName: "New 2", StreamURL: "http://example/902.ts"},
	}); err != nil {
		t.Fatal(err)
	}
	passes, err = s.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range passes {
		if p.ChannelID == old4 || p.ChannelID == old9 {
			t.Fatalf("pass %q points at a freed id %d", p.Title, p.ChannelID)
		}
	}
}

func TestARefreshDoesNotBringBackARemovedTuner(t *testing.T) {
	s, ctx := openTwins(t)
	if err := s.RemoveDevice(ctx, "DUO1"); err != nil {
		t.Fatal(err)
	}
	duo := hdhr.Device{DeviceID: "DUO1", BaseURL: "http://duo", LineupURL: "http://duo/lineup.json", TunerCount: 2}
	lineup := []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV-DT", StreamURL: "http://duo:5004/auto/v4.1"}}
	if err := s.RefreshDevice(ctx, duo, lineup); err != nil {
		t.Fatal(err)
	}
	if got := byNumber(t, s, ctx)["4.1"]; len(got) != 1 || got[0].DeviceID != "FLEX1" {
		t.Fatalf("4.1 rows %+v", got)
	}
	// A search the viewer starts adds it again.
	if err := s.UpsertDevice(ctx, duo, lineup); err != nil {
		t.Fatal(err)
	}
	if got := byNumber(t, s, ctx)["4.1"]; len(got) != 2 {
		t.Fatalf("4.1 rows after a search %+v", got)
	}
}
