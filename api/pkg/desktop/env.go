package desktop

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Environment detection with caching.
var (
	envOnce sync.Once
	envInfo struct {
		gnome       bool
		kde         bool
		x11         bool
		x11Checked  bool
		waylandDisp string
	}
)

func detectEnvironment() {
	envOnce.Do(func() {
		desktop := os.Getenv("XDG_CURRENT_DESKTOP")
		session := os.Getenv("DESKTOP_SESSION")

		// GNOME detection
		if strings.Contains(strings.ToUpper(desktop), "GNOME") ||
			strings.Contains(strings.ToLower(session), "gnome") {
			envInfo.gnome = true
		}

		// KDE detection
		if strings.Contains(strings.ToUpper(desktop), "KDE") ||
			os.Getenv("KDE_SESSION_VERSION") != "" ||
			strings.Contains(strings.ToLower(session), "plasma") ||
			strings.Contains(strings.ToLower(session), "kde") {
			envInfo.kde = true
		}
	})
}

// isGNOMEEnvironment returns true if running in GNOME.
func isGNOMEEnvironment() bool {
	detectEnvironment()
	return envInfo.gnome
}

// isKDEEnvironment returns true if running in KDE Plasma.
func isKDEEnvironment() bool {
	detectEnvironment()
	return envInfo.kde
}

// isX11Mode returns true if we should use X11 tools (xclip, scrot).
// This checks for DISPLAY and working xclip.
func isX11Mode() bool {
	detectEnvironment()

	if envInfo.x11Checked {
		return envInfo.x11
	}
	envInfo.x11Checked = true

	display := os.Getenv("DISPLAY")
	if display == "" {
		envInfo.x11 = false
		return false
	}

	// Check if xclip is available
	if _, err := exec.LookPath("xclip"); err != nil {
		envInfo.x11 = false
		return false
	}

	// Test if xclip can connect
	cmd := exec.Command("xclip", "-selection", "clipboard", "-o")
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	output, err := cmd.CombinedOutput()
	if err != nil && strings.Contains(string(output), "cannot open display") {
		envInfo.x11 = false
		return false
	}

	envInfo.x11 = true
	return true
}

// getWaylandDisplay finds an available Wayland display socket.
func getWaylandDisplay(xdgRuntimeDir string) string {
	if envInfo.waylandDisp != "" {
		return envInfo.waylandDisp
	}

	entries, err := os.ReadDir(xdgRuntimeDir)
	if err != nil {
		return ""
	}

	// Find the first valid Wayland socket file (not lock file)
	// Just check file exists - don't spawn wl-paste (causes "Unknown" processes in GNOME panel)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "wayland-") && !strings.HasSuffix(name, ".lock") {
			socketPath := xdgRuntimeDir + "/" + name
			// Check socket file exists and is a socket
			if info, err := os.Stat(socketPath); err == nil && (info.Mode()&os.ModeSocket) != 0 {
				envInfo.waylandDisp = name
				return name
			}
		}
	}

	return ""
}
