//go:build !linux && !windows

package disk

import "fmt"

func Stat(string) (Space, error) {
	return Space{}, fmt.Errorf("disk space is not available on this system")
}
