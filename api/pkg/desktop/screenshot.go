package desktop

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// handleScreenshot handles GET /screenshot requests.
func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "jpeg"
	}

	quality := 70
	if q := r.URL.Query().Get("quality"); q != "" {
		if parsed, err := strconv.Atoi(q); err == nil {
			quality = clamp(parsed, 1, 100)
		}
	}

	data, actualFormat, err := s.captureScreenshot(format, quality)
	if err != nil {
		s.logger.Error("screenshot capture failed", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/"+actualFormat)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(data)

	s.logger.Info("screenshot captured", "format", actualFormat, "quality", quality, "size", len(data))
}

// captureScreenshot captures a screenshot using the appropriate method.
//
// GNOME: Uses org.gnome.Shell.Screenshot D-Bus API exclusively.
// Requires gnome-shell to be started with --unsafe-mode flag to allow D-Bus access.
// This avoids PipeWire conflicts with Wolf's video pipeline.
// See design doc: design/2026-01-05-screenshot-video-pipeline-interference.md
func (s *Server) captureScreenshot(format string, quality int) ([]byte, string, error) {
	// GNOME: Use D-Bus Screenshot API exclusively (no fallbacks)
	// gnome-shell must be started with --unsafe-mode to allow D-Bus access
	if isGNOMEEnvironment() {
		return s.captureGNOMEScreenshot(format, quality)
	}

	// KDE: use D-Bus API (doesn't conflict with video pipeline)
	if isKDEEnvironment() {
		return s.captureKDE(format, quality)
	}

	// Sway/wlroots: grim (uses wlr-screencopy protocol, no PipeWire conflict)
	if data, actualFormat, err := s.captureGrim(format, quality); err == nil {
		return data, actualFormat, nil
	}

	// X11 fallback
	if isX11Mode() {
		return s.captureX11(format, quality)
	}

	return nil, "", fmt.Errorf("no screenshot method available for this desktop environment")
}

// capturePipeWire captures from the PipeWire stream via gst-launch-1.0.
// It retries a few times since the stream may need time to stabilize after
// session creation, and uses a timeout to prevent hanging.
func (s *Server) capturePipeWire(format string, quality int) ([]byte, string, error) {
	s.logger.Debug("capturing via PipeWire", "node_id", s.nodeID)

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			s.logger.Debug("PipeWire capture retry", "attempt", attempt+1)
			time.Sleep(500 * time.Millisecond)
		}

		data, actualFormat, err := s.tryCapturePipeWire(format, quality)
		if err == nil {
			return data, actualFormat, nil
		}
		lastErr = err
		s.logger.Debug("PipeWire capture attempt failed", "attempt", attempt+1, "err", err)
	}

	return nil, "", fmt.Errorf("PipeWire capture failed after %d attempts: %w", maxRetries, lastErr)
}

// tryCapturePipeWire attempts a single PipeWire capture with timeout.
func (s *Server) tryCapturePipeWire(format string, quality int) ([]byte, string, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	// Use context with timeout to prevent hanging on unresponsive PipeWire streams
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gst-launch-1.0", "-q",
		"pipewiresrc", fmt.Sprintf("path=%d", s.nodeID), "num-buffers=1", "do-timestamp=true",
		"!", "videoconvert",
		"!", "pngenc",
		"!", "filesink", "location="+tmpFile,
	)
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+s.config.XDGRuntimeDir)

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, "", fmt.Errorf("gst-launch timed out after 5 seconds")
	}
	if err != nil {
		return nil, "", fmt.Errorf("gst-launch failed: %w, output: %s", err, string(output))
	}

	pngData, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", fmt.Errorf("read screenshot: %w", err)
	}

	if len(pngData) == 0 {
		return nil, "", fmt.Errorf("gst-launch produced empty output")
	}

	if format == "jpeg" {
		jpegData, err := convertPNGtoJPEG(pngData, quality)
		if err != nil {
			s.logger.Warn("JPEG conversion failed, returning PNG", "err", err)
			return pngData, "png", nil
		}
		return jpegData, "jpeg", nil
	}

	return pngData, "png", nil
}

