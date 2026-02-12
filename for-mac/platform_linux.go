//go:build linux

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// getHelixDataDir returns the Linux-conventional data directory for Helix.
// Uses ~/.local/share/helix (XDG_DATA_HOME/helix).
func getHelixDataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "helix")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".local", "share", "helix")
}

// getAppBundlePath returns empty string on Linux (no .app bundles).
func getAppBundlePath() string {
	return ""
}

// openBrowser opens a URL in the default browser on Linux
func openBrowser(url string) error {
	cmd := exec.Command("xdg-open", url)
	return cmd.Start()
}

// preferredNIC returns empty string on Linux — no preferred interface name.
func preferredNIC() string {
	return ""
}

// getSystemMemoryMB returns the total physical memory in MB on Linux.
func getSystemMemoryMB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var memKB int64
	for _, line := range splitLines(string(data)) {
		if len(line) > 9 && line[:9] == "MemTotal:" {
			fmt.Sscanf(line, "MemTotal: %d kB", &memKB)
			break
		}
	}
	return int(memKB / 1024)
}

// generateMachineID creates a unique machine identifier for license binding.
// On Linux, uses /etc/machine-id.
func generateMachineID() string {
	data, err := os.ReadFile("/etc/machine-id")
	if err == nil && len(data) > 0 {
		return fmt.Sprintf("linux-%s", string(data[:16]))
	}
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("linux-%s", hex.EncodeToString(b)[:16])
}
