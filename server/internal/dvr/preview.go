package dvr

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// StoragePlan is what the storage rules would do right now. It deletes nothing.
type StoragePlan struct {
	DeleteWatchedDays int               `json:"deleteWatchedDays"`
	MakeRoom          bool              `json:"makeRoom"`
	ProtectFavorites  bool              `json:"protectFavorites"`
	WatermarkGB       int               `json:"watermarkGB"`
	FreeBytes         uint64            `json:"freeBytes"`
	ReserveBytes      uint64            `json:"reserveBytes"`
	ShortBytes        uint64            `json:"shortBytes"`
	Covers            bool              `json:"covers"`
	RoomNote          string            `json:"roomNote,omitempty"`
	Remove            []StoragePlanItem `json:"remove"`
	Held              []StoragePlanItem `json:"held"`
	Again             []StoragePlanItem `json:"again"`
}

// StoragePlanItem is one recording a rule would remove or leave alone.
// When is "now" for a clean-up that would run on its own, and "next" for a
// keep rule that runs when another episode of that show finishes.
type StoragePlanItem struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Bytes  int64  `json:"bytes"`
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
	When   string `json:"when,omitempty"`
}

// Preview reads the catalog and the disk and says what a clean-up would
// remove. The files stay where they are.
func Preview(ctx context.Context, st *store.Store, hub *live.Hub, now time.Time) (StoragePlan, error) {
	if st == nil || hub == nil || hub.Dir == "" {
		return StoragePlan{}, fmt.Errorf("player is not configured")
	}
	values, err := st.Settings(ctx)
	if err != nil {
		return StoragePlan{}, err
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		return StoragePlan{}, err
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		return StoragePlan{}, err
	}
	space, err := previewSpace(hub.Recordings())
	if err != nil {
		return StoragePlan{}, err
	}
	gb := disk.WatermarkGB(values["watermarkGB"])
	reserve := disk.WatermarkBytes(values["watermarkGB"])
	fav, err := favoriteIDs(ctx, st, values)
	if err != nil {
		return StoragePlan{}, err
	}
	return buildPreview(previewInput{
		Now: now, Days: WatchedDays(values["deleteWatchedDays"]),
		MakeRoom: values["makeRoom"] == "1", ProtectFavorites: values["protectFavorites"] == "1",
		WatermarkGB: gb, Reserve: reserve, Free: space.Free, Root: hub.Recordings(),
		Favorites: fav, Passes: passes, Recs: recs,
	}), nil
}

// previewSpace reads free space for the recordings folder. A fresh install
// has not created that folder, or the work folder above it, and Statfs fails
// on a path that is not there. An ancestor that exists is the same disk.
// A dry run does not create the folder.
func previewSpace(dir string) (disk.Space, error) {
	space, err := diskStat(dir)
	if err == nil || !os.IsNotExist(err) {
		return space, err
	}
	cur := dir
	for {
		parent := filepath.Dir(cur)
		if parent == cur || parent == "" {
			return space, err
		}
		parentSpace, parentErr := diskStat(parent)
		if parentErr == nil {
			return parentSpace, nil
		}
		if !os.IsNotExist(parentErr) {
			return space, err
		}
		cur = parent
	}
}

type previewInput struct {
	Now              time.Time
	Days             int
	MakeRoom         bool
	ProtectFavorites bool
	WatermarkGB      int
	Reserve          uint64
	Free             uint64
	Root             string
	Favorites        map[int64]bool
	Passes           []store.Pass
	Recs             []store.Recording
}

