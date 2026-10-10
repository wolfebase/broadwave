package dvr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// libraryPerHour is how many library files are read in one hour. A library
// can be a large share, so it is not walked flat out. The recordings folder
// is not capped. One file is read at a time, and only while nobody is watching.
const libraryPerHour = 6

const libraryWindow = time.Hour

// BreakQueue scans recordings for breaks one at a time, while nobody watches
// live TV: a scan reads the whole file and takes a couple of CPU cores that a
// small server needs for the screens that are playing. It also listens to
// the ends of every recording for its intro and end titles.
type BreakQueue struct {
	st     *store.Store
	scan   func(context.Context, store.Recording) error
	listen func(context.Context, store.Recording) error
	busy   func() bool
	add    chan int64
	// root is the recordings folder. A file outside it is a library file: the
	// backfill does not scan it for breaks (Find commercials still does, when
	// asked). A series there is listened to for its intro, under libraryPerHour.
	root string

	// poll is how often a waiting scan checks again; patience is how long it
	// waits for a quiet moment before it runs anyway. A library file does not
	// use patience: it waits until nobody is watching.
	poll     time.Duration
	patience time.Duration
	// backfill is how often recordings that were never scanned are queued.
	backfill time.Duration
	// now is the clock. Tests set it; production leaves it nil.
	now func() time.Time
	// libraryAt is when each library file started being read, inside the window.
	libraryAt []time.Time
	// libraryHeld is a library file that is waiting for a quiet hour with room.
	libraryHeld bool
}

// NewBreakQueue scans with the hub's ffmpeg and waits while anyone watches.
func NewBreakQueue(st *store.Store, hub *live.Hub) *BreakQueue {
	q := newBreakQueue(st, func(ctx context.Context, rec store.Recording) error {
		_, err := IndexBreaks(ctx, st, hub.FFmpeg, rec, true)
		return err
	}, func(ctx context.Context, rec store.Recording) error {
		return FindEpisodeEnds(ctx, st, hub.FFmpeg, rec)
	}, hub.Watching)
	q.root = hub.Recordings()
	return q
}

func newBreakQueue(st *store.Store, scan, listen func(context.Context, store.Recording) error, busy func() bool) *BreakQueue {
	return &BreakQueue{
		st: st, scan: scan, listen: listen, busy: busy, add: make(chan int64, 256),
		poll: 30 * time.Second, patience: 3 * time.Hour, backfill: 6 * time.Hour,
	}
}

// Add queues a recording. A full queue drops it; the backfill finds it later.
func (q *BreakQueue) Add(id int64) {
	if q == nil {
		return
	}
	select {
	case q.add <- id:
	default:
	}
}

// Run scans queued recordings until ctx ends. It starts by queueing every
// recording that was never scanned.
func (q *BreakQueue) Run(ctx context.Context) {
	var pending []int64
	queued := map[int64]bool{}
	failed := map[int64]int{}
	var libraryQueued int64
	push := func(id int64) {
		if id != 0 && !queued[id] {
			queued[id] = true
			pending = append(pending, id)
		}
	}
	q.queueUnscanned(ctx, push)
	backfill := time.NewTicker(q.backfill)
	defer backfill.Stop()
	for {
		if len(pending) == 0 {
			q.enqueueLibrary(ctx, push, &libraryQueued)
		}
		if len(pending) == 0 {
			var poll <-chan time.Time
			if q.libraryHeld {
				poll = time.After(q.poll)
			}
			select {
			case <-ctx.Done():
				return
			case id := <-q.add:
				push(id)
			case <-backfill.C:
				q.queueUnscanned(ctx, push)
			case <-poll:
			}
			continue
		}
		// A library file at the head is set aside while someone is watching or the
		// hour is full. It does not run when patience is up, and it does not hold
		// up a recording in the recordings folder.
		if pending[0] == libraryQueued && q.libraryBlocked() {
			id := pending[0]
			pending = pending[1:]
			delete(queued, id)
			libraryQueued = 0
			q.libraryHeld = true
			continue
		}
		if !q.waitQuiet(ctx, push) {
			return
		}
		id := pending[0]
		pending = pending[1:]
		delete(queued, id)
		if id == libraryQueued {
			libraryQueued = 0
		}
		rec, err := q.st.Recording(ctx, id)
		if err != nil || rec.Status != "complete" {
			continue
		}
		passes, err := q.st.Passes(ctx)
		if err != nil {
			continue
		}
		scan, listen := wants(rec, passes)
		if q.isLibraryPath(rec.Path) {
			if q.libraryBlocked() {
				q.libraryHeld = true
				continue
			}
			// The backfill listens only. Find commercials scans a library file
			// when the viewer asks, which does not come through here.
			scan = false
			listen = !rec.Listened
			if listen {
				q.noteLibrary(q.clock())
				q.libraryHeld = false
			}
		}
		if !scan && !listen {
			continue
		}
		if err := q.work(ctx, rec, scan, listen); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn(fmt.Sprintf("breaks: %s: %v", rec.Title, err))
			// A share that was away or an ffmpeg that was stopped gets
			// another try at the next backfill; a file that fails three
			// times, or is gone, is left to Find commercials.
			failed[id]++
			if _, statErr := os.Stat(rec.Path); statErr == nil && failed[id] < 3 {
				continue
			}
			q.giveUp(ctx, id)
		}
	}
}

// wants is what a recording still needs: a break scan when its pass wants
// breaks found, and its ends listened to.
func wants(rec store.Recording, passes []store.Pass) (scan, listen bool) {
	return !rec.BreaksScanned && commercialsOn(passes, rec), !rec.Listened
}

