package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

// rankGuide is a fictional lineup: KBWV on 4.1 with its ATSC 3.0 twin on
// 104.1 (shown in its place, the default), KRVT on 41.1 (whose news is titled "KRVT 41 News"), a few subchannels,
// and a shopping channel.
func rankGuide(t *testing.T) (*Store, map[string]Channel, time.Time) {
	t.Helper()
	s := openTestStore(t)
	ctx := context.Background()
	flex := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := s.UpsertDevice(ctx, flex, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "4.2", GuideName: "Rewind", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "4.10", GuideName: "Cedar", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "16.2", GuideName: "Shopline", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "41.1", GuideName: "KRVT-TV", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "41.2", GuideName: "Prairie", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4"},
	}); err != nil {
		t.Fatal(err)
	}
	chs := map[string]Channel{}
	for n, list := range byNumber(t, s, ctx) {
		chs[n] = list[0]
	}
	if chs["104.1"].TwinID != chs["4.1"].ID {
		t.Fatalf("104.1 twin %d, want 4.1 (%d)", chs["104.1"].TwinID, chs["4.1"].ID)
	}
	now := time.Date(2026, 10, 9, 18, 10, 0, 0, time.UTC)
	at := func(h float64) time.Time { return now.Add(time.Duration(h * float64(time.Hour))) }
	slot := func(ch string, title, sub, desc, cat string, start float64) Airing {
		return Airing{ChannelID: chs[ch].ID, Title: title, Subtitle: sub, Description: desc, Category: cat, Start: at(start), End: at(start + 0.5)}
	}
	rows := []Airing{
		// What the 4.1 pair shows now and next; 104.1 lists the same.
		slot("4.1", "Harbor Tonight", "", "", "News", -0.1),
		slot("4.1", "Wheel Spin", "", "", "Game show", 0.4),
		slot("104.1", "Harbor Tonight", "", "", "News", -0.1),
		slot("104.1", "Wheel Spin", "", "", "Game show", 0.4),
		slot("4.10", "Cedar Gardens", "", "", "Home", -0.1),
		slot("41.1", "KRVT 41 News at 6", "", "", "News", -0.1),
		slot("41.1", "KRVT 41 News at 10", "", "", "News", 4),
		slot("4.2", "Evening News", "", "", "News", 1),
		// Shopping and paid programming.
		slot("16.2", "3 Blenders in 1! NEW Mega Mixer!", "", "Fresh juice for the whole family.", "Shop, Consumer, How-to", 0.2),
		slot("16.2", "Paid Programming", "Morning News Special", "", "", 0.7),
		slot("16.2", "Shopping News Hour", "", "Deals of the day.", "Shopping", 1.5),
		// Description-only mentions of a team.
		slot("41.2", "Lone Rider", "", "The rangers ride into town.", "Western", 0),
		slot("41.2", "Badge City", "", "Two rangers chase a lead.", "Crime drama", 1),
		// Whole word beats a word start.
		slot("4.2", "Arthur", "", "", "Children", 0.2),
		slot("4.2", "Art Attack", "", "", "Children", 2),
		// Accents, people, categories.
		slot("41.2", "Pokémon Quest", "", "", "Animated", 2),
		slot("41.2", "Pocket Monsters", "", "A pokemon tale.", "Animated", 0.2),
		slot("4.2", "Hidden Valleys", "", "", "Documentary", 3),
		{ChannelID: chs["4.2"].ID, Title: "Dinner Party", Cast: "Mara Quill", Start: at(5), End: at(6)},
	}
	if err := s.ReplaceAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	return s, chs, now
}

func hitList(hits []AiringHit) []string {
	out := make([]string, len(hits))
	for i, hit := range hits {
		out[i] = hit.GuideNumber + " " + hit.Title
	}
	return out
}

