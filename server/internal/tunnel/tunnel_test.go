package tunnel

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// Lines as cloudflared 2025.x prints them.
const (
	configLine     = `2026-10-13T19:00:01Z INF Updated to new configuration config="{\"ingress\":[{\"hostname\":\"Broadwave.example.net\", \"originRequest\":{}, \"service\":\"http://localhost:8477\"}, {\"service\":\"http_status:404\"}], \"warp-routing\":{\"enabled\":false}}" version=1`
	registeredLine = `2026-10-13T19:00:02Z INF Registered tunnel connection connIndex=0 connection=8c0f event=0 ip=198.41.200.13 location=dfw08 protocol=quic`
	unauthLine     = `2026-10-13T19:00:01Z ERR Couldn't start tunnel error="Unauthorized: Invalid tunnel secret"`
)

func TestNoteReadsHostnamesConnectionsAndErrors(t *testing.T) {
	r := &Runner{status: Status{State: "starting"}}
	r.note(unauthLine)
	if st := r.Status(); st.State != "error" || !strings.Contains(st.Message, "did not accept the token") {
		t.Fatalf("unauthorized %+v", st)
	}
	r.note(`2026-10-13T19:32:33Z ERR Serve tunnel error error="control stream encountered a failure while serving" connIndex=0`)
	r.note(`2026-10-13T19:32:33Z ERR failed to run the datagram handler error="context canceled" connIndex=0`)
	if st := r.Status(); !strings.Contains(st.Message, "did not accept the token") {
		t.Fatalf("control stream %+v", st)
	}
	r.note(configLine)
	r.note(registeredLine)
	r.note(strings.Replace(registeredLine, "connIndex=0", "connIndex=1", 1))
	st := r.Status()
	if st.State != "connected" || st.Connections != 2 || st.Message != "" {
		t.Fatalf("connected %+v", st)
	}
	if len(st.Hostnames) != 1 || st.Hostnames[0] != "broadwave.example.net" {
		t.Fatalf("hostnames %q", st.Hostnames)
	}
	r.note(`2026-10-13T19:05:00Z INF Unregistered tunnel connection connIndex=0`)
	r.note(`2026-10-13T19:05:00Z INF Unregistered tunnel connection connIndex=1`)
	if st := r.Status(); st.State != "starting" || st.Connections != 0 {
		t.Fatalf("dropped %+v", st)
	}
}

func TestApplyRunsOnceAndStops(t *testing.T) {
	started := make(chan string, 4)
	r := &Runner{Bin: "/bin/cloudflared"}
	r.start = func(ctx context.Context, bin, token string) (io.ReadCloser, func() error, error) {
		started <- token
		pr, pw := io.Pipe()
		go func() {
			_, _ = io.WriteString(pw, registeredLine+"\n")
			<-ctx.Done()
			_ = pw.Close()
		}()
		return pr, func() error { return ctx.Err() }, nil
	}
	r.Apply("tok-a")
	if got := <-started; got != "tok-a" {
		t.Fatal(got)
	}
	r.Apply("tok-a")
	waitFor(t, func() bool { return r.Status().State == "connected" })
	r.Apply("tok-b")
	if got := <-started; got != "tok-b" {
		t.Fatal(got)
	}
	r.Apply("")
	if st := r.Status(); st.State != "off" {
		t.Fatalf("off %+v", st)
	}
	select {
	case extra := <-started:
		t.Fatalf("started again with %q", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMissingCloudflaredSaysSo(t *testing.T) {
	r := &Runner{}
	r.Apply("tok")
	if st := r.Status(); st.State != "missing" {
		t.Fatalf("%+v", st)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
