package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/dvr"
	"broadwave/internal/guide"
	"broadwave/internal/live"
	"broadwave/internal/realtime"
	"broadwave/internal/store"
)

type watchBody struct {
	ChannelID   int64      `json:"channelId"`
	Caps        *live.Caps `json:"caps"`
	Prefs       live.Prefs `json:"prefs"`
	Rendition   string     `json:"rendition"`
	Profile     string     `json:"profile"`
	Audio       string     `json:"audio"`
	Picture     string     `json:"pictureMode"`
	ConfirmLive bool       `json:"confirmLive"`
	// Room is the sync room a player that starts on the room's frame joins.
	Room string `json:"room"`
}

// decide picks the rendition a watch plays: the one the player named, or the
// stream decision for its capabilities.
func (s *Server) decide(ctx context.Context, body watchBody) (live.Decision, bool, error) {
	src, err := s.Hub.SourceOf(ctx, body.ChannelID)
	if err != nil {
		return live.Decision{}, false, err
	}
	if forced, chosen := live.ParseRenditionKey(body.Rendition); chosen {
		return s.unvoiced(src, live.Decision{Rendition: forced, Reason: "Chosen in the player"}), true, nil
	}
	caps, prefs := live.LegacyCaps(body.Profile, body.Audio, body.Picture)
	if body.Caps != nil {
		caps, prefs = *body.Caps, body.Prefs
	}
	if prefs.Picture == "" {
		_, prefs.Picture, _ = s.playbackChoice(ctx, 0, "")
	}
	return s.unvoiced(src, live.DecideFor(src, caps, prefs, s.Hub.Encoder, s.Hub.Host)), false, nil
}

const noAC4Reason = "No sound: this server's ffmpeg can't decode ATSC 3.0 sound (AC-4)."

// unvoiced plays an AC-4 channel's picture alone on an ffmpeg that cannot
// decode its sound, as the hub will, and says why.
func (s *Server) unvoiced(src live.Source, d live.Decision) live.Decision {
	if !s.Hub.NoAC4 {
		return d
	}
	if r := live.Unvoiced(src.AudioCodec, d.Rendition); r != d.Rendition {
		d.Rendition = r
		d.Reason = noAC4Reason
	}
	return d
}

// ranAs says why the hub played another encode than the one decided, or ""
// when the decision's reason still holds.
func ranAs(noAC4 bool, want live.Rendition, session live.Session, alternates bool) string {
	// A player that switches sound in place plays the main encode, which carries its track.
	main := want
	main.Track = ""
	inPlace := alternates && want.Key() != main.Key() && session.Rendition == main.Key()
	switch {
	case noAC4 && session.Stream.Audio == "none" && live.Unvoiced(session.Stream.SourceAudio, want) != want:
		// A link's codecs are learned when it opens, and AC-4 found then
		// plays silent.
		return noAC4Reason
	case session.Rendition != want.Key() && !inPlace && session.Stream.Video != "" && session.Stream.Video != "copy":
		return "Playing the " + session.Stream.Video + "p picture already running."
	}
	return ""
}

func (s *Server) watch(w http.ResponseWriter, r *http.Request) {
	asked := time.Now()
	if s.Hub == nil {
		httpError(w, "Live TV is not set up on this server.", http.StatusServiceUnavailable)
		return
	}
	var body watchBody
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	decision, chosen, err := s.decide(r.Context(), body)
	if err != nil {
		watchError(w, err)
		return
	}
	if !body.ConfirmLive {
		if msg := s.liveWarning(r.Context(), body.ChannelID); msg != "" {
			apiError(w, http.StatusConflict, "recording_soon", msg, nil)
			return
		}
	}
	alternates := body.Caps != nil && body.Caps.Alternates
	room, inRoom := s.roomOf(body)
	var frame time.Time
	if inRoom {
		frame = roomFrame(room, time.Now())
	}
	// A rendition the player named is played as named.
	session, err := s.Hub.WatchAt(r.Context(), body.ChannelID, decision.Rendition, alternates && !chosen, frame)
	if err != nil {
		watchError(w, err)
		return
	}
	session.Stream.Reason = decision.Reason
	if ch, err := s.Store.SourceChannel(r.Context(), body.ChannelID); err == nil && ch.PlaysAs != 0 {
		session.Stream.Reason = "The 3.0 version is encrypted. Showing the regular broadcast."
	}
	// The viewer is counted before a segment exists. A channel change closes
	// this request; waiting out the deadline would keep that tuner.
	if r.Context().Err() != nil {
		s.Hub.Release(session.ChannelID, session.Rendition)
		return
	}
	dark := waitServable(r.Context(), s.Hub, session.ChannelID, session.Rendition, 12*time.Second)
	if r.Context().Err() != nil || dark {
		s.Hub.Release(session.ChannelID, session.Rendition)
		if dark {
			s.Hub.DropDark(session.ChannelID)
			writeError(w, live.ErrNoSignal)
		}
		return
	}
	session.Rendition = s.Hub.Current(session.ChannelID, session.Rendition)
	if inRoom && s.Hub.FromBuffer(session.ChannelID, session.Rendition) {
		waitFrame(r.Context(), s.Hub, session.ChannelID, session.Rendition, room, roomFrameWait)
		if r.Context().Err() != nil {
			s.Hub.Release(session.ChannelID, session.Rendition)
			return
		}
	}
	if steps := s.Hub.NoteStart(session.ChannelID, session.Rendition, asked); steps != "" {
		slog.Info(steps)
	}
	if fresh, ok := s.Hub.Session(session.ChannelID, session.Rendition); ok {
		reason := session.Stream.Reason
		fresh.Tuners = session.Tuners
		session = fresh
		session.Stream.Reason = reason
	}
	if why := ranAs(s.Hub.NoAC4, decision.Rendition, session, alternates); why != "" {
		session.Stream.Reason = why
	}
	// A master names its default sound as the main one, so an encode that
	// carries another track first keeps its one-sound playlist.
	if spec, ok := live.ParseRenditionKey(session.Rendition); alternates && ok && spec.Track == "" {
		if master, ok := s.Hub.MasterPath(session.ChannelID, session.Rendition); ok {
			session.MainPlaylist = master
		}
	}
	// Tuner status is a separate request. Reading it here holds the hub lock
	// after the first segment already exists, so the player cannot start.
	writeJSON(w, http.StatusOK, watchReply{Session: session, Boot: s.boot()})
	if r.Context().Err() != nil {
		s.Hub.Release(session.ChannelID, session.Rendition)
	}
}

