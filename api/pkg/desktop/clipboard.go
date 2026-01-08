package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/helixml/helix/api/pkg/types"
)

// ClipboardData is an alias for types.ClipboardData for package-internal use.
type ClipboardData = types.ClipboardData

// handleClipboard handles GET/POST /clipboard requests.
func (s *Server) handleClipboard(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetClipboard(w, r)
	case http.MethodPost:
		s.handleSetClipboard(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetClipboard(w http.ResponseWriter, r *http.Request) {
	data, err := s.getClipboard()
	if err != nil {
		s.logger.Error("get clipboard failed", "err", err)
		// Return empty clipboard instead of error
		data = &ClipboardData{Type: "text", Data: ""}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleSetClipboard(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var data ClipboardData
	if err := json.Unmarshal(body, &data); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.setClipboard(&data); err != nil {
		s.logger.Error("set clipboard failed", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// getClipboard reads from the clipboard.
func (s *Server) getClipboard() (*ClipboardData, error) {
	if isX11Mode() {
		return s.getClipboardX11()
	}
	// GNOME: Use D-Bus RemoteDesktop clipboard (no subprocess spawning)
	if isGNOMEEnvironment() && s.conn != nil && s.rdSessionPath != "" {
		return s.getClipboardGNOME()
	}
	// Sway/wlroots: Use wl-paste with timeout
	return s.getClipboardWayland()
}

// setClipboard writes to the clipboard.
func (s *Server) setClipboard(data *ClipboardData) error {
	if isX11Mode() {
		return s.setClipboardX11(data)
	}
	// GNOME: Use D-Bus RemoteDesktop clipboard (no subprocess spawning)
	if isGNOMEEnvironment() && s.conn != nil && s.rdSessionPath != "" {
		return s.setClipboardGNOME(data)
	}
	// Sway/wlroots: Use wl-copy with timeout
	return s.setClipboardWayland(data)
}

// GNOME D-Bus clipboard using RemoteDesktop session
// This avoids spawning wl-paste/wl-copy processes that show up as "Unknown" in GNOME panel

// Clipboard content pending for SelectionTransfer (protected by mutex)
var (
	pendingClipboardMu      sync.Mutex
	pendingClipboardContent []byte
	pendingClipboardMime    string
)

func (s *Server) getClipboardGNOME() (*ClipboardData, error) {
	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

	// First, enable clipboard if not already enabled
	enableOpts := map[string]dbus.Variant{}
	if err := rdSession.Call(remoteDesktopSessionIface+".EnableClipboard", 0, enableOpts).Err; err != nil {
		s.logger.Debug("EnableClipboard call", "err", err)
	}

	// Try to read text/plain first, then UTF-8 variants
	textMimeTypes := []string{"text/plain;charset=utf-8", "text/plain", "UTF8_STRING", "STRING"}

	for _, mimeType := range textMimeTypes {
		data, err := s.readSelectionGNOME(rdSession, mimeType)
		if err == nil && len(data) > 0 {
			s.logger.Debug("got clipboard via D-Bus", "mime", mimeType, "size", len(data))
			return &ClipboardData{Type: "text", Data: string(data)}, nil
		}
	}

	// Try image/png for image clipboard
	data, err := s.readSelectionGNOME(rdSession, "image/png")
	if err == nil && len(data) > 0 {
		s.logger.Debug("got image clipboard via D-Bus", "size", len(data))
		return &ClipboardData{Type: "image", Data: base64.StdEncoding.EncodeToString(data)}, nil
	}

	// Empty clipboard
	return &ClipboardData{Type: "text", Data: ""}, nil
}

func (s *Server) readSelectionGNOME(rdSession dbus.BusObject, mimeType string) ([]byte, error) {
	// SelectionRead(mime_type: s) -> (fd: h)
	call := rdSession.Call(remoteDesktopSessionIface+".SelectionRead", 0, mimeType)
	if call.Err != nil {
		return nil, call.Err
	}

	if len(call.Body) == 0 {
		return nil, fmt.Errorf("no fd returned")
	}

	fd, ok := call.Body[0].(dbus.UnixFD)
	if !ok {
		return nil, fmt.Errorf("invalid fd type")
	}

	file := os.NewFile(uintptr(fd), "clipboard-read")
	if file == nil {
		return nil, fmt.Errorf("failed to create file from fd")
	}
	defer file.Close()

	return io.ReadAll(file)
}

func (s *Server) setClipboardGNOME(data *ClipboardData) error {
	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

	// Enable clipboard if not already enabled
	enableOpts := map[string]dbus.Variant{}
	if err := rdSession.Call(remoteDesktopSessionIface+".EnableClipboard", 0, enableOpts).Err; err != nil {
		s.logger.Debug("EnableClipboard call", "err", err)
	}

	content, err := decodeClipboardData(data)
	if err != nil {
		return err
	}

	// Determine MIME type
	var mimeType string
	if data.Type == "image" {
		mimeType = "image/png"
	} else {
		mimeType = "text/plain;charset=utf-8"
	}

	// Store content for when SelectionTransfer signal arrives
	pendingClipboardMu.Lock()
	pendingClipboardContent = content
	pendingClipboardMime = mimeType
	pendingClipboardMu.Unlock()

	// SetSelection announces we have clipboard content
	// Format: SetSelection(options: a{sv}) where options contains "mime-types"
	mimeTypes := []string{mimeType}
	if data.Type == "text" {
		// Offer multiple text formats for compatibility
		mimeTypes = []string{"text/plain;charset=utf-8", "text/plain", "UTF8_STRING", "STRING"}
	}

	setOpts := map[string]dbus.Variant{
		"mime-types": dbus.MakeVariant(mimeTypes),
	}

	if err := rdSession.Call(remoteDesktopSessionIface+".SetSelection", 0, setOpts).Err; err != nil {
		return fmt.Errorf("SetSelection: %w", err)
	}

	s.logger.Info("clipboard announced via D-Bus", "type", data.Type, "size", len(content))

	// Start signal handler for SelectionTransfer if not already running
	s.startClipboardSignalHandler()

	return nil
}

// clipboardSignalHandlerStarted tracks if we've started the handler
var clipboardSignalHandlerStarted bool
var clipboardSignalHandlerMu sync.Mutex

func (s *Server) startClipboardSignalHandler() {
	clipboardSignalHandlerMu.Lock()
	defer clipboardSignalHandlerMu.Unlock()

	if clipboardSignalHandlerStarted {
		return
	}
	clipboardSignalHandlerStarted = true

	// Subscribe to SelectionTransfer signal
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(s.rdSessionPath),
		dbus.WithMatchInterface(remoteDesktopSessionIface),
		dbus.WithMatchMember("SelectionTransfer"),
	); err != nil {
		s.logger.Error("failed to subscribe to SelectionTransfer signal", "err", err)
		return
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)

	go func() {
		for sig := range signalChan {
			if sig.Name == remoteDesktopSessionIface+".SelectionTransfer" {
				s.handleSelectionTransfer(sig)
			}
		}
	}()

	s.logger.Info("clipboard signal handler started")
}

func (s *Server) handleSelectionTransfer(sig *dbus.Signal) {
	// SelectionTransfer signal: (mime_type: s, serial: u)
	if len(sig.Body) < 2 {
		s.logger.Warn("SelectionTransfer signal missing arguments")
		return
	}

	mimeType, ok := sig.Body[0].(string)
	if !ok {
		s.logger.Warn("SelectionTransfer: invalid mime_type")
		return
	}

	serial, ok := sig.Body[1].(uint32)
	if !ok {
		s.logger.Warn("SelectionTransfer: invalid serial")
		return
	}

	s.logger.Debug("SelectionTransfer received", "mime", mimeType, "serial", serial)

	// Get pending content
	pendingClipboardMu.Lock()
	content := pendingClipboardContent
	pendingClipboardMu.Unlock()

	if len(content) == 0 {
		s.logger.Warn("SelectionTransfer but no pending content")
		// Signal failure
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		rdSession.Call(remoteDesktopSessionIface+".SelectionWriteDone", 0, serial, false)
		return
	}

	// Call SelectionWrite to get fd for writing
	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	call := rdSession.Call(remoteDesktopSessionIface+".SelectionWrite", 0, serial)
	if call.Err != nil {
		s.logger.Error("SelectionWrite failed", "err", call.Err)
		return
	}

	if len(call.Body) == 0 {
		s.logger.Error("SelectionWrite returned no fd")
		rdSession.Call(remoteDesktopSessionIface+".SelectionWriteDone", 0, serial, false)
		return
	}

	fd, ok := call.Body[0].(dbus.UnixFD)
	if !ok {
		s.logger.Error("SelectionWrite returned invalid fd type")
		rdSession.Call(remoteDesktopSessionIface+".SelectionWriteDone", 0, serial, false)
		return
	}

	// Write content to fd
	file := os.NewFile(uintptr(fd), "clipboard-write")
	if file == nil {
		s.logger.Error("failed to create file from fd")
		rdSession.Call(remoteDesktopSessionIface+".SelectionWriteDone", 0, serial, false)
		return
	}

	_, writeErr := file.Write(content)
	file.Close()

	// Signal completion
	success := writeErr == nil
	if err := rdSession.Call(remoteDesktopSessionIface+".SelectionWriteDone", 0, serial, success).Err; err != nil {
		s.logger.Warn("SelectionWriteDone failed", "err", err)
	}

	if success {
		s.logger.Debug("clipboard content written via D-Bus", "size", len(content))
	} else {
		s.logger.Error("clipboard write failed", "err", writeErr)
	}
}

// Wayland clipboard using wl-paste/wl-copy (for Sway/wlroots)

func (s *Server) getClipboardWayland() (*ClipboardData, error) {
	waylandDisplay := getWaylandDisplay(s.config.XDGRuntimeDir)
	if waylandDisplay == "" {
		return &ClipboardData{Type: "text", Data: ""}, nil
	}

	env := append(os.Environ(),
		"WAYLAND_DISPLAY="+waylandDisplay,
		"XDG_RUNTIME_DIR="+s.config.XDGRuntimeDir,
	)

	// Try CLIPBOARD first, then PRIMARY
	for _, primary := range []bool{false, true} {
		clipType, data, ok := s.tryGetClipboardWayland(env, primary)
		if ok && len(data) > 0 {
			return &ClipboardData{Type: clipType, Data: encodeClipboardData(clipType, data)}, nil
		}
	}

	return &ClipboardData{Type: "text", Data: ""}, nil
}

// clipboardTimeout is the max time to wait for wl-paste/wl-copy commands.
// Short timeout prevents "Unknown" processes piling up in GNOME panel when
// clipboard owner is unresponsive or clipboard is empty.
const clipboardTimeout = 2 * time.Second

func (s *Server) tryGetClipboardWayland(env []string, primary bool) (string, []byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	args := []string{"--list-types"}
	if primary {
		args = []string{"--primary", "--list-types"}
	}

	cmd := exec.CommandContext(ctx, "wl-paste", args...)
	cmd.Env = env
	typesOutput, err := cmd.Output()
	if err != nil {
		return "", nil, false
	}

	mimeTypes := string(typesOutput)
	isImage := strings.Contains(mimeTypes, "image/png") || strings.Contains(mimeTypes, "image/jpeg")

	// Create new context for data fetch (previous may be near timeout)
	dataCtx, dataCancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer dataCancel()

	var dataCmd *exec.Cmd
	if isImage {
		if primary {
			dataCmd = exec.CommandContext(dataCtx, "wl-paste", "--primary", "-t", "image/png")
		} else {
			dataCmd = exec.CommandContext(dataCtx, "wl-paste", "-t", "image/png")
		}
	} else {
		if primary {
			dataCmd = exec.CommandContext(dataCtx, "wl-paste", "--primary", "--no-newline")
		} else {
			dataCmd = exec.CommandContext(dataCtx, "wl-paste", "--no-newline")
		}
	}

	dataCmd.Env = env
	data, err := dataCmd.Output()
	if err != nil {
		return "", nil, false
	}

	clipType := "text"
	if isImage {
		clipType = "image"
	}
	return clipType, data, true
}

func (s *Server) setClipboardWayland(data *ClipboardData) error {
	waylandDisplay := getWaylandDisplay(s.config.XDGRuntimeDir)
	if waylandDisplay == "" {
		return fmt.Errorf("no Wayland display")
	}

	env := append(os.Environ(),
		"WAYLAND_DISPLAY="+waylandDisplay,
		"XDG_RUNTIME_DIR="+s.config.XDGRuntimeDir,
	)

	content, err := decodeClipboardData(data)
	if err != nil {
		return err
	}

	// Set both CLIPBOARD and PRIMARY with timeout to prevent hanging
	for _, primary := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)

		var cmd *exec.Cmd
		if data.Type == "image" {
			if primary {
				cmd = exec.CommandContext(ctx, "wl-copy", "--primary", "-t", "image/png")
			} else {
				cmd = exec.CommandContext(ctx, "wl-copy", "-t", "image/png")
			}
		} else {
			if primary {
				cmd = exec.CommandContext(ctx, "wl-copy", "--primary")
			} else {
				cmd = exec.CommandContext(ctx, "wl-copy")
			}
		}

		cmd.Env = env
		cmd.Stdin = bytes.NewReader(content)
		err := cmd.Run()
		cancel()

		if err != nil && !primary {
			return fmt.Errorf("wl-copy: %w", err)
		}
	}

	s.logger.Info("clipboard set", "type", data.Type, "size", len(content))
	return nil
}

// X11 clipboard using xclip

func (s *Server) getClipboardX11() (*ClipboardData, error) {
	display := os.Getenv("DISPLAY")
	env := append(os.Environ(), "DISPLAY="+display)

	// Try CLIPBOARD first, then PRIMARY
	for _, selection := range []string{"clipboard", "primary"} {
		clipType, data, ok := s.tryGetClipboardX11(env, selection)
		if ok && len(data) > 0 {
			return &ClipboardData{Type: clipType, Data: encodeClipboardData(clipType, data)}, nil
		}
	}

	return &ClipboardData{Type: "text", Data: ""}, nil
}

func (s *Server) tryGetClipboardX11(env []string, selection string) (string, []byte, bool) {
	// Check MIME types
	cmd := exec.Command("xclip", "-selection", selection, "-t", "TARGETS", "-o")
	cmd.Env = env
	targetsOutput, err := cmd.Output()
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
	dataCmd.Env = env

	data, err := dataCmd.Output()
	if err != nil || len(data) == 0 {
		return "", nil, false
	}

	clipType := "text"
	if isImage {
		clipType = "image"
	}
	return clipType, data, true
}

func (s *Server) setClipboardX11(data *ClipboardData) error {
	display := os.Getenv("DISPLAY")
	env := append(os.Environ(), "DISPLAY="+display)

	content, err := decodeClipboardData(data)
	if err != nil {
		return err
	}

	// Set both CLIPBOARD and PRIMARY
	for _, selection := range []string{"clipboard", "primary"} {
		var cmd *exec.Cmd
		if data.Type == "image" {
			cmd = exec.Command("xclip", "-selection", selection, "-t", "image/png", "-i")
		} else {
			cmd = exec.Command("xclip", "-selection", selection, "-i")
		}

		cmd.Env = env
		cmd.Stdin = bytes.NewReader(content)
		if err := cmd.Run(); err != nil && selection == "clipboard" {
			return fmt.Errorf("xclip: %w", err)
		}
	}

	s.logger.Info("clipboard set via X11", "type", data.Type, "size", len(content))
	return nil
}

// Helper functions

func encodeClipboardData(clipType string, data []byte) string {
	if clipType == "image" {
		return base64.StdEncoding.EncodeToString(data)
	}
	return string(data)
}

func decodeClipboardData(data *ClipboardData) ([]byte, error) {
	if data.Type == "image" {
		return base64.StdEncoding.DecodeString(data.Data)
	}
	return []byte(data.Data), nil
}
