package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

var cachedWaylandDisplay string
var jpegSupported = true  // Will be set to false if grim reports "jpeg support disabled"
var useKDEScreenshot = false // Will be set to true if grim fails with screencopy protocol error (KDE)

// KDE environment detection - cached after first check
var kdeChecked bool
var isKDE bool

// Clipboard mode detection - cached after first check
var clipboardModeChecked bool
var useX11Clipboard bool

// GNOME environment detection - cached after first check
var gnomeChecked bool
var isGNOME bool

// isGNOMEEnvironment returns true if we're running in GNOME environment
// GNOME 41+ blocks org.gnome.Shell.Screenshot D-Bus API, so we use scrot via XWayland
// Wolf uses gst-wayland-src (GStreamer), so grim doesn't work on the outer compositor
func isGNOMEEnvironment() bool {
	if gnomeChecked {
		return isGNOME
	}
	gnomeChecked = true

	// Check XDG_CURRENT_DESKTOP (most reliable)
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	if strings.Contains(strings.ToUpper(desktop), "GNOME") {
		log.Printf("[Screenshot] GNOME detected via XDG_CURRENT_DESKTOP=%s, will use scrot via XWayland", desktop)
		isGNOME = true
		return true
	}

	// Check DESKTOP_SESSION
	session := os.Getenv("DESKTOP_SESSION")
	if strings.Contains(strings.ToLower(session), "gnome") {
		log.Printf("[Screenshot] GNOME detected via DESKTOP_SESSION=%s, will use scrot via XWayland", session)
		isGNOME = true
		return true
	}

	isGNOME = false
	return false
}

// isKDEEnvironment returns true if we're running in KDE Plasma environment
// KDE's KWin doesn't support wlr-screencopy protocol, so we use D-Bus API
func isKDEEnvironment() bool {
	if kdeChecked {
		return isKDE
	}
	kdeChecked = true

	// Check XDG_CURRENT_DESKTOP (most reliable)
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	if strings.Contains(strings.ToUpper(desktop), "KDE") {
		log.Printf("[Screenshot] KDE detected via XDG_CURRENT_DESKTOP=%s, will use grim on Wolf compositor", desktop)
		isKDE = true
		return true
	}

	// Check KDE_SESSION_VERSION (set in KDE Plasma sessions)
	if os.Getenv("KDE_SESSION_VERSION") != "" {
		log.Printf("[Screenshot] KDE detected via KDE_SESSION_VERSION, will use grim on Wolf compositor")
		isKDE = true
		return true
	}

	// Check DESKTOP_SESSION
	session := os.Getenv("DESKTOP_SESSION")
	if strings.Contains(strings.ToLower(session), "plasma") || strings.Contains(strings.ToLower(session), "kde") {
		log.Printf("[Screenshot] KDE detected via DESKTOP_SESSION=%s, will use grim on Wolf compositor", session)
		isKDE = true
		return true
	}

	isKDE = false
	return false
}

// isX11Mode returns true if we should use X11 clipboard (xclip) instead of Wayland (wl-paste/wl-copy)
// This is needed for Ubuntu GNOME which runs on Xwayland (X11 on top of Wayland)
func isX11Mode() bool {
	if clipboardModeChecked {
		return useX11Clipboard
	}
	clipboardModeChecked = true

	// Check if DISPLAY is set (indicates X11/Xwayland environment)
	display := os.Getenv("DISPLAY")
	if display == "" {
		log.Printf("[Clipboard] No DISPLAY set, using Wayland mode")
		useX11Clipboard = false
		return false
	}

	// Check if xclip is available
	_, err := exec.LookPath("xclip")
	if err != nil {
		log.Printf("[Clipboard] DISPLAY=%s but xclip not found, using Wayland mode", display)
		useX11Clipboard = false
		return false
	}

	// Test if xclip can actually connect to the X server
	testCmd := exec.Command("xclip", "-selection", "clipboard", "-o")
	testCmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))
	// We don't care about the output, just whether it can run without "cannot open display" error
	output, err := testCmd.CombinedOutput()
	if err != nil && strings.Contains(string(output), "cannot open display") {
		log.Printf("[Clipboard] xclip cannot connect to DISPLAY=%s, using Wayland mode", display)
		useX11Clipboard = false
		return false
	}

	log.Printf("[Clipboard] Using X11 mode with DISPLAY=%s", display)
	useX11Clipboard = true
	return true
}

func main() {
	port := os.Getenv("SCREENSHOT_PORT")
	if port == "" {
		port = "9876"
	}

	http.HandleFunc("/screenshot", handleScreenshot)
	http.HandleFunc("/clipboard", handleClipboard)
	http.HandleFunc("/keyboard-state", handleKeyboardState)
	http.HandleFunc("/keyboard-reset", handleKeyboardReset)
	http.HandleFunc("/upload", handleUpload)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Screenshot server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check for format and quality query parameters
	// ?format=jpeg&quality=60 for lower bandwidth (default: jpeg with quality 70)
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "jpeg" // Default to JPEG for smaller file sizes
	}
	// If JPEG not supported by grim, force PNG
	if !jpegSupported && format == "jpeg" {
		format = "png"
	}
	qualityStr := r.URL.Query().Get("quality")
	quality := 70 // Default JPEG quality (good balance of size/quality)
	if qualityStr != "" {
		if q, err := fmt.Sscanf(qualityStr, "%d", &quality); err == nil && q > 0 {
			if quality < 1 {
				quality = 1
			} else if quality > 100 {
				quality = 100
			}
		}
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets" // Wolf-UI uses /tmp/sockets not /run/user/wolf
	}

	// Capture screenshot with format fallback
	data, actualFormat, err := captureScreenshot(xdgRuntimeDir, format, quality)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to capture screenshot: %v", err), http.StatusInternalServerError)
		return
	}

	// Serve the image with correct content type
	if actualFormat == "jpeg" {
		w.Header().Set("Content-Type", "image/jpeg")
	} else {
		w.Header().Set("Content-Type", "image/png")
	}
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	log.Printf("Screenshot captured successfully (%d bytes, format=%s, quality=%d)", len(data), actualFormat, quality)
}

