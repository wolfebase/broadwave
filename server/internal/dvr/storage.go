package dvr

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// tidyMu keeps the hourly clean-up and a recording's make-room from
// choosing the same victims at once.
var tidyMu sync.Mutex

// diskStat reads free space; tests stand in for a filling disk.
var diskStat = disk.Stat

// recentPlay is how long after its playhead last moved a recording is left
// alone: someone may be watching it now.
const recentPlay = 6 * time.Hour

// WatchedDays is how many days a watched recording stays: 0 keeps it.
func WatchedDays(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Tidy removes recordings watched more than deleteWatchedDays ago and, when
// makeRoom is on and free space is under the reserve, makes room.
func Tidy(ctx context.Context, st *store.Store, hub *live.Hub, now time.Time) {
	if st == nil || hub == nil || hub.Dir == "" {
		return
	}
	ctx = context.WithoutCancel(ctx)
	values, err := st.Settings(ctx)
	if err != nil {
		return
	}
	if days := WatchedDays(values["deleteWatchedDays"]); days > 0 {
		tidyMu.Lock()
		recs, err := st.Recordings(ctx)
		if err == nil {
			cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
			for _, rec := range cleanable(hub, recs, now) {
				if at := rec.WatchedSince(); !at.IsZero() && at.Before(cutoff) {
					remove(ctx, st, hub, rec, now, fmt.Sprintf("Removed %s, watched more than %d days ago", rec.Title, days))
				}
			}
		}
		tidyMu.Unlock()
	}
	if reserve := disk.WatermarkBytes(values["watermarkGB"]); reserve > 0 {
		MakeRoom(ctx, st, hub, reserve)
	}
}

// MakeRoom removes the oldest watched recordings until the recordings folder
// has need bytes free, when makeRoom is on. Unwatched and kept recordings
// are never removed. When the watched ones can't free enough, it removes
// nothing: the recording is refused either way.
func MakeRoom(ctx context.Context, st *store.Store, hub *live.Hub, need uint64) {
	if st == nil || hub == nil || hub.Dir == "" {
		return
	}
	ctx = context.WithoutCancel(ctx)
	values, err := st.Settings(ctx)
	if err != nil || values["makeRoom"] != "1" {
		return
	}
	space, err := diskStat(filepath.Join(hub.Dir, "recordings"))
	if err != nil || !disk.BelowReserve(space.Free, need) {
		return
	}
	short := need - space.Free
	tidyMu.Lock()
	defer tidyMu.Unlock()
	recs, err := st.Recordings(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	victims := RoomOrder(cleanable(hub, recs, now))
	var could uint64
	for _, rec := range victims {
		could += fileSize(rec.Path)
	}
	if could < short {
		_ = st.AddEvent(ctx, "disk", "Free space is under the reserve, and deleting every watched recording would not free enough")
		return
	}
	// Freed bytes are counted, not read back: some filesystems report the
	// space a few seconds after a delete.
	var freed uint64
	for _, rec := range victims {
		if freed >= short {
			return
		}
		size := fileSize(rec.Path)
		if remove(ctx, st, hub, rec, now, "Made room: removed "+rec.Title+", already watched") {
			freed += size
		}
	}
}

// RoomOrder is the watched recordings in the order making room removes
// them: watched longest ago first, then the oldest recording.
func RoomOrder(recs []store.Recording) []store.Recording {
	var out []store.Recording
	for _, rec := range recs {
		if finished(rec) {
			out = append(out, rec)
		}
	}
	slices.SortStableFunc(out, func(a, b store.Recording) int {
		if c := a.WatchedSince().Compare(b.WatchedSince()); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return out
}

// finished is watched in a way a clean-up trusts: marked watched, or played
// to its last seconds. Stopping at 90% of a long game is not.
func finished(rec store.Recording) bool {
	switch rec.Watched {
	case 1:
		return true
	case 0:
		return rec.Duration >= 10 && rec.Position >= rec.Duration-15
	}
	return false
}

// cleanable is what a clean-up may remove: finished recordings inside the
// recordings folder that nobody kept forever or played in the last hours.
func cleanable(hub *live.Hub, recs []store.Recording, now time.Time) []store.Recording {
	root := filepath.Join(hub.Dir, "recordings")
	var out []store.Recording
	for _, rec := range recs {
		if rec.Keep || rec.Status != "complete" || !finished(rec) || !insideDir(root, rec.Path) {
			continue
		}
		if rec.ProgressAt != nil && now.Sub(*rec.ProgressAt) < recentPlay {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// remove deletes a recording the clean-up chose, after reading it again:
// someone may have kept it or marked it unwatched since.
func remove(ctx context.Context, st *store.Store, hub *live.Hub, rec store.Recording, now time.Time, why string) bool {
	fresh, err := st.Recording(ctx, rec.ID)
	if err != nil || len(cleanable(hub, []store.Recording{fresh}, now)) == 0 {
		return false
	}
	live.RemoveRecordingFiles(hub.Dir, fresh)
	if err := st.DeleteRecording(ctx, fresh.ID); err != nil {
		slog.Warn(fmt.Sprintf("storage: delete %d: %v", fresh.ID, err))
		return false
	}
	_ = st.AddEvent(ctx, "delete", why)
	return true
}

func fileSize(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil || info.Size() < 0 {
		return 0
	}
	return uint64(info.Size())
}
