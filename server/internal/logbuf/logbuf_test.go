package logbuf

import (
	"bytes"
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
