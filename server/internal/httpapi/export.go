package httpapi

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"waveguide/internal/live"
	"waveguide/internal/store"
)

func asBusy(err error, target **live.BusyError) bool {
	return errors.As(err, target)
}

// exportLineup is an M3U playlist of the guide lineup for other apps. Streams
// ride the shared tune.
func (s *Server) exportLineup(w http.ResponseWriter, r *http.Request) {
	channels, err := s.Store.Channels(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	base := "http://" + r.Host
	var b strings.Builder
	b.WriteString(fmt.Sprintf("#EXTM3U url-tvg=\"%s/export/guide.xml\"\n", base))
	for _, ch := range channels {
		id := strconv.FormatInt(ch.ID, 10)
		b.WriteString(fmt.Sprintf("#EXTINF:-1 tvg-id=\"%s\" tvg-chno=\"%s\" tvg-name=%q channel-id=\"%s\",%s\n",
			xmltvChannelID(ch), ch.DisplayNumber, ch.DisplayName, ch.DisplayNumber, ch.DisplayName))
		b.WriteString(base + "/export/stream/" + id + "\n")
	}
	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) exportStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || s.Hub == nil {
		http.NotFound(w, r)
		return
	}
	exportChannel(w, r, s.Hub, id)
}

func xmltvChannelID(ch store.Channel) string {
	return "ota." + strings.ReplaceAll(ch.DisplayNumber, ".", "-")
}

type xmltvDoc struct {
	XMLName    xml.Name       `xml:"tv"`
	Generator  string         `xml:"generator-info-name,attr"`
	Channels   []xmltvChannel `xml:"channel"`
	Programmes []xmltvProgram `xml:"programme"`
}

type xmltvChannel struct {
	ID      string   `xml:"id,attr"`
	Display []string `xml:"display-name"`
}

type xmltvProgram struct {
	Start    string       `xml:"start,attr"`
	Stop     string       `xml:"stop,attr"`
	Channel  string       `xml:"channel,attr"`
	Title    string       `xml:"title"`
	SubTitle string       `xml:"sub-title,omitempty"`
	Desc     string       `xml:"desc,omitempty"`
	Category []string     `xml:"category,omitempty"`
	Date     string       `xml:"date,omitempty"`
	Episode  []xmltvEpNum `xml:"episode-num,omitempty"`
	New      *struct{}    `xml:"new,omitempty"`
	Live     *struct{}    `xml:"live,omitempty"`
}

type xmltvEpNum struct {
	System string `xml:"system,attr"`
	Value  string `xml:",chardata"`
}

// exportGuide is XMLTV for the same lineup, 14 days ahead.
func (s *Server) exportGuide(w http.ResponseWriter, r *http.Request) {
	channels, err := s.Store.Channels(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	airings, err := s.Store.Airings(r.Context(), time.Now().Add(-2*time.Hour), time.Now().Add(14*24*time.Hour))
	if err != nil {
		writeError(w, err)
		return
	}
	doc := xmltvDoc{Generator: "Waveguide"}
	ids := map[int64]string{}
	for _, ch := range channels {
		id := xmltvChannelID(ch)
		ids[ch.ID] = id
		doc.Channels = append(doc.Channels, xmltvChannel{ID: id, Display: []string{ch.DisplayName, ch.DisplayNumber}})
	}
	const layout = "20060102150405 -0700"
	for _, a := range airings {
		id, ok := ids[a.ChannelID]
		if !ok {
			continue
		}
		p := xmltvProgram{Start: a.Start.Format(layout), Stop: a.End.Format(layout), Channel: id, Title: a.Title, SubTitle: a.Subtitle, Desc: a.Description}
		for _, c := range strings.Split(a.Category, ",") {
			if c = strings.TrimSpace(c); c != "" {
				p.Category = append(p.Category, c)
			}
		}
		p.Date = strings.ReplaceAll(a.OriginalAir, "-", "")
		if a.ProgramID != "" {
			p.Episode = append(p.Episode, xmltvEpNum{System: "dd_progid", Value: a.ProgramID})
		}
		if a.EpisodeLabel != "" {
			p.Episode = append(p.Episode, xmltvEpNum{System: "onscreen", Value: a.EpisodeLabel})
		}
		if a.New {
			p.New = &struct{}{}
		}
		if a.Live {
			p.Live = &struct{}{}
		}
		doc.Programmes = append(doc.Programmes, p)
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", " ")
	_ = enc.Encode(doc)
}