// captureScreenshot captures a screenshot using grim (Wayland), spectacle (KDE), or scrot (X11)
func captureScreenshot(xdgRuntimeDir, format string, quality int) ([]byte, string, error) {
	// Check if we're in X11 mode (legacy or forced X11)
	if isX11Mode() {
		return captureScreenshotX11(format, quality)
	}

	// Check if we're in KDE environment
	// Use org.kde.KWin.ScreenShot2 D-Bus API (silent, no popup)
	if isKDEEnvironment() {
		return captureScreenshotKDE(format, quality)
	}

	// Check if we're in GNOME environment
	// Use org.gnome.Shell.Screenshot D-Bus API, registering as "org.gnome.Screenshot"
	// to pass GNOME 41+'s allowlist check. Wolf uses gst-wayland-src (not wlroots)
	// so grim doesn't work on the outer compositor.
	if isGNOMEEnvironment() {
		return captureScreenshotGNOME(format, quality)
	}

	// Check if we should use KDE screenshot method (set by previous grim failure on unknown compositor)
	if useKDEScreenshot {
		return captureScreenshotKDE(format, quality)
	}

	// Wayland mode - use grim
	// Create temporary file for screenshot
	tmpDir := os.TempDir()
	ext := "jpg"
	if format == "png" {
		ext = "png"
	}
	filename := filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.%s", time.Now().UnixNano(), ext))
	defer os.Remove(filename)

	// Build grim arguments based on format and quality
	// -c includes the cursor in the screenshot
	grimArgs := []string{"-c"}
	if format == "jpeg" {
		grimArgs = append(grimArgs, "-t", "jpeg", "-q", fmt.Sprintf("%d", quality))
	} else {
		grimArgs = append(grimArgs, "-t", "png")
	}
	grimArgs = append(grimArgs, filename)

	var output []byte
	var err error

	// Try cached display first if available
	if cachedWaylandDisplay != "" {
		cmd := exec.Command("grim", grimArgs...)
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", cachedWaylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		output, err = cmd.CombinedOutput()

		// Check for JPEG not supported error
		if err != nil && strings.Contains(string(output), "jpeg support disabled") {
			log.Printf("JPEG not supported by grim, falling back to PNG")
			jpegSupported = false
			return captureScreenshot(xdgRuntimeDir, "png", quality)
		}

		// Check for screencopy protocol error (KDE doesn't support wlr-screencopy)
		if err != nil && strings.Contains(string(output), "screencopy") {
			log.Printf("Compositor doesn't support screencopy protocol, trying Wolf's outer compositor")
			useKDEScreenshot = true
			return captureScreenshotKDE(format, quality)
		}
	}

	// If cached failed or doesn't exist, try all sockets
	if err != nil || cachedWaylandDisplay == "" {
		// Find all wayland-* sockets
		entries, readErr := os.ReadDir(xdgRuntimeDir)
		if readErr != nil {
			return nil, "", fmt.Errorf("failed to read XDG_RUNTIME_DIR: %v", readErr)
		}

		waylandSockets := []string{}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "wayland-") && !strings.HasSuffix(entry.Name(), ".lock") {
				waylandSockets = append(waylandSockets, entry.Name())
			}
		}

		if len(waylandSockets) == 0 {
			return nil, "", fmt.Errorf("no Wayland sockets found in %s", xdgRuntimeDir)
		}

		// Try each socket until one works
		for _, socket := range waylandSockets {
			cmd := exec.Command("grim", grimArgs...)
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("WAYLAND_DISPLAY=%s", socket),
				fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
			)
			output, err = cmd.CombinedOutput()

			// Check for JPEG not supported error
			if err != nil && strings.Contains(string(output), "jpeg support disabled") {
				log.Printf("JPEG not supported by grim, falling back to PNG")
				jpegSupported = false
				return captureScreenshot(xdgRuntimeDir, "png", quality)
			}

			// Check for screencopy protocol error (KDE doesn't support wlr-screencopy)
			if err != nil && strings.Contains(string(output), "screencopy") {
				log.Printf("Compositor doesn't support screencopy protocol, trying Wolf's outer compositor")
				useKDEScreenshot = true
				return captureScreenshotKDE(format, quality)
			}

			if err == nil {
				cachedWaylandDisplay = socket // Cache for next time
				break
			}
			log.Printf("Failed to capture with %s: %v, output: %s", socket, err, string(output))
		}

		if err != nil {
			return nil, "", fmt.Errorf("failed to capture with any Wayland socket: %v", err)
		}
	}

	// Read the screenshot file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read screenshot file: %v", err)
	}

	return data, format, nil
}

// captureScreenshotX11 captures a screenshot using scrot (for X11/Xwayland environments)
func captureScreenshotX11(format string, quality int) ([]byte, string, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return nil, "", fmt.Errorf("DISPLAY not set for X11 screenshot")
	}

	// Create temporary file for screenshot
	// scrot determines format from file extension
	tmpDir := os.TempDir()
	var filename string
	var outputFormat string

	if format == "jpeg" {
		filename = filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.jpg", time.Now().UnixNano()))
		outputFormat = "jpeg"
	} else {
		filename = filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
		outputFormat = "png"
	}
	defer os.Remove(filename)

	// Use scrot for X11 screenshots
	// -o = overwrite file, -z = silent mode, -p = capture mouse pointer, -q = quality (0-100)
	// scrot uses file extension to determine output format
	cmd := exec.Command("scrot", "-o", "-z", "-p", "-q", fmt.Sprintf("%d", quality), filename)
	cmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, "", fmt.Errorf("scrot failed: %v, output: %s", err, string(output))
	}

	// Read the screenshot file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read screenshot file: %v", err)
	}

	log.Printf("[X11] Screenshot captured as %s (%d bytes, quality=%d)", outputFormat, len(data), quality)
	return data, outputFormat, nil
}

