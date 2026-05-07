//go:build !windows

package sandbox

import (
	"os"
	"syscall"
)

var sigWinch os.Signal = syscall.SIGWINCH
