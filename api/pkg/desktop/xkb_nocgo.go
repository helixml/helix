//go:build !cgo

// Package desktop provides fallback keysym to keycode conversion when CGO is disabled.
// Without xkbcommon, we fall back to a static QWERTY mapping.
package desktop

// XKBKeysymToEvdev returns 0 when CGO is disabled.
// The caller should fall back to keysymToEvdev() in ws_input.go.
func XKBKeysymToEvdev(keysym uint32) int {
	return 0
}

// XKBKeysymNeedsShift returns false when CGO is disabled.
func XKBKeysymNeedsShift(keysym uint32) bool {
	return false
}

// IsXKBAvailable returns false when CGO is disabled.
func IsXKBAvailable() bool {
	return false
}

// StopXKBPolling is a no-op when CGO is disabled.
func StopXKBPolling() {}

// GetCurrentLayout returns empty string when CGO is disabled.
func GetCurrentLayout() string {
	return ""
}
