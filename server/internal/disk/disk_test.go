package disk

import (
	"errors"
	"os"
	"strings"
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
	if !strings.Contains(err.Error(), "full") {
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

func TestWritableRejectsALockedFolder(t *testing.T) {
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
