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
