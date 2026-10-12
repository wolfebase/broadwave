package doctor

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"testing/fstest"
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

func TestApplyIdentitySetsAnOctalUmaskWithoutSwitchingUser(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root would switch user")
	}
	old := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(old) })
	t.Setenv("PUID", "")
	t.Setenv("PGID", "")

	t.Setenv("UMASK", "")
	if err := ApplyIdentity(t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	if got := syscall.Umask(0o077); got != 0o077 {
		t.Fatalf("no UMASK changed the mask to %o", got)
	}
	t.Setenv("UMASK", " 022 ")
	if err := ApplyIdentity("", ""); err != nil {
		t.Fatal(err)
	}
	if got := syscall.Umask(0o022); got != 0o022 {
		t.Fatalf("umask %o, want 022", got)
	}
	t.Setenv("UMASK", "not-octal")
	if err := ApplyIdentity("", ""); err != nil {
		t.Fatal(err)
	}
	if got := syscall.Umask(0o022); got != 0o022 {
		t.Fatalf("a bad UMASK changed the mask to %o", got)
	}
}

func TestChownRootOwnedSkipsAMissingFolderAndOtherOwners(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root owns the test files")
	}
	if err := chownRootOwned(filepath.Join(t.TempDir(), "missing"), 99, 100); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog")
	if err := os.WriteFile(catalog, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := chownRootOwned(dir, 99, 100); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if got := owner(info); got != os.Getuid() {
		t.Fatalf("catalog owner %d, want %d", got, os.Getuid())
	}
}

type plainInfo struct{ fs.FileInfo }

func (plainInfo) Sys() any { return nil }

func TestOwnerIsUnknownWithoutAStat(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := owner(info); got != os.Getuid() {
		t.Fatalf("owner %d, want %d", got, os.Getuid())
	}
	if got := owner(plainInfo{info}); got != -1 {
		t.Fatalf("owner %d, want -1", got)
	}
}

func TestDeviceGroupsSkipsOrdinaryFiles(t *testing.T) {
	regular := filepath.Join(t.TempDir(), "clip.ts")
	if err := os.WriteFile(regular, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := deviceGroups(44, []string{regular}); !slices.Equal(got, []int{44}) {
		t.Fatalf("groups %v, want [44]", got)
	}
}

func TestHandOverDoesNothingWithoutARootOwner(t *testing.T) {
	never := func(path string, _, _ int) error {
		t.Fatalf("chowned %s", path)
		return nil
	}
	if err := handOver(os.DirFS(filepath.Join(t.TempDir(), "gone")), 0, 1, 2, never); err != nil {
		t.Fatal(err)
	}
	if err := handOver(fstest.MapFS{"note": {Mode: 0o644}}, 0, 1, 2, never); err != nil {
		t.Fatal(err)
	}
}

func TestHandOverWalksPastAFolderItCannotList(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can list a mode 0 folder")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{filepath.Join(locked, "clip.ts"), filepath.Join(dir, "next.ts")} {
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	var got []string
	me := os.Getuid()
	err := handOver(os.DirFS(dir), me, me+1, os.Getgid(), func(path string, _, _ int) error {
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "next.ts") || slices.Contains(got, "locked/clip.ts") {
		t.Fatalf("chowned %v", got)
	}
}

func TestHandOverStaysOnOneDiskAndKeepsAMatchingOwner(t *testing.T) {
	// Dev is int32 on darwin and uint64 on linux; untyped constants fit either.
	fsys := fstest.MapFS{
		".":                {Mode: fs.ModeDir | 0o755, Sys: &syscall.Stat_t{Dev: 1, Uid: 10, Gid: 20}},
		"library.db":       {Mode: 0o644, Sys: &syscall.Stat_t{Dev: 1, Uid: 10, Gid: 21}},
		"other.db":         {Mode: 0o644, Sys: &syscall.Stat_t{Dev: 1, Uid: 11, Gid: 20}},
		"kept.db":          {Mode: 0o644, Sys: &syscall.Stat_t{Dev: 1, Uid: 10, Gid: 20}},
		"remote.ts":        {Mode: 0o644, Sys: &syscall.Stat_t{Dev: 2, Uid: 10, Gid: 21}},
		"share":            {Mode: fs.ModeDir | 0o755, Sys: &syscall.Stat_t{Dev: 3, Uid: 10, Gid: 21}},
		"share/episode.ts": {Mode: 0o644, Sys: &syscall.Stat_t{Dev: 1, Uid: 10, Gid: 21}},
		"note":             {Mode: 0o644},
	}
	var got []string
	err := handOver(fsys, 10, 10, 20, func(path string, _, _ int) error {
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"library.db"}) {
		t.Fatalf("chowned %v, want [library.db]", got)
	}
}
