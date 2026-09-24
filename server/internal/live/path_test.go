package live

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUniquePathNeverReusesAFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "20260924_150405_4.1_WDAF.ts")
	if got := uniquePath(first); got != first {
		t.Fatalf("free name changed: %s", got)
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := uniquePath(first)
	if second != filepath.Join(dir, "20260924_150405_4.1_WDAF-2.ts") {
		t.Fatalf("second = %s", second)
	}
	if err := os.WriteFile(second, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if third := uniquePath(first); third != filepath.Join(dir, "20260924_150405_4.1_WDAF-3.ts") {
		t.Fatalf("third = %s", third)
	}
}
