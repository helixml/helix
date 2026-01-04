package desktop

import (
	"bytes"
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
func (s *Server) captureScreenshot(format string, quality int) ([]byte, string, error) {
	// Use PipeWire if we have a node ID (GNOME)
	if s.nodeID != 0 {
		return s.capturePipeWire(format, quality)
	}

	// KDE: use D-Bus API
	if isKDEEnvironment() {
		return s.captureKDE(format, quality)
	}

	// X11 fallback
	if isX11Mode() {
		return s.captureX11(format, quality)
	}

	// Try grim for wlroots-based compositors
	return s.captureGrim(format, quality)
}

// capturePipeWire captures from the PipeWire stream via gst-launch-1.0.
func (s *Server) capturePipeWire(format string, quality int) ([]byte, string, error) {
	s.logger.Debug("capturing via PipeWire", "node_id", s.nodeID)

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	cmd := exec.Command("gst-launch-1.0", "-q",
		"pipewiresrc", fmt.Sprintf("path=%d", s.nodeID), "num-buffers=1", "do-timestamp=true",
		"!", "videoconvert",
		"!", "pngenc",
		"!", "filesink", "location="+tmpFile,
	)
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+s.config.XDGRuntimeDir)

	if output, err := cmd.CombinedOutput(); err != nil {
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