// captureScreenshotGNOME captures a screenshot for GNOME environment
// Uses org.gnome.Shell.Screenshot D-Bus API. GNOME 41+ restricts this API to an
// allowlist of callers, so we register as "org.gnome.Screenshot" to pass the check.
// See: https://gitlab.gnome.org/GNOME/gnome-shell/-/issues/3943
func captureScreenshotGNOME(format string, quality int) ([]byte, string, error) {
	log.Printf("[GNOME] Capturing screenshot via D-Bus (org.gnome.Shell.Screenshot)")

	// Connect to session D-Bus
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, "", fmt.Errorf("failed to connect to session D-Bus: %w", err)
	}
	defer conn.Close()

	// CRITICAL: Request the name "org.gnome.Screenshot" to pass GNOME's allowlist check
	// GNOME 41+ restricts org.gnome.Shell.Screenshot API to these callers:
	// - org.gnome.SettingsDaemon.MediaKeys
	// - org.gnome.Screenshot
	// - org.freedesktop.impl.portal.desktop.gtk
	reply, err := conn.RequestName("org.gnome.Screenshot", dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Printf("[GNOME] Warning: Failed to request D-Bus name org.gnome.Screenshot: %v", err)
		// Continue anyway - might work if the name check is lenient
	} else if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Printf("[GNOME] Warning: Could not become primary owner of org.gnome.Screenshot (reply=%d)", reply)
	} else {
		log.Printf("[GNOME] Successfully registered as org.gnome.Screenshot on D-Bus")
	}

	// Create temp file path for screenshot
	tmpDir := os.TempDir()
	filename := filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))

	// Get the GNOME Shell Screenshot object
	obj := conn.Object("org.gnome.Shell", "/org/gnome/Shell/Screenshot")

	// Call Screenshot method
	// GNOME 49+ signature: Screenshot(include_cursor: b, flash: b, filename: s) → (success: b, filename_used: s)
	// Note: GNOME 49 changed the argument order - filename is now LAST (was first in older versions)
	var success bool
	var filenameUsed string
	err = obj.Call("org.gnome.Shell.Screenshot.Screenshot", 0,
		true,      // include_cursor
		false,     // flash (disable flash for silent capture)
		filename,  // filename (now last in GNOME 49+)
	).Store(&success, &filenameUsed)

	if err != nil {
		// GNOME 49+ has stricter security - the Screenshot D-Bus API may be blocked
		// Fall back to ScreenCast-based screenshot which is NOT blocked
		log.Printf("[GNOME] Screenshot D-Bus API failed (%v), trying ScreenCast fallback...", err)
		return captureScreenshotGNOMEScreenCast(format, quality)
	}

	if !success {
		// Also try ScreenCast fallback if the call succeeded but returned false
		log.Printf("[GNOME] Screenshot D-Bus API returned success=false, trying ScreenCast fallback...")
		return captureScreenshotGNOMEScreenCast(format, quality)
	}

	// Read the PNG file
	pngData, err := os.ReadFile(filenameUsed)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read screenshot file %s: %w", filenameUsed, err)
	}
	defer os.Remove(filenameUsed)

	log.Printf("[GNOME] D-Bus screenshot captured (%d bytes PNG)", len(pngData))

	// If JPEG is requested, convert PNG to JPEG
	if format == "jpeg" {
		jpegData, err := convertPNGtoJPEG(pngData, quality)
		if err != nil {
			log.Printf("[GNOME] Failed to convert to JPEG, returning PNG: %v", err)
			return pngData, "png", nil
		}
		log.Printf("[GNOME] Converted to JPEG (%d bytes, quality=%d)", len(jpegData), quality)
		return jpegData, "jpeg", nil
	}

	return pngData, "png", nil
}

// findXwaylandDisplay tries to detect the Xwayland display from socket files
// GNOME/Mutter starts Xwayland on-demand, so we check for X socket files
func findXwaylandDisplay() string {
	// Check environment first
	if display := os.Getenv("DISPLAY"); display != "" {
		return display
	}

	// Check for X socket files in XDG_RUNTIME_DIR (Xwayland creates these)
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/run/user/1000"
	}

	// Display numbers to check - includes :99 which we start explicitly for GNOME headless
	displayNumbers := []int{0, 1, 2, 99}

	// Look for X socket files (format: X{N} where N is display number)
	for _, i := range displayNumbers {
		socketPath := filepath.Join(xdgRuntimeDir, fmt.Sprintf(".X11-unix/X%d", i))
		if _, err := os.Stat(socketPath); err == nil {
			display := fmt.Sprintf(":%d", i)
			log.Printf("[GNOME/X11] Found Xwayland socket at %s, using DISPLAY=%s", socketPath, display)
			return display
		}
	}

	// Also check /tmp/.X11-unix (traditional location)
	for _, i := range displayNumbers {
		socketPath := fmt.Sprintf("/tmp/.X11-unix/X%d", i)
		if _, err := os.Stat(socketPath); err == nil {
			display := fmt.Sprintf(":%d", i)
			log.Printf("[GNOME/X11] Found X socket at %s, using DISPLAY=%s", socketPath, display)
			return display
		}
	}

	log.Printf("[GNOME/X11] No X11 display socket found, defaulting to :99")
	return ":99"
}

