// Package fake is an HDHomeRun that stays on localhost: discover, lineup, control, and a looping MPEG-TS.
package fake

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	typeGetSetReq   = 0x0004
	typeGetSetRpy   = 0x0005
	typeDiscoverRpy = 0x0003
	tagGetSetName   = 0x03
	tagGetSetValue  = 0x04
	tagError        = 0x05

	tagDeviceType = 0x01
	tagDeviceID   = 0x02
	tagTunerCount = 0x10
	tagLineupURL  = 0x27
	tagStorageURL = 0x28
	tagBaseURL    = 0x2A
	tagDeviceAuth = 0x2B
)

// Channel is one lineup row on a frequency.
type Channel struct {
	Number string
	Name   string
	Freq   int
	Video  string
	Audio  string
	// ATSC3 is an ATSC 3.0 row (virtual channel 100+ on a FLEX 4K).
	ATSC3 bool
	DRM   bool
	// Copy is "copy-once" or "copy-never" on a CableCARD channel. Empty means copy freely.
	Copy string
}

// Server is an HDHomeRun. HTTP serves discover, lineup, and the stream. Control is TCP.
// Profile empty is the original two-tuner fake. TunerCount, when set, is 1 through 8.
type Server struct {
	Channels []Channel
	TS       string
	// Realtime spreads one pass of TS across four seconds so a relay can build a live playlist.
	Realtime   bool
	Profile    string
	TunerCount int

	spec   profile
	httpLn net.Listener
	ctrlLn net.Listener

	mu       sync.Mutex
	tuners   []tuner
	streams  int
	scanning bool
	scanOnce bool
}

type tuner struct {
	guide     string
	freq      int
	held      bool
	dead      bool
	transcode string
	stop      chan struct{}
}

type discoverJSON struct {
	FriendlyName     string `json:"FriendlyName"`
	ModelNumber      string `json:"ModelNumber"`
	FirmwareName     string `json:"FirmwareName"`
	FirmwareVersion  string `json:"FirmwareVersion"`
	UpgradeAvailable string `json:"UpgradeAvailable,omitempty"`
	DeviceID         string `json:"DeviceID"`
	DeviceAuth       string `json:"DeviceAuth,omitempty"`
	BaseURL          string `json:"BaseURL"`
	LineupURL        string `json:"LineupURL"`
	StorageURL       string `json:"StorageURL,omitempty"`
	TunerCount       int    `json:"TunerCount"`
}

type lineupJSON struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
	VideoCodec  string `json:"VideoCodec,omitempty"`
	AudioCodec  string `json:"AudioCodec,omitempty"`
	HD          int    `json:"HD,omitempty"`
	DRM         *int   `json:"DRM,omitempty"`
	Tags        string `json:"Tags,omitempty"`
}

// Start listens on 127.0.0.1. BaseURL is the HTTP origin. Control is the TCP port.
func (s *Server) Start() (baseURL, control string, err error) {
	if err := s.applyProfile(); err != nil {
		return "", "", err
	}
	s.httpLn, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", err
	}
	s.ctrlLn, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = s.httpLn.Close()
		return "", "", err
	}
	go s.serveHTTP()
	go s.serveControl()
	_, port, _ := net.SplitHostPort(s.ctrlLn.Addr().String())
	return "http://" + s.httpLn.Addr().String(), port, nil
}

func (s *Server) applyProfile() error {
	if s.Profile == "" {
		s.spec = defaultProfile()
	} else {
		p, ok := lookupProfile(s.Profile)
		if !ok {
			return fmt.Errorf("unknown hdhr profile %q", s.Profile)
		}
		s.spec = p
	}
	if s.TunerCount > 0 {
		n := s.TunerCount
		if n > maxTuners {
			n = maxTuners
		}
		s.spec.Tuners = n
		s.spec.legacy = false
	}
	if s.spec.Tuners < 0 {
		s.spec.Tuners = 0
	}
	s.tuners = make([]tuner, s.spec.Tuners)
	if len(s.Channels) == 0 {
		if s.spec.ownLineup {
			s.Channels = append([]Channel(nil), s.spec.channels...)
		} else {
			s.Channels = antennaChannels()
		}
	}
	return nil
}

