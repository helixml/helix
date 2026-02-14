package main

import (
	_ "embed"
	"log"

	"github.com/energye/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/tray-icon.png
var trayIconBytes []byte

// TrayManager handles the macOS menu bar system tray
type TrayManager struct {
	app     *App
	mStart  *systray.MenuItem
	mStop   *systray.MenuItem
	endFunc func()
}

// NewTrayManager creates a new tray manager
func NewTrayManager(app *App) *TrayManager {
	return &TrayManager{app: app}
}

// Start initializes the system tray. Must be called after Wails startup.
// The actual systray creation is dispatched to the main thread via GCD
// because NSStatusItem must be instantiated on the main thread.
func (t *TrayManager) Start() {
	start, end := systray.RunWithExternalLoop(t.onReady, t.onExit)
	t.endFunc = end
	startSystrayOnMainThread(start)
}

// Stop shuts down the system tray
func (t *TrayManager) Stop() {
	if t.endFunc != nil {
		t.endFunc()
	}
}

func (t *TrayManager) onReady() {
	systray.SetIcon(trayIconBytes)
	fixTrayIconSize()
	systray.SetTooltip("Helix Desktop")

	mShow := systray.AddMenuItem("Show Helix", "Show the Helix window")
	mShow.Click(func() {
		// Wails runtime functions dispatch to the main thread via
		// performSelectorOnMainThread:waitUntilDone:YES. The systray click
		// callback already runs on the main thread (NSMenu action), so calling
		// Wails directly here deadlocks/segfaults. Run in a goroutine instead.
		go func() {
			if t.app.ctx != nil {
				wailsRuntime.Show(t.app.ctx)
				wailsRuntime.WindowShow(t.app.ctx)
			}
		}()
	})

	systray.AddSeparator()

	t.mStart = systray.AddMenuItem("Start", "Start the Helix environment")
	t.mStart.Click(func() {
		go func() {
			if err := t.app.StartVM(); err != nil {
				log.Printf("Tray: start VM failed: %v", err)
			}
		}()
	})

	t.mStop = systray.AddMenuItem("Stop", "Stop the Helix environment")
	t.mStop.Disable()
	t.mStop.Click(func() {
		go func() {
			if err := t.app.StopVM(); err != nil {
				log.Printf("Tray: stop VM failed: %v", err)
			}
		}()
	})

	systray.AddSeparator()

	mSettings := systray.AddMenuItem("Settings...", "Open settings")
	mSettings.Click(func() {
		go func() {
			if t.app.ctx != nil {
				wailsRuntime.Show(t.app.ctx)
				wailsRuntime.WindowShow(t.app.ctx)
				wailsRuntime.EventsEmit(t.app.ctx, "settings:show")
			}
		}()
	})

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Quit Helix", "Quit the application")
	mQuit.Click(func() {
		go func() {
			if t.app.ctx != nil {
				wailsRuntime.Quit(t.app.ctx)
			}
		}()
	})

	// Attach the menu to the status item so clicking the icon shows the dropdown.
	// Without this call, energye/systray on macOS leaves the menu detached and
	// clicking the icon does nothing.
	systray.CreateMenu()
}

func (t *TrayManager) onExit() {
	// Cleanup if needed
}

// UpdateState updates the tray menu items based on VM state
func (t *TrayManager) UpdateState(state string) {
	if t.mStart == nil || t.mStop == nil {
		return
	}

	switch state {
	case "running":
		t.mStart.Disable()
		t.mStop.Enable()
		systray.SetTooltip("Helix Desktop — Running")
	case "stopped":
		t.mStart.Enable()
		t.mStop.Disable()
		systray.SetTooltip("Helix Desktop — Stopped")
	case "starting":
		t.mStart.Disable()
		t.mStop.Disable()
		systray.SetTooltip("Helix Desktop — Starting...")
	case "stopping":
		t.mStart.Disable()
		t.mStop.Disable()
		systray.SetTooltip("Helix Desktop — Stopping...")
	case "error":
		t.mStart.Enable()
		t.mStop.Disable()
		systray.SetTooltip("Helix Desktop — Error")
	}
}
