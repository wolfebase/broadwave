package dvr

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"broadwave/internal/live"
	"broadwave/internal/nfo"
	"broadwave/internal/store"
)

// OnSaved runs after a recording's file is closed. It writes an .nfo when that
// is on, indexes commercials, applies keep rules, and then counts the file.
func OnSaved(ctx context.Context, st *store.Store, hub *live.Hub, rec store.Recording) {
	if st == nil || rec.ID == 0 {
		return
	}
	defer measureHealth(ctx, st, rec)
	writeNFO(ctx, st, hub, rec)
	passes, err := st.Passes(ctx)
	if err != nil {
		return
	}
	if commercialsOn(passes, rec) && hub != nil && rec.Path != "" {
		found, err := live.IndexBreaks(hub.FFmpeg, rec.Path)
		if err != nil {
			slog.Error(fmt.Sprintf("breaks: %v", err))
		} else if _, err := st.Recording(ctx, rec.ID); err != nil {
			// Deleted while its breaks were indexed.
		} else if len(found) > 0 {
			markers := make([]store.Marker, 0, len(found))
			for _, item := range found {
				markers = append(markers, store.Marker{Start: item.Start, End: item.End})
			}
			if err := st.ReplaceMarkers(ctx, rec.ID, markers); err == nil {
				fresh, _ := st.Markers(ctx, rec.ID)
				_ = live.WriteEDL(rec.Path, fresh)
			}
		}
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		return
	}
	for _, pass := range passes {
		for _, id := range KeepVictims(pass, recs) {
			victim, err := st.Recording(ctx, id)
			if err != nil || victim.Status == "recording" {
				continue
			}
			if hub != nil {
				_ = os.Remove(victim.Path)
				base := stringsTrimExt(victim.Path)
				_ = os.Remove(base + ".edl")
				_ = os.Remove(base + ".json")
				_ = os.Remove(base + ".nfo")
				_ = os.Remove(filepath.Join(hub.Dir, "posters", strconv.FormatInt(victim.ID, 10)+".jpg"))
			}
			if err := st.DeleteRecording(ctx, id); err == nil {
				_ = st.AddEvent(ctx, "delete", "Keep rule removed "+victim.Title)
			}
		}
	}
}

func writeNFO(ctx context.Context, st *store.Store, hub *live.Hub, rec store.Recording) {
	if hub == nil || hub.Dir == "" || rec.Status != "complete" {
		return
	}
	values, err := st.Settings(ctx)
	if err != nil || values["writeNfo"] != "1" {
		return
	}
	root := filepath.Join(hub.Dir, "recordings")
	if _, err := nfo.Write(root, RecordingNFO(ctx, st, rec)); err != nil {
		slog.Error(fmt.Sprintf("nfo: %v", err))
	}
}

func commercialsOn(passes []store.Pass, rec store.Recording) bool {
	matched := false
	for _, pass := range passes {
		if !sameShow(pass, rec) {
			continue
		}
		matched = true
		if pass.Commercials {
			return true
		}
	}
	return !matched
}

func stringsTrimExt(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path
	}
	return path[:len(path)-len(ext)]
}