// Close releases both listeners and any profile stream still writing.
func (s *Server) Close() {
	s.mu.Lock()
	for i := range s.tuners {
		s.closeStopLocked(i)
	}
	s.mu.Unlock()
	if s.httpLn != nil {
		_ = s.httpLn.Close()
	}
	if s.ctrlLn != nil {
		_ = s.ctrlLn.Close()
	}
}

// KillTuner closes one tuner mid-stream. The next tune skips it.
func (s *Server) KillTuner(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 || n >= len(s.tuners) {
		return
	}
	s.closeStopLocked(n)
	s.tuners[n].dead = true
	s.tuners[n].held = false
	s.tuners[n].guide = ""
	s.tuners[n].freq = 0
	s.tuners[n].transcode = ""
}

func (s *Server) closeStopLocked(n int) {
	if s.tuners[n].stop != nil {
		close(s.tuners[n].stop)
		s.tuners[n].stop = nil
	}
}

// DiscoveryReply is the UDP discovery datagram for this device.
// It is not sent. Tests parse it without binding port 65001.
func (s *Server) DiscoveryReply() []byte {
	if s.httpLn == nil {
		return nil
	}
	base := "http://" + s.httpLn.Addr().String()
	kind := byte(1)
	if s.spec.Tuners == 0 && s.spec.Storage {
		kind = 5
	}
	var id [4]byte
	if n, err := strconv.ParseUint(s.spec.DeviceID, 16, 32); err == nil && len(s.spec.DeviceID) == 8 {
		binary.BigEndian.PutUint32(id[:], uint32(n))
	} else {
		copy(id[:], s.spec.DeviceID)
	}
	payload := tlvRaw(tagDeviceType, []byte{0, 0, 0, kind})
	payload = append(payload, tlvRaw(tagDeviceID, id[:])...)
	payload = append(payload, tlvRaw(tagTunerCount, []byte{byte(len(s.tuners))})...)
	payload = append(payload, tlvRaw(tagBaseURL, []byte(base))...)
	payload = append(payload, tlvRaw(tagLineupURL, []byte(base+"/lineup.json"))...)
	if s.spec.Storage {
		payload = append(payload, tlvRaw(tagStorageURL, []byte(base+"/recorded_files.json"))...)
	}
	if s.spec.Auth {
		payload = append(payload, tlvRaw(tagDeviceAuth, []byte(fakeDeviceAuth))...)
	}
	return seal(typeDiscoverRpy, payload)
}

func (s *Server) serveHTTP() {
	mux := http.NewServeMux()
	mux.HandleFunc("/discover.json", s.discover)
	mux.HandleFunc("/lineup.json", s.lineup)
	mux.HandleFunc("/lineup_status.json", s.lineupStatus)
	mux.HandleFunc("/lineup.post", s.lineupPost)
	mux.HandleFunc("/status.json", s.status)
	mux.HandleFunc("/auto/", s.stream)
	for i := range s.tuners {
		mux.HandleFunc(fmt.Sprintf("/tuner%d/", i), s.stream)
	}
	if s.spec.Storage {
		mux.HandleFunc("/recorded_files.json", s.recorded)
	}
	// No firmware is installed from here. These routes never write a file.
	mux.HandleFunc("/upgrade", refuseUpgrade)
	mux.HandleFunc("/firmware", refuseUpgrade)
	_ = http.Serve(s.httpLn, mux)
}

func refuseUpgrade(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "firmware is not installed from this device", http.StatusNotFound)
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	doc := discoverJSON{
		FriendlyName:     s.spec.Friendly,
		ModelNumber:      s.spec.Model,
		FirmwareName:     s.spec.Firmware,
		FirmwareVersion:  s.spec.Version,
		UpgradeAvailable: s.spec.Upgrade,
		DeviceID:         s.spec.DeviceID,
		BaseURL:          base,
		LineupURL:        base + "/lineup.json",
		TunerCount:       len(s.tuners),
	}
	if s.spec.Auth {
		doc.DeviceAuth = fakeDeviceAuth
	}
	if s.spec.Storage {
		doc.StorageURL = base + "/recorded_files.json"
	}
	writeJSON(w, doc)
}