// captureKDE captures via KWin D-Bus API.
func (s *Server) captureKDE(format string, quality int) ([]byte, string, error) {
	s.logger.Debug("capturing via KDE D-Bus")

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return s.captureX11(format, quality) // Fallback
	}
	defer conn.Close()

	obj := conn.Object("org.kde.KWin", "/org/kde/KWin/ScreenShot2")

	options := map[string]dbus.Variant{
		"include-cursor":    dbus.MakeVariant(true),
		"native-resolution": dbus.MakeVariant(true),
	}

	readFd, writeFd, err := os.Pipe()
	if err != nil {
		return nil, "", fmt.Errorf("create pipe: %w", err)
	}
	defer readFd.Close()

	call := obj.Call("org.kde.KWin.ScreenShot2.CaptureActiveScreen", 0, options, dbus.UnixFD(writeFd.Fd()))
	writeFd.Close()

	if call.Err != nil {
		s.logger.Debug("KDE D-Bus failed, falling back to X11", "err", call.Err)
		return s.captureX11(format, quality)
	}

	pngData, err := io.ReadAll(readFd)
	if err != nil || len(pngData) == 0 {
		return s.captureX11(format, quality)
	}

	if format == "jpeg" {
		jpegData, err := convertPNGtoJPEG(pngData, quality)
		if err != nil {
			return pngData, "png", nil
		}
		return jpegData, "jpeg", nil
	}

	return pngData, "png", nil
}

// captureX11 captures via scrot.
func (s *Server) captureX11(format string, quality int) ([]byte, string, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}

	s.logger.Debug("capturing via scrot", "display", display)

	ext := "png"
	if format == "jpeg" {
		ext = "jpg"
	}
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.%s", time.Now().UnixNano(), ext))
	defer os.Remove(tmpFile)

	cmd := exec.Command("scrot", "-o", "-z", "-p", "-q", strconv.Itoa(quality), tmpFile)
	cmd.Env = append(os.Environ(), "DISPLAY="+display)

	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("scrot failed: %w, output: %s", err, string(output))
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", fmt.Errorf("read screenshot: %w", err)
	}

	return data, format, nil
}

// D-Bus constants for GNOME Shell Screenshot interface
const (
	shellScreenshotBus   = "org.gnome.Shell"
	shellScreenshotPath  = "/org/gnome/Shell/Screenshot"
	shellScreenshotIface = "org.gnome.Shell.Screenshot"
)

// captureGNOMEScreenshot captures a screenshot in GNOME environment.
//
// Strategy:
// 1. If we have a dedicated screenshot ScreenCast session (ssNodeID > 0),
//    use fast PipeWire capture - this doesn't interfere with Wolf's video.
// 2. Otherwise, fall back to D-Bus Screenshot API (slower, ~400ms, serialized).
func (s *Server) captureGNOMEScreenshot(format string, quality int) ([]byte, string, error) {
	// Fast path: Use dedicated screenshot PipeWire node if available
	if s.ssNodeID > 0 {
		return s.captureScreenshotPipeWire(format, quality)
	}

	// Slow path: Fall back to D-Bus Screenshot API
	if s.conn == nil {
		return nil, "", fmt.Errorf("D-Bus connection not available")
	}

	// Serialize screenshot requests - GNOME only allows one at a time per D-Bus connection
	s.screenshotMu.Lock()
	defer s.screenshotMu.Unlock()

	return s.captureShellScreenshotDBus(format, quality)
}

