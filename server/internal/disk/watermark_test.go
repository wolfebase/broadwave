package disk

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWatermarkBytesUsesDecimalGigabytes(t *testing.T) {
	if WatermarkBytes("") != 10_000_000_000 || WatermarkBytes("2") != 2_000_000_000 {
		t.Fatalf("empty %d two %d", WatermarkBytes(""), WatermarkBytes("2"))
	}
	if WatermarkBytes("0") != 0 {
		t.Fatal(WatermarkBytes("0"))
	}
	if WatermarkBytes("  4 ") != 4_000_000_000 {
		t.Fatal(WatermarkBytes("  4 "))
	}
	if WatermarkBytes("-3") != 10_000_000_000 || WatermarkBytes("nope") != 10_000_000_000 {
		t.Fatal("bad values must use the default")
	}
	if WatermarkBytes("1000001") != 1_000_000_000_000_000 {
		t.Fatal(WatermarkBytes("1000001"))
	}
}

func TestWritableRemovesItsProbe(t *testing.T) {
	dir := t.TempDir()
	if err := Writable(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("probe file left behind: %v", entries)
	}
}

func TestWritableRefusesAFileUsedAsAFolder(t *testing.T) {
	file := filepath.Join(t.TempDir(), "show.ts")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Writable(filepath.Join(file, "season"))
	var blocked *WriteError
	if !errors.As(err, &blocked) || blocked.Full {
		t.Fatal(err)
	}
}

func TestClassifyWriteLeavesUnknownErrorsAlone(t *testing.T) {
	if ClassifyWrite(nil) != nil {
		t.Fatal("nil")
	}
	other := errors.New("disk name")
	if ClassifyWrite(other) != other {
		t.Fatal("unknown error was rewritten")
	}
	full := &WriteError{Full: true}
	if ClassifyWrite(full) != full {
		t.Fatal("full flag")
	}
	blocked := ClassifyWrite(os.ErrPermission)
	var write *WriteError
	if !errors.As(blocked, &write) || write.Full {
		t.Fatal(blocked)
	}
}

func TestStatMissingPathAndSameDevice(t *testing.T) {
	dir := t.TempDir()
	if _, err := Stat(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing path")
	}
	if !SameDevice(dir, dir) {
		t.Fatal("a folder is not on its own disk")
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if !SameDevice(dir, nested) {
		t.Fatal("a folder and its child are on different disks")
	}
	if SameDevice(dir, filepath.Join(dir, "missing")) {
		t.Fatal("a missing path compared equal")
	}
}