func (s *Server) lineup(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	rows := make([]lineupJSON, 0, len(s.Channels))
	for _, ch := range s.Channels {
		row := lineupJSON{
			GuideNumber: ch.Number,
			GuideName:   ch.Name,
			URL:         base + "/auto/v" + ch.Number,
			HD:          1,
		}
		if !s.spec.Old {
			video, audio := ch.Video, ch.Audio
			if video == "" {
				video = "MPEG2"
			}
			if audio == "" {
				audio = "AC3"
			}
			row.VideoCodec = video
			row.AudioCodec = audio
		}
		if ch.DRM || ch.Copy != "" {
			one := 1
			row.DRM = &one
			tags := ch.Copy
			if tags != "" {
				tags += ",drm"
			} else {
				tags = "drm"
			}
			row.Tags = tags
		}
		rows = append(rows, row)
	}
	writeJSON(w, rows)
}

func (s *Server) lineupPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Query().Get("scan") != "start" {
		http.Error(w, "bad scan", http.StatusBadRequest)
		return
	}
	if s.spec.Tuners == 0 {
		http.Error(w, "no tuner", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.scanning = true
	s.scanOnce = false
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) lineupStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if s.scanning && !s.scanOnce {
		s.scanOnce = true
		fmt.Fprint(w, `{"Scan":1,"ScanInProgress":1,"Found":0,"Progress":0}`)
		return
	}
	s.scanning = false
	fmt.Fprintf(w, `{"Scan":0,"Found":%d,"Progress":100,"ScanInProgress":0}`, len(s.Channels))
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type row struct {
		Resource              string `json:"Resource"`
		VctNumber             string `json:"VctNumber"`
		VctName               string `json:"VctName"`
		TargetIP              string `json:"TargetIP"`
		SignalStrengthPercent int    `json:"SignalStrengthPercent"`
		SignalQualityPercent  int    `json:"SignalQualityPercent"`
		SymbolQualityPercent  int    `json:"SymbolQualityPercent"`
	}
	rows := make([]row, len(s.tuners))
	for i, t := range s.tuners {
		target := ""
		switch {
		case t.dead:
			target = "closed"
		case t.held:
			target = "127.0.0.1"
		}
		rows[i] = row{
			Resource: "tuner" + strconv.Itoa(i), VctNumber: t.guide, VctName: t.guide, TargetIP: target,
			SignalStrengthPercent: 90, SignalQualityPercent: 88, SymbolQualityPercent: 100,
		}
	}
	writeJSON(w, rows)
}

func (s *Server) recorded(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	writeJSON(w, []map[string]string{{
		"Title":        "News",
		"EpisodeTitle": "At 6",
		"Filename":     "news.ts",
		"PlayURL":      base + "/recorded/news.ts",
	}})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if s.legacyStream() {
		s.streamLegacy(w, r)
		return
	}
	s.streamAllocated(w, r)
}

// legacyStream keeps the original HTTP stream for the default server.
// A control hold does not consume one of those streams: both tuners can be
// tuned and /auto/v4.1 still reads.
func (s *Server) legacyStream() bool {
	return s.spec.legacy && s.TunerCount == 0
}

func (s *Server) streamLegacy(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "/ch") || strings.Contains(r.URL.Path, "/auto/") {
		s.mu.Lock()
		limit := len(s.tuners)
		if limit == 0 {
			limit = 2
		}
		if s.streams >= limit {
			s.mu.Unlock()
			w.Header().Set("X-HDHomeRun-Error", "805 All Tuners In Use")
			http.Error(w, "805 All tuners in use", http.StatusServiceUnavailable)
			return
		}
		s.streams++
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.streams--
			if n := tunerFromPath(r.URL.Path); n >= 0 && n < len(s.tuners) {
				dead := s.tuners[n].dead
				s.tuners[n] = tuner{dead: dead}
			}
			s.mu.Unlock()
		}()
	}
	w.Header().Set("Content-Type", "video/mp2t")
	if s.Realtime {
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-re",
			"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=60000/1001",
			"-f", "lavfi", "-i", "sine=frequency=500",
			"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p",
			"-c:a", "ac3", "-f", "mpegts", "pipe:1")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := cmd.Start(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
		_, _ = io.Copy(w, stdout)
		return
	}
	s.loopFile(w, nil, nil)
}