// captureScreenshotGNOMEScreenCast captures a screenshot using scrot via X11.
// DISABLED: The PipeWire-based gnome-screenshot.py conflicts with Wolf's video capture.
// Reading from the same PipeWire node as Wolf causes stream interference and crashes.
// Fall back to scrot which uses X11/Xwayland instead.
func captureScreenshotGNOMEScreenCast(format string, quality int) ([]byte, string, error) {
	log.Printf("[GNOME/X11] Screenshot D-Bus blocked, falling back to scrot via Xwayland...")

	// Detect Xwayland display from socket files
	display := findXwaylandDisplay()
	log.Printf("[GNOME/X11] Using DISPLAY=%s for scrot", display)

	// Create temporary file for screenshot
	tmpDir := os.TempDir()
	var filename string
	var outputFormat string

	if format == "jpeg" {
		filename = filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.jpg", time.Now().UnixNano()))
		outputFormat = "jpeg"
	} else {
		filename = filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
		outputFormat = "png"
	}
	defer os.Remove(filename)

	// Use scrot for X11 screenshots
	cmd := exec.Command("scrot", "-o", "-z", "-p", "-q", fmt.Sprintf("%d", quality), filename)
	cmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[GNOME/X11] scrot failed with DISPLAY=%s: %v, output: %s", display, err, string(output))
		return nil, "", fmt.Errorf("scrot failed: %v, output: %s", err, string(output))
	}

	// Read the screenshot file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read screenshot file: %v", err)
	}

	log.Printf("[GNOME/X11] Screenshot captured as %s (%d bytes, quality=%d)", outputFormat, len(data), quality)
	return data, outputFormat, nil
}

// captureScreenshotKDE captures a screenshot for KDE Plasma environment
// Uses KWin's D-Bus API (org.kde.KWin.ScreenShot2) which is the proper way
// to capture screenshots on KDE Wayland. grim doesn't work because Wolf uses
// gst-wayland-src (GStreamer), not wlroots, so wlr-screencopy isn't available.
func captureScreenshotKDE(format string, quality int) ([]byte, string, error) {
	log.Printf("[KDE] Capturing screenshot via D-Bus (org.kde.KWin.ScreenShot2)")

	// Connect to session D-Bus
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, "", fmt.Errorf("failed to connect to session D-Bus: %w", err)
	}
	defer conn.Close()

	// Get the KWin Screenshot2 object
	// The CaptureActiveScreen method captures the screen without any UI popup
	obj := conn.Object("org.kde.KWin", "/org/kde/KWin/ScreenShot2")

	// Options for the screenshot
	// "include-cursor" = true to include mouse cursor
	// "native-resolution" = true to capture at native resolution
	options := map[string]dbus.Variant{
		"include-cursor":    dbus.MakeVariant(true),
		"native-resolution": dbus.MakeVariant(true),
	}

	// Create a pipe for the screenshot data
	// KWin writes PNG data to the file descriptor
	readFd, writeFd, err := os.Pipe()
	if err != nil {
		return nil, "", fmt.Errorf("failed to create pipe: %w", err)
	}
	defer readFd.Close()

	// Call CaptureActiveScreen which captures without UI
	// Method signature: CaptureActiveScreen(options: a{sv}) -> (fd: h)
	// The 'h' type means it returns data via a file descriptor
	call := obj.Call("org.kde.KWin.ScreenShot2.CaptureActiveScreen", 0, options, dbus.UnixFD(writeFd.Fd()))
	writeFd.Close() // Close write end after passing to D-Bus

	if call.Err != nil {
		// Try alternative method: CaptureScreen with screen index
		log.Printf("[KDE] CaptureActiveScreen failed (%v), trying CaptureScreen", call.Err)

		// Recreate pipe
		readFd.Close()
		readFd, writeFd, err = os.Pipe()
		if err != nil {
			return nil, "", fmt.Errorf("failed to create pipe: %w", err)
		}
		defer readFd.Close()

		// Try CaptureScreen with screen name (usually "0" or a connector name)
		call = obj.Call("org.kde.KWin.ScreenShot2.CaptureScreen", 0, "0", options, dbus.UnixFD(writeFd.Fd()))
		writeFd.Close()

		if call.Err != nil {
			// D-Bus failed (likely authorization in Plasma 6.5+), fall back to X11
			log.Printf("[KDE] D-Bus screenshot failed (%v), falling back to X11/scrot", call.Err)
			return captureScreenshotKDEViaX11(format, quality)
		}
	}

	// Read PNG data from the pipe
	pngData, err := io.ReadAll(readFd)
	if err != nil {
		log.Printf("[KDE] Failed to read from pipe (%v), falling back to X11/scrot", err)
		return captureScreenshotKDEViaX11(format, quality)
	}

	if len(pngData) == 0 {
		log.Printf("[KDE] D-Bus returned empty data, falling back to X11/scrot")
		return captureScreenshotKDEViaX11(format, quality)
	}

	log.Printf("[KDE] D-Bus screenshot captured (%d bytes PNG)", len(pngData))

	// If JPEG is requested, convert PNG to JPEG
	if format == "jpeg" {
		jpegData, err := convertPNGtoJPEG(pngData, quality)
		if err != nil {
			log.Printf("[KDE] Failed to convert to JPEG, returning PNG: %v", err)
			return pngData, "png", nil
		}
		log.Printf("[KDE] Converted to JPEG (%d bytes, quality=%d)", len(jpegData), quality)
		return jpegData, "jpeg", nil
	}

	return pngData, "png", nil
}

// captureScreenshotKDEViaX11 captures a screenshot using scrot via Xwayland
// This is a fallback for KDE Plasma 6.5+ where D-Bus API requires portal authorization
// KDE runs Xwayland at DISPLAY=:0, so scrot can capture the X11 view
func captureScreenshotKDEViaX11(format string, quality int) ([]byte, string, error) {
	// KDE runs Xwayland at :0
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0" // Default for KDE's Xwayland
	}

	log.Printf("[KDE/X11] Capturing screenshot via scrot (DISPLAY=%s)", display)

	// Check if scrot is available
	scrotPath, err := exec.LookPath("scrot")
	if err != nil {
		return nil, "", fmt.Errorf("scrot not found: %w", err)
	}

	// Create temporary file for screenshot
	tmpDir := os.TempDir()
	var filename string
	var outputFormat string

	if format == "jpeg" {
		filename = filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.jpg", time.Now().UnixNano()))
		outputFormat = "jpeg"
	} else {
		filename = filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
		outputFormat = "png"
	}
	defer os.Remove(filename)

	// Use scrot for X11 screenshots
	// -o = overwrite file, -z = silent mode, -p = capture mouse pointer, -q = quality (0-100)
	cmd := exec.Command(scrotPath, "-o", "-z", "-p", "-q", fmt.Sprintf("%d", quality), filename)
	cmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, "", fmt.Errorf("scrot failed: %v, output: %s", err, string(output))
	}

	// Read the screenshot file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read screenshot file: %v", err)
	}

	log.Printf("[KDE/X11] Screenshot captured as %s (%d bytes, quality=%d)", outputFormat, len(data), quality)
	return data, outputFormat, nil
}

