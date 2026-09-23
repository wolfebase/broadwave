package guide

import (
	"testing"

	"waveguide/internal/store"
)

func TestCallSignSuffixStillMatches(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="US1"><display-name>KCTVDT</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="US1">
    <title>News</title>
  </programme>
</tv>`)
	got, _, err := Parse(raw, []store.Channel{{ID: 2, GuideNumber: "5.1", GuideName: "KCTVDT1"}})
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
	got, _, err := Parse(raw, []store.Channel{{ID: 5, GuideNumber: "14.1", GuideName: "KUKC", GuideKey: "DIGI.14"}})
	if err != nil || len(got) != 1 || got[0].ChannelID != 5 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestMissingFeedChannelStaysEmpty(t *testing.T) {
	raw := []byte(`<tv>
  <channel id="4.1"><display-name>4.1</display-name><display-name>WDAFDT</display-name></channel>
  <programme start="20260922150000 -0500" stop="20260922160000 -0500" channel="4.1">
    <title>News</title>
  </programme>
</tv>`)
	got, _, err := Parse(raw, []store.Channel{
		{ID: 1, GuideNumber: "4.1", GuideName: "WDAF-DT"},
		{ID: 5, GuideNumber: "14.1", GuideName: "KUKC"},
	})
	if err != nil || len(got) != 1 || got[0].ChannelID != 1 {
		t.Fatalf("%v %+v", err, got)
	}
}