// watchReply names the server process with the session, so the stop can
// say which process counted the viewer.
type watchReply struct {
	live.Session
	Boot string `json:"boot,omitempty"`
}

func (s *Server) boot() string {
	if s.Bus == nil {
		return ""
	}
	return s.Bus.Boot
}

// warm starts the picture a player is about to ask for, when the channel's
// frequency is already tuned and the picture budget has room. It counts no
// viewer; the next watch joins it.
func (s *Server) warm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid channel", http.StatusBadRequest)
		return
	}
	if s.Hub == nil {
		writeJSON(w, http.StatusOK, map[string]any{"warm": false})
		return
	}
	var body watchBody
	if err := decodeJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.ChannelID = id
	decision, chosen, err := s.decide(r.Context(), body)
	if err != nil {
		watchError(w, err)
		return
	}
	alternates := body.Caps != nil && body.Caps.Alternates
	ok, err := s.Hub.Warm(r.Context(), id, decision.Rendition, alternates && !chosen)
	if err != nil {
		watchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"warm": ok})
}

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid channel", http.StatusBadRequest)
		return
	}
	var body struct {
		Rendition string `json:"rendition"`
		Boot      string `json:"boot"`
	}
	_ = decodeJSON(r, &body)
	// A watch counted by a process that has since restarted is gone with it.
	// Releasing by rendition here would take someone else's viewer.
	if s.Hub != nil && (body.Boot == "" || body.Boot == s.boot()) {
		s.Hub.Release(id, body.Rendition)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) tuners(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		writeJSON(w, http.StatusOK, map[string]any{"tuners": []live.Tuner{}})
		return
	}
	list, err := s.Hub.Tuners(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []live.Tuner{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tuners": list, "encoder": s.Hub.Encoder})
}

func (s *Server) startRecording(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChannelID int64  `json:"channelId"`
		Minutes   int    `json:"minutes"`
		Title     string `json:"title"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	meta := store.Recording{ChannelID: body.ChannelID, Title: strings.TrimSpace(body.Title)}
	minutes := body.Minutes
	if s.Hub != nil && s.Hub.Dir != "" {
		if err := disk.Writable(s.Hub.Recordings()); err != nil {
			writeError(w, err)
			return
		}
	}
	if air, ok := s.listingFor(r.Context(), body.ChannelID, meta.Title); ok {
		meta.Title = air.Title
		meta.Subtitle = air.Subtitle
		meta.Description = air.Description
		meta.Category = air.Category
		meta.ProgramID = air.ProgramID
		meta.GameID = air.GameID
		// The recording starts from the show's beginning when the tuner was
		// already on it and the buffer still holds it.
		meta.StartedAt = air.Start
		pad := 2 + int(dvr.SportsTail(air)/time.Minute)
		left := int(time.Until(air.End.Add(time.Duration(pad)*time.Minute)).Minutes()) + 1
		if left > minutes {
			minutes = left
		}
	}
	rec, err := s.Hub.RecordMeta(r.Context(), minutes, meta)
	if err != nil {
		writeError(w, disk.ClassifyWrite(err))
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) stopRecording(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	s.Hub.StopRecord(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) recordings(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Recordings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Recording{}
	}
	for i := range list {
		if info, err := os.Stat(list[i].Path); err == nil {
			list[i].Bytes = info.Size()
		} else if fileGone(list[i]) {
			// Its stored length is not a file anyone can play.
			list[i].Missing = true
			list[i].Duration = 0
			continue
		}
		if list[i].Duration == 0 && list[i].Status != "recording" && list[i].Path != "" && s.Hub != nil {
			if tool := live.FFProbePath(s.Hub.FFmpeg); tool != "" {
				seconds, err := live.ProbeDuration(tool, list[i].Path)
				if err != nil || seconds <= 0 {
					_ = s.Store.SetDuration(r.Context(), list[i].ID, -1)
				} else {
					_ = s.Store.SetDuration(r.Context(), list[i].ID, seconds)
					list[i].Duration = seconds
				}
			}
		}
		if list[i].Duration < 0 {
			list[i].Duration = 0
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"recordings": list})
}

func (s *Server) deleteRecording(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, "invalid recording", http.StatusBadRequest)
		return
	}
	rec, err := s.Store.Recording(r.Context(), id)
	if err != nil {
		httpError(w, "recording not found", http.StatusNotFound)
		return
	}
	if rec.Status == "recording" {
		httpError(w, "stop the recording before deleting it", http.StatusConflict)
		return
	}
	if s.Hub != nil {
		s.Hub.StopRecord(id)
		s.Hub.RemoveRecordingFiles(rec)
	}
	if err := s.Store.DeleteRecording(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	_ = s.Store.AddEvent(r.Context(), "delete", "Deleted "+rec.Title)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Events(r.Context(), 40)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": list})
}

func (s *Server) airings(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	from := now.Add(-30 * time.Minute)
	to := now.Add(48 * time.Hour)
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := parseGuideTime(raw)
		if err != nil {
			httpError(w, "from needs to be a time", http.StatusBadRequest)
			return
		}
		from = parsed
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := parseGuideTime(raw)
		if err != nil {
			httpError(w, "to needs to be a time", http.StatusBadRequest)
			return
		}
		to = parsed
	} else if raw := r.URL.Query().Get("hours"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 168 {
			to = now.Add(time.Duration(n) * time.Hour)
		}
	}
	if !to.After(from) {
		httpError(w, "The guide window is backwards.", http.StatusBadRequest)
		return
	}
	list, err := s.Store.Airings(r.Context(), from, to)
	if err != nil {
		writeError(w, err)
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("channels")); raw != "" {
		want := map[int64]bool{}
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil {
				httpError(w, "channels needs to be a list of ids", http.StatusBadRequest)
				return
			}
			want[id] = true
		}
		filtered := make([]store.Airing, 0, len(list))
		for _, row := range list {
			if want[row.ChannelID] {
				filtered = append(filtered, row)
			}
		}
		list = filtered
	}
	if list == nil {
		list = []store.Airing{}
	}
	writeCachedJSON(w, r, http.StatusOK, map[string]any{"airings": list})
}

func parseGuideTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, raw)
}

func (s *Server) refreshGuide(w http.ResponseWriter, r *http.Request) {
	_, _, lastManual, err := s.Store.GuideSchedule(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if ok, retry := guide.ManualAllowed(lastManual, time.Now()); !ok {
		mins := int(time.Until(retry).Minutes()) + 1
		if mins < 1 {
			mins = 1
		}
		apiError(w, http.StatusTooManyRequests, "guide_rate_limited", fmt.Sprintf("Listings were just refreshed. Try again in %d minutes.", mins), map[string]any{"retryAt": retry.UTC()})
		return
	}
	if err := s.Store.SetManualGuidePull(r.Context(), time.Now()); err != nil {
		writeError(w, err)
		return
	}
	pull := s.RefreshGuide
	if s.GuidePull != nil {
		pull = s.GuidePull
	}
	n, err := pull(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"airings": n})
}

// GuideDelay is how long the automatic refresh should wait. Zero means pull now.
func (s *Server) GuideDelay(now time.Time) time.Duration {
	_, next, _, err := s.Store.GuideSchedule(context.Background())
	if err != nil {
		return 0
	}
	return guide.Delay(next, now)
}

// DeferGuide schedules another attempt after a failed pull without counting it as a success.
func (s *Server) DeferGuide(ctx context.Context, after time.Duration) {
	_ = s.Store.SetNextGuidePull(ctx, time.Now().Add(after))
}

func (s *Server) RefreshGuide(ctx context.Context) (int, error) {
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		return 0, err
	}
	if len(devices) == 0 {
		return 0, errors.New("no tuner")
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return 0, err
	}
	var antenna []store.Channel
	var ids []int64
	for _, ch := range channels {
		if strings.HasPrefix(ch.DeviceID, "src-") {
			continue
		}
		antenna = append(antenna, ch)
		ids = append(ids, ch.ID)
	}
	settings, _ := s.Store.Settings(ctx)
	if settings == nil {
		settings = map[string]string{}
	}
	tmdbKey := strings.TrimSpace(settings["tmdbKey"])
	if tmdbKey == "" {
		tmdbKey = strings.TrimSpace(os.Getenv("TMDB_API_KEY"))
	}
	// A failed SiliconDust pull still lets Schedules Direct and a guide
	// address fill the guide; its own listings stay until the next pull.
	var rows []store.Airing
	var bases []string
	for _, d := range devices {
		if !strings.HasPrefix(d.DeviceID, "src-") && d.TunerCount > 0 {
			bases = append(bases, d.BaseURL)
		}
	}
	raw, pullErr := guide.Pull(ctx, s.HDHR, bases...)
	if pullErr == nil {
		var art map[int64]string
		rows, art, pullErr = guide.Parse(raw, antenna)
		if pullErr == nil {
			rows = guide.ShareSame(guide.ShareTwins(rows, art, antenna), art, antenna)
			_ = s.Store.SetChannelArt(ctx, art)
			_ = s.Store.SetNetworks(ctx, guide.Networks(raw, antenna))
			rows = guide.FillImages(ctx, tmdbKey, rows)
			rows = tagGuideSource(rows, "silicondust")
			if err := s.Store.ReplaceAiringsFor(ctx, ids, rows); err != nil {
				return 0, err
			}
		}
	}
	user := strings.TrimSpace(settings["sdUser"])
	pass := settings["sdPassword"]
	lineup := strings.TrimSpace(settings["sdLineup"])
	if user == "" {
		user = strings.TrimSpace(os.Getenv("SD_USERNAME"))
	}
	if pass == "" {
		pass = os.Getenv("SD_PASSWORD")
	}
	if lineup == "" {
		lineup = strings.TrimSpace(os.Getenv("SD_LINEUP"))
	}
	if extra, _, err := guide.SchedulesDirect(ctx, antenna, user, pass, lineup); err == nil && len(extra) > 0 {
		extra = guide.ShareSame(guide.ShareTwins(extra, nil, antenna), nil, antenna)
		rows = s.fillUnlisted(ctx, rows, tagGuideSource(extra, "schedules-direct"))
	}
	if rawURL := strings.TrimSpace(settings["guideUrl"]); rawURL != "" {
		if body, err := guide.PullURL(ctx, rawURL); err == nil {
			if extra, extraArt, err := guide.Parse(body, antenna); err == nil && len(extra) > 0 {
				extra = guide.ShareSame(guide.ShareTwins(extra, extraArt, antenna), extraArt, antenna)
				_ = s.Store.SetChannelArt(ctx, extraArt)
				_ = s.Store.SetNetworks(ctx, guide.Networks(body, antenna))
				extra = guide.FillImages(ctx, tmdbKey, extra)
				rows = s.fillUnlisted(ctx, rows, tagGuideSource(extra, "xmltv"))
			}
		}
	}
	if pullErr != nil {
		_ = s.Store.SetGuideError(ctx, pullErr.Error(), time.Now())
		if len(rows) == 0 {
			return 0, pullErr
		}
		// The other sources filled the guide. SiliconDust is tried again soon.
		slog.Warn(fmt.Sprintf("guide: SiliconDust: %v; %d airings from the other sources", pullErr, len(rows)))
		s.DeferGuide(ctx, guide.RetryAfterError)
		_ = s.Store.AddEvent(ctx, "guide", fmt.Sprintf("Guide updated, %d airings", len(rows)))
		s.LinkGames(ctx)
		return len(rows), nil
	}
	now := time.Now().UTC()
	span := int64(guide.PullMax - guide.PullMin)
	jitter := time.Duration(rand.Int64N(span + 1))
	next := guide.NextPull(now, jitter)
	if err := s.Store.SetGuideSchedule(ctx, now, next); err != nil {
		return len(rows), err
	}
	listed := map[int64]struct{}{}
	for _, row := range rows {
		listed[row.ChannelID] = struct{}{}
	}
	slog.Info(fmt.Sprintf("guide: source=silicondust-xmltv airings=%d channels=%d next=%s", len(rows), len(listed), next.Format(time.RFC3339)))
	_ = s.Store.AddEvent(ctx, "guide", fmt.Sprintf("Guide updated, %d airings", len(rows)))
	s.LinkGames(ctx)
	return len(rows), nil
}

// fillUnlisted keeps the listings a channel already has and adds rows only for channels that have none.
func (s *Server) fillUnlisted(ctx context.Context, rows, extra []store.Airing) []store.Airing {
	have := map[int64]bool{}
	for _, row := range rows {
		have[row.ChannelID] = true
	}
	var fill []store.Airing
	for _, row := range extra {
		if !have[row.ChannelID] {
			fill = append(fill, row)
		}
	}
	if len(fill) == 0 {
		return rows
	}
	var fillIDs []int64
	seen := map[int64]bool{}
	for _, row := range fill {
		if !seen[row.ChannelID] {
			seen[row.ChannelID] = true
			fillIDs = append(fillIDs, row.ChannelID)
		}
	}
	_ = s.Store.ReplaceAiringsFor(ctx, fillIDs, fill)
	return append(rows, fill...)
}

func (s *Server) schedule(w http.ResponseWriter, r *http.Request) {
	snap, err := s.loadSchedule(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tunerCount": snap.count, "items": snap.items})
}

type scheduleSnap struct {
	items   []dvr.Planned
	passes  []store.Pass
	airings []store.Airing
	count   int
}

func (s *Server) loadSchedule(ctx context.Context) (scheduleSnap, error) {
	passes, err := s.Store.Passes(ctx)
	if err != nil {
		return scheduleSnap{}, err
	}
	return s.planWith(ctx, passes)
}

// planWith plans the next 14 days for the given passes, ordered by priority as the store lists them.
func (s *Server) planWith(ctx context.Context, passes []store.Pass) (scheduleSnap, error) {
	now := time.Now()
	end := now.Add(14 * 24 * time.Hour)
	airings, err := s.Store.RecordingAirings(ctx, now.Add(-time.Minute), end)
	if err != nil {
		return scheduleSnap{}, err
	}
	count := s.tunerCount(ctx)
	recs, _ := s.Store.Recordings(ctx)
	seen, _ := s.Store.SeenDeleted(ctx)
	skips, _ := s.Store.Skips(ctx)
	items := dvr.PlanLibrary(passes, airings, count, now, end, recs, seen, skips)
	items = dvr.AttachSuggestions(items, passes, airings, count, now, end, s.guideNumbers(ctx))
	if items == nil {
		items = []dvr.Planned{}
	}
	return scheduleSnap{items: items, passes: passes, airings: airings, count: count}, nil
}

func (s *Server) fixSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PassID              int64     `json:"passId"`
		ChannelID           int64     `json:"channelId"`
		Start               time.Time `json:"start"`
		SuggestionChannelID int64     `json:"suggestionChannelId"`
		SuggestionStart     time.Time `json:"suggestionStart"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.PassID == 0 || body.ChannelID == 0 || body.Start.IsZero() || body.SuggestionChannelID == 0 || body.SuggestionStart.IsZero() {
		httpError(w, "A pass and an airing are required.", http.StatusBadRequest)
		return
	}
	snap, err := s.loadSchedule(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var item *dvr.Planned
	for i := range snap.items {
		if snap.items[i].PassID == body.PassID && snap.items[i].Airing.ChannelID == body.ChannelID && snap.items[i].Airing.Start.Equal(body.Start) {
			item = &snap.items[i]
			break
		}
	}
	if item == nil || !item.Skipped || item.Suggestion == nil {
		httpError(w, "That showing is not waiting on a tuner.", http.StatusConflict)
		return
	}
	if item.Suggestion.ChannelID != body.SuggestionChannelID || !item.Suggestion.Start.Equal(body.SuggestionStart) {
		httpError(w, "That later airing no longer fits.", http.StatusConflict)
		return
	}
	pass, ok := passIn(snap.passes, body.PassID)
	suggestion, sugOK := airingAt(snap.airings, body.SuggestionChannelID, body.SuggestionStart)
	if !ok || !sugOK {
		httpError(w, "That showing is not waiting on a tuner.", http.StatusConflict)
		return
	}
	fix := dvr.PlanFix(pass, item.Airing, suggestion)
	// Save the replacement before skipping. A failed save must leave the original airing in place.
	if fix.SetChannel != 0 && pass.ChannelID != fix.SetChannel {
		pass.ChannelID = fix.SetChannel
		if err := s.Store.UpdatePassRules(r.Context(), pass); err != nil {
			httpError(w, "pass not found", http.StatusNotFound)
			return
		}
	}
	if fix.OneShot != nil && !dvr.HaveOneShot(snap.passes, suggestion) {
		if err := s.addOneShot(r.Context(), *fix.OneShot); err != nil {
			writeError(w, err)
			return
		}
	}
	if err := s.skipShowing(r.Context(), fix.Skip); err != nil {
		writeError(w, err)
		return
	}
	title := strings.TrimSpace(suggestion.Title)
	if title == "" {
		title = "The show"
	}
	_ = s.Store.AddEvent(r.Context(), "recording", fmt.Sprintf("%s will record on %s instead.", title, suggestion.Start.In(time.Local).Format("Jan 2 at 3:04 PM")))
	s.schedule(w, r)
}

func passIn(passes []store.Pass, id int64) (store.Pass, bool) {
	for _, pass := range passes {
		if pass.ID == id {
			return pass, true
		}
	}
	return store.Pass{}, false
}

func airingAt(airings []store.Airing, channelID int64, start time.Time) (store.Airing, bool) {
	for _, air := range airings {
		if air.ChannelID == channelID && air.Start.Equal(start) {
			return air, true
		}
	}
	return store.Airing{}, false
}

func (s *Server) skipShowing(ctx context.Context, air store.Airing) error {
	key, starts := dvr.SkipParts(air)
	if key == "" {
		return nil
	}
	return s.Store.SkipAiring(ctx, key, starts)
}

func (s *Server) addOneShot(ctx context.Context, shot store.Pass) error {
	before, err := s.Store.Passes(ctx)
	if err != nil {
		return err
	}
	if err := s.Store.AddOncePass(ctx, shot.Title, shot.ChannelID, shot.AiringStart, shot.PadBefore, shot.PadAfter); err != nil {
		return err
	}
	after, err := s.Store.Passes(ctx)
	if err != nil {
		return err
	}
	id := newPassID(before, after)
	if id == 0 {
		return fmt.Errorf("the pass was not saved")
	}
	shot.ID = id
	return s.Store.UpdatePassRules(ctx, shot)
}

func newPassID(before, after []store.Pass) int64 {
	have := map[int64]struct{}{}
	for _, pass := range before {
		have[pass.ID] = struct{}{}
	}
	var id int64
	for _, pass := range after {
		if _, ok := have[pass.ID]; ok {
			continue
		}
		if pass.ID > id {
			id = pass.ID
		}
	}
	return id
}

func (s *Server) guideNumbers(ctx context.Context) map[int64]string {
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return nil
	}
	out := make(map[int64]string, len(channels))
	for _, ch := range channels {
		number := ch.DisplayNumber
		if number == "" {
			number = ch.GuideNumber
		}
		if number != "" {
			out[ch.ID] = number
		}
	}
	return out
}

// liveWarning is empty when this channel can take a tuner without leaving a
// recording short. A channel already on a tuned mux shares that tuner.
func (s *Server) liveWarning(ctx context.Context, channelID int64) string {
	ch, err := s.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return ""
	}
	tuned, busy := s.tunersInUse(ctx, channelID)
	if s.onTunedMux(ctx, ch, tuned) {
		return ""
	}
	allow, warning := dvr.LiveWatch(s.tunerCount(ctx), busy, s.upcomingSoon(ctx, ch, tuned), s.now())
	if allow {
		return ""
	}
	return warning
}

