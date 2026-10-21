package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/helixml/helix/api/pkg/types"
	_ "golang.org/x/image/webp"
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

// gnomeTextMimeTypes are the text targets read from the remote clipboard, in
// order of preference. They are also offered when the bridge sets text.
var gnomeTextMimeTypes = []string{"text/plain;charset=utf-8", "text/plain", "UTF8_STRING", "STRING"}

// enableClipboardGNOME turns on clipboard integration for the current
// RemoteDesktop session and subscribes to its clipboard signals. Called every
// time a session starts, including after recreation, because both the clipboard
// enablement and the signal match are per session.
func (s *Server) enableClipboardGNOME() error {
	// Forget the previous session's owner first, so a failed enable can't leave
	// stale clipboard state behind for the new session.
	s.clipboardMu.Lock()
	s.clipboardOffer = nil
	s.clipboardSessionOwned = false
	s.clipboardMu.Unlock()

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	if err := rdSession.Call(remoteDesktopSessionIface+".EnableClipboard", 0, map[string]dbus.Variant{}).Err; err != nil {
		return fmt.Errorf("EnableClipboard: %w", err)
	}

	for _, member := range []string{"SelectionOwnerChanged", "SelectionTransfer"} {
		if err := s.conn.AddMatchSignal(
			dbus.WithMatchObjectPath(s.rdSessionPath),
			dbus.WithMatchInterface(remoteDesktopSessionIface),
			dbus.WithMatchMember(member),
		); err != nil {
			return fmt.Errorf("subscribe to %s: %w", member, err)
		}
	}

	s.clipboardSignalsOnce.Do(func() {
		signalChan := make(chan *dbus.Signal, 10)
		s.conn.Signal(signalChan)
		go func() {
			for sig := range signalChan {
				switch sig.Name {
				case remoteDesktopSessionIface + ".SelectionOwnerChanged":
					s.handleSelectionOwnerChanged(sig)
				case remoteDesktopSessionIface + ".SelectionTransfer":
					s.handleSelectionTransfer(sig)
				}
			}
		}()
	})

	s.logger.Info("clipboard enabled", "rd_session", s.rdSessionPath)
	return nil
}

// handleSelectionOwnerChanged records what the new clipboard owner offers.
// SelectionOwnerChanged signal: (options: a{sv}) with "mime-types" (as) and
// "session-is-owner" (b).
func (s *Server) handleSelectionOwnerChanged(sig *dbus.Signal) {
	if sig.Path != s.rdSessionPath || len(sig.Body) < 1 {
		return
	}
	options, ok := sig.Body[0].(map[string]dbus.Variant)
	if !ok {
		s.logger.Warn("SelectionOwnerChanged: invalid options")
		return
	}

	var mimeTypes []string
	if v, ok := options["mime-types"]; ok {
		if err := v.Store(&mimeTypes); err != nil {
			s.logger.Warn("SelectionOwnerChanged: invalid mime-types", "err", err)
			return
		}
	}
	var sessionOwned bool
	if v, ok := options["session-is-owner"]; ok {
		_ = v.Store(&sessionOwned)
	}

	s.clipboardMu.Lock()
	s.clipboardOffer = mimeTypes
	s.clipboardSessionOwned = sessionOwned
	s.clipboardMu.Unlock()

	s.logger.Debug("clipboard owner changed", "mime_types", mimeTypes, "session_is_owner", sessionOwned)
}

func (s *Server) getClipboardGNOME() (*ClipboardData, error) {
	s.clipboardMu.Lock()
	offer := s.clipboardOffer
	sessionOwned := s.clipboardSessionOwned
	s.clipboardMu.Unlock()

	// The bridge set this selection itself; hand back what it holds rather than
	// asking Mutter to transfer it from us to ourselves.
	if sessionOwned {
		pendingClipboardMu.Lock()
		content, mime := pendingClipboardContent, pendingClipboardMime
		pendingClipboardMu.Unlock()
		if mime == "image/png" {
			return &ClipboardData{Type: "image", Data: base64.StdEncoding.EncodeToString(content)}, nil
		}
		return &ClipboardData{Type: "text", Data: string(content)}, nil
	}

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	read := func(mimeType string) ([]byte, error) { return s.readSelectionGNOME(rdSession, mimeType) }
	return readClipboardOffer(offer, read)
}

// readClipboardOffer picks what to read from a clipboard owner that advertises
// the given MIME types. Only advertised types are requested: Mutter forwards
// requests for any type to the owner, and owners answer unadvertised types
// inconsistently (some return nothing, wl-copy returns its image bytes as
// "text"). An image wins over text, matching the Sway path, because image
// sources also advertise text (Firefox's "Copy Image" adds the URL). A single
// local image file (copied in Loupe or Files) is returned as the image itself.
func readClipboardOffer(offer []string, read func(mimeType string) ([]byte, error)) (*ClipboardData, error) {
	if slices.Contains(offer, "image/png") {
		data, err := read("image/png")
		if err != nil {
			return nil, fmt.Errorf("read image/png: %w", err)
		}
		if len(data) > 0 {
			return &ClipboardData{Type: "image", Data: base64.StdEncoding.EncodeToString(data)}, nil
		}
	}

	if slices.Contains(offer, "text/uri-list") {
		data, err := read("text/uri-list")
		if err != nil {
			return nil, fmt.Errorf("read text/uri-list: %w", err)
		}
		if png, ok := imageFileFromURIList(data); ok {
			return &ClipboardData{Type: "image", Data: base64.StdEncoding.EncodeToString(png)}, nil
		}
	}

	for _, mimeType := range gnomeTextMimeTypes {
		if !slices.Contains(offer, mimeType) {
			continue
		}
		data, err := read(mimeType)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", mimeType, err)
		}
		if len(data) > 0 {
			return &ClipboardData{Type: "text", Data: string(data)}, nil
		}
	}

	return &ClipboardData{Type: "text", Data: ""}, nil
}

// maxClipboardImageFileBytes bounds how large an image file copied in the
// desktop (Loupe, Files) is read into the clipboard.
const maxClipboardImageFileBytes = 32 << 20

// imageFileFromURIList returns the PNG encoding of the image when a
// text/uri-list names exactly one local image file. Image viewers and file
// managers copy a file reference rather than pixels, which otherwise reaches
// the browser as the file's path.
func imageFileFromURIList(data []byte) ([]byte, bool) {
	var uris []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			uris = append(uris, line)
		}
	}
	if len(uris) != 1 {
		return nil, false
	}

	u, err := url.Parse(uris[0])
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return nil, false
	}
	info, err := os.Stat(u.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxClipboardImageFileBytes {
		return nil, false
	}
	raw, err := os.ReadFile(u.Path)
	if err != nil {
		return nil, false
	}

	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	if format == "png" {
		return raw, true
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
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
		mimeTypes = gnomeTextMimeTypes
	}

	setOpts := map[string]dbus.Variant{
		"mime-types": dbus.MakeVariant(mimeTypes),
	}

	if err := rdSession.Call(remoteDesktopSessionIface+".SetSelection", 0, setOpts).Err; err != nil {
		return fmt.Errorf("SetSelection: %w", err)
	}

	s.logger.Info("clipboard announced via D-Bus", "type", data.Type, "size", len(content))
	return nil
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
