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
