package live

import (
	"os"
	"path/filepath"
	"testing"

	"broadwave/internal/store"
)

func TestWriteEDLSkipsADeletedRecording(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "show.ts")
	markers := []store.Marker{{Start: 10, End: 70}}
	if err := WriteEDL(path, markers); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "show.edl")); !os.IsNotExist(err) {
		t.Fatalf("edl written for a missing recording: %v", err)
	}
	if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteEDL(path, markers); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "show.edl"))
	if err != nil || string(body) != "10.000 70.000 0\n" {
		t.Fatalf("edl %q %v", body, err)
	}
}
