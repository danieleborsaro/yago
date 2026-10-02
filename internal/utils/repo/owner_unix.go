//go:build unix

package repo

import (
	"os"
	"syscall"
)

// lstat like git, so a .git symlink belongs to whoever made the link, not to what it points at
func lookupOwner(path string) (uint32, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}
