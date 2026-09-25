package guide

import (
	"encoding/xml"

	"broadwave/internal/store"
)

// Networks reads an XMLTV document and returns the network for each lineup channel it matches.
func Networks(data []byte, lineup []store.Channel) map[int64]string {
	var doc tv
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	assigned := assign(doc.Channels, lineup)
	byID := make(map[int64]store.Channel, len(lineup))
	for _, ch := range lineup {
		byID[ch.ID] = ch
	}
	names := map[int64][]string{}
	for _, ch := range doc.Channels {
		id := assigned[ch.ID]
		if id == 0 {
			continue
		}
		names[id] = append(names[id], ch.Names...)
	}
	out := map[int64]string{}
	for id, list := range names {
		line := byID[id]
		if net := Affiliation(list, line.GuideName, line.DisplayName); net != "" {
			out[id] = net
		}
	}
	return out
}
