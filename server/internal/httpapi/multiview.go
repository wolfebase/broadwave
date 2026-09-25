package httpapi

import (
	"context"
	"net/http"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func (s *Server) multiviewPlan(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		httpError(w, "Live TV is not set up on this server.", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ChannelIDs []int64 `json:"channelIds"`
		Picker     bool    `json:"picker"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	var want []live.PlanChannel
	var missing []live.Blocked
	for _, id := range body.ChannelIDs {
		ch, err := s.Store.SourceChannel(r.Context(), id)
		if err != nil {
			missing = append(missing, live.Blocked{ChannelID: id, Reason: "That channel is not in the lineup.", Holders: []string{}})
			continue
		}
		want = append(want, planChannel(ch))
	}
	var ours []live.TunedFreq
	var foreign []string
	tunerCount := 0
	if s.Hub != nil {
		for _, feed := range s.Hub.Status() {
			label := feed.GuideNumber
			if label == "" {
				label = feed.Name
			}
			ours = append(ours, live.TunedFreq{FrequencyHz: feed.FrequencyHz, Labels: []string{label}})
		}
		if tuners, err := s.Hub.Tuners(r.Context()); err == nil {
			tunerCount = len(tuners)
			for _, tuner := range tuners {
				if tuner.Ours || (tuner.Guide == "" && tuner.Target == "") {
					continue
				}
				label := tuner.Name
				if label == "" {
					label = tuner.Guide
				}
				foreign = append(foreign, label)
			}
		}
	}
	if tunerCount == 0 {
		if devices, err := s.Store.Devices(r.Context()); err == nil {
			for _, device := range devices {
				tunerCount += device.TunerCount
			}
		}
	}
	var candidates []live.PlanChannel
	if body.Picker {
		candidates = s.pickerChannels(r.Context())
	}
	offers, stops := live.Picker(want, candidates, tunerCount, ours, foreign, s.recordingReservations(r.Context(), tunerCount), s.now())
	plan := live.PlanMultiview(want, tunerCount, ours, foreign)
	plan.Offers = offers
	plan.Stops = stops
	if len(missing) > 0 {
		plan.Blocked = append(missing, plan.Blocked...)
	}
	writeJSON(w, http.StatusOK, plan)
}

func planChannel(ch store.SourceChannel) live.PlanChannel {
	number := ch.DisplayNumber
	if number == "" {
		number = ch.GuideNumber
	}
	name := ch.DisplayName
	if name == "" {
		name = ch.GuideName
	}
	return live.PlanChannel{
		ID: ch.ID, FrequencyHz: ch.FrequencyHz, Number: number, Name: name,
		Direct: ch.TunerCount == 0 && ch.StreamURL != "",
	}
}

func (s *Server) pickerChannels(ctx context.Context) []live.PlanChannel {
	list, err := s.Store.Channels(ctx, true)
	if err != nil {
		return nil
	}
	var out []live.PlanChannel
	for _, ch := range list {
		full, err := s.Store.SourceChannel(ctx, ch.ID)
		if err != nil || !full.Enabled || full.Hidden || !full.Present {
			continue
		}
		out = append(out, planChannel(full))
	}
	return out
}

// recordingReservations is the recordings that will take a tuner within 30 minutes.
// A skipped or conflicting pass is left out. Staging still reports them so the warning shows.
func (s *Server) recordingReservations(ctx context.Context, tunerCount int) []live.Reservation {
	if s.Store == nil {
		return nil
	}
	now := s.now()
	const window = 30 * time.Minute
	passes, err := s.Store.Passes(ctx)
	if err != nil || len(passes) == 0 {
		return nil
	}
	airings, err := s.Store.Airings(ctx, now.Add(-time.Minute), now.Add(window))
	if err != nil {
		return nil
	}
	if tunerCount < 1 {
		tunerCount = s.tunerCount(ctx)
	}
	planned := dvr.Plan(passes, airings, tunerCount, now.Add(-time.Minute), now.Add(window))
	recs, _ := s.Store.Recordings(ctx)
	seen, _ := s.Store.SeenDeleted(ctx)
	skips, _ := s.Store.Skips(ctx)
	planned = dvr.ApplyLibrary(planned, passes, recs, seen, skips)
	var out []live.Reservation
	for _, item := range planned {
		if item.Skipped || item.Conflict || !item.Airing.End.After(now) {
			continue
		}
		lead := 90 * time.Second
		for _, pass := range passes {
			if pass.ID != item.PassID {
				continue
			}
			if extra := time.Duration(pass.PadBefore) * time.Minute; extra > lead {
				lead = extra
			}
			break
		}
		at := item.Airing.Start.Add(-lead)
		if at.After(now.Add(window)) {
			continue
		}
		ch, err := s.Store.SourceChannel(ctx, item.Airing.ChannelID)
		if err != nil {
			continue
		}
		out = append(out, live.Reservation{Channel: planChannel(ch), Title: item.Airing.Title, At: at})
	}
	return out
}
