//go:build darwin

package hydra

import "syscall"

// setStatfsBsize fills Statfs_t.Bsize without forcing a typed literal in the
// shared test (linux: int64, darwin: uint32).
func setStatfsBsize(stat *syscall.Statfs_t, v int64) {
	stat.Bsize = uint32(v)
}
