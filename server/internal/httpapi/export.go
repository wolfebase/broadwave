package httpapi

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
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
	encoder := ""
	if s.Hub != nil {
		encoder = s.Hub.Encoder
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("#EXTM3U url-tvg=\"%s/export/guide.xml\"\n", base))
	for _, ch := range channels {
		id := strconv.FormatInt(ch.ID, 10)
		// channel-id matches the XMLTV id. Channels treats that attribute as the
		// guide id when tvc-guide-stationid is absent, and reads the on-screen
		// number from channel-number.
		b.WriteString(m3uEntry(m3uText(ch.DisplayName), base+"/export/stream/"+id,
			m3uAttr("tvg-id", xmltvChannelID(ch)),
			m3uAttr("tvg-chno", m3uText(ch.DisplayNumber)),
			m3uAttr("tvg-name", m3uText(ch.DisplayName)),
			m3uAttr("channel-id", xmltvChannelID(ch)),
			m3uAttr("channel-number", m3uText(ch.DisplayNumber)),
			m3uAttr("tvc-stream-vcodec", channelsStreamCodec(ch.VideoCodec)),
			m3uAttr("tvc-stream-acodec", channelsStreamCodec(ch.AudioCodec)),
		))
	}
	video, audio := mosaicAdvertised(encoder)
	for _, mo := range sharedMosaics(r.Context(), s.Store) {
		b.WriteString(m3uEntry(m3uText(mo.name), base+"/export/mosaic/"+mo.key,
			m3uAttr("tvg-id", "mosaic."+mo.key),
			m3uAttr("tvg-chno", mo.number),
			m3uAttr("tvg-name", m3uText(mo.name)),
			m3uAttr("channel-id", "mosaic."+mo.key),
			m3uAttr("channel-number", mo.number),
			m3uAttr("tvc-stream-vcodec", channelsStreamCodec(video)),
			m3uAttr("tvc-stream-acodec", channelsStreamCodec(audio)),
			m3uAttr("group-title", "Multiview"),
		))
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
	Start    string         `xml:"start,attr"`
	Stop     string         `xml:"stop,attr"`
	Channel  string         `xml:"channel,attr"`
	Title    string         `xml:"title"`
	SubTitle string         `xml:"sub-title,omitempty"`
	Desc     string         `xml:"desc,omitempty"`
	Category []string       `xml:"category,omitempty"`
	Date     string         `xml:"date,omitempty"`
	Series   *xmltvSeriesID `xml:"series-id,omitempty"`
	Episode  []xmltvEpNum   `xml:"episode-num,omitempty"`
	New      *struct{}      `xml:"new,omitempty"`
	Live     *struct{}      `xml:"live,omitempty"`
}

type xmltvSeriesID struct {
	System string `xml:"system,attr"`
	Value  string `xml:",chardata"`
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
	doc := xmltvDoc{Generator: "Broadwave"}
	ids := map[int64]string{}
	for _, ch := range channels {
		id := xmltvChannelID(ch)
		ids[ch.ID] = id
		doc.Channels = append(doc.Channels, xmltvChannel{ID: id, Display: []string{ch.DisplayName, ch.DisplayNumber}})
	}
	const layout = "20060102150405 -0700"
	// A mosaic has no listings of its own. Two-hour blocks say what it shows,
	// so apps that hide a channel with no guide still list it.
	from := time.Now().Truncate(2 * time.Hour)
	for _, mo := range sharedMosaics(r.Context(), s.Store) {
		id := "mosaic." + mo.key
		doc.Channels = append(doc.Channels, xmltvChannel{ID: id, Display: []string{mo.name, mo.number}})
		for at := from; at.Before(from.Add(14 * 24 * time.Hour)); at = at.Add(2 * time.Hour) {
			doc.Programmes = append(doc.Programmes, xmltvProgram{Start: at.Format(layout), Stop: at.Add(2 * time.Hour).Format(layout), Channel: id, Title: "Multiview", Desc: mo.about})
		}
	}
	for _, a := range airings {
		id, ok := ids[a.ChannelID]
		if !ok {
			continue
		}
		p := xmltvProgram{Start: a.Start.Format(layout), Stop: a.End.Format(layout), Channel: id, Title: a.Title, SubTitle: a.Subtitle, Desc: a.Description}
		p.Category = xmltvCategories(a.Category)
		p.Date = strings.ReplaceAll(a.OriginalAir, "-", "")
		if a.SeriesID != "" {
			p.Series = &xmltvSeriesID{System: "broadwave", Value: a.SeriesID}
		}
		p.Episode = xmltvEpisodes(a)
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

func m3uAttr(key, value string) string {
	if value == "" {
		return ""
	}
	return key + `="` + value + `"`
}

func m3uEntry(title, url string, attrs ...string) string {
	var b strings.Builder
	b.WriteString("#EXTINF:-1")
	for _, attr := range attrs {
		if attr == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(attr)
	}
	b.WriteByte(',')
	b.WriteString(title)
	b.WriteByte('\n')
	b.WriteString(url)
	b.WriteByte('\n')
	return b.String()
}

// channelsStreamCodec is a codec name Channels records against. Anything else
// is left off the playlist rather than sent as a name Channels does not know.
func channelsStreamCodec(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "-", "")
	switch s {
	case "mpeg2", "mp2v":
		return "mpeg2"
	case "h264", "avc":
		return "h264"
	case "hevc", "h265":
		return "hevc"
	case "ac3":
		return "ac3"
	case "aac":
		return "aac"
	case "eac3":
		return "eac3"
	default:
		return ""
	}
}

// mosaicAdvertised is the picture and sound a shared multiview is encoded as.
// Sound is AAC. Picture follows the server encoder, and is H.264 when that
// encoder is unset or is not HEVC.
func mosaicAdvertised(encoder string) (video, audio string) {
	audio = "AAC"
	enc := strings.ToLower(encoder)
	if strings.Contains(enc, "hevc") || strings.Contains(enc, "h265") || strings.Contains(enc, "265") {
		return "HEVC", audio
	}
	return "H264", audio
}

func xmltvCategories(raw string) []string {
	var out []string
	for _, c := range strings.Split(raw, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.EqualFold(c, "movie") || strings.EqualFold(c, "movies") {
			c = "Movie"
		}
		out = append(out, c)
	}
	return out
}

// xmltvEpisodes prefers a zero-padded on-screen number when the listing stored
// a season and an episode. A label is used only when those integers are absent.
func xmltvEpisodes(a store.Airing) []xmltvEpNum {
	var out []xmltvEpNum
	if a.ProgramID != "" {
		out = append(out, xmltvEpNum{System: "dd_progid", Value: a.ProgramID})
	}
	switch {
	case a.Season > 0 && a.Episode > 0:
		out = append(out, xmltvEpNum{System: "onscreen", Value: fmt.Sprintf("S%02dE%02d", a.Season, a.Episode)})
	case a.EpisodeLabel != "":
		out = append(out, xmltvEpNum{System: "onscreen", Value: a.EpisodeLabel})
	}
	return out
}
