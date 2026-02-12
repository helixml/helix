//go:build linux

package main

// startSystrayOnMainThread on Linux just calls start() directly.
func startSystrayOnMainThread(start func()) {
	start()
}

// fixTrayIconSize is a no-op on Linux.
func fixTrayIconSize() {}
