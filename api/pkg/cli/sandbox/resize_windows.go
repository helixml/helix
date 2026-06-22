//go:build windows

package sandbox

import "os"

var sigWinch os.Signal = nil
