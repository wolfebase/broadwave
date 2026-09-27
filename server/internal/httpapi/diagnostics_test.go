package httpapi

import (
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestATuneMeansTheTunerIsAnswering(t *testing.T) {
	old := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	devices := []store.Device{{Device: hdhr.Device{DeviceID: "DUO", TunerCount: 2}, LastSeen: old}}
	if !tunerWentQuiet(devices, false, time.Now()) {
		t.Fatal("an idle tuner last seen 10 minutes ago should be quiet")
	}
	if tunerWentQuiet(devices, true, time.Now()) {
		t.Fatal("a tune in progress is not a tuner that stopped answering")
	}
	fresh := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if tunerWentQuiet([]store.Device{{Device: hdhr.Device{TunerCount: 2}, LastSeen: fresh}}, false, time.Now()) {
		t.Fatal("a recent reply is not quiet")
	}
}

func TestLocalZoneReadsTheHostClock(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("no zone database:", err)
	}
	if got := localZone("", chicago); got != "CDT" && got != "CST" {
		t.Fatalf("a host set to Chicago without TZ read %q", got)
	}
	if got := localZone("", time.UTC); got != "" {
		t.Fatalf("bare UTC must read as unset, got %q", got)
	}
	if got := localZone(" Europe/London ", time.UTC); got != "Europe/London" {
		t.Fatalf("TZ wins, got %q", got)
	}
}
