package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"broadwave/internal/discovery"
)

// homeWatchEvery is how often a running server looks for a device that was not here last time.
// The look is discovery only. It does not tune.
const homeWatchEvery = 45 * time.Second

// WatchHome baselines the house, then raises one activity event per device that appears later.
func (s *Server) WatchHome(ctx context.Context) {
	s.noteArrivals(ctx)
	tick := time.NewTicker(homeWatchEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.noteArrivals(ctx)
		}
	}
}

func (s *Server) noteArrivals(ctx context.Context) {
	s.homeMu.Lock()
	if s.arrivals == nil {
		s.arrivals = &discovery.Arrivals{}
	}
	arrivals := s.arrivals
	s.homeMu.Unlock()
	places, err := s.homePlaces(ctx, true)
	if err != nil {
		slog.Error(fmt.Sprintf("home: %v", err))
		return
	}
	for _, msg := range arrivals.Observe(places, time.Now()) {
		slog.Info(fmt.Sprintf("home: %s", msg))
		if s.Store != nil {
			_ = s.Store.AddEvent(ctx, "home", msg)
		}
	}
}
