package hdhr

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestControlGetReturnsTheServerValue(t *testing.T) {
	addr, hits := startControl(t, func(name, value string) []byte {
		return seal(typeGetSetRpy, tlvString(tagGetSetValue, "20260326"))
	})
	got, err := (Control{Addr: addr}).Get("/sys/version")
	if err != nil || got != "20260326" {
		t.Fatalf("%q %v", got, err)
	}
	gotHits := hits.all()
	want := tlvString(tagGetSetName, "/sys/version")
	if len(gotHits) != 1 || !sealedRequest(gotHits[0].raw) || !bytes.Equal(controlPayload(gotHits[0].raw), want) {
		t.Fatal("name tlv mismatch")
	}
}

func TestControlSetSendsTheValueAndReturnsTheReply(t *testing.T) {
	const sent = "auto:533000000"
	addr, hits := startControl(t, func(name, value string) []byte {
		if value != sent {
			return seal(typeGetSetRpy, tlvString(tagErrorMessage, "missing value"))
		}
		return seal(typeGetSetRpy, tlvString(tagGetSetValue, "reply-value"))
	})
	got, err := (Control{Addr: addr}).Set("/tuner0/channel", sent)
	if err != nil || got != "reply-value" {
		t.Fatalf("%q %v", got, err)
	}
	want := append(tlvString(tagGetSetName, "/tuner0/channel"), tlvString(tagGetSetValue, sent)...)
	gotHits := hits.all()
	if len(gotHits) != 1 || !sealedRequest(gotHits[0].raw) || !bytes.Equal(controlPayload(gotHits[0].raw), want) {
		t.Fatal("set tlv mismatch")
	}
}

func TestControlErrorTagReturnsTheText(t *testing.T) {
	addr, _ := startControl(t, func(name, value string) []byte {
		payload := append(tlvString(tagGetSetValue, "ignored"), tlvString(tagErrorMessage, "not allowed")...)
		return seal(typeGetSetRpy, payload)
	})
	got, err := (Control{Addr: addr}).Get("/sys/version")
	if err == nil || err.Error() != "not allowed" || got != "" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestControlRejectsBadReplies(t *testing.T) {
	addr, _ := startControl(t, func(name, value string) []byte {
		switch name {
		case "/wrong-type":
			return seal(0x0006, tlvString(tagGetSetValue, "x"))
		case "/bad-crc":
			pkt := seal(typeGetSetRpy, tlvString(tagGetSetValue, "x"))
			pkt[len(pkt)-1] ^= 0xFF
			return pkt
		case "/truncated":
			return seal(typeGetSetRpy, []byte{tagGetSetValue, 10, 'x'})
		case "/bad-length":
			return seal(typeGetSetRpy, []byte{tagGetSetValue, 0x80})
		case "/short":
			return []byte{0x00, 0x05}
		case "/short-body":
			return []byte{0x00, 0x05, 0x00, 0x01}
		default:
			return nil
		}
	})
	c := Control{Addr: addr}
	if _, err := c.Get("/wrong-type"); err == nil || !strings.Contains(err.Error(), "control reply type") {
		t.Fatal(err)
	}
	if _, err := c.Get("/bad-crc"); err == nil || !strings.Contains(err.Error(), "control crc mismatch") {
		t.Fatal(err)
	}
	if _, err := c.Get("/truncated"); err == nil || !strings.Contains(err.Error(), "control tlv overrun") {
		t.Fatal(err)
	}
	if _, err := c.Get("/bad-length"); err == nil || !strings.Contains(err.Error(), "truncated tlv length") {
		t.Fatal(err)
	}
	if _, err := c.Get("/short"); err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Fatal(err)
	}
	if _, err := c.Get("/short-body"); err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Fatal(err)
	}
}

func TestControlDialKeepsHostPortAndAddsTheDefault(t *testing.T) {
	if got := controlDial("127.0.0.1:9"); got != "127.0.0.1:9" {
		t.Fatalf("kept %s", got)
	}
	if got := controlDial("203.0.113.10"); got != "203.0.113.10:65001" {
		t.Fatalf("added %s", got)
	}
}

