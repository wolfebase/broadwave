package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ApplyIdentity honors PUID, PGID, and UMASK. It only changes the user when this process is root.
// What root left in the config folder goes to that user first: a catalog an earlier run wrote as
// root would be read-only after the switch. Root's group is dropped, but the groups that own the
// GPU devices are kept so hardware encoding still works. An error means the process may still be
// root and should not start.
func ApplyIdentity(config, recordings string) error {
	if raw := strings.TrimSpace(os.Getenv("UMASK")); raw != "" {
		if n, err := strconv.ParseUint(raw, 8, 32); err == nil {
			syscall.Umask(int(n))
		}
	}
	if os.Getuid() != 0 {
		return nil
	}
	rawUID, rawGID := strings.TrimSpace(os.Getenv("PUID")), strings.TrimSpace(os.Getenv("PGID"))
	if rawUID == "" || rawGID == "" {
		return nil
	}
	uid, uerr := parseID(rawUID)
	gid, gerr := parseID(rawGID)
	if uerr != nil || gerr != nil {
		return fmt.Errorf("PUID and PGID must both be numbers from 0 to 4294967294 (PUID %q, PGID %q)", rawUID, rawGID)
	}
	if config != "" {
		if err := chownRootOwned(config, uid, gid); err != nil {
			return err
		}
	}
	if recordings != "" {
		if info, err := os.Lstat(recordings); err == nil && owner(info) == 0 {
			_ = os.Lchown(recordings, uid, gid)
		}
	}
	devices, _ := filepath.Glob("/dev/dri/*")
	nvidia, _ := filepath.Glob("/dev/nvidia*")
	if err := syscall.Setgroups(deviceGroups(gid, append(devices, nvidia...))); err != nil {
		return fmt.Errorf("set groups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("set group %d: %w", gid, err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("set user %d: %w", uid, err)
	}
	if os.Getuid() != uid || os.Getegid() != gid {
		return fmt.Errorf("still running as %d:%d after switching to %d:%d", os.Getuid(), os.Getegid(), uid, gid)
	}
	return nil
}

func parseID(raw string) (int, error) {
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || n == 1<<32-1 {
		return 0, errors.New("bad id")
	}
	return int(n), nil
}

func owner(info fs.FileInfo) int {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}

// chownRootOwned gives uid and gid everything under dir that root owns. It works
// through an os.Root, so a symlink or a swapped folder cannot lead it outside dir,
// and it stays on dir's filesystem, so a recordings share mounted inside keeps its owners.
func chownRootOwned(dir string, uid, gid int) error {
	if filepath.Clean(dir) == "/" {
		return errors.New("the config folder cannot be /")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil
	}
	defer root.Close()
	return handOver(root.FS(), 0, uid, gid, root.Lchown)
}

// handOver chowns what from owns in fsys to uid and gid.
func handOver(fsys fs.FS, from, uid, gid int, chown func(string, int, int) error) error {
	top, err := fs.Stat(fsys, ".")
	if err != nil {
		return nil
	}
	base, ok := top.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}
		if st.Dev != base.Dev {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if int(st.Uid) == from && (from != uid || int(st.Gid) != gid) {
			_ = chown(path, uid, gid)
		}
		return nil
	})
}

// deviceGroups is gid plus the group of each GPU device that only its group can use.
func deviceGroups(gid int, devices []string) []int {
	out := []int{gid}
	for _, path := range devices {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		g, keep := deviceGroup(info.Mode(), int(st.Gid))
		if !keep {
			continue
		}
		seen := false
		for _, have := range out {
			seen = seen || have == g
		}
		if !seen {
			out = append(out, g)
		}
	}
	return out
}

// deviceGroup keeps a character device's group when that group can read and write it.
// A device anyone can use needs no group, and root's group is never kept.
func deviceGroup(mode fs.FileMode, gid int) (int, bool) {
	perm := mode.Perm()
	if mode&fs.ModeCharDevice == 0 || gid == 0 || perm&0o060 != 0o060 || perm&0o006 == 0o006 {
		return 0, false
	}
	return gid, true
}
