package desktop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// KeyboardState represents the current keyboard state.
type KeyboardState struct {
	Timestamp     int64         `json:"timestamp"`
	PressedKeys   []int         `json:"pressed_keys"`
	KeyNames      []string      `json:"key_names"`
	ModifierState ModifierState `json:"modifier_state"`
	DeviceName    string        `json:"device_name"`
	DevicePath    string        `json:"device_path"`
}

// ModifierState represents the state of modifier keys.
type ModifierState struct {
	Shift bool `json:"shift"`
	Ctrl  bool `json:"ctrl"`
	Alt   bool `json:"alt"`
	Meta  bool `json:"meta"`
}

// Linux input key codes for modifiers.
const (
	keyLeftCtrl   = 29
	keyLeftShift  = 42
	keyLeftAlt    = 56
	keyLeftMeta   = 125
	keyRightCtrl  = 97
	keyRightShift = 54
	keyRightAlt   = 100
	keyRightMeta  = 126
)

// keyCodeNames maps Linux key codes to human-readable names.
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

// handleKeyboardState returns the current keyboard state.
func (s *Server) handleKeyboardState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := KeyboardState{
		Timestamp: time.Now().UnixMilli(),
	}

	device, name := s.findWolfKeyboard()
	if device == "" {
		s.logger.Debug("Wolf virtual keyboard not found")
		state.DeviceName = "Not found"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
		return
	}

	state.DevicePath = device
	state.DeviceName = name

	// Query all common keys to find which are pressed
	for keyCode := range keyCodeNames {
		cmd := exec.Command("evtest", "--query", device, "EV_KEY", fmt.Sprintf("%d", keyCode))
		if cmd.Run() == nil {
			state.PressedKeys = append(state.PressedKeys, keyCode)
			state.KeyNames = append(state.KeyNames, keyCodeNames[keyCode])
		}
	}

	// Check modifier state
	for _, keyCode := range state.PressedKeys {
		switch keyCode {
		case keyLeftShift, keyRightShift:
			state.ModifierState.Shift = true
		case keyLeftCtrl, keyRightCtrl:
			state.ModifierState.Ctrl = true
		case keyLeftAlt, keyRightAlt:
			state.ModifierState.Alt = true
		case keyLeftMeta, keyRightMeta:
			state.ModifierState.Meta = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)

	s.logger.Debug("keyboard state queried",
		"pressed", len(state.PressedKeys),
		"shift", state.ModifierState.Shift,
		"ctrl", state.ModifierState.Ctrl,
	)
}

// handleKeyboardReset releases all stuck modifier keys.
func (s *Server) handleKeyboardReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	device, _ := s.findWolfKeyboard()
	if device == "" {
		http.Error(w, "Wolf virtual keyboard not found", http.StatusNotFound)
		return
	}

	modifierKeys := []int{
		keyLeftCtrl, keyRightCtrl,
		keyLeftShift, keyRightShift,
		keyLeftAlt, keyRightAlt,
		keyLeftMeta, keyRightMeta,
	}

	var releasedKeys []string
	for _, keyCode := range modifierKeys {
		// Check if key is pressed
		checkCmd := exec.Command("evtest", "--query", device, "EV_KEY", fmt.Sprintf("%d", keyCode))
		if checkCmd.Run() == nil {
			// Key is pressed, release it
			releaseCmd := exec.Command("evemu-event", device,
				"--type", "EV_KEY", "--code", fmt.Sprintf("%d", keyCode), "--value", "0", "--sync")
			if err := releaseCmd.Run(); err != nil {
				s.logger.Warn("failed to release key", "keycode", keyCode, "err", err)
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

	s.logger.Info("keyboard reset", "released", releasedKeys)
}

// findWolfKeyboard finds Wolf's virtual keyboard device.
func (s *Server) findWolfKeyboard() (devicePath, name string) {
	entries, err := os.ReadDir("/dev/input")
	if err != nil {
		return "", ""
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}

		devPath := filepath.Join("/dev/input", entry.Name())
		sysPath := fmt.Sprintf("/sys/class/input/%s/device/name", entry.Name())

		nameBytes, err := os.ReadFile(sysPath)
		if err != nil {
			continue
		}

		devName := strings.TrimSpace(string(nameBytes))
		if strings.Contains(strings.ToLower(devName), "wolf") &&
			strings.Contains(strings.ToLower(devName), "keyboard") {
			return devPath, devName
		}
	}

	return "", ""
}
