package fake

import (
	"context"
	"strings"
	"testing"

	"broadwave/internal/hdhr"
)

func TestHealthReadReportsVersionAndLock(t *testing.T) {
	srv := &Server{}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", port)

	ctrl := hdhr.Control{Addr: "127.0.0.1:" + port}
	if _, err := ctrl.Set("/tuner0/vchannel", "4.1"); err != nil {
		t.Fatal(err)
	}

	before := len(srv.Requests())
	health, err := (&hdhr.Client{}).ReadHealth(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if health.Model != "HDHR4-2US" || health.FirmwareVersion != "20260101" || health.DeviceID != "FAKEHDHR" {
		t.Fatalf("%+v", health)
	}
	if len(health.Tuners) != 2 || !health.Tuners[0].Locked || health.Tuners[1].Locked {
		t.Fatalf("tuners %+v", health.Tuners)
	}

	paths := srv.Requests()[before:]
	for _, path := range paths {
		low := strings.ToLower(path)
		if strings.Contains(low, "upgrade") || strings.Contains(low, "firmware") || strings.Contains(low, "sys/upgrade") {
			t.Fatalf("health read requested %s", path)
		}
	}
	if strings.Join(paths, ",") != "/discover.json,/sys/version,/tuner0/status,/tuner1/status" {
		t.Fatalf("paths %v", paths)
	}
}
