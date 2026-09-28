//go:build darwin || freebsd

package disk

import "golang.org/x/sys/unix"

func Stat(path string) (Space, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Space{}, err
	}
	bsize := uint64(st.Bsize)
	if bsize == 0 {
		return Space{}, unix.EINVAL
	}
	return Space{
		Free:  bsize * uint64(st.Bavail),
		Total: bsize * uint64(st.Blocks),
	}, nil
}

// SameDevice reports whether two existing paths are on one filesystem.
func SameDevice(a, b string) bool {
	var sa, sb unix.Stat_t
	if unix.Stat(a, &sa) != nil || unix.Stat(b, &sb) != nil {
		return false
	}
	return sa.Dev == sb.Dev
}
