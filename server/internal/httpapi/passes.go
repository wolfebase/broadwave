package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/store"
)

func (s *Server) passes(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Passes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Pass{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"passes": list})
}

func (s *Server) addPass(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	title, _ := body["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		httpError(w, "title required", http.StatusBadRequest)
		return
	}
	channelID := int64(num(body["channelId"]))
	before, after := 1, 2
	if v, ok := body["padBefore"]; ok {
		before = clampPad(int(num(v)))
	}
	if v, ok := body["padAfter"]; ok {
		after = clampPad(int(num(v)))
	}
	if v, ok := body["airingStart"].(string); ok && v != "" {
		start, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httpError(w, "invalid airingStart", http.StatusBadRequest)
			return
		}
		if channelID == 0 {
			httpError(w, "channelId required to record one airing", http.StatusBadRequest)
			return
		}
		if err := s.Store.AddOncePass(r.Context(), title, channelID, start, before, after); err != nil {
			writeError(w, err)
			return
		}
		s.passes(w, r)
		return
	}
	pass, msg := newSeriesPass(title, body)
	if msg != "" {
		httpError(w, msg, http.StatusBadRequest)
		return
	}
	if _, err := s.Store.AddSeriesPass(r.Context(), pass); err != nil {
		writeError(w, err)
		return
	}
	s.passes(w, r)
}

func newSeriesPass(title string, body map[string]any) (store.Pass, string) {
	pass := store.Pass{Title: title, Kind: "series", PadBefore: 1, PadAfter: 2,
		Episodes: "all", KeepMode: "all", MatchKind: "title", Commercials: true}
	msg := applyPassRules(&pass, body)
	pass.Title = title
	return pass, msg
}

func (s *Server) updatePass(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid pass", http.StatusBadRequest)
		return
	}
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	current, err := s.passByID(r.Context(), id)
	if err != nil {
		httpError(w, "pass not found", http.StatusNotFound)
		return
	}
	if msg := applyPassRules(&current, body); msg != "" {
		httpError(w, msg, http.StatusBadRequest)
		return
	}
	current.ID = id
	if err := s.Store.UpdatePassRules(r.Context(), current); err != nil {
		httpError(w, "pass not found", http.StatusNotFound)
		return
	}
	s.passes(w, r)
}

// applyPassRules copies the rules named in body onto pass and returns a
// message for the viewer when one is out of range. A once pass takes pads,
// priority, and commercial marking: keep rules would delete other recordings
// of the title, and the rest would move it. A team pass keeps its team.
func applyPassRules(pass *store.Pass, body map[string]any) string {
	orig := *pass
	// A rename has its own field: Apple sends the title it last loaded with every change.
	if v, ok := body["rename"].(string); ok && strings.TrimSpace(v) != "" {
		pass.Title = strings.TrimSpace(v)
	}
	if v, ok := body["padBefore"]; ok {
		pass.PadBefore = clampPad(int(num(v)))
	}
	if v, ok := body["padAfter"]; ok {
		pass.PadAfter = clampPad(int(num(v)))
	}
	if v, ok := body["priority"]; ok {
		pass.Priority = clampPriority(int(num(v)))
	}
	if v, ok := body["episodes"].(string); ok && v != "" {
		if v != "all" && v != "new" {
			return "Episodes is all or new."
		}
		pass.Episodes = v
	}
	if v, ok := body["keepMode"].(string); ok && v != "" {
		if v != "all" && v != "unwatched" && v != "last" {
			return "Keep is all, unwatched, or last."
		}
		pass.KeepMode = v
	}
	if v, ok := body["keepCount"]; ok {
		pass.KeepCount = clampCount(int(num(v)))
	}
	if v, ok := body["limitCount"]; ok {
		pass.LimitCount = clampCount(int(num(v)))
	}
	if v, ok := body["rerecord"].(bool); ok {
		pass.Rerecord = v
	}
	if v, ok := body["commercials"].(bool); ok {
		pass.Commercials = v
	}
	if v, ok := body["timeStart"].(string); ok {
		pass.TimeStart = strings.TrimSpace(v)
	}
	if v, ok := body["timeEnd"].(string); ok {
		pass.TimeEnd = strings.TrimSpace(v)
	}
	_, hasStart := body["timeStart"]
	_, hasEnd := body["timeEnd"]
	// Only a window sent now is checked, so an older one doesn't block other changes.
	if hasStart || hasEnd {
		if !validClock(pass.TimeStart) || !validClock(pass.TimeEnd) {
			return "Times are HH:MM, from 00:00 to 23:59."
		}
		if (pass.TimeStart == "") != (pass.TimeEnd == "") {
			return "Set both a start and an end time, or neither."
		}
		if pass.TimeStart != "" && pass.TimeStart == pass.TimeEnd {
			return "The start and end time are the same."
		}
	}
	if v, ok := body["days"]; ok {
		list, isList := v.([]any)
		if v != nil && !isList {
			return "Days is a list of weekdays, 0 for Sunday to 6 for Saturday."
		}
		days := []int{}
		for _, d := range list {
			n, isNum := d.(float64)
			if !isNum || n != float64(int(n)) || n < 0 || n > 6 {
				return "Days is a list of weekdays, 0 for Sunday to 6 for Saturday."
			}
			days = append(days, int(n))
		}
		pass.Days = store.DaysOf(store.DaysMask(days))
	}
	if v, ok := body["matchKind"].(string); ok && v != "" {
		if v != "title" && v != "contains" && v != "category" {
			return "Match is title, contains, or category."
		}
		pass.MatchKind = v
	}
	if v, ok := body["channelId"]; ok {
		pass.ChannelID = int64(num(v))
	}
	switch orig.Kind {
	case "once":
		edited := *pass
		*pass = orig
		pass.PadBefore, pass.PadAfter, pass.Priority, pass.Commercials = edited.PadBefore, edited.PadAfter, edited.Priority, edited.Commercials
	case "team":
		pass.Title, pass.MatchKind = orig.Title, orig.MatchKind
	}
	return ""
}

