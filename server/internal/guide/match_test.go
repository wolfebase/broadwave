package guide

import (
	"testing"

	"broadwave/internal/store"
)

func TestCallSignSuffixStillMatches(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="US1"><display-name>WTSTDT</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="US1">
    <title>News</title>
  </programme>
</tv>`)
	got, _, err := Parse(raw, []store.Channel{{ID: 2, GuideNumber: "5.1", GuideName: "WTSTDT1"}})
	if err != nil || len(got) != 1 || got[0].ChannelID != 2 || got[0].Title != "News" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestGuideKeyPinsAChannelTheFeedNamesDifferently(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="DIGI.14"><display-name>Univision</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="DIGI.14">
    <title>Noticias</title>
  </programme>
</tv>`)
	got, _, err := Parse(raw, []store.Channel{{ID: 5, GuideNumber: "14.1", GuideName: "KOPQ", GuideKey: "DIGI.14"}})
	if err != nil || len(got) != 1 || got[0].ChannelID != 5 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestMissingFeedChannelStaysEmpty(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="4.1"><display-name>4.1</display-name><display-name>KBWVDT</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="4.1">
    <title>News</title>
  </programme>
</tv>`)
	got, _, err := Parse(raw, []store.Channel{
		{ID: 1, GuideNumber: "4.1", GuideName: "KBWV-DT"},
		{ID: 5, GuideNumber: "14.1", GuideName: "KOPQ"},
	})
	if err != nil || len(got) != 1 || got[0].ChannelID != 1 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestASharedNumberNeedsTheNameToAgree(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="A"><display-name>7.1</display-name><display-name>KQRS</display-name></channel>
  <channel id="B"><display-name>7.1</display-name><display-name>WQRS</display-name></channel>
  <channel id="C"><display-name>7.1</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="A"><title>East News</title></programme>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="B"><title>West News</title></programme>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="C"><title>Nobody</title></programme>
</tv>`)
	lineup := []store.Channel{
		{ID: 1, GuideNumber: "7.1", GuideName: "WQRS-DT"},
		{ID: 2, GuideNumber: "7.1", GuideName: "KQRS"},
	}
	got, _, err := Parse(raw, lineup)
	if err != nil {
		t.Fatal(err)
	}
	titles := map[int64]string{}
	for _, a := range got {
		titles[a.ChannelID] += a.Title
	}
	if titles[1] != "West News" || titles[2] != "East News" || len(got) != 2 {
		t.Fatalf("shared 7.1: %v", titles)
	}
}

func TestOneStationOnTwoTunersStillMatchesByNumber(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="7.1"><display-name>7.1</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="7.1"><title>News</title></programme>
</tv>`)
	got, _, err := Parse(raw, []store.Channel{{ID: 1, GuideNumber: "7.1", GuideName: "KQRS"}, {ID: 2, GuideNumber: "7.1", GuideName: "KQRS-DT"}})
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %+v", err, got)
	}
}