// convertPNGtoJPEG converts PNG image data to JPEG with specified quality
func convertPNGtoJPEG(pngData []byte, quality int) ([]byte, error) {
	// Decode PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode PNG: %w", err)
	}

	// Encode as JPEG
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("failed to encode JPEG: %w", err)
	}

	return jpegBuf.Bytes(), nil
}

func handleClipboard(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetClipboard(w, r)
	case http.MethodPost:
		handleSetClipboard(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetClipboard(w http.ResponseWriter, r *http.Request) {
	// Check if we should use X11 clipboard (legacy X11 environments only)
	if isX11Mode() {
		handleGetClipboardX11(w, r)
		return
	}

	// Wayland mode (original implementation)
	// Get Wayland display (same logic as screenshot)
	waylandDisplay := getWaylandDisplay()
	if waylandDisplay == "" {
		log.Printf("No Wayland display available for clipboard access")
		// Return empty clipboard instead of error (clipboard might just be empty)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
		return
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets"
	}

	// Try BOTH clipboard selections (CLIPBOARD and PRIMARY) and return whichever has content
	// Zed and other apps might use different selections
	// CLIPBOARD = Ctrl+C/V, PRIMARY = text selection/middle-click

	// Helper to try getting clipboard from a selection
	tryGetClipboard := func(usePrimary bool) (string, []byte, bool) {
		args := []string{"--list-types"}
		if usePrimary {
			args = []string{"--primary", "--list-types"}
		}

		listCmd := exec.Command("wl-paste", args...)
		listCmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)

		typesOutput, err := listCmd.Output()
		if err != nil {
			return "", nil, false // Empty or error
		}

		mimeTypes := string(typesOutput)
		isImage := strings.Contains(mimeTypes, "image/png") || strings.Contains(mimeTypes, "image/jpeg")

		// Get the actual data
		var dataCmd *exec.Cmd
		if isImage {
			if usePrimary {
				dataCmd = exec.Command("wl-paste", "--primary", "-t", "image/png")
			} else {
				dataCmd = exec.Command("wl-paste", "-t", "image/png")
			}
		} else {
			if usePrimary {
				dataCmd = exec.Command("wl-paste", "--primary", "--no-newline")
			} else {
				dataCmd = exec.Command("wl-paste", "--no-newline")
			}
		}

		dataCmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)

		data, err := dataCmd.Output()
		if err != nil {
			return "", nil, false
		}

		clipboardType := "text"
		if isImage {
			clipboardType = "image"
		}

		return clipboardType, data, true
	}

	// Try CLIPBOARD first, then PRIMARY
	clipType, clipData, found := tryGetClipboard(false)
	if !found {
		clipType, clipData, found = tryGetClipboard(true)
		if !found {
			// Both selections empty
			log.Printf("Both CLIPBOARD and PRIMARY selections are empty")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(""))
			return
		}
		log.Printf("Using PRIMARY selection")
	}

	// Return clipboard data as JSON
	if clipType == "image" {
		response := map[string]string{
			"type": "image",
			"data": base64.StdEncoding.EncodeToString(clipData),
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
		log.Printf("Clipboard image retrieved (%d bytes)", len(clipData))
	} else {
		response := map[string]string{
			"type": "text",
			"data": string(clipData),
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
		log.Printf("Clipboard text retrieved (%d bytes)", len(clipData))
	}
}

// handleGetClipboardX11 reads clipboard using xclip (for X11/Xwayland environments like Ubuntu GNOME)
func handleGetClipboardX11(w http.ResponseWriter, r *http.Request) {
	display := os.Getenv("DISPLAY")

	// Helper to try getting clipboard from a selection using xclip
	tryGetClipboardX11 := func(selection string) (string, []byte, bool) {
		// First check what MIME types are available
		targetsCmd := exec.Command("xclip", "-selection", selection, "-t", "TARGETS", "-o")
		targetsCmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))
		targetsOutput, err := targetsCmd.Output()
		if err != nil {
			return "", nil, false
		}

		targets := string(targetsOutput)
		isImage := strings.Contains(targets, "image/png")

		var dataCmd *exec.Cmd
		if isImage {
			dataCmd = exec.Command("xclip", "-selection", selection, "-t", "image/png", "-o")
		} else {
			dataCmd = exec.Command("xclip", "-selection", selection, "-o")
		}
		dataCmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))

		data, err := dataCmd.Output()
		if err != nil {
			return "", nil, false
		}

		// Empty clipboard
		if len(data) == 0 {
			return "", nil, false
		}

		clipboardType := "text"
		if isImage {
			clipboardType = "image"
		}

		return clipboardType, data, true
	}

	// Try CLIPBOARD first (Ctrl+C/V), then PRIMARY (text selection)
	clipType, clipData, found := tryGetClipboardX11("clipboard")
	if !found {
		clipType, clipData, found = tryGetClipboardX11("primary")
		if !found {
			log.Printf("[X11] Both CLIPBOARD and PRIMARY selections are empty")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"type": "text", "data": ""})
			return
		}
		log.Printf("[X11] Using PRIMARY selection")
	}

	// Return clipboard data as JSON
	if clipType == "image" {
		response := map[string]string{
			"type": "image",
			"data": base64.StdEncoding.EncodeToString(clipData),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		log.Printf("[X11] Clipboard image retrieved (%d bytes)", len(clipData))
	} else {
		response := map[string]string{
			"type": "text",
			"data": string(clipData),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		log.Printf("[X11] Clipboard text retrieved (%d bytes)", len(clipData))
	}
}

func handleSetClipboard(w http.ResponseWriter, r *http.Request) {
	// Check if we should use X11 clipboard (legacy X11 environments only)
	if isX11Mode() {
		handleSetClipboardX11(w, r)
		return
	}

	// Wayland mode (original implementation)
	// Get Wayland display (same logic as screenshot)
	waylandDisplay := getWaylandDisplay()
	if waylandDisplay == "" {
		log.Printf("No Wayland display available for clipboard access")
		http.Error(w, "No Wayland display available", http.StatusInternalServerError)
		return
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets"
	}

	// Read request body (JSON with type and data)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse JSON to determine clipboard type
	var clipboardData struct {
		Type string `json:"type"` // "text" or "image"
		Data string `json:"data"` // text content or base64-encoded image
	}
	if err := json.Unmarshal(body, &clipboardData); err != nil {
		log.Printf("Failed to parse clipboard JSON: %v", err)
		http.Error(w, "Invalid clipboard data format", http.StatusBadRequest)
		return
	}

	if clipboardData.Type == "image" {
		// Decode base64 image
		imageBytes, err := base64.StdEncoding.DecodeString(clipboardData.Data)
		if err != nil {
			log.Printf("Failed to decode base64 image: %v", err)
			http.Error(w, "Invalid base64 image data", http.StatusBadRequest)
			return
		}

		// Set BOTH clipboard selections for maximum compatibility
		// Execute wl-copy with image/png MIME type (CLIPBOARD selection)
		cmd := exec.Command("wl-copy", "-t", "image/png")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		cmd.Stdin = bytes.NewReader(imageBytes)

		if err := cmd.Run(); err != nil {
			log.Printf("Failed to set image clipboard: %v", err)
			http.Error(w, "Failed to set image clipboard", http.StatusInternalServerError)
			return
		}

		// Also set PRIMARY selection
		cmdPrimary := exec.Command("wl-copy", "--primary", "-t", "image/png")
		cmdPrimary.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		cmdPrimary.Stdin = bytes.NewReader(imageBytes)
		cmdPrimary.Run() // Ignore error - PRIMARY is best-effort

		w.WriteHeader(http.StatusOK)
		log.Printf("Clipboard image set in both selections (%d bytes)", len(imageBytes))
	} else {
		// Set BOTH clipboard selections for maximum compatibility
		// Text clipboard (CLIPBOARD selection)
		cmd := exec.Command("wl-copy")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		cmd.Stdin = strings.NewReader(clipboardData.Data)

		if err := cmd.Run(); err != nil {
			log.Printf("Failed to set text clipboard: %v", err)
			http.Error(w, "Failed to set clipboard", http.StatusInternalServerError)
			return
		}

		// Also set PRIMARY selection
		cmdPrimary := exec.Command("wl-copy", "--primary")
		cmdPrimary.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		cmdPrimary.Stdin = strings.NewReader(clipboardData.Data)
		cmdPrimary.Run() // Ignore error - PRIMARY is best-effort

		w.WriteHeader(http.StatusOK)
		log.Printf("Clipboard text set in both selections (%d bytes)", len(clipboardData.Data))
	}
}

// handleSetClipboardX11 writes clipboard using xclip (for X11/Xwayland environments like Ubuntu GNOME)
func handleSetClipboardX11(w http.ResponseWriter, r *http.Request) {
	display := os.Getenv("DISPLAY")

	// Read request body (JSON with type and data)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[X11] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse JSON to determine clipboard type
	var clipboardData struct {
		Type string `json:"type"` // "text" or "image"
		Data string `json:"data"` // text content or base64-encoded image
	}
	if err := json.Unmarshal(body, &clipboardData); err != nil {
		log.Printf("[X11] Failed to parse clipboard JSON: %v", err)
		http.Error(w, "Invalid clipboard data format", http.StatusBadRequest)
		return
	}

	if clipboardData.Type == "image" {
		// Decode base64 image
		imageBytes, err := base64.StdEncoding.DecodeString(clipboardData.Data)
		if err != nil {
			log.Printf("[X11] Failed to decode base64 image: %v", err)
			http.Error(w, "Invalid base64 image data", http.StatusBadRequest)
			return
		}

		// Set CLIPBOARD selection with image/png MIME type
		cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-i")
		cmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))
		cmd.Stdin = bytes.NewReader(imageBytes)

		if err := cmd.Run(); err != nil {
			log.Printf("[X11] Failed to set image clipboard: %v", err)
			http.Error(w, "Failed to set image clipboard", http.StatusInternalServerError)
			return
		}

		// Also set PRIMARY selection (best-effort)
		cmdPrimary := exec.Command("xclip", "-selection", "primary", "-t", "image/png", "-i")
		cmdPrimary.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))
		cmdPrimary.Stdin = bytes.NewReader(imageBytes)
		cmdPrimary.Run() // Ignore error

		w.WriteHeader(http.StatusOK)
		log.Printf("[X11] Clipboard image set in both selections (%d bytes)", len(imageBytes))
	} else {
		// Set CLIPBOARD selection with text
		cmd := exec.Command("xclip", "-selection", "clipboard", "-i")
		cmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))
		cmd.Stdin = strings.NewReader(clipboardData.Data)

		if err := cmd.Run(); err != nil {
			log.Printf("[X11] Failed to set text clipboard: %v", err)
			http.Error(w, "Failed to set clipboard", http.StatusInternalServerError)
			return
		}

		// Also set PRIMARY selection (best-effort)
		cmdPrimary := exec.Command("xclip", "-selection", "primary", "-i")
		cmdPrimary.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))
		cmdPrimary.Stdin = strings.NewReader(clipboardData.Data)
		cmdPrimary.Run() // Ignore error

		w.WriteHeader(http.StatusOK)
		log.Printf("[X11] Clipboard text set in both selections (%d bytes)", len(clipboardData.Data))
	}
}

