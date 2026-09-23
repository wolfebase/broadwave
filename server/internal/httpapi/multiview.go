package httpapi

import (
	"net/http"

	"waveguide/internal/live"
)

func (s *Server) multiviewPlan(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		httpError(w, "Live TV is not set up on this server.", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ChannelIDs []int64 `json:"channelIds"`
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
		number := ch.DisplayNumber
		if number == "" {
			number = ch.GuideNumber
		}
		name := ch.DisplayName
		if name == "" {
			name = ch.GuideName
		}
		want = append(want, live.PlanChannel{
			ID: ch.ID, FrequencyHz: ch.FrequencyHz, Number: number, Name: name,
			Direct: ch.TunerCount == 0 && ch.StreamURL != "",
		})
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
	plan := live.PlanMultiview(want, tunerCount, ours, foreign)
	if len(missing) > 0 {
		plan.Blocked = append(missing, plan.Blocked...)
	}
	writeJSON(w, http.StatusOK, plan)
}
