package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

// SetWindowTheme sets the window background behind the webview to the theme
// in effect ("light" or "dark"), so resizing and loading don't flash the
// other theme. The app never pins the macOS appearance: the Helix UI must keep
// seeing the system theme (prefers-color-scheme), which is its default.
func (a *App) SetWindowTheme(mode string) {
	if mode == "light" {
		runtime.WindowSetBackgroundColour(a.ctx, 255, 255, 255, 255)
	} else {
		runtime.WindowSetBackgroundColour(a.ctx, 18, 18, 20, 255) // #121214
	}
}