// Removed duplicate handleSetClipboard - merged into the one above

// KeyboardState represents the current keyboard state from Wolf's virtual keyboard
type KeyboardState struct {
	Timestamp    int64    `json:"timestamp"`
	PressedKeys  []int    `json:"pressed_keys"`
	KeyNames     []string `json:"key_names"`
	ModifierState struct {
		Shift bool `json:"shift"`
		Ctrl  bool `json:"ctrl"`
		Alt   bool `json:"alt"`
		Meta  bool `json:"meta"`
	} `json:"modifier_state"`
	DeviceName string `json:"device_name"`
	DevicePath string `json:"device_path"`
}

// Linux input key codes for modifiers
const (
	KEY_LEFTCTRL   = 29
	KEY_LEFTSHIFT  = 42
	KEY_LEFTALT    = 56
	KEY_LEFTMETA   = 125
	KEY_RIGHTCTRL  = 97
	KEY_RIGHTSHIFT = 54
	KEY_RIGHTALT   = 100
	KEY_RIGHTMETA  = 126
)

// keyCodeNames maps Linux key codes to human-readable names (common keys)
var keyCodeNames = map[int]string{
	1: "ESC", 2: "1", 3: "2", 4: "3", 5: "4", 6: "5", 7: "6", 8: "7", 9: "8", 10: "9",
	11: "0", 12: "-", 13: "=", 14: "BACKSPACE", 15: "TAB",
	16: "Q", 17: "W", 18: "E", 19: "R", 20: "T", 21: "Y", 22: "U", 23: "I", 24: "O", 25: "P",
	26: "[", 27: "]", 28: "ENTER", 29: "LEFTCTRL",
	30: "A", 31: "S", 32: "D", 33: "F", 34: "G", 35: "H", 36: "J", 37: "K", 38: "L",
	39: ";", 40: "'", 41: "`", 42: "LEFTSHIFT", 43: "\\",
	44: "Z", 45: "X", 46: "C", 47: "V", 48: "B", 49: "N", 50: "M",
	51: ",", 52: ".", 53: "/", 54: "RIGHTSHIFT", 55: "*",
	56: "LEFTALT", 57: "SPACE", 58: "CAPSLOCK",
	59: "F1", 60: "F2", 61: "F3", 62: "F4", 63: "F5", 64: "F6", 65: "F7", 66: "F8", 67: "F9", 68: "F10",
	87: "F11", 88: "F12",
	97: "RIGHTCTRL", 100: "RIGHTALT", 125: "LEFTMETA", 126: "RIGHTMETA",
	102: "HOME", 103: "UP", 104: "PAGEUP", 105: "LEFT", 106: "RIGHT",
	107: "END", 108: "DOWN", 109: "PAGEDOWN", 110: "INSERT", 111: "DELETE",
}

