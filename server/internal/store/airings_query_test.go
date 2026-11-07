package store

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestQueryAiringsKeepsTheWindowAndNarrowsByTeam(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	rows := []Airing{
		{ChannelID: 1, Title: "Wolves at Harbor", Subtitle: "City league", Category: "Sports", Start: from, End: from.Add(3 * time.Hour)},
		{ChannelID: 2, Title: "Evening News", Description: "The Wolves won last night", Start: from.Add(time.Hour), End: from.Add(2 * time.Hour)},
		{ChannelID: 1, Title: "Wolves preview", Start: from.Add(-48 * time.Hour), End: from.Add(-47 * time.Hour)},
		{ChannelID: 3, Title: "Quiz Night", Subtitle: "Home of the Harbor", Start: from.Add(4 * time.Hour), End: from.Add(5 * time.Hour)},
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	window, err := st.Airings(ctx, from.Add(-time.Hour), from.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(window); strings.Join(got, ",") != "Wolves at Harbor,Evening News,Quiz Night" {
		t.Fatalf("window %v", got)
	}
	teams, err := st.QueryAirings(ctx, AiringQuery{
		From: from.Add(-time.Hour), To: from.Add(6 * time.Hour), Teams: []string{"Wolves", "Harbor"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(teams); strings.Join(got, ",") != "Wolves at Harbor,Quiz Night" {
		t.Fatalf("teams %v", got)
	}
	none, err := st.QueryAirings(ctx, AiringQuery{
		From: from.Add(-time.Hour), To: from.Add(6 * time.Hour), Teams: []string{"Q"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("short name returned %v", titlesOf(none))
	}
	one, err := st.QueryAirings(ctx, AiringQuery{
		From: from.Add(-time.Hour), To: from.Add(6 * time.Hour), Channels: []int64{2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(one); strings.Join(got, ",") != "Evening News" {
		t.Fatalf("channel %v", got)
	}
	// Punctuation is not query syntax, and a name with no word must not error.
	dashes, err := st.QueryAirings(ctx, AiringQuery{
		From: from.Add(-time.Hour), To: from.Add(6 * time.Hour), Teams: []string{"---"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dashes) != 0 {
		t.Fatalf("dashes %v", titlesOf(dashes))
	}
	// A quote is punctuation, not query syntax. The name still matches.
	quoted, err := st.QueryAirings(ctx, AiringQuery{
		From: from.Add(-time.Hour), To: from.Add(6 * time.Hour), Teams: []string{`Wolves"`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(quoted); strings.Join(got, ",") != "Wolves at Harbor" {
		t.Fatalf("quoted name %v", got)
	}
}

func TestAiringSelectUsesTheIndex(t *testing.T) {
	st := openTestStore(t)
	from := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	q := AiringQuery{From: from, To: from.Add(24 * time.Hour), Teams: []string{"Wolves"}}
	sqlText, args, ok := airingSelect(q)
	if !ok {
		t.Fatal("expected a query")
	}
	if plan := queryPlan(t, st, sqlText, args...); !strings.Contains(plan, "airing_search") {
		t.Fatalf("team plan did not use the search index:\n%s", plan)
	}
	q = AiringQuery{From: from, To: from.Add(24 * time.Hour), Channels: []int64{4, 5}}
	sqlText, args, ok = airingSelect(q)
	if !ok {
		t.Fatal("expected a query")
	}
	if plan := queryPlan(t, st, sqlText, args...); !strings.Contains(plan, "airings_channel_start") {
		t.Fatalf("channel plan did not use the channel index:\n%s", plan)
	}
	q = AiringQuery{From: from, To: from.Add(time.Hour)}
	sqlText, args, ok = airingSelect(q)
	if !ok {
		t.Fatal("expected a query")
	}
	plan := queryPlan(t, st, sqlText, args...)
	if !strings.Contains(plan, "airings_starts") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("open window left the start index:\n%s", plan)
	}
}

func queryPlan(t *testing.T, st *Store, query string, args ...any) string {
	t.Helper()
	rows, err := st.db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		b.WriteString(detail)
		b.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func titlesOf(rows []Airing) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.Title
	}
	return out
}

// A team name is read the way the listing was indexed. Stripping the accent
// or the ampersand from the name made a word the index never held.
func TestQueryAiringsFindsATeamWithAnAccentOrAmpersand(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	rows := []Airing{
		{ChannelID: 1, Title: "MLS Soccer", Subtitle: "CF Montréal at Toronto FC", Start: from, End: from.Add(2 * time.Hour)},
		{ChannelID: 2, Title: "Texas A&M at Alabama", Start: from.Add(time.Hour), End: from.Add(4 * time.Hour)},
		{ChannelID: 3, Title: "Evening News", Start: from, End: from.Add(time.Hour)},
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"Montréal": "MLS Soccer", "Texas A&M": "Texas A&M at Alabama"} {
		got, err := st.QueryAirings(ctx, AiringQuery{From: from, To: from.Add(6 * time.Hour), Teams: []string{name}})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(titlesOf(got), ",") != want {
			t.Fatalf("%s: %v", name, titlesOf(got))
		}
	}
}

// Subchannels of a frequency share a tune. A 3.0 channel and a stream do not.
func TestRecordingAiringsNameTheirTune(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "BOX", TunerCount: 2}, []hdhr.Channel{
		{GuideNumber: "4.1"}, {GuideNumber: "4.2"}, {GuideNumber: "5.1"}, {GuideNumber: "104.1", VideoCodec: "HEVC", AudioCodec: "AC4"}, {GuideNumber: "6.1"},
	}); err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, ch := range chs {
		ids[ch.GuideNumber] = ch.ID
	}
	for number, freq := range map[string]int{"4.1": 593000000, "4.2": 593000000, "5.1": 605000000, "104.1": 593000000} {
		if err := st.RememberProgram(ctx, "BOX", number, freq, 1); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().Add(time.Hour).Truncate(time.Second)
	var rows []Airing
	for _, id := range ids {
		rows = append(rows, Airing{ChannelID: id, Title: "Show", ProgramID: strconv.FormatInt(id, 10), Start: start, End: start.Add(time.Hour)})
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	got, err := st.RecordingAirings(ctx, start.Add(-time.Minute), start.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	tunes := map[int64]string{}
	for _, a := range got {
		tunes[a.ChannelID] = a.Tune
	}
	if tunes[ids["4.1"]] == "" || tunes[ids["4.1"]] != tunes[ids["4.2"]] {
		t.Fatalf("one frequency: %q %q", tunes[ids["4.1"]], tunes[ids["4.2"]])
	}
	if tunes[ids["5.1"]] == "" || tunes[ids["5.1"]] == tunes[ids["4.1"]] {
		t.Fatalf("another frequency: %q", tunes[ids["5.1"]])
	}
	if tunes[ids["104.1"]] != "" || tunes[ids["6.1"]] != "" {
		t.Fatalf("a 3.0 channel or one with no frequency has no shared tune: %q %q", tunes[ids["104.1"]], tunes[ids["6.1"]])
	}
}
