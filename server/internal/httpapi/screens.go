package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

// Screen is one app open on the event socket that another screen can send a
// channel to.
type Screen struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ChannelID int64  `json:"channelId,omitempty"`
}

func (s *Server) screens(w http.ResponseWriter, r *http.Request) {
	list := []Screen{}
	if s.Bus != nil {
		for _, p := range s.Bus.Screens() {
			if p.ID == "" {
				continue
			}
			list = append(list, Screen{ID: p.ID, Name: p.Name, Kind: p.Kind, ChannelID: p.ChannelID})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"screens": list})
}

// screenWatch tells one screen to play a channel. It joins the channel's room
// like any other screen, so it lands on the same moment as the sender.
func (s *Server) screenWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChannelID int64  `json:"channelId"`
		From      string `json:"from"`
	}
	if err := decodeJSON(r, &body); err != nil || body.ChannelID <= 0 {
		httpError(w, "Send a channelId.", http.StatusBadRequest)
		return
	}
	ch, err := s.Store.Channel(r.Context(), body.ChannelID)
	if errors.Is(err, sql.ErrNoRows) {
		httpError(w, "channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if s.Bus == nil {
		httpError(w, "Live updates are not available.", http.StatusServiceUnavailable)
		return
	}
	// The name is shown on another screen: no control characters, 64 at most.
	from := []rune(strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, body.From)))
	event := map[string]any{"channelId": ch.ID, "from": string(from[:min(len(from), 64)])}
	if s.Bus.SendTo(r.PathValue("id"), "screen.watch", event) == 0 {
		httpError(w, "That screen isn't open right now.", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