// handleKeyboardState returns the current keyboard state from Wolf's virtual keyboard
func handleKeyboardState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := KeyboardState{
		Timestamp: time.Now().UnixMilli(),
	}

	// Find Wolf's virtual keyboard device (created by inputtino)
	// It's typically named "Wolf (virtual) Keyboard" or similar
	entries, err := os.ReadDir("/dev/input")
	if err != nil {
		log.Printf("Failed to read /dev/input: %v", err)
		http.Error(w, "Failed to read input devices", http.StatusInternalServerError)
		return
	}

	var keyboardDevice string
	var keyboardName string

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}

		devicePath := filepath.Join("/dev/input", entry.Name())

		// Read device name from sysfs
		sysPath := fmt.Sprintf("/sys/class/input/%s/device/name", entry.Name())
		nameBytes, err := os.ReadFile(sysPath)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(string(nameBytes))
		// Look for Wolf's virtual keyboard
		if strings.Contains(strings.ToLower(name), "wolf") && strings.Contains(strings.ToLower(name), "keyboard") {
			keyboardDevice = devicePath
			keyboardName = name
			break
		}
	}

	if keyboardDevice == "" {
		// No Wolf keyboard found - return empty state
		log.Printf("Wolf virtual keyboard not found in /dev/input")
		state.DeviceName = "Not found"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
		return
	}

	state.DevicePath = keyboardDevice
	state.DeviceName = keyboardName

	// Use evtest to get current key state (--query mode)
	// evtest --query /dev/input/eventX EV_KEY <keycode>
	// Returns exit code 0 if key is pressed, non-zero otherwise

	// Query all common keys to find which are pressed
	pressedKeys := []int{}
	keyNames := []string{}

	for keyCode := range keyCodeNames {
		cmd := exec.Command("evtest", "--query", keyboardDevice, "EV_KEY", fmt.Sprintf("%d", keyCode))
		err := cmd.Run()
		if err == nil {
			// Key is pressed (exit code 0)
			pressedKeys = append(pressedKeys, keyCode)
			keyNames = append(keyNames, keyCodeNames[keyCode])
		}
	}

	state.PressedKeys = pressedKeys
	state.KeyNames = keyNames

	// Check modifier state explicitly
	for _, keyCode := range pressedKeys {
		switch keyCode {
		case KEY_LEFTSHIFT, KEY_RIGHTSHIFT:
			state.ModifierState.Shift = true
		case KEY_LEFTCTRL, KEY_RIGHTCTRL:
			state.ModifierState.Ctrl = true
		case KEY_LEFTALT, KEY_RIGHTALT:
			state.ModifierState.Alt = true
		case KEY_LEFTMETA, KEY_RIGHTMETA:
			state.ModifierState.Meta = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)

	log.Printf("Keyboard state queried: %d keys pressed, modifiers: shift=%v ctrl=%v alt=%v meta=%v",
		len(pressedKeys), state.ModifierState.Shift, state.ModifierState.Ctrl,
		state.ModifierState.Alt, state.ModifierState.Meta)
}

