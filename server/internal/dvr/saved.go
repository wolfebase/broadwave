package dvr

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"waveguide/internal/live"
	"waveguide/internal/store"
)

// OnSaved indexes commercials and applies keep rules after a recording finishes.
func OnSaved(ctx context.Context, st *store.Store, hub *live.Hub, rec store.Recording) {
	if st == nil || rec.ID == 0 {
		return
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		return
	}
	if commercialsOn(passes, rec) && hub != nil && rec.Path != "" {
		found, err := live.IndexBreaks(hub.FFmpeg, rec.Path)
		if err != nil {
			log.Printf("breaks: %v", err)
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
				_ = os.Remove(filepath.Join(hub.Dir, "posters", strconv.FormatInt(victim.ID, 10)+".jpg"))
			}
			if err := st.DeleteRecording(ctx, id); err == nil {
				_ = st.AddEvent(ctx, "delete", "Keep rule removed "+victim.Title)
			}
		}
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
