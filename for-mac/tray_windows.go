//go:build windows

package main

// startSystrayOnMainThread on Windows just calls start() directly.
// No GCD dispatch needed — Windows systray library handles threading internally.
func startSystrayOnMainThread(start func()) {
	start()
}

// fixTrayIconSize is a no-op on Windows.
// The default icon size works fine without adjustment.
func fixTrayIconSize() {}