// handleKeyboardReset releases all keys on Wolf's virtual keyboard
func handleKeyboardReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Find Wolf's virtual keyboard device
	entries, err := os.ReadDir("/dev/input")
	if err != nil {
		log.Printf("Failed to read /dev/input: %v", err)
		http.Error(w, "Failed to read input devices", http.StatusInternalServerError)
		return
	}

	var keyboardDevice string

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}

		// Read device name from sysfs
		sysPath := fmt.Sprintf("/sys/class/input/%s/device/name", entry.Name())
		nameBytes, err := os.ReadFile(sysPath)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(string(nameBytes))
		if strings.Contains(strings.ToLower(name), "wolf") && strings.Contains(strings.ToLower(name), "keyboard") {
			keyboardDevice = filepath.Join("/dev/input", entry.Name())
			break
		}
	}

	if keyboardDevice == "" {
		http.Error(w, "Wolf virtual keyboard not found", http.StatusNotFound)
		return
	}

	// Use evemu-event to release all modifier keys
	// evemu-event <device> --type EV_KEY --code <keycode> --value 0 --sync
	releasedKeys := []string{}

	modifierKeys := []int{KEY_LEFTCTRL, KEY_RIGHTCTRL, KEY_LEFTSHIFT, KEY_RIGHTSHIFT,
		KEY_LEFTALT, KEY_RIGHTALT, KEY_LEFTMETA, KEY_RIGHTMETA}

	for _, keyCode := range modifierKeys {
		// First check if key is pressed
		checkCmd := exec.Command("evtest", "--query", keyboardDevice, "EV_KEY", fmt.Sprintf("%d", keyCode))
		if checkCmd.Run() == nil {
			// Key is pressed, release it
			releaseCmd := exec.Command("evemu-event", keyboardDevice,
				"--type", "EV_KEY", "--code", fmt.Sprintf("%d", keyCode), "--value", "0", "--sync")
			if err := releaseCmd.Run(); err != nil {
				log.Printf("Failed to release key %d: %v", keyCode, err)
			} else {
				releasedKeys = append(releasedKeys, keyCodeNames[keyCode])
			}
		}
	}

	response := map[string]interface{}{
		"success":       true,
		"released_keys": releasedKeys,
		"message":       fmt.Sprintf("Released %d stuck modifier keys", len(releasedKeys)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("Keyboard reset: released %d keys: %v", len(releasedKeys), releasedKeys)
}

// getWaylandDisplay returns the cached or detected Wayland display
func getWaylandDisplay() string {
	// Return cached if available
	if cachedWaylandDisplay != "" {
		return cachedWaylandDisplay
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets"
	}

	// Find all wayland-* sockets
	entries, err := os.ReadDir(xdgRuntimeDir)
	if err != nil {
		log.Printf("Failed to read XDG_RUNTIME_DIR: %v", err)
		return ""
	}

	waylandSockets := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wayland-") && !strings.HasSuffix(entry.Name(), ".lock") {
			waylandSockets = append(waylandSockets, entry.Name())
		}
	}

	if len(waylandSockets) == 0 {
		log.Printf("No Wayland sockets found in %s", xdgRuntimeDir)
		return ""
	}

	// Try each socket with a simple test (wl-paste will fail gracefully if clipboard is empty)
	for _, socket := range waylandSockets {
		cmd := exec.Command("wl-paste", "--list-types")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", socket),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		if err := cmd.Run(); err == nil {
			cachedWaylandDisplay = socket
			log.Printf("Detected Wayland display for clipboard: %s", socket)
			return socket
		}
	}

	// If all failed, just use the first one and hope for the best
	cachedWaylandDisplay = waylandSockets[0]
	log.Printf("Using fallback Wayland display: %s", cachedWaylandDisplay)
	return cachedWaylandDisplay
}

// uniqueFilePath returns a unique file path by adding numeric suffix if file exists
// e.g., "report.zip" -> "report (1).zip" -> "report (2).zip"
func uniqueFilePath(destPath, filename string) (string, string) {
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return destPath, filename
	}

	dir := filepath.Dir(destPath)
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	for i := 1; i < 1000; i++ {
		newFilename := fmt.Sprintf("%s (%d)%s", base, i, ext)
		newPath := filepath.Join(dir, newFilename)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath, newFilename
		}
	}

	// Fallback: use timestamp if somehow we have 1000 files with same name
	newFilename := fmt.Sprintf("%s (%d)%s", base, time.Now().UnixNano(), ext)
	return filepath.Join(dir, newFilename), newFilename
}

// handleUpload handles file uploads to the sandbox incoming folder
// POST /upload
// Body: multipart/form-data with "file" field
// Response: JSON { "path": "/home/retro/work/incoming/filename", "size": 12345, "filename": "filename" }
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 500MB)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		log.Printf("Failed to parse multipart form: %v", err)
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("Failed to get file from form: %v", err)
		http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Always upload to ~/work/incoming/
	destDir := "/home/retro/work/incoming"

	// Ensure directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.Printf("Failed to create incoming directory: %v", err)
		http.Error(w, "Failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Sanitize filename to prevent path traversal
	filename := filepath.Base(header.Filename)
	destPath := filepath.Join(destDir, filename)

	// If file exists, add numeric suffix like "file (1).ext", "file (2).ext"
	destPath, filename = uniqueFilePath(destPath, filename)

	// Create destination file
	dst, err := os.Create(destPath)
	if err != nil {
		log.Printf("Failed to create destination file %s: %v", destPath, err)
		http.Error(w, "Failed to create file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy file content
	written, err := io.Copy(dst, file)
	if err != nil {
		log.Printf("Failed to write file content: %v", err)
		http.Error(w, "Failed to write file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success with filename for toast display
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":     destPath,
		"size":     written,
		"filename": filename,
	})

	log.Printf("File uploaded successfully: %s (%d bytes)", destPath, written)
}
