package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// mode: "light" or "dark" pins the appearance; anything else follows macOS.
static void setAppAppearance(const char *mode) {
	NSAppearanceName name = nil;
	if (strcmp(mode, "light") == 0) {
		name = NSAppearanceNameAqua;
	} else if (strcmp(mode, "dark") == 0) {
		name = NSAppearanceNameDarkAqua;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		NSApp.appearance = name ? [NSAppearance appearanceNamed:name] : nil;
	});
}
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SetAppearance matches the window to the Helix UI's theme. explicit is true
// when the user picked the mode with the UI's theme toggle: the app then pins
// that appearance (title bar, traffic lights, menus, the webview's
// prefers-color-scheme). Otherwise it follows macOS. mode is the theme in
// effect either way, and sets the window background shown behind the webview.
func (a *App) SetAppearance(mode string, explicit bool) {
	pinned := ""
	if explicit {
		pinned = mode
	}
	cMode := C.CString(pinned)
	defer C.free(unsafe.Pointer(cMode))
	C.setAppAppearance(cMode)

	if mode == "light" {
		runtime.WindowSetBackgroundColour(a.ctx, 255, 255, 255, 255)
	} else {
		runtime.WindowSetBackgroundColour(a.ctx, 18, 18, 20, 255) // #121214
	}
}