func TestControlDialClosedPortErrors(t *testing.T) {
	start := time.Now()
	_, err := (Control{Addr: "127.0.0.1:1"}).Get("/sys/version")
	if err == nil {
		t.Fatal("expected dial error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("dial hung")
	}
}

func TestControlExchangeSendsLockKey(t *testing.T) {
	const key uint32 = 0xA1B2C3D4
	addr, hits := startControl(t, func(name, value string) []byte {
		return seal(typeGetSetRpy, tlvString(tagGetSetValue, "none"))
	})
	got, err := (Control{Addr: addr}).exchange("/tuner0/channel", "none", key, true)
	if err != nil || got != "none" {
		t.Fatalf("%q %v", got, err)
	}
	want := tlvString(tagGetSetName, "/tuner0/channel")
	want = append(want, tlvString(tagGetSetValue, "none")...)
	want = append(want, tlvU32(tagLockKey, key)...)
	gotHits := hits.all()
	if len(gotHits) != 1 || !sealedRequest(gotHits[0].raw) || !bytes.Equal(controlPayload(gotHits[0].raw), want) {
		t.Fatal("lock key tlv mismatch")
	}
}

func TestVarLenRoundTripsPast127(t *testing.T) {
	long := strings.Repeat("a", 180)
	enc := varLen(len(long) + 1)
	if len(enc) != 2 || enc[0]&0x80 == 0 {
		t.Fatalf("encoding %x", enc)
	}
	n, rest, err := readVarLen(append(enc, 'Z'))
	if err != nil || n != len(long)+1 || string(rest) != "Z" {
		t.Fatalf("n %d rest %q err %v", n, rest, err)
	}
	if _, _, err := readVarLen(nil); err == nil {
		t.Fatal("missing length")
	}
	if _, _, err := readVarLen([]byte{0x80}); err == nil {
		t.Fatal("truncated length")
	}

	addr, hits := startControl(t, func(name, value string) []byte {
		return seal(typeGetSetRpy, tlvString(tagGetSetValue, value))
	})
	got, err := (Control{Addr: addr}).Set("/sys/debug", long)
	if err != nil || got != long {
		t.Fatalf("len %d err %v", len(got), err)
	}
	want := append(tlvString(tagGetSetName, "/sys/debug"), tlvString(tagGetSetValue, long)...)
	gotHits := hits.all()
	if len(gotHits) != 1 || !bytes.Equal(controlPayload(gotHits[0].raw), want) {
		t.Fatal("long value tlv mismatch")
	}
}

func TestReadHealthPrefersControlFirmwareAndTunerLocks(t *testing.T) {
	addr, hits := startControl(t, func(name, value string) []byte {
		switch name {
		case "/sys/version":
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "  20260326\n"))
		case "/tuner0/status":
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "ch=none lock=none ss=0 snq=0 seq=0"))
		case "/tuner1/status":
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "ch=8vsb:533000000 lock=8vsb ss=80 snq=70 seq=100"))
		default:
			return seal(typeGetSetRpy, tlvString(tagErrorMessage, "unknown"))
		}
	})
	setControlPort(t, addr)
	srv := serveDiscover(t, func(base string) string {
		return discoverJSON(base, "HDHR5-2US", "20190101", 2)
	})
	health, err := (&Client{HTTP: srv.Client()}).ReadHealth(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if health.DeviceID != "ABCDEF01" || health.Model != "HDHR5-2US" || health.FirmwareVersion != "20260326" {
		t.Fatalf("%+v", health)
	}
	if len(health.Tuners) != 2 || health.Tuners[0].Index != 0 || health.Tuners[0].Locked || health.Tuners[1].Index != 1 || !health.Tuners[1].Locked {
		t.Fatalf("%+v", health.Tuners)
	}
	if got := strings.Join(hitNames(hits.all()), ","); got != "/sys/version,/tuner0/status,/tuner1/status" {
		t.Fatalf("paths %s", got)
	}
}

func TestReadHealthHonorsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	health, err := (&Client{}).ReadHealth(ctx, "http://127.0.0.1:1/discover.json")
	if err != ctx.Err() {
		t.Fatal(err)
	}
	if health.DeviceID != "" || len(health.Tuners) != 0 {
		t.Fatalf("%+v", health)
	}
}

func TestReadHealthStopsWhenCancelledAfterVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, hits := startControl(t, func(name, value string) []byte {
		if name == "/sys/version" {
			cancel()
		}
		return seal(typeGetSetRpy, tlvString(tagGetSetValue, "20260326"))
	})
	setControlPort(t, addr)
	srv := serveDiscover(t, func(base string) string {
		return discoverJSON(base, "HDHR5-2US", "20190101", 2)
	})
	_, err := (&Client{HTTP: srv.Client()}).ReadHealth(ctx, srv.URL)
	if err != context.Canceled {
		t.Fatal(err)
	}
	if got := strings.Join(hitNames(hits.all()), ","); got != "/sys/version" {
		t.Fatalf("paths %s", got)
	}
}

func TestReadHealthReturnsFetchErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := srv.URL
	srv.Close()
	_, err := (&Client{HTTP: srv.Client()}).ReadHealth(context.Background(), base)
	if err == nil {
		t.Fatal("expected fetch error")
	}
}

