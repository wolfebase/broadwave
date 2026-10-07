package dvr

import (
	"broadwave/internal/breaks"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
		if _, err := IndexBreaks(ctx, st, hub.FFmpeg, rec, true); err != nil {
			slog.Error(fmt.Sprintf("breaks: %v", err))
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
			// A keep rule removes only what Broadwave recorded, never a file in a library folder.
			if hub != nil && !insideDir(filepath.Join(hub.Dir, "recordings"), victim.Path) {
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

// indexFile is the break scan; tests stand in for ffmpeg and comskip.
var indexFile = breaks.Index

// IndexBreaks scans a recording for breaks and stores them as its markers,
// with the .edl beside it. A scan someone asked for is a fresh start; one the
// server runs on its own keeps the breaks someone marked by hand, maybe
// while the show was still recording, and adds only what they don't cover.
func IndexBreaks(ctx context.Context, st *store.Store, ffmpeg string, rec store.Recording, keepHand bool) ([]store.Marker, error) {
	found, err := indexFile(ffmpeg, rec.Path)
	if err != nil {
		return nil, err
	}
	var markers []store.Marker
	if keepHand {
		old, err := st.Markers(ctx, rec.ID)
		if err != nil {
			return nil, err
		}
		for _, m := range old {
			if m.Confidence >= 1 {
				markers = append(markers, m)
			}
		}
	}
	hand := len(markers)
	for _, item := range found {
		covered := false
		for _, m := range markers[:hand] {
			if m.Start < item.End && m.End > item.Start {
				covered = true
			}
		}
		if !covered {
			markers = append(markers, store.Marker{Start: item.Start, End: item.End, Confidence: item.Confidence})
		}
	}
	if _, err := st.Recording(ctx, rec.ID); err != nil {
		return nil, nil // deleted while it was scanned
	}
	if err := st.ReplaceMarkers(ctx, rec.ID, markers); err != nil {
		return nil, err
	}
	fresh, err := st.Markers(ctx, rec.ID)
	if err != nil {
		return nil, err
	}
	_ = live.WriteEDL(rec.Path, fresh)
	return fresh, nil
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

func insideDir(root, path string) bool {
	if path == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func stringsTrimExt(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path
	}
	return path[:len(path)-len(ext)]
}
