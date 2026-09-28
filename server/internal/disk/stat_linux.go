//go:build linux

package disk

import "syscall"

func Stat(path string) (Space, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Space{}, err
	}
	bsize := st.Frsize
	if bsize <= 0 {
		bsize = st.Bsize
	}
	if bsize <= 0 {
		return Space{}, syscall.EINVAL
	}
	return Space{
		Free:  uint64(bsize) * st.Bavail,
		Total: uint64(bsize) * st.Blocks,
	}, nil
}

// SameDevice reports whether two existing paths are on one filesystem.
func SameDevice(a, b string) bool {
	var sa, sb syscall.Stat_t
	if syscall.Stat(a, &sa) != nil || syscall.Stat(b, &sb) != nil {
		return false
	}
	return sa.Dev == sb.Dev
}
