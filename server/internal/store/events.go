package store

import (
	"context"
	"strings"
	"time"
)

type Event struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
}

func (s *Store) AddEvent(ctx context.Context, kind, message string) error {
	kind = clip(kind, 40)
	message = clip(strings.TrimSpace(message), 240)
	if kind == "" || message == "" {
		return nil
	}
	if kind == "guide" {
		recent, err := s.Events(ctx, 5)
		if err == nil {
			for _, ev := range recent {
				if ev.Kind == "guide" && ev.Message == message && time.Since(ev.At) < 15*time.Minute {
					return nil
				}
			}
		}
	}
	return s.AddEventAt(ctx, time.Now().UTC(), kind, message)
}

// AddEventAt records an activity line at a chosen time. The live path uses AddEvent.
func (s *Store) AddEventAt(ctx context.Context, at time.Time, kind, message string) error {
	kind = clip(kind, 40)
	message = clip(strings.TrimSpace(message), 240)
	if kind == "" || message == "" {
		return nil
	}
	at = at.UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO events (at, kind, message) VALUES (?, ?, ?)`, at.Format(time.RFC3339), kind, message)
	if err != nil {
		return err
	}
	if s.OnEvent != nil {
		id, _ := res.LastInsertId()
		s.OnEvent(Event{ID: id, At: at, Kind: kind, Message: message})
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 200)`)
	return err
}

func (s *Store) Events(ctx context.Context, limit int) ([]Event, error) {
	if limit < 1 || limit > 200 {
		limit = 40
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, at, kind, message FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var ev Event
		var at string
		if err := rows.Scan(&ev.ID, &at, &ev.Kind, &ev.Message); err != nil {
			return nil, err
		}
		ev.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func clip(value string, n int) string {
	value = strings.TrimSpace(value)
	if len(value) <= n {
		return value
	}
	return value[:n]
}