func TestReadHealthCapsTunerCountAtEight(t *testing.T) {
	addr, hits := startControl(t, func(name, value string) []byte {
		if name == "/sys/version" {
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "20260326"))
		}
		return seal(typeGetSetRpy, tlvString(tagGetSetValue, "lock=8vsb"))
	})
	setControlPort(t, addr)
	srv := serveDiscover(t, func(base string) string {
		return discoverJSON(base, "HDHR5-2US", "20190101", 12)
	})
	health, err := (&Client{HTTP: srv.Client()}).ReadHealth(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(health.Tuners) != 8 {
		t.Fatalf("%d tuners", len(health.Tuners))
	}
	for i, tuner := range health.Tuners {
		if tuner.Index != i || !tuner.Locked {
			t.Fatalf("%+v", health.Tuners)
		}
	}
	want := []string{"/sys/version"}
	for i := 0; i < 8; i++ {
		want = append(want, fmt.Sprintf("/tuner%d/status", i))
	}
	if got := hitNames(hits.all()); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("paths %v", got)
	}
}

func TestReadHealthTreatsNegativeTunerCountAsNone(t *testing.T) {
	addr, hits := startControl(t, func(name, value string) []byte {
		if strings.HasPrefix(name, "/tuner") {
			return seal(typeGetSetRpy, tlvString(tagErrorMessage, "tuner queried"))
		}
		return seal(typeGetSetRpy, tlvString(tagGetSetValue, "20260326"))
	})
	setControlPort(t, addr)
	srv := serveDiscover(t, func(base string) string {
		return discoverJSON(base, "HDHR5-2US", "20190101", -4)
	})
	health, err := (&Client{HTTP: srv.Client()}).ReadHealth(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(health.Tuners) != 0 {
		t.Fatalf("%+v", health.Tuners)
	}
	if got := strings.Join(hitNames(hits.all()), ","); got != "/sys/version" {
		t.Fatalf("paths %s", got)
	}
}

func TestReadHealthFailsWhenTunerStatusFails(t *testing.T) {
	addr, _ := startControl(t, func(name, value string) []byte {
		switch name {
		case "/sys/version":
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "20260326"))
		case "/tuner0/status":
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "lock=none"))
		default:
			return seal(typeGetSetRpy, tlvString(tagErrorMessage, "status failed"))
		}
	})
	setControlPort(t, addr)
	srv := serveDiscover(t, func(base string) string {
		return discoverJSON(base, "HDHR5-2US", "20190101", 2)
	})
	health, err := (&Client{HTTP: srv.Client()}).ReadHealth(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "status failed") || len(health.Tuners) != 0 {
		t.Fatalf("%v %+v", err, health)
	}
}

func TestReadHealthFillsAnEmptyModelFromControl(t *testing.T) {
	addr, hits := startControl(t, func(name, value string) []byte {
		switch name {
		case "/sys/version":
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "20260326"))
		case "/sys/model":
			return seal(typeGetSetRpy, tlvString(tagGetSetValue, "\tHDHR5-2US\r\n"))
		default:
			return seal(typeGetSetRpy, tlvString(tagErrorMessage, "unknown"))
		}
	})
	setControlPort(t, addr)
	srv := serveDiscover(t, func(base string) string {
		return discoverJSON(base, "", "20190101", 0)
	})
	health, err := (&Client{HTTP: srv.Client()}).ReadHealth(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if health.Model != "HDHR5-2US" || len(health.Tuners) != 0 {
		t.Fatalf("%+v", health)
	}
	if got := strings.Join(hitNames(hits.all()), ","); got != "/sys/version,/sys/model" {
		t.Fatalf("paths %s", got)
	}
}

func TestRefuseInstallBlocksFirmwarePaths(t *testing.T) {
	blocked := []string{"/sys/upgrade", "/UPGRADE", "firmware", "MyFirmware.bin", "sys/upgrade", "SYS/UPGRADE"}
	for _, path := range blocked {
		if err := refuseInstall(path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	if err := refuseInstall("/sys/version"); err != nil {
		t.Fatal(err)
	}
	if err := refuseInstall("/tuner0/status"); err != nil {
		t.Fatal(err)
	}
}

func TestControlAddrUsesHostAndOptionalPort(t *testing.T) {
	t.Setenv("HDHR_CONTROL_PORT", "")
	if got := controlAddr("http://203.0.113.10:5004/discover.json"); got != "203.0.113.10" {
		t.Fatalf("host %s", got)
	}
	if got := controlAddr("203.0.113.10"); got != "203.0.113.10" {
		t.Fatalf("raw %s", got)
	}
	t.Setenv("HDHR_CONTROL_PORT", "65001")
	if got := controlAddr("http://127.0.0.1:5004/"); got != "127.0.0.1:65001" {
		t.Fatalf("port %s", got)
	}
	if got := controlAddr("203.0.113.10"); got != "203.0.113.10:65001" {
		t.Fatalf("raw port %s", got)
	}
}

func TestDeviceAuthReturnsTheDiscoverToken(t *testing.T) {
	const token = "test-auth-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discover.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"DeviceID":"ABCDEF01","DeviceAuth":%q}`, token)
	}))
	defer srv.Close()
	got, err := (&Client{HTTP: srv.Client()}).DeviceAuth(context.Background(), " "+srv.URL+"/")
	if err != nil || got != token {
		t.Fatal("device auth")
	}
}

func TestDeviceAuthReturnsTransportErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := srv.URL
	srv.Close()
	got, err := (&Client{HTTP: srv.Client()}).DeviceAuth(context.Background(), base)
	if err == nil || got != "" {
		t.Fatal("expected transport error")
	}
}

func TestBroadcastTargetsAreIPv4(t *testing.T) {
	targets := broadcastTargets()
	saw := false
	for _, target := range targets {
		ip := net.ParseIP(target)
		if ip == nil || ip.To4() == nil {
			t.Fatal("broadcast target is not ipv4")
		}
		if target == "255.255.255.255" {
			saw = true
		}
	}
	if !saw {
		t.Fatal("missing 255.255.255.255")
	}
}

func TestDiscoverHostLoopbackTimesOut(t *testing.T) {
	reply, err := DiscoverHost("127.0.0.1", 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected loopback discovery to fail")
	}
	if reply.DeviceID != "" || reply.BaseURL != "" {
		t.Fatal("loopback discovery returned a device")
	}
}

type controlHit struct {
	name  string
	value string
	raw   []byte
}

type controlLog struct {
	mu   sync.Mutex
	hits []controlHit
}

func (l *controlLog) add(name, value string, raw []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = append(l.hits, controlHit{name, value, append([]byte(nil), raw...)})
}

func (l *controlLog) all() []controlHit {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]controlHit, len(l.hits))
	copy(out, l.hits)
	return out
}

func startControl(t *testing.T, reply func(name, value string) []byte) (string, *controlLog) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	log := &controlLog{}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				pkt, err := readPacket(conn)
				if err != nil {
					return
				}
				name, value := parseControlRequest(pkt)
				log.add(name, value, pkt)
				out := reply(name, value)
				if len(out) > 0 {
					_, _ = conn.Write(out)
				}
			}(conn)
		}
	}()
	return ln.Addr().String(), log
}

func parseControlRequest(packet []byte) (name, value string) {
	if len(packet) < 8 {
		return "", ""
	}
	n := int(binary.BigEndian.Uint16(packet[2:4]))
	if len(packet) < 4+n {
		return "", ""
	}
	rest := packet[4 : 4+n]
	for len(rest) > 0 {
		tag := rest[0]
		rest = rest[1:]
		ln, next, err := readVarLen(rest)
		if err != nil || len(next) < ln {
			return name, value
		}
		text := strings.TrimRight(string(next[:ln]), "\x00")
		rest = next[ln:]
		switch tag {
		case tagGetSetName:
			name = text
		case tagGetSetValue:
			value = text
		}
	}
	return name, value
}

func controlPayload(packet []byte) []byte {
	if len(packet) < 4 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(packet[2:4]))
	if len(packet) < 4+n {
		return nil
	}
	return packet[4 : 4+n]
}

func sealedRequest(packet []byte) bool {
	if len(packet) < 8 || binary.BigEndian.Uint16(packet[:2]) != typeGetSetReq {
		return false
	}
	n := int(binary.BigEndian.Uint16(packet[2:4]))
	if len(packet) != 4+n+4 {
		return false
	}
	sum := binary.LittleEndian.Uint32(packet[4+n:])
	return crc32.ChecksumIEEE(packet[:4+n]) == sum
}

func setControlPort(t *testing.T, addr string) {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HDHR_CONTROL_PORT", port)
}

func serveDiscover(t *testing.T, doc func(base string) string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discover.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(doc(srv.URL)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func discoverJSON(base, model, firmware string, tuners int) string {
	return fmt.Sprintf(`{"DeviceID":"ABCDEF01","ModelNumber":%q,"FirmwareVersion":%q,"TunerCount":%d,"BaseURL":%q}`, model, firmware, tuners, base)
}

func hitNames(hits []controlHit) []string {
	out := make([]string, len(hits))
	for i, hit := range hits {
		out[i] = hit.name
	}
	return out
}
