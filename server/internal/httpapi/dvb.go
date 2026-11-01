package httpapi

import (
	"context"
	"slices"
	"strings"

	"broadwave/internal/dvbsi"
	"broadwave/internal/hdhr"
	"broadwave/internal/psip"
	"broadwave/internal/store"
)

// ApplyDVB fills guide gaps from one DVB mux's listings, as ApplyBroadcast
// does for ATSC. A listing for the tuned mux goes to the channel tuned at
// that frequency with its service as the program; one for another mux, or a
// service not tuned yet, to the DVB channel named as the SDT names it.
func (s *Server) ApplyDVB(ctx context.Context, freqHz int, g dvbsi.Guide) (int, error) {
	if s == nil || s.Store == nil || len(g.Events) == 0 {
		return 0, nil
	}
	// Each window of a mux arrives on its own goroutine; two at once would
	// both see the same gap.
	s.dvbMu.Lock()
	defer s.dvbMu.Unlock()
	rows, err := s.Store.BroadcastChannels(ctx)
	if err != nil {
		return 0, err
	}
	byProgram := map[int][]int64{}
	byName := map[string][]int64{}
	perDevice := map[string]int{}
	for _, row := range rows {
		if freqHz > 0 && row.FrequencyHz == freqHz && row.ProgramNum > 0 {
			byProgram[row.ProgramNum] = append(byProgram[row.ProgramNum], row.ID)
		}
		if hdhr.DVB(row.Model) && row.Name != "" {
			name := strings.ToLower(row.Name)
			byName[name] = append(byName[name], row.ID)
			perDevice[row.DeviceID+"\x00"+name]++
		}
	}
	// A name one tuner gives two channels (regional versions of one service)
	// says nothing about which.
	for key, n := range perDevice {
		if n > 1 {
			_, name, _ := strings.Cut(key, "\x00")
			delete(byName, name)
		}
	}
	names := map[[3]int]string{}
	for _, svc := range g.Services {
		names[[3]int{svc.NetworkID, svc.StreamID, svc.ID}] = strings.ToLower(svc.Name)
	}
	events := slices.Clone(g.Events)
	// The present/following table is the more current; it claims its time first.
	slices.SortStableFunc(events, func(a, b dvbsi.Event) int {
		switch {
		case a.NowNext == b.NowNext:
			return 0
		case a.NowNext:
			return -1
		default:
			return 1
		}
	})
	var incoming []store.Airing
	var spans []psip.Span
	for _, ev := range events {
		var ids []int64
		if ev.Actual {
			ids = byProgram[ev.ServiceID]
		}
		if len(ids) == 0 {
			ids = byName[names[[3]int{ev.NetworkID, ev.StreamID, ev.ServiceID}]]
		}
		for _, id := range ids {
			span := psip.Span{ChannelID: id, Start: ev.Start, End: ev.End}
			if len(psip.FillGaps(spans, []psip.Span{span})) == 0 {
				continue
			}
			spans = append(spans, span)
			incoming = append(incoming, store.Airing{
				ChannelID:   id,
				Title:       psip.TitleCase(ev.Title),
				Subtitle:    ev.Subtitle,
				Description: ev.Description,
				Category:    strings.Join(ev.Genres, ", "),
				Rating:      ev.Rating,
				Live:        ev.Live,
				GuideSource: "broadcast",
				Start:       ev.Start,
				End:         ev.End,
			})
		}
	}
	return s.fillBroadcast(ctx, incoming)
}
