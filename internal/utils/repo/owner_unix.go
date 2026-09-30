//go:build unix

package repo

import (
	"os"
	"syscall"
)

func lookupOwner(path string) (uint32, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}
