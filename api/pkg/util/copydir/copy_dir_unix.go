//go:build !windows

package copydir

import "syscall"

func sameFilesystem(path1, path2 string) bool {
	var stat1, stat2 syscall.Stat_t
	if err := syscall.Stat(path1, &stat1); err != nil {
		return false
	}
	if err := syscall.Stat(path2, &stat2); err != nil {
		return false
	}
	return stat1.Dev == stat2.Dev
}