func validClock(v string) bool {
	if v == "" {
		return true
	}
	if len(v) != 5 || v[2] != ':' {
		return false
	}
	h, err1 := strconv.Atoi(v[:2])
	m, err2 := strconv.Atoi(v[3:])
	return err1 == nil && err2 == nil && h >= 0 && h <= 23 && m >= 0 && m <= 59
}

func clampCount(n int) int {
	if n < 0 {
		return 0
	}
	if n > 99 {
		return 99
	}
	return n
}

// orderPasses ranks every pass in the order given, first highest.
func (s *Server) orderPasses(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	list, err := s.Store.Passes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	// A once pass ends on its own between a list and its reorder, so ids that
	// are gone are skipped, and a pass not named ranks last.
	have := map[int64]bool{}
	for _, pass := range list {
		have[pass.ID] = true
	}
	ids := []int64{}
	for _, id := range body.IDs {
		if have[id] {
			ids = append(ids, id)
			delete(have, id)
		}
	}
	for _, pass := range list {
		if have[pass.ID] {
			ids = append(ids, pass.ID)
		}
	}
	if err := s.Store.SetPassOrder(r.Context(), ids); err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, err)
		return
	}
	s.passes(w, r)
}

// previewPass plans the next 14 days as if the pass in the body were saved:
// the airings it would record or skip, and the recordings of other passes it
// would push out.
func (s *Server) previewPass(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	passes, err := s.Store.Passes(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	var pass store.Pass
	id := int64(num(body["id"]))
	if id != 0 {
		found := false
		for _, p := range passes {
			if p.ID == id {
				pass, found = p, true
			}
		}
		if !found {
			httpError(w, "pass not found", http.StatusNotFound)
			return
		}
		if msg := applyPassRules(&pass, body); msg != "" {
			httpError(w, msg, http.StatusBadRequest)
			return
		}
	} else {
		title, _ := body["title"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			httpError(w, "title required", http.StatusBadRequest)
			return
		}
		var msg string
		pass, msg = newSeriesPass(title, body)
		if msg != "" {
			httpError(w, msg, http.StatusBadRequest)
			return
		}
		// A new pass has no id yet; this one can't collide with a saved pass.
		pass.ID = -1
	}
	before, err := s.planWith(ctx, passes)
	if err != nil {
		writeError(w, err)
		return
	}
	after := make([]store.Pass, 0, len(passes)+1)
	for _, p := range passes {
		if p.ID != pass.ID {
			after = append(after, p)
		}
	}
	after = append(after, pass)
	// Saved passes come ordered by priority, so the pass claims airings as it will once saved.
	sortByPriority(after)
	snap, err := s.planWith(ctx, after)
	if err != nil {
		writeError(w, err)
		return
	}
	recording := map[string]bool{}
	for _, item := range before.items {
		if !item.Skipped {
			recording[plannedKey(item)] = true
		}
	}
	items := []dvr.Planned{}
	bumps := []dvr.Planned{}
	for _, item := range snap.items {
		switch {
		case item.PassID == pass.ID:
			items = append(items, item)
		case item.Skipped && item.Conflict && recording[plannedKey(item)]:
			bumps = append(bumps, item)
		}
	}
	zone, offset := time.Now().Zone()
	if name := time.Local.String(); name != "Local" {
		zone = name
	}
	writeJSON(w, http.StatusOK, map[string]any{"tunerCount": snap.count, "items": items, "bumps": bumps, "timeZone": zone, "utcOffset": offset})
}

func plannedKey(item dvr.Planned) string {
	return fmt.Sprintf("%d|%d|%d", item.PassID, item.Airing.ChannelID, item.Airing.Start.Unix())
}

// sortByPriority orders passes as the store lists them: priority, then title
// without case, then id, with an unsaved pass (id -1) after the saved ones.
func sortByPriority(passes []store.Pass) {
	sort.SliceStable(passes, func(i, j int) bool {
		a, b := passes[i], passes[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if x, y := strings.ToLower(a.Title), strings.ToLower(b.Title); x != y {
			return x < y
		}
		return uint64(a.ID) < uint64(b.ID)
	})
}

func (s *Server) passByID(ctx context.Context, id int64) (store.Pass, error) {
	list, err := s.Store.Passes(ctx)
	if err != nil {
		return store.Pass{}, err
	}
	for _, pass := range list {
		if pass.ID == id {
			return pass, nil
		}
	}
	return store.Pass{}, sql.ErrNoRows
}
