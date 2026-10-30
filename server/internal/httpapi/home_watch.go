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
	places, err := s.homePlaces(ctx, true, false)
	if err != nil {
		slog.Error(fmt.Sprintf("home: %v", err))
		return
	}
	for _, msg := range arrivals.Observe(places, time.Now()) {
		s.noteArrival(ctx, msg)
	}
}

// noteArrival raises a device's arrival once. The watch forgets the house on
// every restart, and a TV that changes its id came back as new: one Android
// TV was announced 46 times. The line names the device, so a line already
// in the activity is not raised again.
func (s *Server) noteArrival(ctx context.Context, msg string) {
	if s.Store == nil {
		slog.Info(fmt.Sprintf("home: %s", msg))
		return
	}
	if seen, err := s.Store.HasEvent(ctx, "home", msg); err == nil && seen {
		return
	}
	slog.Info(fmt.Sprintf("home: %s", msg))
	_ = s.Store.AddEvent(ctx, "home", msg)
}
