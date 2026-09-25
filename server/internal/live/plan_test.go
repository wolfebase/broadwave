package live

import (
	"testing"
	"time"
)

func TestPickerCostsAtEveryTunerCount(t *testing.T) {
	now := time.Date(2026, 9, 25, 14, 40, 0, 0, time.Local)
	for _, tuners := range []int{1, 2, 4, 8} {
		current := []PlanChannel{ch(1, 593000000, "4.1")}
		candidates := []PlanChannel{ch(2, 593000000, "4.2"), ch(3, 533000000, "5.1")}
		offers, stops := Picker(current, candidates, tuners, nil, nil, nil, now)
		if len(stops) != 0 {
			t.Fatalf("%d tuners stopped %+v", tuners, stops)
		}
		if offers[0].Cost != "same" || offers[0].Label != "Same tune as 4.1" {
			t.Fatalf("%d same %+v", tuners, offers[0])
		}
		if tuners == 1 {
			if offers[1].Cost != "none" || offers[1].Label != "No tuner free" {
				t.Fatalf("1 tuner %+v", offers[1])
			}
			continue
		}
		if offers[1].Cost != "tuner" || offers[1].Label != "Uses a tuner" {
			t.Fatalf("%d tuners %+v", tuners, offers[1])
		}
		full := make([]PlanChannel, tuners)
		for i := range full {
			full[i] = ch(int64(i+1), 1000+i, "c")
		}
		offers, _ = Picker(full, []PlanChannel{ch(99, 9000, "99.1")}, tuners, nil, nil, nil, now)
		if offers[0].Cost != "none" || offers[0].Label != "No tuner free" {
			t.Fatalf("full %d %+v", tuners, offers[0])
		}
	}
}

func TestPickerRecordingTakesTheHighestTile(t *testing.T) {
	now := time.Date(2026, 9, 25, 14, 40, 0, 0, time.Local)
	at := time.Date(2026, 9, 25, 15, 0, 0, 0, time.Local)
	current := []PlanChannel{ch(1, 593000000, "4.1"), ch(4, 575000000, "9.1")}
	reserved := []Reservation{{
		Channel: ch(3, 533000000, "5.1"),
		Title:   "Jeopardy",
		At:      at,
	}}
	candidates := []PlanChannel{
		ch(2, 593000000, "4.2"),
		ch(5, 533000000, "5.2"),
		ch(3, 533000000, "5.1"),
		ch(6, 605000000, "7.1"),
	}
	offers, stops := Picker(current, candidates, 2, nil, nil, reserved, now)
	if len(stops) != 1 || stops[0].ChannelID != 4 {
		t.Fatalf("9.1 should stop %+v", stops)
	}
	if stops[0].Reason != "9.1 stops at 3:00 PM. Jeopardy is recording." {
		t.Fatalf("reason %q", stops[0].Reason)
	}
	if !stops[0].At.Equal(at) {
		t.Fatal(stops[0].At)
	}
	byID := map[int64]Offer{}
	for _, o := range offers {
		byID[o.ChannelID] = o
	}
	if byID[2].Label != "Same tune as 4.1" || byID[5].Label != "Same tune as 5.1" {
		t.Fatalf("share %+v", offers)
	}
	if byID[3].Cost != "tuner" || byID[3].Label != "Uses a tuner" {
		t.Fatalf("recording channel %+v", byID[3])
	}
	if byID[6].Cost != "none" || byID[6].Label != "No tuner free" {
		t.Fatalf("seventh %+v", byID[6])
	}
}

func TestPickerRecordingRidesASpareTuner(t *testing.T) {
	now := time.Date(2026, 9, 25, 14, 40, 0, 0, time.Local)
	at := now.Add(10 * time.Minute)
	current := []PlanChannel{ch(1, 593000000, "4.1")}
	reserved := []Reservation{{Channel: ch(3, 533000000, "5.1"), Title: "Jeopardy", At: at}}
	offers, stops := Picker(current, []PlanChannel{ch(2, 593000000, "4.2"), ch(6, 605000000, "7.1"), ch(3, 533000000, "5.1")}, 2, nil, nil, reserved, now)
	if len(stops) != 0 {
		t.Fatalf("a free tuner should take the recording %+v", stops)
	}
	if offers[0].Label != "Same tune as 4.1" || offers[1].Label != "No tuner free" || offers[2].Label != "Uses a tuner" {
		t.Fatalf("%+v", offers)
	}
}

func TestPickerRecordingSharesTheChannelItIsOn(t *testing.T) {
	now := time.Date(2026, 9, 25, 14, 40, 0, 0, time.Local)
	current := []PlanChannel{ch(1, 593000000, "4.1")}
	reserved := []Reservation{{Channel: ch(1, 593000000, "4.1"), Title: "Jeopardy", At: now.Add(time.Hour)}}
	_, stops := Picker(current, []PlanChannel{ch(3, 533000000, "5.1")}, 2, nil, nil, reserved, now)
	if len(stops) != 0 {
		t.Fatalf("recording the channel on screen does not stop it %+v", stops)
	}
}

func TestPickerWarnsForTomorrow(t *testing.T) {
	now := time.Date(2026, 9, 25, 14, 40, 0, 0, time.Local)
	at := now.Add(24 * time.Hour)
	current := []PlanChannel{ch(1, 100, "4.1")}
	_, stops := Picker(current, nil, 1, nil, nil, []Reservation{{Channel: ch(2, 200, "5.1"), Title: "News", At: at}}, now)
	if len(stops) != 1 || stops[0].Reason != "4.1 stops at Saturday at 2:40 PM. News is recording." {
		t.Fatalf("%+v", stops)
	}
}

