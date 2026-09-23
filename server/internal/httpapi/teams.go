package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"waveguide/internal/dvr"
	"waveguide/internal/store"
)

func (s *Server) teams(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var team store.TeamFollow
		if err := decodeJSON(r, &team); err != nil || team.Name == "" {
			httpError(w, "a team needs a name", http.StatusBadRequest)
			return
		}
		if err := s.Store.FollowTeam(r.Context(), team); err != nil {
			writeError(w, err)
			return
		}
	}
	list, err := s.Store.TeamFollows(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"teams": list})
}

func (s *Server) unfollowTeam(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid team", http.StatusBadRequest)
		return
	}
	if err := s.Store.UnfollowTeam(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	s.teams(w, r)
}

func (s *Server) NoteTeams(ctx context.Context) {
	if s == nil || s.Store == nil {
		return
	}
	follows, err := s.Store.TeamFollows(ctx)
	if err != nil || len(follows) == 0 {
		return
	}
	now := time.Now()
	airings, err := s.Store.Airings(ctx, now, now.Add(36*time.Hour))
	if err != nil {
		return
	}
	for _, team := range follows {
		airing, note, ok := dvr.TeamNotice(team, airings, now)
		if !ok {
			continue
		}
		if err := s.Store.AddEvent(ctx, "sports", note); err != nil {
			continue
		}
		_ = s.Store.SetTeamNotice(ctx, team.ID, strconv.FormatInt(airing.ID, 10))
	}
}
