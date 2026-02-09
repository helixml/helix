//go:build ignore

package main

import (
	"fmt"
	"log"

	"github.com/getlantern/systray"
)

// TrayManager manages the macOS menu bar icon
type TrayManager struct {
	app          *App
	mOpen        *systray.MenuItem
	mStartStop   *systray.MenuItem
	mStatus      *systray.MenuItem
	mQuit        *systray.MenuItem
}

// NewTrayManager creates a new tray manager
func NewTrayManager(app *App) *TrayManager {
	return &TrayManager{
		app: app,
	}
}

// Run starts the system tray — must be called from the main goroutine on macOS.
// onReady is called when the tray is initialized.
// onExit is called when the tray exits.
func (t *TrayManager) Run(onReady func(), onExit func()) {
	systray.Run(func() {
		t.onReady()
		if onReady != nil {
			onReady()
		}
	}, func() {
		if onExit != nil {
			onExit()
		}
	})
}

func (t *TrayManager) onReady() {
	systray.SetIcon(trayIconData)
	systray.SetTooltip("Helix Desktop")

	// Menu items
	t.mStatus = systray.AddMenuItem("Helix: Stopped", "Current VM status")
	t.mStatus.Disable()

	systray.AddSeparator()

	t.mOpen = systray.AddMenuItem("Open Helix", "Open the Helix window")
	t.mStartStop = systray.AddMenuItem("Start VM", "Start or stop the VM")

	systray.AddSeparator()

	t.mQuit = systray.AddMenuItem("Quit Helix", "Quit the application")

	// Handle menu item clicks
	go t.handleClicks()
}

func (t *TrayManager) handleClicks() {
	for {
		select {
		case <-t.mOpen.ClickedCh:
			// Show the Wails window
			if t.app.ctx != nil {
				// Wails runtime show window
				log.Println("Tray: Open Helix clicked")
			}

		case <-t.mStartStop.ClickedCh:
			status := t.app.GetVMStatus()
			if status.State == VMStateStopped || status.State == VMStateError {
				log.Println("Tray: Starting VM...")
				t.mStartStop.SetTitle("Starting...")
				t.mStartStop.Disable()
				go func() {
					if err := t.app.StartVM(); err != nil {
						log.Printf("Tray: Failed to start VM: %v", err)
					}
					t.updateStatus()
				}()
			} else if status.State == VMStateRunning {
				log.Println("Tray: Stopping VM...")
				t.mStartStop.SetTitle("Stopping...")
				t.mStartStop.Disable()
				go func() {
					if err := t.app.StopVM(); err != nil {
						log.Printf("Tray: Failed to stop VM: %v", err)
					}
					t.updateStatus()
				}()
			}

		case <-t.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

// UpdateStatus refreshes the tray menu based on current VM state
func (t *TrayManager) updateStatus() {
	if t.mStatus == nil {
		return
	}

	status := t.app.GetVMStatus()
	switch status.State {
	case VMStateRunning:
		t.mStatus.SetTitle(fmt.Sprintf("Helix: Running (%d sessions)", status.Sessions))
		t.mStartStop.SetTitle("Stop VM")
		t.mStartStop.Enable()
	case VMStateStopped:
		t.mStatus.SetTitle("Helix: Stopped")
		t.mStartStop.SetTitle("Start VM")
		t.mStartStop.Enable()
	case VMStateStarting:
		t.mStatus.SetTitle("Helix: Starting...")
		t.mStartStop.SetTitle("Starting...")
		t.mStartStop.Disable()
	case VMStateStopping:
		t.mStatus.SetTitle("Helix: Stopping...")
		t.mStartStop.SetTitle("Stopping...")
		t.mStartStop.Disable()
	case VMStateError:
		t.mStatus.SetTitle("Helix: Error")
		t.mStartStop.SetTitle("Start VM")
		t.mStartStop.Enable()
	}
}
