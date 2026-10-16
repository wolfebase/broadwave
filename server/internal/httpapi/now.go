package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"broadwave/internal/store"
)

// nowChannel is one guide channel's row on GET /now.
type nowChannel struct {
	ChannelID int64 `json:"channelId"`
	// Now is the listing on the air; absent when the guide has none.
	Now *store.Airing `json:"now,omitempty"`
	// Next is the listing after it; absent when the guide has none.
	Next *store.Airing `json:"next,omitempty"`
	// Progress is how far into Now the minute of "at" is, 0 to 1.
	Progress float64 `json:"progress"`
	// Art is the picture to show for the row: the listing's when it has
	// one, else the channel's. A path on this server.
	Art string `json:"art"`
}

// nowAhead is how far past now the next listing is looked for.
const nowAhead = 12 * time.Hour

// nowNext answers "what's on" for every guide channel in one small read: the
// listing on now and the one after, without a guide window. It is cached by
// minute and by the guide and channel versions.
func (s *Server) nowNext(w http.ResponseWriter, r *http.Request) {
	at := s.now().UTC().Truncate(time.Minute)
	key := s.versionKey(r.Context(), "now|"+at.Format(time.RFC3339), "guide", "channels")
	s.serveCached(w, r, key, func(ctx context.Context) (any, error) {
		channels, err := s.Store.Channels(ctx, true)
		if err != nil {
			return nil, err
		}
		list, err := s.Store.QueryAirings(ctx, store.AiringQuery{From: at, To: at.Add(nowAhead)})
		if err != nil {
			return nil, err
		}
		return map[string]any{"at": at, "channels": nowRows(channels, list, at)}, nil
	})
}

func nowRows(channels []store.Channel, list []store.Airing, at time.Time) []nowChannel {
	byChannel := map[int64][]store.Airing{}
	for _, a := range list {
		byChannel[a.ChannelID] = append(byChannel[a.ChannelID], gridListing(a))
	}
	out := make([]nowChannel, 0, len(channels))
	for _, ch := range channels {
		row := nowChannel{ChannelID: ch.ID, Art: "/media/art/channel/" + strconv.FormatInt(ch.ID, 10)}
		rows := byChannel[ch.ID]
		sort.Slice(rows, func(i, j int) bool { return rows[i].Start.Before(rows[j].Start) })
		for i := range rows {
			a := rows[i]
			if row.Now == nil && !a.Start.After(at) && a.End.After(at) {
				row.Now = &rows[i]
				continue
			}
			if a.Start.After(at) || a.Start.Equal(at) {
				if row.Now == nil || !a.Start.Before(row.Now.End) {
					row.Next = &rows[i]
					break
				}
			}
		}
		if row.Now != nil {
			if span := row.Now.End.Sub(row.Now.Start); span > 0 {
				row.Progress = float64(at.Sub(row.Now.Start)) / float64(span)
			}
			if row.Now.ImageURL != "" {
				row.Art = "/media/art/airing/" + strconv.FormatInt(row.Now.ID, 10)
			}
		}
		out = append(out, row)
	}
	return out
}
