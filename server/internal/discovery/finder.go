package discovery

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// FinderPort is the UDP port the apps probe when Bonjour finds nothing.
// A probe is the five bytes BWDP?. The reply is BWDP! plus JSON
// {"id","name","url"}. TCP 8478 stays the HDHomeRun emulator.
const FinderPort = 8479

const finderAsk = "BWDP?"
const finderYes = "BWDP!"

// FinderReply is the JSON body after BWDP!. Pub and Sig are set when the
// probe included a 16-byte nonce. Sig is Ed25519 over nonce, URL, and id.
type FinderReply struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Pub  string `json:"pub,omitempty"`
	Sig  string `json:"sig,omitempty"`
}

// LoadKey reads the Ed25519 private key at path, or creates one. The file is
// the signing key for address follow. It is not a password and not logged.
func LoadKey(path string) (ed25519.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(b), nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, priv, 0o600); err != nil {
		return nil, err
	}
	return priv, nil
}

// PublicKeyString is the base64 public key the apps store from GET /server.
func PublicKeyString(priv ed25519.PrivateKey) string {
	if len(priv) != ed25519.PrivateKeySize {
		return ""
	}
	return base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
}

// Finder answers probes until Close.
type Finder struct {
	conn   *net.UDPConn
	cancel context.CancelFunc
}

// ListenFinder binds FinderPort on every IPv4 interface. A second server on
// the same computer logs the error and keeps serving; Bonjour still works.
func ListenFinder(httpPort int, id, name string, key ed25519.PrivateKey) (*Finder, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: FinderPort})
	if err != nil {
		return nil, fmt.Errorf("udp %d: %w", FinderPort, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go serveFinder(ctx, conn, httpPort, id, name, key)
	slog.Info(fmt.Sprintf("discovery: udp %d", FinderPort))
	return &Finder{conn: conn, cancel: cancel}, nil
}

func (f *Finder) Close() error {
	if f == nil {
		return nil
	}
	f.cancel()
	return f.conn.Close()
}

func serveFinder(ctx context.Context, conn *net.UDPConn, httpPort int, id, name string, key ed25519.PrivateKey) {
	buf := make([]byte, 64)
	var mu sync.Mutex
	last := map[string]time.Time{}
	for {
		if ctx.Err() != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil || finderShouldStop(err) {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			// A port-unreachable from an ICMP reply is a normal UDP error.
			// Stopping here would take discovery down for the whole process.
			slog.Debug(fmt.Sprintf("discovery: %v", err))
			continue
		}
		if addr == nil {
			continue
		}
		source := addr.IP.String()
		mu.Lock()
		if !admit(last, source, time.Now()) {
			mu.Unlock()
			continue
		}
		reply := answerProbe(buf[:n], addr.IP, httpPort, id, name, key)
		if reply != nil {
			last[source] = time.Now()
		}
		mu.Unlock()
		if reply == nil {
			continue
		}
		_, _ = conn.WriteToUDP(reply, addr)
	}
}

// answerProbe builds a reply or returns nil. The packet body is never copied
// into the reply. Only an exact BWDP? is answered.
func answerProbe(pkt []byte, remote net.IP, httpPort int, id, name string, key ed25519.PrivateKey) []byte {
	var nonce []byte
	switch {
	case string(pkt) == finderAsk:
	case len(pkt) == len(finderAsk)+16 && string(pkt[:len(finderAsk)]) == finderAsk:
		nonce = append([]byte(nil), pkt[len(finderAsk):]...)
	default:
		return nil
	}
	if id == "" || name == "" || httpPort < 1 || httpPort > 65535 || len(id) > 64 || len(name) > 63 {
		return nil
	}
	ip := ipFacing(remote)
	if ip == nil {
		return nil
	}
	reply := FinderReply{
		ID:   id,
		Name: name,
		URL:  fmt.Sprintf("http://%s:%d", ip.String(), httpPort),
	}
	if len(nonce) == 16 && len(key) == ed25519.PrivateKeySize {
		reply.Pub = PublicKeyString(key)
		reply.Sig = base64.StdEncoding.EncodeToString(ed25519.Sign(key, signMessage(nonce, reply.URL, reply.ID)))
	}
	body, err := json.Marshal(reply)
	if err != nil || len(body) > 480 {
		return nil
	}
	out := make([]byte, 0, len(finderYes)+len(body))
	out = append(out, finderYes...)
	return append(out, body...)
}

// finderShouldStop is true when the socket is gone. Other read errors,
// including ICMP port unreachable, keep the loop alive.
func finderShouldStop(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

// admit is the per-source rate limit. A new address is refused once 256
// sources are remembered, so a scan cannot wipe the map and start over.
func admit(last map[string]time.Time, source string, now time.Time) bool {
	if _, seen := last[source]; !seen && len(last) >= 256 {
		return false
	}
	if t, ok := last[source]; ok && now.Sub(t) < 50*time.Millisecond {
		return false
	}
	return true
}

// signMessage binds the caller's nonce to the URL and id we chose.
func signMessage(nonce []byte, url, id string) []byte {
	msg := make([]byte, 0, len(nonce)+len(url)+len(id)+2)
	msg = append(msg, nonce...)
	msg = append(msg, 0)
	msg = append(msg, url...)
	msg = append(msg, 0)
	msg = append(msg, id...)
	return msg
}

// AnswersOn reports whether this interface may answer a probe. Tunnels and
// public addresses stay quiet. Loopback and private LAN interfaces answer.
func AnswersOn(name string, ip net.IP) bool {
	if ip = ip.To4(); ip == nil || tunnel(name) {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func tunnel(name string) bool {
	n := strings.ToLower(name)
	for _, prefix := range []string{"utun", "tun", "tap", "wg", "ipsec", "ppp", "gif", "stf", "awdl", "llw"} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// ipFacing is this computer's address on the sender's subnet, so the URL we
// hand back is one the sender can open. A sender who is not on a private
// interface we own gets nothing.
func ipFacing(remote net.IP) net.IP {
	remote = remote.To4()
	if remote == nil {
		return nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			if !AnswersOn(iface.Name, ip4) {
				continue
			}
			if remote.IsLoopback() && ip4.IsLoopback() {
				return net.IPv4(127, 0, 0, 1)
			}
			if ip4.IsLoopback() {
				continue
			}
			if ipnet.Contains(remote) {
				return ip4
			}
		}
	}
	if remote.IsLoopback() {
		return net.IPv4(127, 0, 0, 1)
	}
	return nil
}