func (s *Server) tunersInUse(ctx context.Context, channelID int64) (map[string]struct{}, int) {
	tuned := map[string]struct{}{}
	busy := 0
	if s.Hub != nil {
		// Status is also read when the tune starts. Don't make a quiet tuner add another long wait.
		statusCtx, cancel := context.WithTimeout(ctx, time.Second)
		list, err := s.Hub.Tuners(statusCtx)
		cancel()
		if err == nil {
			for _, tuner := range list {
				if tuner.Guide == "" && tuner.Target == "" && !tuner.Ours {
					continue
				}
				busy++
				if tuner.Guide != "" {
					tuned[tuner.Guide] = struct{}{}
				}
			}
		}
	}
	return tuned, busy + s.recordingBusy(ctx, channelID, tuned)
}

func (s *Server) recordingBusy(ctx context.Context, except int64, tuned map[string]struct{}) int {
	recs, err := s.Store.Recordings(ctx)
	if err != nil {
		return 0
	}
	seen := map[int64]struct{}{}
	n := 0
	for _, rec := range recs {
		if rec.Status != "recording" || rec.ChannelID == 0 || rec.ChannelID == except {
			continue
		}
		if rec.GuideNumber != "" {
			if _, ok := tuned[rec.GuideNumber]; ok {
				continue
			}
		}
		if _, ok := seen[rec.ChannelID]; ok {
			continue
		}
		seen[rec.ChannelID] = struct{}{}
		n++
	}
	return n
}

