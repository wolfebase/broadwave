package discovery

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestFinderAnswersOnLoopback(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer conn.Close()
	go serveFinder(ctx, conn, 18477, "abc", `Room "A"`, nil)

	client, err := net.DialUDP("udp4", nil, conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("nope")); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 512)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("garbage got a reply")
	}

	if _, err := client.Write([]byte(finderAsk)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n < len(finderYes) || string(buf[:len(finderYes)]) != finderYes {
		t.Fatalf("reply %q", buf[:n])
	}
	var got FinderReply
	if err := json.Unmarshal(buf[len(finderYes):n], &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "abc" || got.Name != `Room "A"` || got.URL != "http://127.0.0.1:18477" {
		t.Fatalf("reply %+v", got)
	}
}

func TestAnswerProbeRejectsAnythingButTheAsk(t *testing.T) {
	remote := net.ParseIP("127.0.0.1")
	if answerProbe([]byte("BWDP!"), remote, 8477, "abc", "Home", nil) != nil {
		t.Fatal("reply echoed")
	}
	if answerProbe([]byte("BWDP?x"), remote, 8477, "abc", "Home", nil) != nil {
		t.Fatal("long packet answered")
	}
	if answerProbe([]byte(finderAsk), remote, 0, "abc", "Home", nil) != nil {
		t.Fatal("port 0 answered")
	}
	if answerProbe([]byte(finderAsk), remote, 8477, "", "Home", nil) != nil {
		t.Fatal("empty id answered")
	}
	pkt := answerProbe([]byte(finderAsk), remote, 8477, "abc", "Home", nil)
	if pkt == nil {
		t.Fatal("expected a reply")
	}
	var got FinderReply
	if err := json.Unmarshal(pkt[len(finderYes):], &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "http://127.0.0.1:8477" || got.ID != "abc" || got.Name != "Home" {
		t.Fatalf("reply %+v", got)
	}
	outside := net.ParseIP("203.0.113.9")
	if answerProbe([]byte(finderAsk), outside, 8477, "abc", "Home", nil) != nil {
		t.Fatal("an address outside every local subnet was answered")
	}
}

func TestSignedProbeBindsTheNonce(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{7}, 16)
	pkt := append([]byte(finderAsk), nonce...)
	raw := answerProbe(pkt, net.ParseIP("127.0.0.1"), 8477, "abc", "Home", priv)
	if raw == nil {
		t.Fatal("expected a signed reply")
	}
	var got FinderReply
	if err := json.Unmarshal(raw[len(finderYes):], &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "http://127.0.0.1:8477" || got.Pub != base64.StdEncoding.EncodeToString(pub) {
		t.Fatalf("reply %+v", got)
	}
	sig, err := base64.StdEncoding.DecodeString(got.Sig)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, signMessage(nonce, got.URL, got.ID), sig) {
		t.Fatal("signature did not verify")
	}
	if ed25519.Verify(pub, signMessage(nonce, "http://127.0.0.1:9", got.ID), sig) {
		t.Fatal("a different url verified")
	}
	if ed25519.Verify(pub, signMessage(bytes.Repeat([]byte{1}, 16), got.URL, got.ID), sig) {
		t.Fatal("a different nonce verified")
	}
	_, other, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if ed25519.Verify(other.Public().(ed25519.PublicKey), signMessage(nonce, got.URL, got.ID), sig) {
		t.Fatal("a different key verified")
	}
}

func TestAnswersOnSkipsTunnelsAndPublicAddresses(t *testing.T) {
	lan := net.ParseIP("192.168.1.20")
	if AnswersOn("utun4", lan) {
		t.Fatal("a tunnel answered")
	}
	if AnswersOn("wg0", lan) {
		t.Fatal("a wireguard interface answered")
	}
	if !AnswersOn("en0", lan) {
		t.Fatal("a private interface stayed quiet")
	}
	if AnswersOn("en0", net.ParseIP("203.0.113.9")) {
		t.Fatal("a public address answered")
	}
	if !AnswersOn("lo0", net.ParseIP("127.0.0.1")) {
		t.Fatal("loopback stayed quiet")
	}
}

func TestFinderReadErrorDoesNotStop(t *testing.T) {
	if !finderShouldStop(net.ErrClosed) {
		t.Fatal("a closed socket should stop")
	}
	refused := &net.OpError{Op: "read", Net: "udp", Err: syscall.ECONNREFUSED}
	if finderShouldStop(refused) {
		t.Fatal("port unreachable stopped discovery")
	}
}

func TestAdmitRefusesANewAddressAtTheCap(t *testing.T) {
	last := map[string]time.Time{}
	now := time.Now()
	for i := 0; i < 256; i++ {
		source := fmtIP(i)
		if !admit(last, source, now) {
			t.Fatalf("source %d refused early", i)
		}
		last[source] = now
	}
	if admit(last, "10.9.9.9", now) {
		t.Fatal("a new address was admitted at the cap")
	}
	if !admit(last, fmtIP(0), now.Add(time.Second)) {
		t.Fatal("a known address was refused")
	}
	if admit(last, fmtIP(0), now.Add(10*time.Millisecond)) {
		t.Fatal("a burst was admitted")
	}
}

func fmtIP(i int) string {
	return net.IPv4(10, byte(i>>8), byte(i), 1).String()
}

func TestLoadKeyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery.key")
	first, err := LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Equal(second) {
		t.Fatal("the key changed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
}