// work scans first, so the intro search knows the breaks, and listens even
// when the scan failed. What succeeded is not done again on a retry.
func (q *BreakQueue) work(ctx context.Context, rec store.Recording, scan, listen bool) error {
	var errs []error
	if scan {
		if err := q.scan(ctx, rec); err != nil {
			errs = append(errs, err)
		} else {
			_ = q.st.MarkBreaksScanned(ctx, rec.ID)
		}
	}
	if listen && q.listen != nil && ctx.Err() == nil {
		errs = append(errs, q.listen(ctx, rec))
	}
	return errors.Join(errs...)
}

// giveUp marks what a recording still wanted as done.
func (q *BreakQueue) giveUp(ctx context.Context, id int64) {
	rec, err := q.st.Recording(ctx, id)
	if err != nil {
		return
	}
	passes, err := q.st.Passes(ctx)
	if err != nil {
		return
	}
	scan, listen := wants(rec, passes)
	if scan {
		_ = q.st.MarkBreaksScanned(ctx, id)
	}
	if listen {
		_ = q.st.SaveEpisodePrints(ctx, id, store.EpisodePrints{})
	}
}

// waitQuiet waits until nobody watches, or patience runs out, and keeps
// taking new recordings meanwhile. It reports false when ctx ends.
func (q *BreakQueue) waitQuiet(ctx context.Context, push func(int64)) bool {
	deadline := time.Now().Add(q.patience)
	for q.busy() && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case id := <-q.add:
			push(id)
		case <-time.After(q.poll):
		}
	}
	return ctx.Err() == nil
}

func (q *BreakQueue) clock() time.Time {
	if q.now != nil {
		return q.now()
	}
	return time.Now()
}

// libraryBlocked is true while someone is watching, or the hour already has
// libraryPerHour files in it. A library file waits; it does not run anyway.
func (q *BreakQueue) libraryBlocked() bool {
	if q.busy != nil && q.busy() {
		return true
	}
	return !q.libraryRoom(q.clock())
}

func (q *BreakQueue) libraryRoom(now time.Time) bool {
	q.pruneLibrary(now)
	return len(q.libraryAt) < libraryPerHour
}

func (q *BreakQueue) noteLibrary(now time.Time) {
	q.pruneLibrary(now)
	q.libraryAt = append(q.libraryAt, now)
}

func (q *BreakQueue) pruneLibrary(now time.Time) {
	kept := make([]time.Time, 0, len(q.libraryAt))
	for _, at := range q.libraryAt {
		if now.Sub(at) < libraryWindow {
			kept = append(kept, at)
		}
	}
	q.libraryAt = kept
}

func (q *BreakQueue) isLibraryPath(path string) bool {
	if q.root == "" || path == "" {
		return false
	}
	return !insideDir(q.root, path)
}

// enqueueLibrary queues one library episode of a show that has at least two
// episodes, when nobody is watching and the hour still has room.
func (q *BreakQueue) enqueueLibrary(ctx context.Context, push func(int64), libraryQueued *int64) {
	if *libraryQueued != 0 {
		return
	}
	id := q.nextLibraryID(ctx)
	if id == 0 {
		q.libraryHeld = false
		return
	}
	if q.libraryBlocked() {
		q.libraryHeld = true
		return
	}
	q.libraryHeld = false
	*libraryQueued = id
	push(id)
}

// nextLibraryID is the oldest unlistened library episode whose show has at
// least two episodes, in the library or the recordings folder. Two files of
// the same season and episode count as one.
func (q *BreakQueue) nextLibraryID(ctx context.Context) int64 {
	recs, err := q.st.Recordings(ctx)
	if err != nil {
		return 0
	}
	episodes := map[string]map[string]bool{}
	for _, rec := range recs {
		key, ok := episodeKey(rec)
		if !ok || rec.Status != "complete" {
			continue
		}
		title := showKey(rec.Title)
		if episodes[title] == nil {
			episodes[title] = map[string]bool{}
		}
		episodes[title][key] = true
	}
	for i := len(recs) - 1; i >= 0; i-- {
		rec := recs[i]
		if _, ok := episodeKey(rec); !ok || rec.Listened || rec.Status != "complete" || rec.Path == "" || !q.isLibraryPath(rec.Path) {
			continue
		}
		if len(episodes[showKey(rec.Title)]) < 2 {
			continue
		}
		return rec.ID
	}
	return 0
}

func showKey(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

func episodeKey(rec store.Recording) (string, bool) {
	if rec.Season <= 0 || rec.Episode <= 0 {
		return "", false
	}
	return fmt.Sprintf("%d-%d", rec.Season, rec.Episode), true
}

// queueUnscanned queues finished recordings in the recordings folder that were
// never listened to, or never scanned while their pass wants breaks found.
// Library episodes are queued by enqueueLibrary, one at a time.
func (q *BreakQueue) queueUnscanned(ctx context.Context, push func(int64)) {
	recs, err := q.st.Recordings(ctx)
	if err != nil {
		return
	}
	passes, err := q.st.Passes(ctx)
	if err != nil {
		return
	}
	// Oldest first: the list is newest first.
	for i := len(recs) - 1; i >= 0; i-- {
		rec := recs[i]
		if scan, listen := wants(rec, passes); (!scan && !listen) || rec.Status != "complete" || rec.Path == "" {
			continue
		}
		if q.root != "" && !insideDir(q.root, rec.Path) {
			continue
		}
		push(rec.ID)
	}
}
