package dvr

import (
	"context"
	"strings"
	"time"

	"broadwave/internal/store"
)

// Recorder starts a recording. *live.Hub is one.
type Recorder interface {
	RecordMeta(ctx context.Context, minutes int, meta store.Recording) (store.Recording, error)
}

// restartNote is the error a recording cut short by a restart keeps.
const restartNote = "Stopped when the server restarted."

// Recover closes recordings that were cut short when the process stopped and
// starts the rest of each show that is still on. A crash leaves the row
// "recording"; a clean stop (SIGTERM) already wrote "interrupted". Either way
// the file holds the show up to the stop, so it stays as an interrupted,
// partial recording and the rest becomes a second, partial piece with the
// same listing, pass, and episode. Neither piece counts as having the episode.
// The time left is the recording's planned end, its padding and any game
// extension included. A show already resumed is not resumed twice.
func Recover(ctx context.Context, st *store.Store, now time.Time, resume func(store.Recording, time.Duration) error) error {
	recs, err := st.Recordings(ctx)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if rec.Status != "recording" && rec.Status != "interrupted" {
			continue
		}
		if rec.Status == "recording" {
			if err := st.FinishRecording(ctx, rec.ID, "interrupted", restartNote); err != nil {
				return err
			}
		}
		if !rec.Partial {
			if err := st.MarkPartial(ctx, rec.ID, nil); err != nil {
				return err
			}
		}
		if resumedLater(recs, rec) || rec.EndsAt == nil {
			continue
		}
		left := rec.EndsAt.Sub(now)
		if left >= time.Minute && resume != nil {
			if err := resume(rec, left); err != nil {
				return err
			}
		}
	}
	return nil
}

// resumedLater is a later recording of the same show on the same channel,
// started before this one was due to end: this one was already picked up.
// Next week's episode of the same title is not a resume.
func resumedLater(recs []store.Recording, rec store.Recording) bool {
	for _, other := range recs {
		if other.ID > rec.ID && other.ChannelID == rec.ChannelID && strings.EqualFold(other.Title, rec.Title) &&
			!other.StartedAt.Before(rec.StartedAt) && (rec.EndsAt == nil || other.StartedAt.Before(*rec.EndsAt)) {
			return true
		}
	}
	return false
}

// minutesLeft rounds up, so the rest of a show is never cut a minute short.
func minutesLeft(left time.Duration) int {
	return int((left + time.Minute - 1) / time.Minute)
}

// Resume records the rest of an interrupted show for the time left, to the
// first piece's planned end and not past it: the
// same listing, pass, and game, and the episode the first piece had. The
// listing at the new start can already be the next show's, so the episode
// is copied rather than looked up again.
func Resume(ctx context.Context, st *store.Store, rec Recorder, prev store.Recording, left time.Duration) (store.Recording, error) {
	minutes := minutesLeft(left)
	if minutes < 1 {
		return store.Recording{}, nil
	}
	got, err := rec.RecordMeta(ctx, minutes, store.Recording{
		ChannelID: prev.ChannelID, Title: prev.Title, Subtitle: prev.Subtitle,
		Description: prev.Description, Category: prev.Category, ProgramID: prev.ProgramID, GameID: prev.GameID,
		PassID: prev.PassID, EndsAt: prev.EndsAt,
	})
	if err != nil {
		return got, err
	}
	// The channel's feed may already be recording something else; that one
	// is returned instead and is left alone.
	if got.ID > prev.ID && strings.EqualFold(got.Title, prev.Title) {
		if err := st.MarkPartial(ctx, got.ID, &prev); err != nil {
			return got, err
		}
	}
	return got, nil
}
