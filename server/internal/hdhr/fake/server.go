// Package fake is an HDHomeRun that stays on localhost: discover, lineup, control, and a looping MPEG-TS.
package fake

import (
	"encoding/binary"
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
	typeGetSetReq  = 0x0004
	typeGetSetRpy  = 0x0005
	tagGetSetName  = 0x03
	tagGetSetValue = 0x04
	tagError       = 0x05
)

// Channel is one lineup row on a frequency.
type Channel struct {
	Number string
	Name   string
	Freq   int
}

// Server is a two-tuner HDHomeRun. HTTP serves discover, lineup, and the stream. Control is TCP.
type Server struct {
	Channels []Channel
	TS       string
	// Realtime spreads one pass of TS across four seconds so a relay can build a live playlist.
	Realtime bool

	httpLn net.Listener
	ctrlLn net.Listener

	mu      sync.Mutex
	tuners  [2]tuner
	streams int
}

type tuner struct {
	guide string
	freq  int
	held  bool
}

// Start listens on 127.0.0.1. BaseURL is the HTTP origin. Control is host:port for the TCP protocol.
func (s *Server) Start() (baseURL, control string, err error) {
	if len(s.Channels) == 0 {
		s.Channels = []Channel{
			{Number: "4.1", Name: "WDAF", Freq: 593000000},
			{Number: "4.2", Name: "WDAF2", Freq: 593000000},
			{Number: "5.1", Name: "KCTV", Freq: 533000000},
		}
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

// Close releases both listeners.
func (s *Server) Close() {
	if s.httpLn != nil {
		_ = s.httpLn.Close()
	}
	if s.ctrlLn != nil {
		_ = s.ctrlLn.Close()
	}
}

func (s *Server) serveHTTP() {
	mux := http.NewServeMux()
	mux.HandleFunc("/discover.json", s.discover)
	mux.HandleFunc("/lineup.json", s.lineup)
	mux.HandleFunc("/lineup_status.json", s.lineupStatus)
	mux.HandleFunc("/status.json", s.status)
	mux.HandleFunc("/tuner0/", s.stream)
	mux.HandleFunc("/tuner1/", s.stream)
	mux.HandleFunc("/auto/", s.stream)
	_ = http.Serve(s.httpLn, mux)
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	fmt.Fprintf(w, `{"FriendlyName":"Fake HDHomeRun","ModelNumber":"HDHR4-2US","FirmwareName":"hdhomerun_fake","FirmwareVersion":"20260101","DeviceID":"FAKEHDHR","BaseURL":"%s","LineupURL":"%s/lineup.json","TunerCount":2}`, base, base)
}

func (s *Server) lineup(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, "[")
	for i, ch := range s.Channels {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, `{"GuideNumber":"%s","GuideName":"%s","URL":"%s/auto/v%s","VideoCodec":"MPEG2","AudioCodec":"AC3","HD":1}`, ch.Number, ch.Name, base, ch.Number)
	}
	fmt.Fprint(w, "]")
}

func (s *Server) lineupStatus(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, `{"Scan":0,"Found":3,"Progress":100,"ScanInProgress":0}`)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprint(w, "[")
	for i, t := range s.tuners {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		target := ""
		if t.held {
			target = "127.0.0.1"
		}
		fmt.Fprintf(w, `{"Resource":"tuner%d","VctNumber":"%s","VctName":"%s","TargetIP":"%s","SignalStrengthPercent":90,"SignalQualityPercent":88,"SymbolQualityPercent":100}`, i, t.guide, t.guide, target)
	}
	fmt.Fprint(w, "]")
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "/ch") || strings.Contains(r.URL.Path, "/auto/") {
		s.mu.Lock()
		if s.streams >= 2 {
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
				s.tuners[n] = tuner{}
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
					time.Sleep(wait)
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
		return "HDHR4-2US", ""
	}
	index, tail, ok := strings.Cut(strings.TrimPrefix(name, "/tuner"), "/")
	if !ok {
		return "", "unknown"
	}
	n, err := strconv.Atoi(index)
	if err != nil || n < 0 || n > 1 {
		return "", "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &s.tuners[n]
	switch tail {
	case "channel", "vchannel":
		if !set {
			if t.freq == 0 {
				return "none", ""
			}
			return fmt.Sprintf("auto:%d", t.freq), ""
		}
		if value == "none" {
			*t = tuner{}
			return "none", ""
		}
		ch, found := s.find(value)
		if !found {
			return "", "unknown channel"
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
		if !t.held && busy >= 2 {
			return "", "805 All tuners in use"
		}
		t.guide, t.freq, t.held = ch.Number, ch.Freq, true
		return fmt.Sprintf("auto:%d", ch.Freq), ""
	case "status":
		if t.freq == 0 {
			return "ch=none lock=none ss=0 snq=0 seq=0", ""
		}
		return fmt.Sprintf("ch=8vsb:%d lock=8vsb ss=90 snq=88 seq=100", t.freq), ""
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

func tlv(tag byte, value string) []byte {
	raw := append([]byte(value), 0)
	out := []byte{tag, byte(len(raw))}
	return append(out, raw...)
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
