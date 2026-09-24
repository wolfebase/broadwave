package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"waveguide/internal/psip"
	"waveguide/internal/store"
)

// ApplyBroadcast fills guide gaps from one mux's PSIP. Listings that overlap
// a listing already stored are left alone.
func (s *Server) ApplyBroadcast(ctx context.Context, g psip.Guide) (int, error) {
	if s == nil || s.Store == nil || len(g.Events) == 0 {
		return 0, nil
	}
	channels, err := s.Store.Channels(ctx, false)
	if err != nil {
		return 0, err
	}
	byNumber := map[string]int64{}
	for _, ch := range channels {
		byNumber[ch.GuideNumber] = ch.ID
	}
	sourceChannel := map[int]int64{}
	for _, ch := range g.Channels {
		id, ok := byNumber[fmt.Sprintf("%d.%d", ch.Major, ch.Minor)]
		if ok {
			sourceChannel[ch.SourceID] = id
		}
	}
	if len(sourceChannel) == 0 {
		return 0, nil
	}
	text := map[string]string{}
	for _, item := range g.Texts {
		text[fmt.Sprintf("%d:%d", item.SourceID, item.EventID)] = item.Body
	}
	var incoming []store.Airing
	seen := map[string]bool{}
	var from, to time.Time
	for _, ev := range g.Events {
		id, ok := sourceChannel[ev.SourceID]
		if !ok || ev.Title == "" || !ev.End.After(ev.Start) {
			continue
		}
		key := fmt.Sprintf("%d:%d", id, ev.Start.Unix())
		if seen[key] {
			continue
		}
		seen[key] = true
		title := psip.TitleCase(ev.Title)
		row := store.Airing{
			ChannelID:   id,
			Title:       title,
			Description: text[fmt.Sprintf("%d:%d", ev.SourceID, ev.EventID)],
			Category:    strings.Join(ev.Genres, ", "),
			GuideSource: "broadcast",
			Start:       ev.Start,
			End:         ev.End,
		}
		incoming = append(incoming, row)
		if from.IsZero() || ev.Start.Before(from) {
			from = ev.Start
		}
		if ev.End.After(to) {
			to = ev.End
		}
	}
	if len(incoming) == 0 {
		return 0, nil
	}
	have, err := s.Store.Airings(ctx, from, to)
	if err != nil {
		return 0, err
	}
	held := make([]psip.Span, 0, len(have))
	for _, row := range have {
		held = append(held, psip.Span{ChannelID: row.ChannelID, Start: row.Start, End: row.End})
	}
	want := make([]psip.Span, len(incoming))
	for i, row := range incoming {
		want[i] = psip.Span{ChannelID: row.ChannelID, Start: row.Start, End: row.End}
	}
	gaps := psip.FillGaps(held, want)
	if len(gaps) == 0 {
		return 0, nil
	}
	keep := map[string]bool{}
	for _, gap := range gaps {
		keep[fmt.Sprintf("%d:%d", gap.ChannelID, gap.Start.Unix())] = true
	}
	var rows []store.Airing
	for _, row := range incoming {
		if keep[fmt.Sprintf("%d:%d", row.ChannelID, row.Start.Unix())] {
			rows = append(rows, row)
		}
	}
	if err := s.Store.InsertAirings(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func tagGuideSource(rows []store.Airing, source string) []store.Airing {
	for i := range rows {
		if rows[i].GuideSource == "" {
			rows[i].GuideSource = source
		}
	}
	return rows
}
