package dvr

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// foundExts are the files a recording is saved as.
var foundExts = []string{".ts", ".mp4", ".mkv", ".m4v"}

// ImportFound adds recordings in the recordings folder that the catalog does
// not have: a file with the .json the server wrote beside it when it saved
// the recording. A catalog restored from an older backup, or a new install
// pointed at an old folder, gets them back this way. Recordings the catalog
// lost track of are looked for first, so a moved one is not added twice.
// It returns how many it added.
func ImportFound(ctx context.Context, st *store.Store, hub *live.Hub) (int, error) {
	if st == nil || hub == nil {
		return 0, nil
	}
	RefindNow(ctx, st, hub)
	return importFound(ctx, st, hub.Recordings())
}

func importFound(ctx context.Context, st *store.Store, root string) (int, error) {
	recs, err := st.Recordings(ctx)
	if err != nil {
		return 0, err
	}
	owned := map[string]bool{}
	for _, rec := range recs {
		if rec.Path != "" {
			owned[filepath.Clean(rec.Path)] = true
		}
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return 0, err
	}
	channels := map[string]int64{}
	if all, err := st.Channels(ctx, false); err == nil {
		for _, ch := range all {
			if _, ok := channels[ch.GuideNumber]; !ok || ch.Present {
				channels[ch.GuideNumber] = ch.ID
			}
		}
	}
	added := 0
	err = filepath.WalkDir(real, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return fs.SkipAll
		}
		if err != nil || !d.Type().IsRegular() || filepath.Ext(path) != ".json" {
			return nil
		}
		side, ok := readSidecar(path)
		if !ok || side.Title == "" || side.StartedAt.IsZero() {
			return nil
		}
		media := foundMedia(real, root, path)
		if media == "" || owned[media] {
			return nil
		}
		rec := store.Recording{
			Title: side.Title, GuideNumber: side.GuideNumber, ChannelID: channels[side.GuideNumber],
			Path: media, Status: "complete", StartedAt: side.StartedAt, EndedAt: side.EndedAt,
		}
		switch side.Status {
		case "interrupted", "failed":
			rec.Status = side.Status
			rec.Partial = true
		}
		id, err := st.AddFoundRecording(ctx, rec)
		if err != nil {
			return err
		}
		owned[media] = true
		added++
		// The .json names the id, so a later move is followed too.
		if saved, err := st.Recording(ctx, id); err == nil {
			live.WriteSidecar(saved)
		}
		return nil
	})
	if added > 0 {
		noun := "recordings"
		if added == 1 {
			noun = "recording"
		}
		_ = st.AddEvent(ctx, "recording", fmt.Sprintf("Added %d %s found in the recordings folder", added, noun))
	}
	return added, err
}

// foundMedia is the recording beside a .json, under the folder's own name;
// empty when there is none. Only plain files count, never links.
func foundMedia(real, root, side string) string {
	stem := strings.TrimSuffix(side, ".json")
	for _, ext := range foundExts {
		info, err := os.Lstat(stem + ext)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		rel, err := filepath.Rel(real, stem+ext)
		if err != nil {
			return ""
		}
		return filepath.Join(root, rel)
	}
	return ""
}
