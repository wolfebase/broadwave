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
