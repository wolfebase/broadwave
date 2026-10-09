package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGuideWindowKeepsTheEarliestSource(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	if err := st.InsertAirings(ctx, []Airing{
		// "xmltv" sorts after "broadcast", so MAX(guide_source) would pick it.
		{ChannelID: 1, Title: "News", GuideSource: "broadcast", Start: now, End: now.Add(time.Hour)},
		{ChannelID: 1, Title: "Quiz Night", GuideSource: "xmltv", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		// An empty source does not hide the later one that names a guide.
		{ChannelID: 2, Title: "Wolves", Start: now, End: now.Add(30 * time.Minute)},
		{ChannelID: 2, Title: "Harbor", GuideSource: "broadcast", Start: now.Add(30 * time.Minute), End: now.Add(time.Hour)},
		// Already over, and one that starts after the window.
		{ChannelID: 3, Title: "Old", GuideSource: "broadcast", Start: now.Add(-2 * time.Hour), End: now.Add(-time.Hour)},
		{ChannelID: 3, Title: "Later", GuideSource: "xmltv", Start: now.Add(15 * 24 * time.Hour), End: now.Add(15*24*time.Hour + time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	total, until, rows, err := st.GuideWindow(ctx, now, now.Add(14*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || len(rows) != 2 {
		t.Fatalf("total %d rows %d", total, len(rows))
	}
	if !until.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("until %s", until)
	}
	got := map[int64]ChannelGuide{}
	for _, row := range rows {
		got[row.ChannelID] = row
	}
	if got[1].Source != "broadcast" || got[1].Airings != 2 || !got[1].Until.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("channel 1 %+v", got[1])
	}
	if got[2].Source != "broadcast" || got[2].Airings != 2 {
		t.Fatalf("channel 2 %+v", got[2])
	}
	listed, err := st.ChannelsListedBetween(ctx, now, now.Add(14*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || !listed[1] || !listed[2] || listed[3] {
		t.Fatalf("listed %v", listed)
	}
}

func TestListingDepthCountsAStartingShowTwice(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	if err := st.InsertAirings(ctx, []Airing{
		{ChannelID: 1, Title: "On now", Start: now.Add(-30 * time.Minute), End: now.Add(30 * time.Minute)},
		{ChannelID: 1, Title: "Later", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		{ChannelID: 2, Title: "Starting", Start: now.Add(30 * time.Second), End: now.Add(30 * time.Minute)},
		{ChannelID: 3, Title: "Just ended", Start: now.Add(-30 * time.Minute), End: now.Add(-30 * time.Second)},
		{ChannelID: 4, Title: "Tomorrow", Start: now.Add(24 * time.Hour), End: now.Add(25 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	depth, err := st.ListingDepth(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if depth[1] != 2 || depth[2] != 2 || depth[3] != 0 || depth[4] != 0 {
		t.Fatalf("depth %v", depth)
	}
}

// These reads group by channel. The planner uses the channel index for that,
// including the starts_at bound on the six-hour depth. An index on ends_at
// alone does not win this plan, and it must not replace airings_channel_start
// for a single channel's window.
func TestGuideQueriesKeepTheChannelIndex(t *testing.T) {
	st := openTestStore(t)
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	from := now.UTC().Format(time.RFC3339)
	to := now.Add(14 * 24 * time.Hour).UTC().Format(time.RFC3339)
	soon := now.Add(time.Minute).UTC().Format(time.RFC3339)
	for _, query := range []struct {
		name string
		sql  string
		args []any
	}{
		{"listed", channelsListedSQL, []any{from, to}},
		{"count", guideCountSQL, []any{from, to}},
		{"depth", listingDepthSQL, []any{soon, from, from, now.Add(-time.Minute).UTC().Format(time.RFC3339), now.Add(6 * time.Hour).UTC().Format(time.RFC3339)}},
	} {
		plan := queryPlan(t, st, query.sql, query.args...)
		if !strings.Contains(plan, "airings_channel_start") {
			t.Fatalf("%s left the channel index:\n%s", query.name, plan)
		}
	}
	q := AiringQuery{From: now, To: now.Add(24 * time.Hour), Channels: []int64{4, 5}}
	sqlText, args, ok := airingSelect(q)
	if !ok {
		t.Fatal("expected a query")
	}
	if plan := queryPlan(t, st, sqlText, args...); !strings.Contains(plan, "airings_channel_start") {
		t.Fatalf("channel plan lost its index:\n%s", plan)
	}
}
