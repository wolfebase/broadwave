package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/store"
)

// addManualPass saves a pass that records a channel for a set time, once on
// a date or on the weekdays given, whether or not the guide lists anything.
func (s *Server) addManualPass(w http.ResponseWriter, r *http.Request, body map[string]any) {
	pass, msg := newManualPass(body)
	if msg != "" {
		httpError(w, msg, http.StatusBadRequest)
		return
	}
	if msg := s.checkManual(r.Context(), pass, time.Now()); msg != "" {
		httpError(w, msg, http.StatusBadRequest)
		return
	}
	if list, err := s.Store.Passes(r.Context()); err == nil {
		rankManual(&pass, body, list)
	}
	if _, err := s.Store.AddManualPass(r.Context(), pass); err != nil {
		writeError(w, err)
		return
	}
	s.passes(w, r)
}

// rankManual puts a new one-time manual pass above every saved pass, as
// recording one airing does, unless the body sets a priority.
func rankManual(pass *store.Pass, body map[string]any, saved []store.Pass) {
	if _, ok := body["priority"]; ok || len(pass.Days) > 0 {
		return
	}
	for _, p := range saved {
		if p.Priority >= pass.Priority {
			pass.Priority = clampPriority(p.Priority + 1)
		}
	}
}

func newManualPass(body map[string]any) (store.Pass, string) {
	pass := store.Pass{Kind: "manual", KeepMode: "all", Commercials: true}
	if v, ok := body["title"].(string); ok {
		pass.Title = strings.TrimSpace(v)
	}
	if msg := applyManualRules(&pass, body); msg != "" {
		return pass, msg
	}
	return pass, ""
}

// checkManual is the message for a manual pass that can't record: a channel
// that isn't there, or a one-time start that has passed.
func (s *Server) checkManual(ctx context.Context, pass store.Pass, now time.Time) string {
	if _, err := s.Store.SourceChannel(ctx, pass.ChannelID); err != nil {
		return "That channel isn't on this server."
	}
	if len(pass.Days) == 0 && !pass.AiringStart.Add(time.Duration(pass.Duration)*time.Minute).After(now) {
		return "That time has already passed."
	}
	return ""
}

// applyManualRules copies a manual pass's fields from body. The time is
// HH:MM on the server's clock; a pass with days repeats on them, one
// without records once on date (YYYY-MM-DD).
func applyManualRules(pass *store.Pass, body map[string]any) string {
	if v, ok := body["rename"].(string); ok {
		pass.Title = strings.TrimSpace(v)
	}
	if v, ok := body["channelId"]; ok {
		pass.ChannelID = int64(num(v))
	}
	if v, ok := body["timeStart"].(string); ok {
		pass.TimeStart = strings.TrimSpace(v)
	}
	if v, ok := body["duration"]; ok {
		n, isNum := v.(float64)
		if !isNum || n != float64(int(n)) {
			return "Duration is a whole number of minutes."
		}
		pass.Duration = int(n)
	}
	if v, ok := body["padBefore"]; ok {
		pass.PadBefore = clampPad(int(num(v)))
	}
	if v, ok := body["padAfter"]; ok {
		pass.PadAfter = clampPad(int(num(v)))
	}
	if v, ok := body["priority"]; ok {
		pass.Priority = clampPriority(int(num(v)))
	}
	if v, ok := body["keepMode"].(string); ok && v != "" {
		if v != "all" && v != "unwatched" && v != "last" {
			return "Keep is all, unwatched, or last."
		}
		pass.KeepMode = v
	}
	if v, ok := body["keepCount"]; ok {
		pass.KeepCount = clampCount(int(num(v)))
	}
	if v, ok := body["limitCount"]; ok {
		pass.LimitCount = clampCount(int(num(v)))
	}
	if v, ok := body["commercials"].(bool); ok {
		pass.Commercials = v
	}
	if v, ok := body["days"]; ok {
		days, msg := parseDays(v)
		if msg != "" {
			return msg
		}
		pass.Days = days
	}
	if v, ok := body["date"].(string); ok {
		pass.Date = strings.TrimSpace(v)
	}
	if pass.ChannelID <= 0 {
		return "Pick a channel to record."
	}
	if pass.TimeStart == "" || !validClock(pass.TimeStart) {
		return "Start time is HH:MM, from 00:00 to 23:59."
	}
	if pass.Duration < 1 || pass.Duration > dvr.MaxManualMinutes {
		return "Duration is 1 to 480 minutes."
	}
	if len(pass.Days) > 0 {
		// A repeating pass has no date.
		pass.Date, pass.AiringStart = "", time.Time{}
		return ""
	}
	day, err := time.ParseInLocation(time.DateOnly, pass.Date, time.Local)
	if err != nil {
		return "Pick a date (YYYY-MM-DD) or the days to repeat on."
	}
	pass.AiringStart, _ = store.ManualStart(*pass, day)
	return ""
}

// parseDays reads weekdays, 0 for Sunday to 6 for Saturday, in order and once each.
func parseDays(v any) ([]int, string) {
	const msg = "Days is a list of weekdays, 0 for Sunday to 6 for Saturday."
	list, isList := v.([]any)
	if v != nil && !isList {
		return nil, msg
	}
	days := []int{}
	for _, d := range list {
		n, isNum := d.(float64)
		if !isNum || n != float64(int(n)) || n < 0 || n > 6 {
			return nil, msg
		}
		days = append(days, int(n))
	}
	return store.DaysOf(store.DaysMask(days)), ""
}

// nameManual gives a manual slot with no title and no listing the channel's
// name, as the recording will have.
func (s *Server) nameManual(ctx context.Context, items []dvr.Planned) {
	var names map[int64]string
	for i := range items {
		if !items[i].Manual || items[i].Airing.Title != "" {
			continue
		}
		if names == nil {
			names = map[int64]string{}
			if chs, err := s.Store.Channels(ctx, false); err == nil {
				for _, ch := range chs {
					names[ch.ID] = ch.DisplayName
				}
			}
		}
		items[i].Airing.Title = names[items[i].Airing.ChannelID]
	}
}
