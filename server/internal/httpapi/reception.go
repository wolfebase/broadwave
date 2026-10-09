package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"broadwave/internal/hdhr"
)

// reception tells a watching device why a picture stalled, without tuner
// addresses, firmware, or the frequency the channel is on.
func (s *Server) reception(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpError(w, "invalid channel", http.StatusBadRequest)
		return
	}
	free, err := s.tunerIsFree(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	answers, err := s.tunerAnswers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	lost, err := s.channelSignalLost(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{
		"freeTuner":    free,
		"tunerAnswers": answers,
		"signalLost":   lost,
	})
}

func (s *Server) tunerIsFree(ctx context.Context) (bool, error) {
	if s.Hub == nil {
		return false, nil
	}
	list, err := s.Hub.Tuners(ctx)
	if err != nil {
		return false, err
	}
	for _, tuner := range list {
		if tuner.Target == "" && tuner.Guide == "" {
			return true, nil
		}
	}
	return false, nil
}

// tunerAnswers is true when an HDHomeRun answers, or this house has none.
// A playlist-only server has nothing to blame.
func (s *Server) tunerAnswers(ctx context.Context) (bool, error) {
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		return false, err
	}
	client := s.HDHR
	if client == nil {
		client = &hdhr.Client{}
	}
	saw := false
	for _, device := range devices {
		if !strings.HasPrefix(device.BaseURL, "http://") && !strings.HasPrefix(device.BaseURL, "https://") {
			continue
		}
		saw = true
		if _, err := client.ReadHealth(ctx, device.BaseURL); err == nil {
			return true, nil
		}
	}
	return !saw, nil
}

// channelSignalLost matches the player: only a live Lost row, not a stored reading.
func (s *Server) channelSignalLost(ctx context.Context, channelID int64) (bool, error) {
	rows, err := s.Store.ChannelSignals(ctx)
	if err != nil {
		return false, err
	}
	if s.Hub == nil {
		return false, nil
	}
	for _, row := range rows {
		if row.ChannelID != channelID {
			continue
		}
		if lock, on := s.Hub.TunedLocks()[row.FrequencyHz]; on {
			verdict, _ := lock.Verdict()
			return verdict == "Lost", nil
		}
		if s.Hub.RecentlyDark(row.ChannelID, row.FrequencyHz) {
			verdict, _ := (hdhr.Lock{}).Verdict()
			return verdict == "Lost", nil
		}
		return false, nil
	}
	return false, nil
}
