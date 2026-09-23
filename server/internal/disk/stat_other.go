//go:build !linux && !windows && !darwin && !freebsd

package disk

import "fmt"

func Stat(string) (Space, error) {
	return Space{}, fmt.Errorf("disk space is not available on this system")
}
