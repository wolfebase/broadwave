package discovery

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestParseSSDPReadsAnHDHomeRun(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nSERVER: HDHomeRun/1.0\r\nLOCATION: http://192.168.1.252/device.xml\r\nUSN: uuid:10611B4C\r\n\r\n"
	got, ok := parseSSDP(raw, "192.168.1.252:1900")
	if !ok || got.Kind != "hdhomerun" || got.Addr != "192.168.1.252" || got.ID != "uuid:10611B4C" {
		t.Fatalf("%+v ok=%v", got, ok)
	}
}

func TestSearchSSDPReadsALoopbackReply(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() {
		buf := make([]byte, 4096)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, addr, err := conn.ReadFrom(buf)
		if err != nil || n == 0 || !strings.Contains(string(buf[:n]), "M-SEARCH") {
			return
		}
		body := "HTTP/1.1 200 OK\r\nSERVER: HDHomeRun/1.0\r\nLOCATION: http://127.0.0.1/device.xml\r\nUSN: uuid:test\r\n\r\n"
		_, _ = conn.WriteTo([]byte(body), addr)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	found, err := SearchSSDP(ctx, conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Kind != "hdhomerun" || found[0].ID != "uuid:test" {
		t.Fatalf("%+v", found)
	}
}

func TestLiveDUOAnswers(t *testing.T) {
	host := os.Getenv("WG_LIVE")
	if host == "" {
		t.Skip("set WG_LIVE to a tuner address to ask it")
	}
	reply, err := hdhr.DiscoverHost(host, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("unicast %+v", reply)
	if reply.DeviceID == "" {
		t.Fatal("unicast reply had no device id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	found, err := SearchSSDP(ctx, net.JoinHostPort(host, "1900"))
	t.Logf("ssdp %+v err %v", found, err)
	broadcast, err := hdhr.Discover(2 * time.Second)
	t.Logf("broadcast %+v err %v", broadcast, err)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	names, err := Browse(ctx2, "_hdhomerun._tcp")
	t.Logf("mdns %+v err %v", names, err)
	hosts, err := net.LookupHost("hdhomerun.local")
	t.Logf("hdhomerun.local %+v err %v", hosts, err)
}
