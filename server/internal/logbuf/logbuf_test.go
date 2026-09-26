package logbuf

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactLeavesOrdinaryLines(t *testing.T) {
	line := "encoder: h264_vaapi deint: yadif http://127.0.0.1:8477/api/v1/health"
	if got := Redact(line); got != line {
		t.Fatalf("%q", got)
	}
}

func TestRedactDropsPasswordAndDeviceAuth(t *testing.T) {
	line := `guide http://ops3user:ops3-fixture-password@playlist.example/pl.m3u?password=ops3-fixture-password DeviceAuth=ops3-device-auth-token "DeviceAuth":"ops3-device-auth-token"`
	got := Redact(line)
	if strings.Contains(got, "ops3-fixture-password") || strings.Contains(got, "ops3-device-auth-token") || strings.Contains(got, "DeviceAuth") {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, "playlist.example") {
		t.Fatalf("%s", got)
	}
}

func TestWriterStoresARedactedLine(t *testing.T) {
	prev := shared
	shared = &ring{max: 8}
	t.Cleanup(func() { shared = prev })
	var dst bytes.Buffer
	raw := "http://ops3user:ops3-fixture-password@playlist.example/pl.m3u?password=ops3-fixture-password DeviceAuth=ops3-device-auth-token\n"
	if _, err := (writer{dst: &dst}).Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dst.String(), "ops3-fixture-password") || strings.Contains(Text(), "ops3-fixture-password") {
		t.Fatalf("dst %q ring %q", dst.String(), Text())
	}
	if strings.Contains(dst.String(), "DeviceAuth") || strings.Contains(dst.String(), "ops3-device-auth-token") {
		t.Fatalf("dst %q", dst.String())
	}
}

func TestRingDropsTheOldestLine(t *testing.T) {
	r := &ring{max: 2}
	r.add("a")
	r.add("b")
	r.add("c")
	if r.text() != "b\nc" {
		t.Fatalf("%q", r.text())
	}
}

func TestTailKeepsTheNewestLines(t *testing.T) {
	prev := shared
	shared = &ring{max: 8}
	t.Cleanup(func() { shared = prev })
	shared.add("a")
	shared.add("b")
	shared.add("c")
	if strings.Join(Tail(2), ",") != "b,c" {
		t.Fatalf("%v", Tail(2))
	}
	if len(Tail(0)) != 3 {
		t.Fatalf("%v", Tail(0))
	}
}

func TestSlogDropsPasswordAndDeviceAuth(t *testing.T) {
	prev := shared
	shared = &ring{max: 20}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	t.Cleanup(func() {
		shared = prev
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
	})
	var dst bytes.Buffer
	Install(&dst)
	const password = "ops3-fixture-password"
	const auth = "ops3-device-auth-token"
	slog.Info("source http://ops3user:" + password + "@playlist.example/pl.m3u?password=" + password + " DeviceAuth=" + auth)
	slog.Info("login", "password", password, "DeviceAuth", auth)
	slog.Info("password=" + password)
	got := dst.String() + "\n" + Text()
	if strings.Contains(got, password) || strings.Contains(got, auth) || strings.Contains(got, "DeviceAuth") {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(Text(), "playlist.example") || !strings.Contains(Text(), "level=INFO") {
		t.Fatalf("%s", Text())
	}
}