func (s *Server) streamAllocated(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	scanning := s.scanning
	s.mu.Unlock()
	if scanning {
		writeErr(w, http.StatusServiceUnavailable, "803 System Busy")
		return
	}
	forced, target := streamRequest(r.URL.Path)
	if target == "" {
		writeErr(w, http.StatusNotFound, "801 Unknown Channel")
		return
	}
	ch, ok := s.find(target)
	if !ok {
		writeErr(w, http.StatusNotFound, "801 Unknown Channel")
		return
	}
	// 811 is immediate. A real PRIME waits out authorization before the same code.
	if ch.DRM || ch.Copy != "" {
		writeErr(w, http.StatusServiceUnavailable, "811 Content Protection Required")
		return
	}
	transcode := r.URL.Query().Get("transcode")
	if transcode != "" && s.spec.Extend {
		if !transcodeOK(transcode) {
			writeErr(w, http.StatusServiceUnavailable, "802 Unknown Transcode Profile")
			return
		}
	} else {
		transcode = ""
	}
	idx, stop, errMsg := s.allocate(forced, ch)
	if errMsg != "" {
		code := http.StatusServiceUnavailable
		if strings.HasPrefix(errMsg, "801") {
			code = http.StatusNotFound
		}
		writeErr(w, code, errMsg)
		return
	}
	defer s.release(idx)
	var pkt []byte
	switch {
	case transcode != "":
		pkt = markerPacket("AVC", "AAC")
	case ch.Video == "HEVC":
		pkt = markerPacket("HEVC", "AC-4")
	case s.TS == "":
		pkt = markerPacket("MPEG2", "AC3")
	}
	done := r.Context().Done()
	if pkt != nil {
		s.loopPacket(w, pkt, stop, done)
		return
	}
	s.loopFile(w, stop, done)
}

func (s *Server) allocate(forced int, ch Channel) (int, <-chan struct{}, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scanning {
		return -1, nil, "803 System Busy"
	}
	if forced >= 0 {
		if forced >= len(s.tuners) {
			return -1, nil, "804 Tuner In Use"
		}
		if s.tuners[forced].dead || !s.canTune(forced, ch) {
			return -1, nil, "806 Tune Failed"
		}
		if s.tuners[forced].held {
			return -1, nil, "804 Tuner In Use"
		}
		return s.assignLocked(forced, ch)
	}
	sawIncapable := false
	for i := range s.tuners {
		if s.tuners[i].dead || s.tuners[i].held {
			continue
		}
		if !s.canTune(i, ch) {
			sawIncapable = true
			continue
		}
		return s.assignLocked(i, ch)
	}
	if sawIncapable {
		return -1, nil, "806 Tune Failed"
	}
	return -1, nil, "805 All tuners in use"
}

func (s *Server) assignLocked(i int, ch Channel) (int, <-chan struct{}, string) {
	stop := make(chan struct{})
	s.tuners[i].stop = stop
	s.tuners[i].held = true
	s.tuners[i].guide = ch.Number
	s.tuners[i].freq = ch.Freq
	return i, stop, ""
}

func (s *Server) canTune(i int, ch Channel) bool {
	if !ch.ATSC3 || s.spec.ATSC3 == 0 {
		return true
	}
	return i < s.spec.ATSC3
}

func (s *Server) release(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 || n >= len(s.tuners) {
		return
	}
	s.closeStopLocked(n)
	dead := s.tuners[n].dead
	s.tuners[n] = tuner{dead: dead}
}

