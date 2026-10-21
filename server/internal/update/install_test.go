package update

import (
	"context"
	"strings"
	"testing"
)

func TestDetectInstall(t *testing.T) {
	env := func(vals map[string]string) func(string) string { return func(k string) string { return vals[k] } }
	none := func(string) bool { return false }
	docker := func(p string) bool { return p == "/.dockerenv" }
	cases := []struct {
		getenv func(string) string
		exists func(string) bool
		want   string
	}{
		{env(map[string]string{"HOST_OS": "Unraid"}), docker, InstallUnraid},
		{env(nil), docker, InstallDocker},
		{env(map[string]string{"container": "podman"}), none, InstallDocker},
		{env(nil), none, InstallBinary},
	}
	for _, c := range cases {
		if got := DetectInstall(c.getenv, c.exists); got != c.want {
			t.Fatalf("got %s, want %s", got, c.want)
		}
		if HowTo(c.want) == "" {
			t.Fatalf("no how-to for %s", c.want)
		}
	}
	if !strings.Contains(HowTo(InstallUnraid), "Check for Updates") || HowTo("other") != "" {
		t.Fatal("how-to text")
	}
}

func TestVisibleSaysHowToUpdate(t *testing.T) {
	c := &Checker{Current: "0.15.0", Install: InstallDocker, last: &Notice{Version: "0.16.0", NotesURL: "https://github.com/wolfebase/broadwave/releases/tag/v0.16.0", Message: "Broadwave 0.16.0 is available."}}
	n := c.Visible(context.Background())
	if n == nil || n.Install != InstallDocker || !strings.Contains(n.HowTo, "docker compose pull") {
		t.Fatalf("notice %+v", n)
	}
}