func TestPickerDropsBothSubchannelsOnOneTuner(t *testing.T) {
	now := time.Date(2026, 9, 25, 14, 0, 0, 0, time.Local)
	at := time.Date(2026, 9, 25, 15, 0, 0, 0, time.Local)
	current := []PlanChannel{ch(1, 593000000, "4.1"), ch(2, 593000000, "4.2")}
	_, stops := Picker(current, nil, 1, nil, nil, []Reservation{{Channel: ch(3, 533000000, "5.1"), Title: "News", At: at}}, now)
	if len(stops) != 2 {
		t.Fatalf("%+v", stops)
	}
	if stops[0].Reason != "4.1 and 4.2 stop at 3:00 PM. News is recording." {
		t.Fatalf("%q", stops[0].Reason)
	}
}

func ch(id int64, hz int, number string) PlanChannel {
	return PlanChannel{ID: id, FrequencyHz: hz, Number: number}
}

func TestPlanSharesOneTunerAcrossSubchannels(t *testing.T) {
	plan := PlanMultiview([]PlanChannel{ch(1, 473000000, "14.1"), ch(2, 473000000, "14.2"), ch(3, 473000000, "14.3")}, 2, nil, nil)
	if len(plan.Playable) != 3 || len(plan.Blocked) != 0 {
		t.Fatalf("three subchannels fit on one tuner: %+v", plan)
	}
	if plan.TunersNeeded != 1 || plan.TunersFree != 1 {
		t.Fatalf("needed 1 free 1, got %+v", plan)
	}
	if !plan.Playable[0].Shared || plan.Note != "14.1, 14.2, and 14.3 share one tuner." {
		t.Fatalf("share note: %+v", plan)
	}
}

func TestPlanUsesTwoTunersForTwoFrequencies(t *testing.T) {
	plan := PlanMultiview([]PlanChannel{ch(1, 100, "4.1"), ch(2, 200, "9.1")}, 2, nil, nil)
	if len(plan.Playable) != 2 || plan.TunersNeeded != 2 || plan.TunersFree != 0 {
		t.Fatalf("%+v", plan)
	}
}

func TestPlanBlocksTheChannelThatDoesNotFit(t *testing.T) {
	plan := PlanMultiview([]PlanChannel{ch(1, 100, "4.1"), ch(2, 200, "9.1"), ch(3, 300, "5.1")}, 2, nil, nil)
	if len(plan.Playable) != 2 || len(plan.Blocked) != 1 || plan.Blocked[0].ChannelID != 3 {
		t.Fatalf("%+v", plan)
	}
	if plan.Blocked[0].Reason != "Both tuners are busy. 4.1 and 9.1 are on." {
		t.Fatalf("reason: %q", plan.Blocked[0].Reason)
	}
}

func TestPlanReusesAFrequencyAlreadyTuned(t *testing.T) {
	ours := []TunedFreq{{FrequencyHz: 100, Labels: []string{"4.1"}}}
	plan := PlanMultiview([]PlanChannel{ch(1, 100, "4.1"), ch(4, 100, "4.2"), ch(2, 200, "9.1")}, 2, ours, nil)
	if len(plan.Blocked) != 0 || plan.TunersNeeded != 2 || plan.TunersFree != 0 {
		t.Fatalf("4.x is free, 9.1 takes the other tuner: %+v", plan)
	}
	if !plan.Playable[0].Shared {
		t.Fatal("4.2 shares the tuner 4.1 already holds")
	}
}

func TestPlanTreatsUnknownFrequenciesAsSeparateTuners(t *testing.T) {
	plan := PlanMultiview([]PlanChannel{ch(1, 0, "4.1"), ch(2, 0, "4.2")}, 1, nil, nil)
	if len(plan.Playable) != 1 || plan.Blocked[0].ChannelID != 2 {
		t.Fatalf("%+v", plan)
	}
	if plan.Blocked[0].Reason != "The tuner is busy. 4.1 is on." {
		t.Fatalf("reason: %q", plan.Blocked[0].Reason)
	}
}

func TestPlanLetsLinksSkipTheTuner(t *testing.T) {
	a := ch(1, 0, "News")
	b := ch(2, 0, "Weather")
	a.Direct, b.Direct = true, true
	plan := PlanMultiview([]PlanChannel{a, b}, 0, nil, nil)
	if len(plan.Playable) != 2 || len(plan.Blocked) != 0 || plan.TunersNeeded != 0 {
		t.Fatalf("a link does not take a tuner: %+v", plan)
	}
}

func TestPlanNamesATunerHeldBySomeoneElse(t *testing.T) {
	plan := PlanMultiview([]PlanChannel{ch(1, 100, "4.1"), ch(2, 200, "9.1")}, 2, nil, []string{"KCTV"})
	if len(plan.Playable) != 1 || plan.Blocked[0].ChannelID != 2 {
		t.Fatalf("%+v", plan)
	}
	if plan.Blocked[0].Reason != "Both tuners are busy. KCTV and 4.1 are on." {
		t.Fatalf("reason: %q holders %v", plan.Blocked[0].Reason, plan.Blocked[0].Holders)
	}
}
