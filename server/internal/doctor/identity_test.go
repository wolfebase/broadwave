package doctor

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestHandOverChownsOnlyWhatTheOldOwnerHas(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	for _, sub := range []string{"work/recordings", "backups"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"broadwave.db", "work/recordings/show.ts", "backups/broadwave.db.1", filepath.Join("..", filepath.Base(outside), "secret")} {
		if err := os.WriteFile(filepath.Join(dir, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A link out of the folder is handed over as a link; its target is not touched.
	if err := os.Symlink(outside, filepath.Join(dir, "work", "escape")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var got []string
	record := func(path string, uid, gid int) error {
		got = append(got, path)
		return nil
	}
	me := os.Getuid()
	// Files that belong to someone else stay theirs.
	if err := handOver(root.FS(), me+1, me+2, os.Getgid(), record); err != nil || len(got) != 0 {
		t.Fatalf("chowned %v (err %v), want nothing", got, err)
	}
	if err := handOver(root.FS(), me, me+1, os.Getgid(), record); err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want := []string{".", "backups", "backups/broadwave.db.1", "broadwave.db", "work", "work/escape", "work/recordings", "work/recordings/show.ts"}
	if !slices.Equal(got, want) {
		t.Fatalf("chowned %v, want %v", got, want)
	}
}

func TestTheConfigFolderCannotBeTheRoot(t *testing.T) {
	if err := chownRootOwned("/", 99, 100); err == nil {
		t.Fatal("chowning / was allowed")
	}
}

func TestDeviceGroupKeepsOnlyAGroupThatNeedsIt(t *testing.T) {
	char := fs.ModeDevice | fs.ModeCharDevice
	cases := []struct {
		name string
		mode fs.FileMode
		gid  int
		keep bool
	}{
		{"render group", char | 0o660, 105, true},
		{"anyone can use it", char | 0o666, 105, false},
		{"root's group", char | 0o660, 0, false},
		{"group can only read", char | 0o640, 105, false},
		{"not a device", 0o660, 105, false},
	}
	for _, c := range cases {
		if _, keep := deviceGroup(c.mode, c.gid); keep != c.keep {
			t.Errorf("%s: keep %v, want %v", c.name, keep, c.keep)
		}
	}
	if got := deviceGroups(1000, []string{filepath.Join(t.TempDir(), "missing")}); !slices.Equal(got, []int{1000}) {
		t.Fatalf("groups %v, want [1000]", got)
	}
}

func TestParseIDRefusesWhatSetuidCannotTake(t *testing.T) {
	for _, bad := range []string{"-1", "4294967295", "4294967296", "abc", ""} {
		if _, err := parseID(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	if n, err := parseID("99"); err != nil || n != 99 {
		t.Fatalf("99 read as %d, %v", n, err)
	}
}
