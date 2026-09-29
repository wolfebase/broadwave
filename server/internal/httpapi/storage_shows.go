package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/nfo"
	"broadwave/internal/store"
)

type storageShow struct {
	Title  string       `json:"title"`
	Count  int          `json:"count"`
	Bytes  int64        `json:"bytes"`
	Oldest time.Time    `json:"oldest"`
	Newest time.Time    `json:"newest"`
	Pass   *storageKeep `json:"pass,omitempty"`
}

type storageKeep struct {
	ID   int64 `json:"id"`
	Keep int   `json:"keep"`
}

func (s *Server) storageShows(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil || s.Hub.Dir == "" {
		httpError(w, "player is not configured", http.StatusServiceUnavailable)
		return
	}
	recs, err := s.Store.Recordings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	passes, err := s.Store.Passes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"shows": groupShows(filepath.Join(s.Hub.Dir, "recordings"), recs, passes),
	})
}

// groupShows totals finished recordings whose files stay in the recordings
// folder. A recording still in progress and a library folder are left out.
// Titles match without case. The newest recording's title is the one shown.
func groupShows(root string, recs []store.Recording, passes []store.Pass) []storageShow {
	type acc struct {
		title          string
		count          int
		bytes          int64
		oldest, newest time.Time
		items          []store.Recording
	}
	by := map[string]*acc{}
	for _, rec := range recs {
		if rec.Status != "complete" {
			continue
		}
		title := strings.TrimSpace(rec.Title)
		if title == "" || !nfo.Inside(root, rec.Path) {
			continue
		}
		key := strings.ToLower(title)
		row := by[key]
		if row == nil {
			row = &acc{title: title, oldest: rec.StartedAt, newest: rec.StartedAt}
			by[key] = row
		}
		row.count++
		row.bytes += fileBytes(rec.Path)
		row.items = append(row.items, rec)
		if rec.StartedAt.Before(row.oldest) {
			row.oldest = rec.StartedAt
		}
		if rec.StartedAt.After(row.newest) {
			row.newest = rec.StartedAt
			row.title = title
		}
	}
	out := make([]storageShow, 0, len(by))
	for _, row := range by {
		out = append(out, storageShow{
			Title:  row.title,
			Count:  row.count,
			Bytes:  row.bytes,
			Oldest: row.oldest,
			Newest: row.newest,
			Pass:   showPass(row.items, passes),
		})
	}
	slices.SortFunc(out, func(a, b storageShow) int {
		if a.Bytes != b.Bytes {
			if a.Bytes > b.Bytes {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
	return out
}

func fileBytes(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	return info.Size()
}

// showPass is the pass that keeps every recording in the group. An exact title
// wins over a contains or category rule, and a numbered keep wins over keeping
// all of them. Once and team passes do not keep a library.
func showPass(items []store.Recording, passes []store.Pass) *storageKeep {
	var best *store.Pass
	bestRank := -1
	for i := range passes {
		p := passes[i]
		if strings.EqualFold(p.Kind, "once") || strings.EqualFold(p.Kind, "team") {
			continue
		}
		if !coversShow(p, items) {
			continue
		}
		rank := 0
		if exactTitle(p) {
			rank += 2
		}
		if strings.EqualFold(strings.TrimSpace(p.KeepMode), "last") {
			rank++
		}
		if best == nil || rank > bestRank || (rank == bestRank && p.ID < best.ID) {
			chosen := p
			best = &chosen
			bestRank = rank
		}
	}
	if best == nil {
		return nil
	}
	return &storageKeep{ID: best.ID, Keep: keepLimit(*best)}
}

func coversShow(pass store.Pass, items []store.Recording) bool {
	if len(items) == 0 {
		return false
	}
	for _, rec := range items {
		if !dvr.SameShow(pass, rec) {
			return false
		}
	}
	return true
}

func exactTitle(pass store.Pass) bool {
	kind := strings.ToLower(strings.TrimSpace(pass.MatchKind))
	return kind == "" || kind == "title"
}

func keepLimit(pass store.Pass) int {
	if !strings.EqualFold(strings.TrimSpace(pass.KeepMode), "last") {
		return 0
	}
	if pass.KeepCount < 1 {
		return 1
	}
	return pass.KeepCount
}
