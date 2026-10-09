package dvr

import (
	"context"
	"sort"
	"strings"
	"time"

	"broadwave/internal/breaks"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// listenFile prints a recording's ends; tests stand in for ffmpeg.
var listenFile = breaks.ListenEnds

// probeLength reads a recording's length when it was not stored yet.
var probeLength = func(ffmpeg, path string) float64 {
	tool := live.FFProbePath(ffmpeg)
	if tool == "" {
		return 0
	}
	seconds, err := live.ProbeDuration(tool, path)
	if err != nil {
		return 0
	}
	return seconds
}

// maxEpisodes is how many other recordings of a show one is compared with,
// the closest in time first.
const maxEpisodes = 8

// FindEpisodeEnds prints the sound of a recording's ends and compares it with
// other recordings of its show, for where its intro and end titles are. The
// first recording of a show has nothing to compare with, so each later one
// also fills in the others that have no intro yet.
func FindEpisodeEnds(ctx context.Context, st *store.Store, ffmpeg string, rec store.Recording) error {
	length := rec.Duration
	if length <= 0 {
		length = probeLength(ffmpeg, rec.Path)
	}
	sound, err := listenFile(ctx, ffmpeg, rec.Path, length)
	if err != nil {
		return err
	}
	from, to := showBounds(ctx, st, rec, length)
	if err := st.SaveEpisodePrints(ctx, rec.ID, store.EpisodePrints{Head: sound.Head, Tail: sound.Tail, TailFrom: sound.TailFrom, ShowStart: from, ShowEnd: to}); err != nil {
		return err
	}
	if _, err := st.Recording(ctx, rec.ID); err != nil || len(sound.Head) == 0 {
		return nil // deleted while it was read, or it has no sound
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		return err
	}
	self := episode(ctx, st, rec, breaks.Sound{Head: sound.Head, Tail: sound.Tail, TailFrom: sound.TailFrom}, from, to)
	siblings := showSiblings(recs, rec)
	loaded := map[int64]breaks.Episode{}
	var others []breaks.Episode
	for _, sib := range siblings {
		if e, ok := loadEpisode(ctx, st, sib); ok {
			loaded[sib.ID] = e
			others = append(others, e)
		}
	}
	ends := breaks.FindEnds(self, others)
	if err := st.SetEpisodeEnds(ctx, rec.ID, ends.IntroStart, ends.IntroEnd, ends.Credits); err != nil {
		return err
	}
	for _, sib := range siblings {
		mine, ok := loaded[sib.ID]
		if sib.IntroEnd > 0 || !ok {
			continue
		}
		rest := []breaks.Episode{self}
		for _, o := range siblings {
			if e, ok := loaded[o.ID]; ok && o.ID != sib.ID && !sameEpisode(o, sib) {
				rest = append(rest, e)
			}
		}
		if e := breaks.FindEnds(mine, rest); e.IntroEnd > 0 {
			_ = st.SetEpisodeEnds(ctx, sib.ID, e.IntroStart, e.IntroEnd, e.Credits)
		}
	}
	return nil
}

// showSiblings are the other listened recordings with the same title, the
// closest in time first. Another airing of the same episode shares all of
// its show, not just the intro, so it is left out.
func showSiblings(recs []store.Recording, rec store.Recording) []store.Recording {
	title := strings.TrimSpace(rec.Title)
	var out []store.Recording
	for _, r := range recs {
		if r.ID == rec.ID || !r.Listened || r.Status != "complete" || r.Missing || !strings.EqualFold(strings.TrimSpace(r.Title), title) || sameEpisode(r, rec) {
			continue
		}
		out = append(out, r)
	}
	gap := func(r store.Recording) time.Duration { return r.StartedAt.Sub(rec.StartedAt).Abs() }
	sort.SliceStable(out, func(i, j int) bool { return gap(out[i]) < gap(out[j]) })
	if len(out) > maxEpisodes {
		out = out[:maxEpisodes]
	}
	return out
}

// sameEpisode is two recordings of one episode, as far as the guide says.
func sameEpisode(a, b store.Recording) bool {
	switch {
	case a.ProgramID != "" && b.ProgramID != "":
		return a.ProgramID == b.ProgramID
	case a.Season > 0 && a.Episode > 0 && b.Season > 0 && b.Episode > 0:
		return a.Season == b.Season && a.Episode == b.Episode
	default:
		return a.Subtitle != "" && strings.EqualFold(strings.TrimSpace(a.Subtitle), strings.TrimSpace(b.Subtitle))
	}
}

func loadEpisode(ctx context.Context, st *store.Store, rec store.Recording) (breaks.Episode, bool) {
	p, ok, err := st.EpisodePrints(ctx, rec.ID)
	if err != nil || !ok || len(p.Head) == 0 {
		return breaks.Episode{}, false
	}
	return episode(ctx, st, rec, breaks.Sound{Head: p.Head, Tail: p.Tail, TailFrom: p.TailFrom}, p.ShowStart, p.ShowEnd), true
}

func episode(ctx context.Context, st *store.Store, rec store.Recording, sound breaks.Sound, from, to float64) breaks.Episode {
	e := breaks.Episode{Sound: sound, ShowStart: from, ShowEnd: to}
	markers, _ := st.Markers(ctx, rec.ID)
	for _, m := range markers {
		e.Breaks = append(e.Breaks, breaks.Break{Start: m.Start, End: m.End, Confidence: m.Confidence})
	}
	return e
}

// showBounds is where the recording's listing starts and ends in the file,
// from the guide while it still holds the listing. The end is 0 when not
// known: no listing, a recording stopped before the listing ended, or a game,
// which can run past it.
func showBounds(ctx context.Context, st *store.Store, rec store.Recording, length float64) (float64, float64) {
	if rec.StartedAt.IsZero() || length <= 0 || rec.GameID != "" {
		return 0, 0
	}
	q := store.AiringQuery{
		From: rec.StartedAt.Add(-time.Hour),
		To:   rec.StartedAt.Add(time.Duration(length * float64(time.Second))),
	}
	if rec.ChannelID != 0 {
		q.Channels = []int64{rec.ChannelID}
	}
	airings, err := st.QueryAirings(ctx, q)
	if err != nil {
		return 0, 0
	}
	ended := rec.StartedAt.Add(time.Duration(length * float64(time.Second)))
	rec.EndedAt = &ended
	air := store.CoveringAiring(rec, airings)
	if air == nil {
		return 0, 0
	}
	from := max(0, air.Start.Sub(rec.StartedAt).Seconds())
	to := air.End.Sub(rec.StartedAt).Seconds()
	if to <= from || to > length {
		return from, 0
	}
	return from, to
}