// captureScreenshotPipeWire captures from the dedicated screenshot PipeWire node.
// This is SEPARATE from Wolf's video node to avoid buffer renegotiation conflicts.
func (s *Server) captureScreenshotPipeWire(format string, quality int) ([]byte, string, error) {
	startTime := time.Now()
	s.logger.Debug("capturing via dedicated PipeWire screenshot node", "node_id", s.ssNodeID)

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	// Use gst-launch with JPEG output directly if requested (faster than PNG->JPEG conversion)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if format == "jpeg" {
		// Direct JPEG output - much faster
		cmd = exec.CommandContext(ctx, "gst-launch-1.0", "-q",
			"pipewiresrc", fmt.Sprintf("path=%d", s.ssNodeID), "num-buffers=1", "do-timestamp=true",
			"!", "videoconvert",
			"!", "jpegenc", fmt.Sprintf("quality=%d", quality),
			"!", "filesink", "location="+tmpFile,
		)
	} else {
		cmd = exec.CommandContext(ctx, "gst-launch-1.0", "-q",
			"pipewiresrc", fmt.Sprintf("path=%d", s.ssNodeID), "num-buffers=1", "do-timestamp=true",
			"!", "videoconvert",
			"!", "pngenc",
			"!", "filesink", "location="+tmpFile,
		)
	}
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+s.config.XDGRuntimeDir)

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, "", fmt.Errorf("gst-launch timed out after 3 seconds")
	}
	if err != nil {
		s.logger.Warn("PipeWire screenshot failed, falling back to D-Bus",
			"err", err,
			"output", string(output))
		// Fall back to D-Bus Screenshot
		s.screenshotMu.Lock()
		defer s.screenshotMu.Unlock()
		return s.captureShellScreenshotDBus(format, quality)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", fmt.Errorf("read screenshot: %w", err)
	}

	if len(data) == 0 {
		return nil, "", fmt.Errorf("gst-launch produced empty output")
	}

	totalTime := time.Since(startTime)
	s.logger.Info("PipeWire screenshot captured",
		"format", format,
		"total_ms", totalTime.Milliseconds(),
		"size", len(data))

	return data, format, nil
}

// captureShellScreenshotDBus captures via org.gnome.Shell.Screenshot D-Bus interface.
// Uses the server's existing D-Bus connection for reliability in headless mode.
func (s *Server) captureShellScreenshotDBus(format string, quality int) ([]byte, string, error) {
	startTime := time.Now()
	s.logger.Debug("capturing via D-Bus org.gnome.Shell.Screenshot")

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	// Call org.gnome.Shell.Screenshot.Screenshot(include_cursor, flash, filename)
	// Returns: (success: bool, filename_used: string)
	obj := s.conn.Object(shellScreenshotBus, dbus.ObjectPath(shellScreenshotPath))

	var success bool
	var filenameUsed string

	// Use CallWithContext for timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	call := obj.CallWithContext(ctx, shellScreenshotIface+".Screenshot", 0,
		true,    // include_cursor
		false,   // flash (don't flash screen)
		tmpFile, // filename
	)

	if call.Err != nil {
		return nil, "", fmt.Errorf("D-Bus Screenshot call failed: %w", call.Err)
	}

	if err := call.Store(&success, &filenameUsed); err != nil {
		return nil, "", fmt.Errorf("D-Bus Screenshot store result failed: %w", err)
	}

	if !success {
		return nil, "", fmt.Errorf("Screenshot method returned success=false")
	}

	dbusTime := time.Since(startTime)
	s.logger.Debug("D-Bus Screenshot succeeded", "filename", filenameUsed, "dbus_ms", dbusTime.Milliseconds())

	// Read the screenshot file
	pngData, err := os.ReadFile(filenameUsed)
	if err != nil {
		return nil, "", fmt.Errorf("read screenshot file: %w", err)
	}

	if len(pngData) == 0 {
		return nil, "", fmt.Errorf("D-Bus Screenshot produced empty file")
	}

	readTime := time.Since(startTime)

	// Convert to JPEG if requested
	if format == "jpeg" {
		jpegData, err := convertPNGtoJPEG(pngData, quality)
		if err != nil {
			s.logger.Warn("JPEG conversion failed, returning PNG", "err", err)
			return pngData, "png", nil
		}
		totalTime := time.Since(startTime)
		s.logger.Info("screenshot timing",
			"dbus_ms", dbusTime.Milliseconds(),
			"read_ms", (readTime - dbusTime).Milliseconds(),
			"convert_ms", (totalTime - readTime).Milliseconds(),
			"total_ms", totalTime.Milliseconds(),
			"png_size", len(pngData),
			"jpeg_size", len(jpegData))
		return jpegData, "jpeg", nil
	}

	totalTime := time.Since(startTime)
	s.logger.Info("screenshot timing",
		"dbus_ms", dbusTime.Milliseconds(),
		"read_ms", (readTime - dbusTime).Milliseconds(),
		"total_ms", totalTime.Milliseconds(),
		"png_size", len(pngData))
	return pngData, "png", nil
}

