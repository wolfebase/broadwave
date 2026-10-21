package disk

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReserve(t *testing.T) {
	if WatermarkGB("") != 10 || WatermarkGB("0") != 0 || WatermarkGB("15") != 15 {
		t.Fatalf("watermark parse")
	}
	if !BelowReserve(9e9, 10e9) || BelowReserve(11e9, 10e9) || BelowReserve(1, 0) {
		t.Fatal("reserve comparison")
	}
	err := &LowError{Free: 4_200_000_000, Need: 10_000_000_000}
	want := "The recordings disk has 4.2 GB free, and Broadwave keeps 10 GB in reserve. Free some space or lower the reserve in Settings."
	if err.Error() != want {
		t.Fatal(err.Error())
	}
	blocked := &WriteError{}
	if !strings.Contains(blocked.Error(), "recordings folder") {
		t.Fatal(blocked.Error())
	}
	if !strings.Contains((&WriteError{Full: true}).Error(), "full") {
		t.Fatal("full write")
	}
}

func TestClassifyWriteKeepsAReadOnlyFolderOffTheFullDisk(t *testing.T) {
	err := ClassifyWrite(syscall.EROFS)
	var blocked *WriteError
	if !errors.As(err, &blocked) || blocked.Full {
		t.Fatal(err)
	}
	if !strings.Contains(err.Error(), "recordings folder") {
		t.Fatal(err.Error())
	}
	full := ClassifyWrite(syscall.ENOSPC)
	if !errors.As(full, &blocked) || !blocked.Full {
		t.Fatal(full)
	}
	if ClassifyWrite(&LowError{Free: 1, Need: 2}) == nil {
		t.Fatal("reserve")
	}
}

func TestWritableRejectsALockedFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write a mode 0555 directory")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err := Writable(dir)
	var blocked *WriteError
	if !errors.As(err, &blocked) || blocked.Full {
		if err == nil {
			t.Skip("this user can still write a mode 0555 directory")
		}
		t.Fatal(err)
	}
}

func TestStatTempDir(t *testing.T) {
	space, err := Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if space.Free == 0 || space.Total == 0 || space.Free > space.Total {
		t.Fatalf("%+v", space)
	}
}

func TestPrepareRefusesToRemakeAFolderItUsed(t *testing.T) {
	state := t.TempDir()
	dir := filepath.Join(t.TempDir(), "share", "recordings")
	// A new install makes the folder and marks it.
	if err := Prepare(dir, state, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, Marker)); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(dir, state, false); err != nil {
		t.Fatal(err)
	}
	// The share is not mounted: the folder is gone.
	if err := os.RemoveAll(filepath.Dir(dir)); err != nil {
		t.Fatal(err)
	}
	var missing *MissingError
	if err := Prepare(dir, state, false); !errors.As(err, &missing) || !strings.Contains(err.Error(), "mounted") {
		t.Fatalf("missing folder: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("made the folder again: %v", err)
	}
	// Docker made an empty folder at the mount point.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(dir, state, false); !errors.As(err, &missing) {
		t.Fatalf("empty mount point: %v", err)
	}
}

func TestPrepareTrustsAFolderAnOlderServerUsed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "show.ts"), []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(dir, t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, Marker)); err != nil {
		t.Fatal(err)
	}
	// Recordings in the catalog, and the folder empty or gone: not mounted.
	var missing *MissingError
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(empty, t.TempDir(), true); !errors.As(err, &missing) {
		t.Fatalf("empty: %v", err)
	}
	if err := Prepare(filepath.Join(t.TempDir(), "gone"), t.TempDir(), true); !errors.As(err, &missing) {
		t.Fatalf("gone: %v", err)
	}
}

func TestPrepareNamesAReadOnlyFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write a mode 0555 directory")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err := Prepare(dir, t.TempDir(), false)
	var blocked *WriteError
	if !errors.As(err, &blocked) || blocked.Full || !strings.Contains(err.Error(), "PUID/PGID") {
		t.Fatal(err)
	}
}
