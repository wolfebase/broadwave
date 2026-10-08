package dvr

import (
	"broadwave/internal/breaks"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"broadwave/internal/live"
	"broadwave/internal/nfo"
	"broadwave/internal/store"
)

// OnSaved runs after a recording's file is closed. It writes an .nfo when that
// is on, queues the break scan and the intro search, applies keep rules, and
// then counts the file.
func OnSaved(ctx context.Context, st *store.Store, hub *live.Hub, queue *BreakQueue, rec store.Recording) {
	if st == nil || rec.ID == 0 {
		return
	}
	defer measureHealth(ctx, st, rec)
	writeNFO(ctx, st, hub, rec)
	passes, err := st.Passes(ctx)
	if err != nil {
		return
	}
	if rec.Path != "" {
		queue.Add(rec.ID)
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
				live.RemoveRecordingFiles(hub.Dir, victim)
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
	library, err := st.Spots(ctx)
	if err != nil {
		return nil, err
	}
	// A scan again does not find the spots it learned here.
	library = slices.DeleteFunc(library, func(sp store.Spot) bool { return sp.RecordingID == rec.ID })
	known := make([]breaks.Spot, len(library))
	for i, sp := range library {
		known[i] = breaks.Spot{Prints: sp.Prints, Start: -1}
	}
	res, err := indexFile(ffmpeg, rec.Path, known)
	if err != nil {
		return nil, err
	}
	found := res.Breaks
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
	_ = st.MarkBreaksScanned(ctx, rec.ID)
	learnSpots(ctx, st, rec.ID, library, res)
	fresh, err := st.Markers(ctx, rec.ID)
	if err != nil {
		return nil, err
	}
	_ = live.WriteEDL(rec.Path, fresh)
	return fresh, nil
}

// learnSpots keeps the recording's new spots and notes the known ones that
// played again.
func learnSpots(ctx context.Context, st *store.Store, recordingID int64, library []store.Spot, res breaks.Result) {
	var seen []int64
	for i, ok := range res.Seen {
		if ok && i < len(library) {
			seen = append(seen, library[i].ID)
		}
	}
	fresh := make([][]uint64, len(res.Spots))
	for i, sp := range res.Spots {
		fresh[i] = sp.Prints
	}
	if err := st.SeeSpots(ctx, seen); err != nil {
		slog.Warn(fmt.Sprintf("breaks: spots: %v", err))
	}
	if err := st.AddSpots(ctx, recordingID, fresh); err != nil {
		slog.Warn(fmt.Sprintf("breaks: spots: %v", err))
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

func insideDir(root, path string) bool {
	if path == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