func (s *Server) onTunedMux(ctx context.Context, ch store.SourceChannel, tuned map[string]struct{}) bool {
	if ch.GuideNumber != "" {
		if _, ok := tuned[ch.GuideNumber]; ok {
			return true
		}
	}
	if ch.DisplayNumber != "" {
		if _, ok := tuned[ch.DisplayNumber]; ok {
			return true
		}
	}
	if ch.FrequencyHz <= 0 {
		return false
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return false
	}
	for _, other := range channels {
		if other.ID == ch.ID {
			continue
		}
		if _, ok := tuned[other.GuideNumber]; !ok {
			continue
		}
		src, err := s.Store.SourceChannel(ctx, other.ID)
		if err != nil || src.FrequencyHz == 0 {
			continue
		}
		if src.FrequencyHz == ch.FrequencyHz {
			return true
		}
	}
	return false
}

func (s *Server) upcomingSoon(ctx context.Context, watch store.SourceChannel, tuned map[string]struct{}) []dvr.Soon {
	now := s.now()
	passes, err := s.Store.Passes(ctx)
	if err != nil || len(passes) == 0 {
		return nil
	}
	from := now.Add(-time.Minute)
	to := now.Add(31 * time.Minute)
	airings, err := s.Store.RecordingAirings(ctx, from, to)
	if err != nil {
		return nil
	}
	items := dvr.Plan(passes, airings, s.tunerCount(ctx), from, to)
	recs, _ := s.Store.Recordings(ctx)
	seen, _ := s.Store.SeenDeleted(ctx)
	skips, _ := s.Store.Skips(ctx)
	items = dvr.ApplyLibrary(items, passes, recs, seen, skips)
	var out []dvr.Soon
	for _, item := range items {
		if item.Skipped || s.sameMux(ctx, watch, item.Airing.ChannelID, tuned) {
			continue
		}
		out = append(out, dvr.Soon{
			Title:     item.Airing.Title,
			ChannelID: item.Airing.ChannelID,
			Start:     item.Airing.Start,
			Pad:       time.Duration(item.PadBefore) * time.Minute,
		})
	}
	return out
}

