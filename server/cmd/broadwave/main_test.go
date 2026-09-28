package main

import (
	"os"
	"testing"
)

func TestLocalBaseReachesTheListenAddress(t *testing.T) {
	cases := map[string]string{
		":8477":             "http://127.0.0.1:8477",
		":8490":             "http://127.0.0.1:8490",
		"0.0.0.0:8490":      "http://127.0.0.1:8490",
		"[::]:8490":         "http://127.0.0.1:8490",
		"127.0.0.1:18731":   "http://127.0.0.1:18731",
		"192.168.10.5:8477": "http://192.168.10.5:8477",
		"[fd00::5]:8477":    "http://[fd00::5]:8477",
		"nonsense":          "http://127.0.0.1:8477",
	}
	for addr, want := range cases {
		if got := localBase(addr); got != want {
			t.Errorf("%q: %s, want %s", addr, got, want)
		}
	}
}

func TestChecksFindAServerStartedOnAnotherPort(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	if got := checkAddr(":8477", false); got != ":8477" {
		t.Fatalf("nothing written down: %s", got)
	}
	if err := os.WriteFile(addrFile(), []byte(":8490\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := checkAddr(":8477", false); got != ":8490" {
		t.Fatalf("written :8490, checked %s", got)
	}
	if got := checkAddr(":9000", true); got != ":9000" {
		t.Fatalf("an -addr on the command line wins: %s", got)
	}
}