func buildPreview(in previewInput) StoragePlan {
	fav := in.Favorites
	if !in.ProtectFavorites {
		fav = nil
	}
	out := StoragePlan{
		DeleteWatchedDays: in.Days,
		MakeRoom:          in.MakeRoom,
		ProtectFavorites:  in.ProtectFavorites,
		WatermarkGB:       in.WatermarkGB,
		FreeBytes:         in.Free,
		ReserveBytes:      in.Reserve,
		Remove:            []StoragePlanItem{},
		Held:              []StoragePlanItem{},
		Again:             []StoragePlanItem{},
		Covers:            true,
	}
	gone := map[int64]bool{}
	byID := map[int64]store.Recording{}
	for _, rec := range in.Recs {
		byID[rec.ID] = rec
	}

	var pool []store.Recording
	if in.Days > 0 || (in.MakeRoom && disk.BelowReserve(in.Free, in.Reserve)) {
		pool = cleanable(in.Root, in.Recs, in.Now, fav)
	}
	// The hourly clean-up deletes these before it reads free space again.
	// Count them here, or make-room lists recordings that pass would leave.
	var daysFreed uint64
	if in.Days > 0 {
		cutoff := in.Now.Add(-time.Duration(in.Days) * 24 * time.Hour)
		var aged []store.Recording
		for _, rec := range pool {
			if at := rec.WatchedSince(); !at.IsZero() && at.Before(cutoff) {
				aged = append(aged, rec)
			}
		}
		slices.SortStableFunc(aged, func(a, b store.Recording) int {
			if c := a.WatchedSince().Compare(b.WatchedSince()); c != 0 {
				return c
			}
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		for _, rec := range aged {
			out.Remove = append(out.Remove, item(rec, "watchedDays", "now", watchedReason(in.Days)))
			gone[rec.ID] = true
			daysFreed += fileSize(rec.Path)
		}
	}
	freeAfterDays := freedSpace(in.Free, daysFreed)

	if in.MakeRoom && disk.BelowReserve(in.Free, in.Reserve) && !disk.BelowReserve(freeAfterDays, in.Reserve) {
		out.RoomNote = "Deleting recordings watched past their days gets back to the reserve. Nothing else would be deleted."
	}
	if in.MakeRoom && disk.BelowReserve(freeAfterDays, in.Reserve) {
		short := in.Reserve - freeAfterDays
		out.ShortBytes = short
		var victims []store.Recording
		var could uint64
		for _, rec := range RoomOrder(pool) {
			if gone[rec.ID] {
				continue
			}
			victims = append(victims, rec)
			could += fileSize(rec.Path)
		}
		if could < short {
			out.Covers = false
			out.RoomNote = "There isn't enough watched video to get back to the reserve. Nothing would be deleted to make room."
		} else {
			var freed uint64
			for _, rec := range victims {
				if freed >= short {
					break
				}
				out.Remove = append(out.Remove, item(rec, "makeRoom", "now", "Watched, and the disk is under the reserve."))
				gone[rec.ID] = true
				freed += fileSize(rec.Path)
			}
			out.RoomNote = "Would delete the oldest watched recordings until the reserve is back."
		}
	}

	eligible := keepEligible(in.Recs, fav)
	for _, pass := range in.Passes {
		mode := keepMode(pass)
		if mode == "" {
			continue
		}
		for _, id := range KeepVictims(pass, eligible) {
			if gone[id] {
				continue
			}
			rec, ok := byID[id]
			if !ok || !insideDir(in.Root, rec.Path) {
				continue
			}
			out.Remove = append(out.Remove, item(rec, mode, "next", keepReason(pass, mode)))
			gone[id] = true
		}
	}

	for _, rec := range in.Recs {
		if row, ok := holdRow(in, rec, fav, gone, freeAfterDays); ok {
			out.Held = append(out.Held, row)
		}
		if row, ok := againRow(rec); ok {
			out.Again = append(out.Again, row)
		}
	}
	slices.SortStableFunc(out.Held, func(a, b StoragePlanItem) int {
		if c := strings.Compare(a.Title, b.Title); c != 0 {
			return c
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out
}

func item(rec store.Recording, rule, when, reason string) StoragePlanItem {
	return StoragePlanItem{ID: rec.ID, Title: rec.Title, Bytes: int64(fileSize(rec.Path)), Rule: rule, Reason: reason, When: when}
}

func watchedReason(days int) string {
	if days == 1 {
		return "Watched more than 1 day ago."
	}
	return fmt.Sprintf("Watched more than %d days ago.", days)
}

func keepMode(pass store.Pass) string {
	switch strings.ToLower(strings.TrimSpace(pass.KeepMode)) {
	case "last":
		return "keepLast"
	case "unwatched":
		return "keepUnwatched"
	default:
		return ""
	}
}

func keepReason(pass store.Pass, mode string) string {
	if mode == "keepUnwatched" {
		return pass.Title + " keeps only episodes you have not watched."
	}
	n := pass.KeepCount
	if n < 1 {
		n = 1
	}
	if n == 1 {
		return pass.Title + " keeps the newest episode."
	}
	return fmt.Sprintf("%s keeps the newest %d episodes.", pass.Title, n)
}

// freedSpace is free space after the days rule unlinks what it would.
// A sum that does not fit in uint64 is treated as no shortfall.
func freedSpace(free, extra uint64) uint64 {
	if extra > ^uint64(0)-free {
		return ^uint64(0)
	}
	return free + extra
}

func holdRow(in previewInput, rec store.Recording, fav map[int64]bool, gone map[int64]bool, free uint64) (StoragePlanItem, bool) {
	if gone[rec.ID] {
		return StoragePlanItem{}, false
	}
	if rec.Status == "recording" {
		return item(rec, "recording", "", "Still recording."), true
	}
	if rec.Status != "complete" {
		return StoragePlanItem{}, false
	}
	inside := insideDir(in.Root, rec.Path)
	if inside && !fileExists(rec.Path) {
		return item(rec, "missing", "", "The file is gone."), true
	}
	if !ruleWouldTake(in, rec, fav, free) {
		return StoragePlanItem{}, false
	}
	switch {
	case rec.Keep:
		return item(rec, "kept", "", "Kept forever."), true
	case fav[rec.ChannelID]:
		return item(rec, "favorite", "", "On a favorite channel."), true
	case !inside:
		return item(rec, "outside", "", "Outside the recordings folder."), true
	case rec.ProgressAt != nil && in.Now.Sub(*rec.ProgressAt) < recentPlay:
		return item(rec, "recent", "", "Played in the last 6 hours."), true
	default:
		return StoragePlanItem{}, false
	}
}

// ruleWouldTake reports whether some rule would remove this recording if
// nothing protected it. Protections are applied by the caller.
func ruleWouldTake(in previewInput, rec store.Recording, fav map[int64]bool, free uint64) bool {
	if rec.Status != "complete" {
		return false
	}
	plain := rec
	plain.Keep = false
	if finished(plain) {
		old := false
		if in.Days > 0 {
			if at := plain.WatchedSince(); !at.IsZero() && at.Before(in.Now.Add(-time.Duration(in.Days)*24*time.Hour)) {
				old = true
			}
		}
		room := in.MakeRoom && disk.BelowReserve(free, in.Reserve)
		if old || room {
			return true
		}
	}
	// A keep rule would count it if the file were here and the channel were
	// not favorite. Ask with those protections off.
	stripped := in.Recs
	if !fileExists(rec.Path) || fav[rec.ChannelID] || rec.Keep {
		copy := append([]store.Recording(nil), in.Recs...)
		for i := range copy {
			if copy[i].ID == rec.ID {
				copy[i].Keep = false
			}
		}
		stripped = copy
	}
	for _, pass := range in.Passes {
		if keepMode(pass) == "" {
			continue
		}
		for _, id := range KeepVictims(pass, stripped) {
			if id == rec.ID {
				return true
			}
		}
	}
	return false
}

func againRow(rec store.Recording) (StoragePlanItem, bool) {
	if rec.Status == "recording" || rec.Health == nil || !rec.Health.Damaged {
		return StoragePlanItem{}, false
	}
	if store.EpisodeKey(rec.ProgramID, rec.Title, rec.Subtitle, rec.ChannelID) == "" {
		return StoragePlanItem{}, false
	}
	return StoragePlanItem{
		ID: rec.ID, Title: rec.Title, Bytes: int64(fileSize(rec.Path)),
		Rule: "lostTime", Reason: lostReason(*rec.Health),
	}, true
}

func lostReason(h store.RecordingHealth) string {
	lost := math.Round(h.LostSeconds)
	if lost >= 1 {
		return fmt.Sprintf("Signal dropped for %.0f s.", lost)
	}
	times := h.ContinuityErrors + h.TransportErrors + h.SyncLosses
	if times == 1 {
		return "Signal broke up once."
	}
	return fmt.Sprintf("Signal broke up %d times.", times)
}
