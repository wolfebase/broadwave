package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// BroadcastScan tunes each frequency that is missing a current listing, dwells
// long enough to read the broadcast guide, and stops if someone wants the tuner.
func (s *Server) BroadcastScan(ctx context.Context) {
	if s == nil || s.Hub == nil || s.Store == nil || s.signalRunning() {
		return
	}
	s.Hub.RunScan(ctx, []int{1}, 20*time.Minute, func(ctx context.Context, _ int) error {
		s.scanMissing(ctx)
		return nil
	})
}

// scanDwell is how long a scan stays on a frequency to read its broadcast guide.
var scanDwell = 40 * time.Second

func (s *Server) scanMissing(ctx context.Context) {
	seen := map[int]bool{}
	tried := map[int64]bool{}
	for {
		if ctx.Err() != nil || !s.Hub.Idle() || s.recordingSoon(ctx) {
			return
		}
		ch, known, ok := s.nextUnlisted(ctx, seen, tried)
		if !ok {
			return
		}
		tried[ch.ID] = true
		freq, err := s.Hub.Dwell(ctx, ch.ID, scanDwell)
		if errors.Is(err, live.ErrNoSignal) {
			// A station off the air: its other channels share the frequency, so
			// tuning each of them only finds the same nothing.
			if known > 0 {
				seen[known] = true
			}
			slog.Info(fmt.Sprintf("guide: scan %s: no signal", ch.GuideNumber))
			continue
		}
		if err != nil {
			slog.Warn(fmt.Sprintf("guide: scan %s: %v", ch.GuideNumber, err))
			continue
		}
		if freq > 0 {
			seen[freq] = true
			_ = s.Store.NoteGuideScan(ctx, freq, time.Now())
			slog.Debug(fmt.Sprintf("guide: scanned %s at %d Hz", ch.GuideNumber, freq))
		}
	}
}

// nextUnlisted is the next channel to scan and its frequency, when known.
func (s *Server) nextUnlisted(ctx context.Context, seen map[int]bool, tried map[int64]bool) (store.Channel, int, bool) {
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return store.Channel{}, 0, false
	}
	now := time.Now()
	current, err := s.Store.ListingDepth(ctx, now)
	if err != nil {
		return store.Channel{}, 0, false
	}
	for _, ch := range channels {
		if ch.Hidden || !ch.Present || tried[ch.ID] {
			continue
		}
		full, err := s.Store.SourceChannel(ctx, ch.ID)
		// Only an antenna tuner carries ATSC listings. A playlist would hold a
		// stream (often a provider's only one), and a cable or DVB tuner a
		// tuner, for a guide that never comes.
		if err != nil || full.TunerCount == 0 || !hdhr.BroadcastGuide(full.ModelNumber) {
			continue
		}
		// An HDHomeRun emulator poses as an antenna model; its channels are
		// a provider's streams.
		if s.Hub != nil && s.Hub.NoStatus(ctx, full.BaseURL) {
			continue
		}
		if current[ch.ID] >= 2 && full.FrequencyHz == 0 {
			continue
		}
		if full.FrequencyHz > 0 && seen[full.FrequencyHz] {
			continue
		}
		return ch, full.FrequencyHz, true
	}
	return store.Channel{}, 0, false
}

func (s *Server) recordingSoon(ctx context.Context) bool {
	return s.RecordingWithin(ctx, 30*time.Minute)
}

// RecordingWithin reports a recording running, or one the passes plan to
// start within d. A catalog that cannot be read counts as one.
func (s *Server) RecordingWithin(ctx context.Context, d time.Duration) bool {
	recs, err := s.Store.Recordings(ctx)
	if err != nil {
		return true
	}
	now := time.Now()
	for _, rec := range recs {
		if rec.Status == "recording" {
			return true
		}
		if rec.Status == "planned" && rec.StartedAt.Before(now.Add(d)) && rec.StartedAt.After(now.Add(-time.Minute)) {
			return true
		}
	}
	passes, _ := s.Store.Passes(ctx)
	airings, err := s.Store.RecordingAirings(ctx, now, now.Add(d))
	if err != nil {
		return true
	}
	items := dvr.Plan(passes, airings, 2, now, now.Add(d))
	return len(items) > 0
}
