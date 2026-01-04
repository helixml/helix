package desktop

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// ClipboardData represents clipboard content.
type ClipboardData struct {
	Type string `json:"type"` // "text" or "image"
	Data string `json:"data"` // text or base64-encoded image
}

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
	return s.getClipboardWayland()
}

// setClipboard writes to the clipboard.
func (s *Server) setClipboard(data *ClipboardData) error {
	if isX11Mode() {
		return s.setClipboardX11(data)
	}
	return s.setClipboardWayland(data)
}

// Wayland clipboard using wl-paste/wl-copy

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

func (s *Server) tryGetClipboardWayland(env []string, primary bool) (string, []byte, bool) {
	args := []string{"--list-types"}
	if primary {
		args = []string{"--primary", "--list-types"}
	}

	cmd := exec.Command("wl-paste", args...)
	cmd.Env = env
	typesOutput, err := cmd.Output()
	if err != nil {
		return "", nil, false
	}

	mimeTypes := string(typesOutput)
	isImage := strings.Contains(mimeTypes, "image/png") || strings.Contains(mimeTypes, "image/jpeg")

	var dataCmd *exec.Cmd
	if isImage {
		if primary {
			dataCmd = exec.Command("wl-paste", "--primary", "-t", "image/png")
		} else {
			dataCmd = exec.Command("wl-paste", "-t", "image/png")
		}
	} else {
		if primary {
			dataCmd = exec.Command("wl-paste", "--primary", "--no-newline")
		} else {
			dataCmd = exec.Command("wl-paste", "--no-newline")
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

	// Set both CLIPBOARD and PRIMARY
	for _, primary := range []bool{false, true} {
		var cmd *exec.Cmd
		if data.Type == "image" {
			if primary {
				cmd = exec.Command("wl-copy", "--primary", "-t", "image/png")
			} else {
				cmd = exec.Command("wl-copy", "-t", "image/png")
			}
		} else {
			if primary {
				cmd = exec.Command("wl-copy", "--primary")
			} else {
				cmd = exec.Command("wl-copy")
			}
		}

		cmd.Env = env
		cmd.Stdin = bytes.NewReader(content)
		if err := cmd.Run(); err != nil && !primary {
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
