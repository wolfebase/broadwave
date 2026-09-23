package live

import "testing"

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
