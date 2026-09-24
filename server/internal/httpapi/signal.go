package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"waveguide/internal/hdhr"
)

func (s *Server) signals(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ChannelSignals(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	live := map[int]hdhr.Lock{}
	if s.Hub != nil {
		live = s.Hub.TunedLocks()
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"channelId":   row.ChannelID,
			"number":      row.GuideNumber,
			"name":        row.GuideName,
			"frequencyHz": row.FrequencyHz,
		}
		lock, on := live[row.FrequencyHz]
		if on {
			item["strength"] = lock.Strength
			item["quality"] = lock.Quality
			item["symbol"] = lock.Symbol
			verdict, tip := lock.Verdict()
			item["verdict"] = verdict
			if tip != "" {
				item["tip"] = tip
			}
			item["live"] = true
		} else if row.HasReading {
			lock := hdhr.Lock{Strength: row.Strength, Quality: row.Quality, Symbol: row.Symbol, Locked: row.Locked}
			item["strength"] = row.Strength
			item["quality"] = row.Quality
			item["symbol"] = row.Symbol
			verdict, tip := lock.Verdict()
			item["verdict"] = verdict
			if tip != "" {
				item["tip"] = tip
			}
			item["checkedAt"] = row.CheckedAt
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": out, "running": s.signalRunning()})
}

func (s *Server) checkSignals(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil || !s.Hub.Idle() || s.recordingSoon(r.Context()) {
		httpError(w, "Both tuners are busy. Stop a recording or watch something already on.", http.StatusConflict)
		return
	}
	if !s.startSignalScan() {
		writeJSON(w, http.StatusOK, map[string]any{"running": true, "message": "Already checking channels."})
		return
	}
	go s.SignalScan(context.Background())
	writeJSON(w, http.StatusOK, map[string]any{"running": true, "message": "Checking channels. This stops if you start watching."})
}

func (s *Server) SignalScan(ctx context.Context) {
	defer s.finishSignalScan()
	if s.Hub == nil || s.Store == nil {
		return
	}
	channels, err := s.Store.ChannelSignals(ctx)
	if err != nil {
		return
	}
	seen := map[int]bool{}
	var ids []int
	for _, ch := range channels {
		if ch.FrequencyHz > 0 {
			if seen[ch.FrequencyHz] {
				continue
			}
			seen[ch.FrequencyHz] = true
		}
		ids = append(ids, int(ch.ChannelID))
	}
	s.Hub.RunScan(ctx, ids, 15*time.Second, func(ctx context.Context, id int) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !s.Hub.Idle() || s.recordingSoon(ctx) {
			return fmt.Errorf("tuners are busy")
		}
		lock, err := s.Hub.Measure(ctx, int64(id))
		if err != nil {
			log.Printf("signal: channel %d: %v", id, err)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return nil
		}
		full, err := s.Store.SourceChannel(ctx, int64(id))
		if err != nil || full.FrequencyHz <= 0 {
			return nil
		}
		if err := s.Store.SaveFrequencySignal(ctx, full.FrequencyHz, lock.Locked, lock.Strength, lock.Quality, lock.Symbol, time.Now()); err != nil {
			log.Printf("signal: save %d: %v", full.FrequencyHz, err)
		}
		verdict, _ := lock.Verdict()
		log.Printf("signal: %s %s strength %d quality %d symbols %d", full.GuideNumber, verdict, lock.Strength, lock.Quality, lock.Symbol)
		return nil
	})
}

var signalMu sync.Mutex

func (s *Server) signalRunning() bool {
	signalMu.Lock()
	defer signalMu.Unlock()
	return s.signalOn
}

func (s *Server) startSignalScan() bool {
	signalMu.Lock()
	defer signalMu.Unlock()
	if s.signalOn {
		return false
	}
	s.signalOn = true
	return true
}

func (s *Server) finishSignalScan() {
	signalMu.Lock()
	s.signalOn = false
	signalMu.Unlock()
}
