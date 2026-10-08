package dvr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// lastLost is the set of missing recordings the last walk looked for. The
// same set is walked for again only after a day: a recording deleted by hand
// would otherwise read every .json in the folder every hour, waking sleeping
// disks.
var (
	lostMu   sync.Mutex
	lastLost string
	lastWalk time.Time
)

// Refind follows recordings moved by hand inside the recordings folder: a
// finished recording whose file is gone is matched by the .json beside its
// file (id, title, and start all agree) and pointed at the file found there.
// Only plain files count, never links.
func Refind(ctx context.Context, st *store.Store, hub *live.Hub) {
	if st == nil || hub == nil {
		return
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		return
	}
	lost := map[int64]store.Recording{}
	owned := map[string]bool{}
	var ids []int64
	for _, rec := range recs {
		owned[filepath.Clean(rec.Path)] = true
		if rec.Status == "recording" || rec.Path == "" {
			continue
		}
		if _, err := os.Lstat(rec.Path); errors.Is(err, fs.ErrNotExist) {
			lost[rec.ID] = rec
			ids = append(ids, rec.ID)
		}
	}
	slices.Sort(ids)
	key := fmt.Sprint(ids)
	lostMu.Lock()
	same := key == lastLost && time.Since(lastWalk) < 24*time.Hour
	if !same {
		lastLost, lastWalk = key, time.Now()
	}
	lostMu.Unlock()
	if len(lost) == 0 || same {
		return
	}
	root := hub.Recordings()
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return
	}
	_ = filepath.WalkDir(real, func(path string, d fs.DirEntry, err error) error {
		if len(lost) == 0 || ctx.Err() != nil {
			return fs.SkipAll
		}
		if err != nil || !d.Type().IsRegular() || filepath.Ext(path) != ".json" {
			return nil
		}
		side, ok := readSidecar(path)
		if !ok {
			return nil
		}
		rec, ok := lost[side.ID]
		if !ok || side.Title != rec.Title || !side.StartedAt.Truncate(time.Second).Equal(rec.StartedAt.Truncate(time.Second)) {
			return nil
		}
		found := strings.TrimSuffix(path, ".json") + filepath.Ext(rec.Path)
		if info, err := os.Lstat(found); err != nil || !info.Mode().IsRegular() {
			return nil
		}
		// Stored under the folder's own name, as every other recording is.
		rel, err := filepath.Rel(real, found)
		if err != nil {
			return nil
		}
		media := filepath.Join(root, rel)
		if owned[media] || st.SetRecordingPath(ctx, rec.ID, media) != nil {
			return nil
		}
		owned[media] = true
		delete(lost, rec.ID)
		_ = st.AddEvent(ctx, "recording", fmt.Sprintf("Found %s at %s", rec.Title, filepath.ToSlash(rel)))
		return nil
	})
}

type sidecar struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	StartedAt time.Time `json:"startedAt"`
}

func readSidecar(path string) (sidecar, bool) {
	var side sidecar
	f, err := os.Open(path)
	if err != nil {
		return side, false
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil || json.Unmarshal(body, &side) != nil {
		return side, false
	}
	return side, true
}
