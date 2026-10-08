package dvr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

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
	// root is the recordings folder. The backfill leaves library folders alone;
	// Find commercials still scans a file there when asked.
	root string

	// poll is how often a waiting scan checks again; patience is how long it
	// waits for a quiet moment before it runs anyway.
	poll     time.Duration
	patience time.Duration
	// backfill is how often recordings that were never scanned are queued.
	backfill time.Duration
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
	push := func(id int64) {
		if !queued[id] {
			queued[id] = true
			pending = append(pending, id)
		}
	}
	q.queueUnscanned(ctx, push)
	backfill := time.NewTicker(q.backfill)
	defer backfill.Stop()
	for {
		if len(pending) == 0 {
			select {
			case <-ctx.Done():
				return
			case id := <-q.add:
				push(id)
			case <-backfill.C:
				q.queueUnscanned(ctx, push)
			}
			continue
		}
		if !q.waitQuiet(ctx, push) {
			return
		}
		id := pending[0]
		pending = pending[1:]
		delete(queued, id)
		rec, err := q.st.Recording(ctx, id)
		if err != nil || rec.Status != "complete" {
			continue
		}
		passes, err := q.st.Passes(ctx)
		if err != nil {
			continue
		}
		scan, listen := wants(rec, passes)
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

// queueUnscanned queues finished recordings in the recordings folder that were
// never listened to, or never scanned while their pass wants breaks found.
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