func (s *Server) sameMux(ctx context.Context, watch store.SourceChannel, channelID int64, tuned map[string]struct{}) bool {
	if channelID == watch.ID {
		return true
	}
	other, err := s.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return false
	}
	if watch.FrequencyHz > 0 && other.FrequencyHz == watch.FrequencyHz {
		return true
	}
	if other.GuideNumber != "" {
		if _, ok := tuned[other.GuideNumber]; ok {
			return true
		}
	}
	return false
}

func (s *Server) tunerCount(ctx context.Context) int {
	devices, err := s.Store.Devices(ctx)
	if err != nil || len(devices) == 0 {
		return 2
	}
	n := 0
	for _, device := range devices {
		n += device.TunerCount
	}
	if n < 1 {
		return 2
	}
	return n
}

func (s *Server) listingFor(ctx context.Context, channelID int64, title string) (store.Airing, bool) {
	now := time.Now()
	rows, err := s.Store.Airings(ctx, now.Add(-3*time.Hour), now.Add(8*time.Hour))
	if err != nil {
		return store.Airing{}, false
	}
	var current, next *store.Airing
	for i := range rows {
		row := &rows[i]
		if row.ChannelID != channelID {
			continue
		}
		if title != "" && strings.EqualFold(row.Title, title) && row.End.After(now) {
			return *row, true
		}
		if !now.Before(row.Start) && now.Before(row.End) && current == nil {
			current = row
		}
		if row.Start.After(now) && (next == nil || row.Start.Before(next.Start)) {
			next = row
		}
	}
	if current != nil {
		return *current, true
	}
	if next != nil && next.Start.Before(now.Add(2*time.Minute)) {
		return *next, true
	}
	return store.Airing{}, false
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func clampPriority(priority int) int {
	if priority < 0 {
		return 0
	}
	if priority > 1000 {
		return 1000
	}
	return priority
}

func clampPad(minutes int) int {
	if minutes < 0 {
		return 0
	}
	if minutes > 30 {
		return 30
	}
	return minutes
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	if s.Hub == nil {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/media/live/"), "/")
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	channelID, err := strconv.ParseInt(parts[0], 10, 64)
	key, name := parts[1], parts[2]
	if _, ok := live.ParseRenditionKey(key); err != nil || !ok {
		http.NotFound(w, r)
		return
	}
	// A rendition rebuilt under another key keeps the URLs it was joined by.
	key = s.Hub.Touch(channelID, key)
	w.Header().Set("Cache-Control", "no-cache")
	if name == "index.m3u8" {
		if msn, part, ok := blockReload(r); ok {
			s.Hub.WaitBlocking(channelID, key, msn, part)
		}
		body, err := s.Hub.Playlist(channelID, key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("_HLS_skip") {
		case "YES", "v2":
			body = live.DeltaPlaylist(body)
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(body)
		return
	}
	if name == "master.m3u8" {
		body, err := s.Hub.MasterPlaylist(r.Context(), channelID, key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(body)
		return
	}
	if body, ok, err := s.viewPlaylist(r, channelID, key, name); ok {
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(body)
		return
	}
	if body, ok, err := s.Hub.ViewMedia(channelID, key, name); ok {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		contentType := "video/iso.segment"
		switch {
		case strings.HasPrefix(name, "init.a"):
			contentType = "audio/mp4"
		case strings.HasPrefix(name, "init."):
			contentType = "video/mp4"
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
		return
	}
	if captionFile(name) {
		var body []byte
		var err error
		contentType := "application/vnd.apple.mpegurl"
		switch name {
		case "main.m3u8":
			body, err = s.Hub.MainPlaylist(channelID, key)
		case "captions.m3u8":
			// It lists the video's whole segments, so a blocking request
			// waits for that segment; it has no parts.
			if msn, _, ok := blockReload(r); ok {
				s.Hub.WaitBlocking(channelID, key, msn, -1)
			}
			body, err = s.Hub.CaptionPlaylist(channelID, key)
		default:
			contentType = "text/vtt; charset=utf-8"
			body, err = s.Hub.CaptionSegment(channelID, key, name)
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
		return
	}
	var contentType string
	switch {
	case strings.Contains(name, ".."):
	case name == "init.mp4":
		contentType = "video/mp4"
	case (strings.HasPrefix(name, "seg") || strings.HasPrefix(name, "part")) && strings.HasSuffix(name, ".m4s"):
		contentType = "video/iso.segment"
	case strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".ts"):
		contentType = "video/mp2t"
	}
	if contentType == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Hub.Dir, "live", parts[0], key, name)
	if _, err := os.Stat(path); err != nil {
		body, ok := live.PartFromSegment(path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
		return
	}
	w.Header().Set("Content-Type", contentType)
	// A restarted rendition reuses seg00000 and part00000. An hour-long cache
	// would play the previous file under that name.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// viewPlaylist answers a view's playlist with the same blocking reload and
// delta form as index.m3u8.
func (s *Server) viewPlaylist(r *http.Request, channelID int64, key, name string) ([]byte, bool, error) {
	if !live.IsViewPlaylist(name) {
		return nil, false, nil
	}
	if msn, part, ok := blockReload(r); ok {
		s.Hub.WaitBlocking(channelID, key, msn, part)
	}
	skip := false
	switch r.URL.Query().Get("_HLS_skip") {
	case "YES", "v2":
		skip = true
	}
	return s.Hub.ViewPlaylist(channelID, key, name, skip)
}

// captionFile names the files a rendition folder serves from memory for captions.
func captionFile(name string) bool {
	vtt := strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".vtt")
	return name == "main.m3u8" || name == "captions.m3u8" || vtt
}

func decodeJSON(r *http.Request, dest any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dest)
}

// blockReload reads an LL-HLS blocking playlist request. A missing part waits
// for the whole segment.
func blockReload(r *http.Request) (msn, part int, ok bool) {
	raw := r.URL.Query().Get("_HLS_msn")
	if raw == "" {
		return 0, 0, false
	}
	msn, err := strconv.Atoi(raw)
	if err != nil {
		return 0, 0, false
	}
	part = -1
	if p := r.URL.Query().Get("_HLS_part"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			part = n
		}
	}
	return msn, part, true
}

// darkAfter is how long a tune may send nothing before its lock is read.
// A tuner that locks sends packets within a second or two.
var darkAfter = 5 * time.Second

// waitServable returns once the playlist has a segment a player can fetch.
// A playlist that lists only parts is not enough: hls.js treats that as empty
// and waits out its retry. The first part still anchors the clock while this waits.
// A cancelled watch returns immediately so the handler can drop that viewer.
// It reports true, and stops waiting, when the tuner has sent nothing for
// darkAfter and says it has no lock: that channel is not coming in.
func waitServable(ctx context.Context, h *live.Hub, channelID int64, key string, d time.Duration) bool {
	if h == nil || key == "" {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	deadline := start.Add(d)
	var checked time.Time
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if time.Since(start) >= darkAfter && time.Since(checked) >= 500*time.Millisecond {
			checked = time.Now()
			if h.NoSignal(channelID) {
				return true
			}
		}
		// A restart replaces the gate. Spending the whole deadline on the
		// old one hides the playlist the new encode is writing.
		slice := 100 * time.Millisecond
		if remain := time.Until(deadline); remain < slice {
			slice = remain
		}
		started := time.Now()
		key = h.Current(channelID, key)
		// Segment 0. A negative part means the whole segment, not an open part.
		h.WaitMedia(channelID, key, 0, -1, slice)
		body, err := h.Playlist(channelID, key)
		if err == nil && strings.Count(string(body), "#EXTINF") >= 1 {
			return false
		}
		if !time.Now().Before(deadline) {
			return false
		}
		// No gate yet, or this wake was for a playlist a restart already removed.
		if time.Since(started) < 50*time.Millisecond {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return false
}

// roomFrameWait caps how long a watch waits for an encode started in the
// buffer to reach its room's frame. Past it the player starts at the edge.
const roomFrameWait = 6 * time.Second

// roomAhead is how far past the room's frame the playlist must reach before
// the watch answers: the player starts half a second ahead of the room and
// needs a little picture beyond that.
const roomAhead = 1500 * time.Millisecond

// roomFrame is the program time the room plays at t.
func roomFrame(st realtime.RoomState, t time.Time) time.Time {
	return time.UnixMilli(int64(st.Target(float64(t.UnixNano()) / 1e6)))
}

// roomOf is the room the watch names, when it plays this channel and has
// screens in it.
func (s *Server) roomOf(body watchBody) (realtime.RoomState, bool) {
	if body.Room == "" || s.Bus == nil || s.Bus.Rooms == nil {
		return realtime.RoomState{}, false
	}
	st, ok := s.Bus.Rooms.State(body.Room)
	if !ok || st.ChannelID != body.ChannelID || st.Members == 0 {
		return realtime.RoomState{}, false
	}
	return st, true
}

// waitFrame waits until the playlist of an encode started in the buffer,
// which grows faster than real time, holds roomAhead past the room's frame.
func waitFrame(ctx context.Context, h *live.Hub, channelID int64, key string, room realtime.RoomState, d time.Duration) {
	deadline := time.Now().Add(d)
	for ctx.Err() == nil && time.Now().Before(deadline) {
		body, err := h.Playlist(channelID, h.Current(channelID, key))
		if err != nil {
			return
		}
		end, ok := playlistEnd(body)
		if !ok || !end.Before(roomFrame(room, time.Now()).Add(roomAhead)) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// playlistEnd is the program time where a stamped playlist's last segment ends.
func playlistEnd(body []byte) (time.Time, bool) {
	var at time.Time
	var dur float64
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"); ok {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return time.Time{}, false
			}
			at, dur = t, 0
		} else if v, ok := strings.CutPrefix(line, "#EXTINF:"); ok && !at.IsZero() {
			sec, err := strconv.ParseFloat(strings.SplitN(v, ",", 2)[0], 64)
			if err == nil {
				dur += sec
			}
		}
	}
	if at.IsZero() {
		return time.Time{}, false
	}
	return at.Add(time.Duration(dur * float64(time.Second))), true
}
