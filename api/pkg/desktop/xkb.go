//go:build cgo

// Package desktop provides xkbcommon integration for keysym to keycode conversion.
// This enables proper keyboard input on Sway/wlroots compositors that don't support
// direct keysym injection like GNOME RemoteDesktop does.
//
// The keymap is dynamically updated when Sway's keyboard layout changes.
package desktop

/*
#cgo pkg-config: xkbcommon
#include <xkbcommon/xkbcommon.h>
#include <stdlib.h>
#include <string.h>

// Build a reverse lookup table: keysym -> (keycode, level, needs_shift)
// Returns the number of entries written to the output arrays.
static int build_keysym_to_keycode_table(
    struct xkb_keymap *keymap,
    uint32_t *keysyms_out,
    uint32_t *keycodes_out,
    uint8_t *needs_shift_out,
    int max_entries
) {
    int count = 0;
    xkb_keycode_t min_key = xkb_keymap_min_keycode(keymap);
    xkb_keycode_t max_key = xkb_keymap_max_keycode(keymap);

    for (xkb_keycode_t keycode = min_key; keycode <= max_key && count < max_entries; keycode++) {
        xkb_layout_index_t num_layouts = xkb_keymap_num_layouts_for_key(keymap, keycode);

        for (xkb_layout_index_t layout = 0; layout < num_layouts && count < max_entries; layout++) {
            xkb_level_index_t num_levels = xkb_keymap_num_levels_for_key(keymap, keycode, layout);

            for (xkb_level_index_t level = 0; level < num_levels && count < max_entries; level++) {
                const xkb_keysym_t *syms;
                int num_syms = xkb_keymap_key_get_syms_by_level(keymap, keycode, layout, level, &syms);

                for (int i = 0; i < num_syms && count < max_entries; i++) {
                    keysyms_out[count] = syms[i];
                    // XKB keycodes are evdev + 8, so subtract 8 to get evdev
                    keycodes_out[count] = keycode - 8;
                    // Level 1 typically means Shift is needed (level 0 = no modifiers)
                    needs_shift_out[count] = (level >= 1) ? 1 : 0;
                    count++;
                }
            }
        }
    }

    return count;
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// KeysymMapping holds the evdev keycode and shift state for a keysym
type KeysymMapping struct {
	EvdevCode  int
	NeedsShift bool
}

var (
	xkbMu           sync.RWMutex
	keysymTable     map[uint32]KeysymMapping
	xkbAvailable    bool
	xkbInitialized  bool
	currentLayout   string // Track current layout to detect changes
	layoutPollStop  chan struct{}
	layoutPollOnce  sync.Once
)

// swayInput represents a Sway input device from swaymsg -t get_inputs
type swayInput struct {
	Identifier          string `json:"identifier"`
	Name                string `json:"name"`
	Type                string `json:"type"`
	XkbActiveLayoutName string `json:"xkb_active_layout_name"`
	XkbLayoutNames      []string `json:"xkb_layout_names"`
}

// getSwayKeyboardLayout queries Sway for the current keyboard layout
func getSwayKeyboardLayout() string {
	// Check if we're running under Sway
	if os.Getenv("SWAYSOCK") == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "swaymsg", "-t", "get_inputs")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	var inputs []swayInput
	if err := json.Unmarshal(output, &inputs); err != nil {
		return ""
	}

	// Find the first keyboard and return its active layout
	for _, input := range inputs {
		if input.Type == "keyboard" && input.XkbActiveLayoutName != "" {
			return input.XkbActiveLayoutName
		}
	}

	return ""
}

// layoutNameToXkbLayout converts Sway's layout name to xkbcommon layout string
// Sway reports names like "English (US)", "German", etc.
// We need to convert to xkb layout codes like "us", "de", etc.
func layoutNameToXkbLayout(name string) string {
	name = strings.ToLower(name)

	// Common mappings
	switch {
	case strings.Contains(name, "english") && strings.Contains(name, "us"):
		return "us"
	case strings.Contains(name, "english") && strings.Contains(name, "uk"):
		return "gb"
	case strings.Contains(name, "german"):
		return "de"
	case strings.Contains(name, "french"):
		return "fr"
	case strings.Contains(name, "spanish"):
		return "es"
	case strings.Contains(name, "italian"):
		return "it"
	case strings.Contains(name, "portuguese"):
		return "pt"
	case strings.Contains(name, "russian"):
		return "ru"
	case strings.Contains(name, "japanese"):
		return "jp"
	case strings.Contains(name, "korean"):
		return "kr"
	case strings.Contains(name, "chinese"):
		return "cn"
	case strings.Contains(name, "arabic"):
		return "ara"
	case strings.Contains(name, "hebrew"):
		return "il"
	case strings.Contains(name, "polish"):
		return "pl"
	case strings.Contains(name, "dutch"):
		return "nl"
	case strings.Contains(name, "swedish"):
		return "se"
	case strings.Contains(name, "norwegian"):
		return "no"
	case strings.Contains(name, "danish"):
		return "dk"
	case strings.Contains(name, "finnish"):
		return "fi"
	case strings.Contains(name, "czech"):
		return "cz"
	case strings.Contains(name, "hungarian"):
		return "hu"
	case strings.Contains(name, "greek"):
		return "gr"
	case strings.Contains(name, "turkish"):
		return "tr"
	case strings.Contains(name, "dvorak"):
		return "us(dvorak)"
	case strings.Contains(name, "colemak"):
		return "us(colemak)"
	default:
		// Try to use the first word as the layout code
		parts := strings.Fields(name)
		if len(parts) > 0 {
			return parts[0]
		}
		return "us" // Default fallback
	}
}

// buildKeymapForLayout builds the keysym table for a specific layout
func buildKeymapForLayout(layout string) (map[uint32]KeysymMapping, bool) {
	table := make(map[uint32]KeysymMapping)

	// Create xkb context
	ctx := C.xkb_context_new(C.XKB_CONTEXT_NO_FLAGS)
	if ctx == nil {
		return nil, false
	}
	defer C.xkb_context_unref(ctx)

	// Set up rule names with the specified layout
	var names C.struct_xkb_rule_names

	if layout != "" {
		cLayout := C.CString(layout)
		defer C.free(unsafe.Pointer(cLayout))
		names.layout = cLayout
	}

	// Create keymap
	keymap := C.xkb_keymap_new_from_names(ctx, &names, C.XKB_KEYMAP_COMPILE_NO_FLAGS)
	if keymap == nil {
		// Fall back to default
		keymap = C.xkb_keymap_new_from_names(ctx, nil, C.XKB_KEYMAP_COMPILE_NO_FLAGS)
		if keymap == nil {
			return nil, false
		}
	}
	defer C.xkb_keymap_unref(keymap)

	// Build the reverse lookup table
	const maxEntries = 8192
	keysyms := make([]C.uint32_t, maxEntries)
	keycodes := make([]C.uint32_t, maxEntries)
	needsShift := make([]C.uint8_t, maxEntries)

	count := C.build_keysym_to_keycode_table(
		keymap,
		(*C.uint32_t)(unsafe.Pointer(&keysyms[0])),
		(*C.uint32_t)(unsafe.Pointer(&keycodes[0])),
		(*C.uint8_t)(unsafe.Pointer(&needsShift[0])),
		C.int(maxEntries),
	)

	// Populate the Go map
	for i := 0; i < int(count); i++ {
		ks := uint32(keysyms[i])
		kc := int(keycodes[i])
		shift := needsShift[i] != 0

		// Only add if not already present, or if this one doesn't need shift
		if existing, ok := table[ks]; !ok || (!shift && existing.NeedsShift) {
			table[ks] = KeysymMapping{
				EvdevCode:  kc,
				NeedsShift: shift,
			}
		}
	}

	return table, true
}

// initXKB initializes the xkbcommon keymap.
func initXKB() {
	xkbMu.Lock()
	defer xkbMu.Unlock()

	if xkbInitialized {
		return
	}

	// Check if we're on Sway
	isSway := os.Getenv("SWAYSOCK") != ""

	var layout string
	if isSway {
		// Get layout from Sway
		swayLayout := getSwayKeyboardLayout()
		if swayLayout != "" {
			layout = layoutNameToXkbLayout(swayLayout)
			currentLayout = swayLayout
			slog.Debug("detected Sway keyboard layout", "sway_name", swayLayout, "xkb_layout", layout)
		}
	} else {
		// Use environment variables for non-Sway (GNOME uses keysyms directly anyway)
		layout = os.Getenv("XKB_DEFAULT_LAYOUT")
	}

	table, ok := buildKeymapForLayout(layout)
	if ok {
		keysymTable = table
		xkbAvailable = true
		slog.Debug("built XKB keymap", "layout", layout, "entries", len(table))
	}

	xkbInitialized = true

	// Start layout polling for Sway
	if isSway {
		startLayoutPolling()
	}
}

// startLayoutPolling starts a background goroutine that polls for layout changes
func startLayoutPolling() {
	layoutPollOnce.Do(func() {
		layoutPollStop = make(chan struct{})
		go pollLayoutChanges()
	})
}

// pollLayoutChanges periodically checks if Sway's keyboard layout has changed
func pollLayoutChanges() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-layoutPollStop:
			return
		case <-ticker.C:
			newLayout := getSwayKeyboardLayout()
			if newLayout == "" {
				continue
			}

			xkbMu.RLock()
			changed := newLayout != currentLayout
			xkbMu.RUnlock()

			if changed {
				xkbLayout := layoutNameToXkbLayout(newLayout)
				slog.Info("Sway keyboard layout changed, rebuilding keymap",
					"old", currentLayout, "new", newLayout, "xkb_layout", xkbLayout)

				table, ok := buildKeymapForLayout(xkbLayout)
				if ok {
					xkbMu.Lock()
					keysymTable = table
					currentLayout = newLayout
					xkbMu.Unlock()
					slog.Debug("rebuilt XKB keymap", "layout", xkbLayout, "entries", len(table))
				}
			}
		}
	}
}

// StopXKBPolling stops the layout polling goroutine
func StopXKBPolling() {
	if layoutPollStop != nil {
		close(layoutPollStop)
	}
}

// XKBKeysymToEvdev converts an X11 keysym to a Linux evdev keycode using xkbcommon.
// This is layout-aware and works with non-QWERTY keyboards.
// Returns 0 if no mapping exists.
func XKBKeysymToEvdev(keysym uint32) int {
	initXKB()

	xkbMu.RLock()
	defer xkbMu.RUnlock()

	if !xkbAvailable {
		return 0
	}

	if mapping, ok := keysymTable[keysym]; ok {
		return mapping.EvdevCode
	}

	return 0
}

// XKBKeysymNeedsShift returns true if the keysym requires Shift to be pressed.
// This is useful for sending the correct modifier state.
func XKBKeysymNeedsShift(keysym uint32) bool {
	initXKB()

	xkbMu.RLock()
	defer xkbMu.RUnlock()

	if !xkbAvailable {
		return false
	}

	if mapping, ok := keysymTable[keysym]; ok {
		return mapping.NeedsShift
	}

	return false
}

// IsXKBAvailable returns true if xkbcommon was successfully initialized.
func IsXKBAvailable() bool {
	initXKB()
	xkbMu.RLock()
	defer xkbMu.RUnlock()
	return xkbAvailable
}

// GetCurrentLayout returns the current keyboard layout being used
func GetCurrentLayout() string {
	initXKB()
	xkbMu.RLock()
	defer xkbMu.RUnlock()
	return currentLayout
}
