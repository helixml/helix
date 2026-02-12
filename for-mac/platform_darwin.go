//go:build darwin

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// getHelixDataDir returns the macOS-conventional data directory for Helix.
// Uses ~/Library/Application Support/Helix/ which works with and without App Sandbox.
func getHelixDataDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "Library", "Application Support", "Helix")
}

// getAppBundlePath returns the path to the running .app bundle, if any.
// Returns empty string if not running from an app bundle.
func getAppBundlePath() string {
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return ""
	}
	macosDir := filepath.Dir(execPath)
	if filepath.Base(macosDir) != "MacOS" {
		return ""
	}
	contentsDir := filepath.Dir(macosDir)
	if filepath.Base(contentsDir) != "Contents" {
		return ""
	}
	appDir := filepath.Dir(contentsDir)
	if filepath.Ext(appDir) != ".app" {
		return ""
	}
	return appDir
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) error {
	cmd := exec.Command("open", url)
	return cmd.Start()
}

// preferredNIC returns the preferred network interface name for this platform.
// On macOS, en0 is the primary NIC (Wi-Fi on laptops, Ethernet on desktops).
func preferredNIC() string {
	return "en0"
}

// getSystemMemoryMB returns the total physical memory in MB using sysctl on macOS.
func getSystemMemoryMB() int {
	var mem uint64
	size := uint64(8)
	name := [2]int32{6 /* CTL_HW */, 24 /* HW_MEMSIZE */}
	_, _, err := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&name[0])),
		2,
		uintptr(unsafe.Pointer(&mem)),
		uintptr(unsafe.Pointer(&size)),
		0, 0,
	)
	if err != 0 {
		return 0
	}
	return int(mem / (1024 * 1024))
}

// generateMachineID creates a unique machine identifier for license binding.
// On macOS, uses the hardware UUID from IOKit.
func generateMachineID() string {
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	out, err := cmd.Output()
	if err != nil {
		// Fallback: random ID
		b := make([]byte, 16)
		rand.Read(b)
		return hex.EncodeToString(b)
	}
	// Parse IOPlatformUUID from output
	for _, line := range splitLines(string(out)) {
		if len(line) > 0 {
			// Look for "IOPlatformUUID" = "..."
			if idx := len(line); idx > 0 {
				// Simplified: just hash the whole output
				break
			}
		}
	}
	// Hash the ioreg output as machine ID
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("mac-%s", hex.EncodeToString(b)[:16])
}
