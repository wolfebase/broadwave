package httpapi

import (
	"context"
	"testing"
	"time"

	"broadwave/internal/dvbsi"
	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestDVBListingsFindTheirChannels(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "DVB1", FriendlyName: "Quatro", ModelNumber: "HDHR5-4DT", BaseURL: "http://127.0.0.1:1", TunerCount: 4}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "1", GuideName: "Channel One HD"},
		{GuideNumber: "2", GuideName: "Channel Two"},
		{GuideNumber: "21", GuideName: "News 21"},
		{GuideNumber: "31", GuideName: "Regional"},
		{GuideNumber: "32", GuideName: "Regional"},
	}); err != nil {
		t.Fatal(err)
	}
	// 1 and 2 were tuned: the mux at 474 MHz carries them as programs 4161 and 4162.
	for guide, program := range map[string]int{"1": 4161, "2": 4162} {
		if err := st.RememberProgram(ctx, "DVB1", guide, 474000000, program); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]int64{}
	channels, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range channels {
		ids[ch.GuideNumber] = ch.ID
	}
	at := time.Date(2026, 3, 14, 19, 0, 0, 0, time.UTC)
	// A listing a downloaded guide already holds is kept.
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: ids["2"], Title: "Downloaded", GuideSource: "xmltv", Start: at, End: at.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	here := func(service, event int, title string, start time.Time, nowNext bool) dvbsi.Event {
		return dvbsi.Event{NetworkID: 0x2001, StreamID: 0x0101, ServiceID: service, EventID: event, Actual: true, NowNext: nowNext,
			Title: title, Start: start, End: start.Add(time.Hour)}
	}
	other := func(stream, service, event int, title string) dvbsi.Event {
		return dvbsi.Event{NetworkID: 0x2001, StreamID: stream, ServiceID: service, EventID: event,
			Title: title, Start: at, End: at.Add(time.Hour)}
	}
	quiz := here(4161, 1, "QUIZ NIGHT", at, true)
	quiz.Subtitle, quiz.Description, quiz.Genres, quiz.Rating = "Round One", "Teams of four.", []string{"Entertainment"}, "12+"
	g := dvbsi.Guide{
		Services: []dvbsi.Service{
			{NetworkID: 0x2001, StreamID: 0x0101, ID: 4161, Name: "Channel One HD", Actual: true},
			{NetworkID: 0x2001, StreamID: 0x0101, ID: 4162, Name: "Channel Two", Actual: true},
			{NetworkID: 0x2001, StreamID: 0x0202, ID: 8261, Name: "News 21"},
			{NetworkID: 0x2001, StreamID: 0x0303, ID: 4161, Name: "Elsewhere"},
			{NetworkID: 0x2001, StreamID: 0x0404, ID: 7001, Name: "Regional"},
		},
		Events: []dvbsi.Event{
			// The schedule's copy of a show running late loses to now/next.
			here(4161, 9, "Early Copy", at.Add(-50*time.Minute), false),
			quiz,
			here(4161, 2, "Harbour Lights", at.Add(time.Hour), false),
			here(4162, 3, "Overlaps The Download", at.Add(30*time.Minute), false),
			here(4162, 4, "After The Download", at.Add(90*time.Minute), false),
			here(9999, 5, "No Such Program", at, false),
			other(0x0202, 8261, 6, "Evening Bulletin"),
			// Program 4161 of another mux is not channel 1.
			other(0x0303, 4161, 7, "Not Channel One"),
			// Two channels share this name; it says nothing.
			other(0x0404, 7001, 8, "Somewhere Regional"),
		},
	}
	n, err := (&Server{Store: st}).ApplyDVB(ctx, 474000000, g)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := st.Airings(ctx, at.Add(-2*time.Hour), at.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64][]store.Airing{}
	for _, r := range rows {
		got[r.ChannelID] = append(got[r.ChannelID], r)
	}
	if n != 4 {
		t.Fatalf("filled %d: %+v", n, rows)
	}
	one := got[ids["1"]]
	if len(one) != 2 || one[0].Title != "Quiz Night" || one[1].Title != "Harbour Lights" {
		t.Fatalf("channel 1 %+v", one)
	}
	if q := one[0]; q.Subtitle != "Round One" || q.Description != "Teams of four." || q.Category != "Entertainment" || q.Rating != "12+" || q.GuideSource != "broadcast" {
		t.Fatalf("quiz %+v", q)
	}
	two := got[ids["2"]]
	if len(two) != 2 || two[0].Title != "Downloaded" || two[1].Title != "After The Download" {
		t.Fatalf("channel 2 %+v", two)
	}
	if news := got[ids["21"]]; len(news) != 1 || news[0].Title != "Evening Bulletin" {
		t.Fatalf("channel 21 %+v", news)
	}
	if len(got[ids["31"]])+len(got[ids["32"]]) != 0 {
		t.Fatalf("regional %+v %+v", got[ids["31"]], got[ids["32"]])
	}
	// The next window brings the same listings again and adds nothing.
	if n, err := (&Server{Store: st}).ApplyDVB(ctx, 474000000, g); err != nil || n != 0 {
		t.Fatalf("again %d %v", n, err)
	}
}

// An antenna tuner's channel is not found by a DVB service's name.
func TestDVBNamesMatchOnlyDVBTuners(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "ATSC", FriendlyName: "Duo", ModelNumber: "HDHR5-2US", BaseURL: "http://127.0.0.1:2", TunerCount: 2},
		[]hdhr.Channel{{GuideNumber: "4.1", GuideName: "News 21"}}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 14, 19, 0, 0, 0, time.UTC)
	n, err := (&Server{Store: st}).ApplyDVB(ctx, 474000000, dvbsi.Guide{
		Services: []dvbsi.Service{{NetworkID: 1, StreamID: 2, ID: 3, Name: "News 21"}},
		Events:   []dvbsi.Event{{NetworkID: 1, StreamID: 2, ServiceID: 3, Title: "Bulletin", Start: at, End: at.Add(time.Hour)}},
	})
	if err != nil || n != 0 {
		t.Fatalf("filled %d %v", n, err)
	}
}