func TestSearchFindsAChannelByNumber(t *testing.T) {
	s, chs, now := rankGuide(t)
	ctx := context.Background()
	// The pair shows only 104.1 by default: 4.1 finds its twin.
	hidden := []string{"104.1 Harbor Tonight", "104.1 Wheel Spin", "4.10 Cedar Gardens"}
	both := []string{"4.1 Harbor Tonight", "4.1 Wheel Spin", "104.1 Harbor Tonight", "104.1 Wheel Spin", "4.10 Cedar Gardens"}
	for _, q := range []string{"4.1", "4-1"} {
		hits, recs, err := s.Search(ctx, q, now, 60)
		if err != nil {
			t.Fatal(err)
		}
		if got := hitList(hits); !slices.Equal(got, hidden) || len(recs) != 0 {
			t.Fatalf("%q: %v, want %v (never 41.1)", q, got, hidden)
		}
	}
	// Both shown: the channel, its 3.0 twin, then 4.10.
	if err := s.SetTwinChoice(ctx, chs["104.1"].ID, TwinBoth); err != nil {
		t.Fatal(err)
	}
	hits, _, err := s.Search(ctx, "4.1", now, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got := hitList(hits); !slices.Equal(got, both) {
		t.Fatalf("both shown: %v, want %v", got, both)
	}
	hits, _, err = s.Search(ctx, "104.1", now, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got := hitList(hits); !slices.Equal(got, []string{"104.1 Harbor Tonight", "104.1 Wheel Spin", "4.1 Harbor Tonight", "4.1 Wheel Spin"}) {
		t.Fatalf("104.1: %v", got)
	}
	// "41" is channel 41's subchannels first, then the show titled 41.
	hits, _, err = s.Search(ctx, "41", now, 60)
	if err != nil {
		t.Fatal(err)
	}
	got := hitList(hits)
	if len(got) < 3 || got[0] != "41.1 KRVT 41 News at 6" || got[1] != "41.1 KRVT 41 News at 10" || got[2] != "41.2 Lone Rider" {
		t.Fatalf("41: %v", got)
	}
	// A number no channel has finds nothing, not the word.
	if hits, _, err = s.Search(ctx, "7.3", now, 60); err != nil || len(hits) != 0 {
		t.Fatalf("7.3: %v %v", hitList(hits), err)
	}
}

func TestSearchRanksTitlesAndLeavesOutMentions(t *testing.T) {
	s, _, now := rankGuide(t)
	ctx := context.Background()
	for _, c := range []struct {
		q    string
		want []string
	}{
		// "NEW Mega Mixer" matches news only by stem, and paid programming
		// named in an episode title sinks; a shopping show titled News stays.
		// Shows titled News come before ones only filed under News.
		{"news", []string{"41.1 KRVT 41 News at 6", "4.2 Evening News", "16.2 Shopping News Hour", "41.1 KRVT 41 News at 10", "104.1 Harbor Tonight"}},
		{"art", []string{"4.2 Art Attack", "4.2 Arthur"}},
		{"pokemon", []string{"41.2 Pokémon Quest"}},
		{"Pokémon", []string{"41.2 Pokémon Quest"}},
		{"documentary", []string{"4.2 Hidden Valleys"}},
		{"quill", []string{"4.2 Dinner Party"}},
		{"kbwv", []string{"104.1 Harbor Tonight", "104.1 Wheel Spin"}},
	} {
		hits, _, err := s.Search(ctx, c.q, now, 60)
		if err != nil {
			t.Fatal(err)
		}
		if got := hitList(hits); !slices.Equal(got, c.want) {
			t.Errorf("%q: %v, want %v", c.q, got, c.want)
		}
	}
}

// A team that plays later: its game leads and the dramas that mention the
// word are gone. With no game, the mentions are all there is.
func TestSearchTeamBeatsMentions(t *testing.T) {
	s, chs, now := rankGuide(t)
	ctx := context.Background()
	hits, _, err := s.Search(ctx, "rangers", now, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got := hitList(hits); !slices.Equal(got, []string{"41.2 Lone Rider", "41.2 Badge City"}) {
		t.Fatalf("no game: %v", got)
	}
	game := now.Add(26 * time.Hour)
	if err := s.InsertAirings(ctx, []Airing{{ChannelID: chs["41.1"].ID, Title: "College Hockey", Subtitle: "Rangers at Pilots", GameID: "g1", Category: "Sports", Start: game, End: game.Add(3 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	hits, _, err = s.Search(ctx, "rangers", now, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got := hitList(hits); !slices.Equal(got, []string{"41.1 College Hockey"}) {
		t.Fatalf("with the game: %v", got)
	}
}

func TestRankListing(t *testing.T) {
	for _, c := range []struct {
		q, title, sub, cast, cat string
		want                     int
	}{
		{"news", "News", "", "", "", tierTitleExact},
		{"news", "Evening News", "", "", "", tierTitle},
		{"new", "Newsroom", "", "", "", tierTitlePart},
		{"news", "Paid Programming", "News", "", "", tierPaid},
		{"news", "Shopping News", "", "", "Shopping", tierTitle},
		{"news", "NEW Mixer", "", "", "Shop", tierPaid},
		{"news", "NEW Mixer", "", "", "", tierMention},
		{"jeop show", "Jeopardy!", "Show 9001", "", "", tierEpisode},
		{"ken", "Jeopardy!", "", "Ken Jennings", "", tierPeople},
		{"senora", "Telenovela: Señora", "", "", "", tierTitle},
		{"sports", "Big Game", "", "", "Sports, Football", tierCategory},
	} {
		words := queryWords(c.q)
		phrase := fmt.Sprint(foldWords(c.q))
		phrase = phrase[1 : len(phrase)-1]
		if got := rankListing(words, phrase, c.title, c.sub, c.cast, c.cat); got != c.want {
			t.Errorf("%q in %q/%q/%q/%q: tier %d, want %d", c.q, c.title, c.sub, c.cast, c.cat, got, c.want)
		}
	}
}

func TestChannelNumber(t *testing.T) {
	for q, want := range map[string][2]any{
		"4.1": {"4.1", true}, "4-1": {"4.1", true}, "41": {"41", false}, "104.1": {"104.1", true},
		"4.": {"", false}, "a4.1": {"", false}, "4.1.2": {"", false}, "news": {"", false}, "": {"", false},
	} {
		n, dotted := channelNumber(q)
		if n != want[0] || dotted != want[1] {
			t.Errorf("channelNumber(%q) = %q %v, want %v", q, n, dotted, want)
		}
	}
}

// BenchmarkSearchRanked is a realistic guide: 70 channels, two days of
// half-hour listings, news on every channel every few hours, shopping on a few.
func BenchmarkSearchRanked(b *testing.B) {
	st, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	ctx := context.Background()
	lineup := make([]hdhr.Channel, 70)
	for i := range lineup {
		lineup[i] = hdhr.Channel{GuideNumber: fmt.Sprintf("%d.%d", i/4+2, i%4+1), GuideName: fmt.Sprintf("Station %d", i)}
	}
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "Duo", BaseURL: "http://127.0.0.1", TunerCount: 4}, lineup); err != nil {
		b.Fatal(err)
	}
	all, err := st.Channels(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	shows := []struct{ title, desc, cat string }{
		{"Evening News", "Local news, weather, and sports.", "News"},
		{"Harbor Drama", "A detective and the rangers who help her.", "Crime drama"},
		{"Kitchen Hour", "New recipes and old favorites.", "Cooking"},
		{"3 Blenders in 1! NEW Mixer!", "Order now.", "Shop, Consumer"},
		{"Nature Walk", "Rangers patrol the park.", "Documentary"},
		{"Game Night", "Teams compete for a prize.", "Game show"},
	}
	var rows []Airing
	for c, ch := range all {
		for slot := range 2 * 48 {
			at := start.Add(time.Duration(slot) * 30 * time.Minute)
			show := shows[(c+slot)%len(shows)]
			rows = append(rows, Airing{ChannelID: ch.ID, Title: show.title, Description: show.desc, Category: show.cat, Start: at, End: at.Add(30 * time.Minute)})
		}
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		b.Fatal(err)
	}
	from := start.Add(6 * time.Hour)
	for _, q := range []string{"news", "rangers", "4.1", "kitchen hour", "zzz"} {
		b.Run(q, func(b *testing.B) {
			for b.Loop() {
				if _, _, err := st.Search(ctx, q, from, 60); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