func (s *Server) loopPacket(w http.ResponseWriter, pkt []byte, stop, done <-chan struct{}) {
	w.Header().Set("Content-Type", "video/mp2t")
	for {
		if stopped(stop, done) {
			return
		}
		if _, err := w.Write(pkt); err != nil {
			return
		}
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-stop:
			timer.Stop()
			return
		case <-done:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func stopped(stop, done <-chan struct{}) bool {
	if stop != nil {
		select {
		case <-stop:
			return true
		default:
		}
	}
	if done != nil {
		select {
		case <-done:
			return true
		default:
		}
	}
	return false
}

func (s *Server) loopFile(w http.ResponseWriter, stop, done <-chan struct{}) {
	f, err := os.Open(s.TS)
	if err != nil {
		http.Error(w, "no sample", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, _ := f.Stat()
	buf := make([]byte, 188*16)
	start := time.Now()
	var sent int64
	for {
		if stopped(stop, done) {
			return
		}
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			sent += int64(n)
			if s.Realtime && info.Size() > 0 {
				want := time.Duration(int64(4*time.Second) * sent / info.Size())
				if wait := want - time.Since(start); wait > 0 {
					timer := time.NewTimer(wait)
					select {
					case <-stop:
						timer.Stop()
						return
					case <-done:
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			}
		}
		if err == io.EOF {
			_, _ = f.Seek(0, io.SeekStart)
			start = time.Now()
			sent = 0
			continue
		}
		if err != nil {
			return
		}
	}
}

func (s *Server) serveControl() {
	for {
		conn, err := s.ctrlLn.Accept()
		if err != nil {
			return
		}
		go s.control(conn)
	}
}

func (s *Server) control(conn net.Conn) {
	defer conn.Close()
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return
	}
	length := int(binary.BigEndian.Uint16(head[2:4]))
	rest := make([]byte, length+4)
	if _, err := io.ReadFull(conn, rest); err != nil {
		return
	}
	packet := append(head, rest...)
	if binary.BigEndian.Uint16(packet[0:2]) != typeGetSetReq {
		return
	}
	name, value, set := "", "", false
	body := packet[4 : 4+length]
	for len(body) > 0 {
		tag := body[0]
		body = body[1:]
		n, next := readLen(body)
		if n < 0 || len(next) < n {
			return
		}
		val := strings.TrimRight(string(next[:n]), "\x00")
		body = next[n:]
		switch tag {
		case tagGetSetName:
			name = val
		case tagGetSetValue:
			value = val
			set = true
		}
	}
	reply, errMsg := s.command(name, value, set)
	payload := tlv(tagGetSetValue, reply)
	if errMsg != "" {
		payload = tlv(tagError, errMsg)
	}
	_, _ = conn.Write(seal(typeGetSetRpy, payload))
}

func (s *Server) command(name, value string, set bool) (string, string) {
	if name == "/sys/model" {
		return s.spec.Model, ""
	}
	if name == "/sys/version" {
		return s.spec.Version, ""
	}
	index, tail, ok := strings.Cut(strings.TrimPrefix(name, "/tuner"), "/")
	if !ok {
		return "", "unknown"
	}
	n, err := strconv.Atoi(index)
	if err != nil || n < 0 || n >= len(s.tuners) {
		return "", "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// The original two-tuner fake stays tunable while a scan is open.
	// A named profile reports 803 until lineup_status.json says the scan is done.
	if s.scanning && !s.spec.legacy && tail != "status" && tail != "streaminfo" {
		return "", "803 System Busy"
	}
	t := &s.tuners[n]
	switch tail {
	case "transcode":
		if !s.spec.Extend {
			return "", "unknown"
		}
		if !set {
			if t.transcode == "" {
				return "none", ""
			}
			return t.transcode, ""
		}
		if !transcodeOK(value) {
			return "", "802 Unknown Transcode Profile"
		}
		t.transcode = value
		return value, ""
	case "channel", "vchannel":
		if !set {
			if t.freq == 0 {
				return "none", ""
			}
			return fmt.Sprintf("auto:%d", t.freq), ""
		}
		if value == "none" {
			t.guide, t.freq, t.held, t.transcode = "", 0, false, ""
			return "none", ""
		}
		if t.dead {
			return "", "806 Tune Failed"
		}
		ch, found := s.find(value)
		if !found {
			return "", "unknown channel"
		}
		if ch.DRM || ch.Copy != "" {
			return "", "811 Content Protection Required"
		}
		if ch.ATSC3 && s.spec.ATSC3 > 0 && n >= s.spec.ATSC3 {
			return "", "806 Tune Failed"
		}
		if t.held && t.freq != ch.Freq {
			return "", "805 All tuners in use"
		}
		busy := 0
		for _, other := range s.tuners {
			if other.held {
				busy++
			}
		}
		if !t.held && busy >= len(s.tuners) {
			return "", "805 All tuners in use"
		}
		t.guide, t.freq, t.held = ch.Number, ch.Freq, true
		return fmt.Sprintf("auto:%d", ch.Freq), ""
	case "status":
		if t.freq == 0 {
			return "ch=none lock=none ss=0 snq=0 seq=0", ""
		}
		mod := s.lockMod(*t)
		return fmt.Sprintf("ch=%s:%d lock=%s ss=90 snq=88 seq=100", mod, t.freq, mod), ""
	case "streaminfo":
		var b strings.Builder
		for _, ch := range s.Channels {
			if ch.Freq == t.freq {
				fmt.Fprintf(&b, "1: %s %s\n", ch.Number, ch.Name)
			}
		}
		return b.String(), ""
	default:
		return "", "unknown"
	}
}

func (s *Server) lockMod(t tuner) string {
	mod := s.spec.Lock
	if mod == "" {
		mod = "8vsb"
	}
	if t.guide != "" {
		if ch, ok := s.find(t.guide); ok && ch.ATSC3 {
			return "atsc3"
		}
	}
	return mod
}

func streamRequest(path string) (tuner int, target string) {
	tuner = tunerFromPath(path)
	rest := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		rest = path[i+1:]
	}
	switch {
	case strings.HasPrefix(rest, "v"):
		target = strings.TrimPrefix(rest, "v")
	case strings.HasPrefix(rest, "ch"):
		target = strings.TrimPrefix(rest, "ch")
		target, _, _ = strings.Cut(target, "-")
	default:
		target = rest
	}
	return tuner, target
}

func tunerFromPath(path string) int {
	rest, ok := strings.CutPrefix(path, "/tuner")
	if !ok {
		return -1
	}
	n, err := strconv.Atoi(strings.SplitN(rest, "/", 2)[0])
	if err != nil {
		return -1
	}
	return n
}

func (s *Server) find(value string) (Channel, bool) {
	value = strings.TrimPrefix(value, "auto:")
	for _, ch := range s.Channels {
		if ch.Number == value || strconv.Itoa(ch.Freq) == value {
			return ch, true
		}
	}
	return Channel{}, false
}

func writeJSON(w http.ResponseWriter, doc any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(doc)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("X-HDHomeRun-Error", msg)
	http.Error(w, msg, code)
}

func tlv(tag byte, value string) []byte {
	raw := append([]byte(value), 0)
	out := []byte{tag, byte(len(raw))}
	return append(out, raw...)
}

func tlvRaw(tag byte, val []byte) []byte {
	out := []byte{tag, byte(len(val))}
	return append(out, val...)
}

func seal(kind uint16, payload []byte) []byte {
	body := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(body[0:2], kind)
	binary.BigEndian.PutUint16(body[2:4], uint16(len(payload)))
	copy(body[4:], payload)
	out := make([]byte, len(body)+4)
	copy(out, body)
	binary.LittleEndian.PutUint32(out[len(body):], crc32.ChecksumIEEE(body))
	return out
}

func readLen(b []byte) (int, []byte) {
	if len(b) == 0 {
		return -1, nil
	}
	if b[0]&0x80 == 0 {
		return int(b[0]), b[1:]
	}
	if len(b) < 2 {
		return -1, nil
	}
	return int(b[0]&0x7f) | int(b[1])<<7, b[2:]
}