// captureGNOMEScreenshotCLI captures via gnome-screenshot CLI.
// This is a fallback when D-Bus Screenshot doesn't work.
func (s *Server) captureGNOMEScreenshotCLI(format string, quality int) ([]byte, string, error) {
	s.logger.Debug("capturing via gnome-screenshot CLI")

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	// Use context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// gnome-screenshot options:
	// -f <filename>: save to file
	// No other options needed - captures full screen by default
	cmd := exec.CommandContext(ctx, "gnome-screenshot", "-f", tmpFile)
	cmd.Env = append(os.Environ(),
		"XDG_RUNTIME_DIR="+s.config.XDGRuntimeDir,
	)

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, "", fmt.Errorf("gnome-screenshot timed out after 5 seconds")
	}
	if err != nil {
		return nil, "", fmt.Errorf("gnome-screenshot failed: %w, output: %s", err, string(output))
	}

	pngData, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", fmt.Errorf("read screenshot: %w", err)
	}

	if len(pngData) == 0 {
		return nil, "", fmt.Errorf("gnome-screenshot produced empty output")
	}

	if format == "jpeg" {
		jpegData, err := convertPNGtoJPEG(pngData, quality)
		if err != nil {
			s.logger.Warn("JPEG conversion failed, returning PNG", "err", err)
			return pngData, "png", nil
		}
		return jpegData, "jpeg", nil
	}

	return pngData, "png", nil
}

// captureGrim captures via grim for wlroots compositors.
func (s *Server) captureGrim(format string, quality int) ([]byte, string, error) {
	s.logger.Debug("capturing via grim")

	xdgRuntimeDir := s.config.XDGRuntimeDir
	waylandDisplay := getWaylandDisplay(xdgRuntimeDir)
	if waylandDisplay == "" {
		return nil, "", fmt.Errorf("no Wayland display found")
	}

	ext := "png"
	if format == "jpeg" {
		ext = "jpg"
	}
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.%s", time.Now().UnixNano(), ext))
	defer os.Remove(tmpFile)

	args := []string{"-c"} // Include cursor
	if format == "jpeg" {
		args = append(args, "-t", "jpeg", "-q", strconv.Itoa(quality))
	} else {
		args = append(args, "-t", "png")
	}
	args = append(args, tmpFile)

	cmd := exec.Command("grim", args...)
	cmd.Env = append(os.Environ(),
		"WAYLAND_DISPLAY="+waylandDisplay,
		"XDG_RUNTIME_DIR="+xdgRuntimeDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check for screencopy protocol error (KDE)
		if strings.Contains(string(output), "screencopy") {
			return s.captureKDE(format, quality)
		}
		return nil, "", fmt.Errorf("grim failed: %w, output: %s", err, string(output))
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", fmt.Errorf("read screenshot: %w", err)
	}

	return data, format, nil
}

// convertPNGtoJPEG converts PNG to JPEG with specified quality.
func convertPNGtoJPEG(pngData []byte, quality int) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("decode PNG: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}

	return buf.Bytes(), nil
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
