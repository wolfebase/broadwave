package httpapi

import (
	"context"
	"log"
	"time"

	"waveguide/internal/dvr"
	"waveguide/internal/store"
)

// BroadcastScan tunes each frequency that is missing a current listing, dwells
// long enough to read the broadcast guide, and stops if someone wants the tuner.
func (s *Server) BroadcastScan(ctx context.Context) {
	if s == nil || s.Hub == nil || s.Store == nil {
		return
	}
	s.Hub.RunScan(ctx, []int{1}, 20*time.Minute, func(ctx context.Context, _ int) error {
		s.scanMissing(ctx)
		return nil
	})
}

func (s *Server) scanMissing(ctx context.Context) {
	seen := map[int]bool{}
	tried := map[int64]bool{}
	for {
		if ctx.Err() != nil || !s.Hub.Idle() || s.recordingSoon(ctx) {
			return
		}
		ch, ok := s.nextUnlisted(ctx, seen, tried)
		if !ok {
			return
		}
		tried[ch.ID] = true
		freq, err := s.Hub.Dwell(ctx, ch.ID, 40*time.Second)
		if err != nil {
			log.Printf("guide: scan %s: %v", ch.GuideNumber, err)
			continue
		}
		if freq > 0 {
			seen[freq] = true
			_ = s.Store.NoteGuideScan(ctx, freq, time.Now())
			log.Printf("guide: scanned %s at %d Hz", ch.GuideNumber, freq)
		}
	}
}

func (s *Server) nextUnlisted(ctx context.Context, seen map[int]bool, tried map[int64]bool) (store.Channel, bool) {
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return store.Channel{}, false
	}
	now := time.Now()
	rows, err := s.Store.Airings(ctx, now.Add(-time.Minute), now.Add(6*time.Hour))
	if err != nil {
		return store.Channel{}, false
	}
	current := map[int64]int{}
	for _, row := range rows {
		if row.Start.Before(now.Add(time.Minute)) && row.End.After(now) {
			current[row.ChannelID]++
		}
		if row.Start.After(now) {
			current[row.ChannelID]++
		}
	}
	for _, ch := range channels {
		if ch.Hidden || !ch.Present || tried[ch.ID] {
			continue
		}
		if current[ch.ID] >= 2 {
			full, err := s.Store.SourceChannel(ctx, ch.ID)
			if err != nil || full.FrequencyHz == 0 || seen[full.FrequencyHz] {
				continue
			}
			return ch, true
		}
		full, err := s.Store.SourceChannel(ctx, ch.ID)
		if err != nil {
			continue
		}
		if full.FrequencyHz > 0 && seen[full.FrequencyHz] {
			continue
		}
		return ch, true
	}
	return store.Channel{}, false
}

func (s *Server) recordingSoon(ctx context.Context) bool {
	recs, err := s.Store.Recordings(ctx)
	if err != nil {
		return true
	}
	now := time.Now()
	for _, rec := range recs {
		if rec.Status == "recording" {
			return true
		}
		if rec.Status == "planned" && rec.StartedAt.Before(now.Add(30*time.Minute)) && rec.StartedAt.After(now.Add(-time.Minute)) {
			return true
		}
	}
	passes, _ := s.Store.Passes(ctx)
	airings, err := s.Store.Airings(ctx, now, now.Add(30*time.Minute))
	if err != nil {
		return true
	}
	items := dvr.Plan(passes, airings, 2, now, now.Add(30*time.Minute))
	return len(items) > 0
}
