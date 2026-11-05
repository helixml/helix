package main

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

// laptopDisplayMode reports the built-in display's current mode (the main
// display if there is no active built-in one, e.g. lid closed on an external
// monitor). CoreGraphics is safe to call off the main thread.
static int laptopDisplayMode(size_t *pointsWide, size_t *pixelsWide) {
	CGDirectDisplayID ids[16];
	uint32_t n = 0;
	if (CGGetActiveDisplayList(16, ids, &n) != kCGErrorSuccess || n == 0) {
		return 0;
	}
	CGDirectDisplayID pick = CGMainDisplayID();
	for (uint32_t i = 0; i < n; i++) {
		if (CGDisplayIsBuiltin(ids[i])) {
			pick = ids[i];
			break;
		}
	}
	CGDisplayModeRef mode = CGDisplayCopyDisplayMode(pick);
	if (mode == NULL) {
		return 0;
	}
	*pointsWide = CGDisplayModeGetWidth(mode);
	*pixelsWide = CGDisplayModeGetPixelWidth(mode);
	CGDisplayModeRelease(mode);
	return 1;
}
*/
import "C"

// laptopDisplay returns the laptop display's width in points and its scale
// (backing pixels per point), e.g. 1800 and 2.0 on a MacBook Pro set to
// "More Space".
func laptopDisplay() (pointsWide int, scale float64, ok bool) {
	var points, pixels C.size_t
	if C.laptopDisplayMode(&points, &pixels) == 0 || points == 0 {
		return 0, 0, false
	}
	return int(points), float64(pixels) / float64(points), true
}
