//go:build cgo

// Package desktop provides xkbcommon integration for keysym to keycode conversion.
// This enables proper keyboard input on Sway/wlroots compositors that don't support
// direct keysym injection like GNOME RemoteDesktop does.
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
	"os"
	"sync"
	"unsafe"
)

// KeysymMapping holds the evdev keycode and shift state for a keysym
type KeysymMapping struct {
	EvdevCode  int
	NeedsShift bool
}

var (
	xkbOnce       sync.Once
	keysymTable   map[uint32]KeysymMapping
	xkbInitErr    error
	xkbAvailable  bool
)

// initXKB initializes the xkbcommon keymap and builds the reverse lookup table.
// It reads the keymap from XKB_DEFAULT_* environment variables.
func initXKB() {
	xkbOnce.Do(func() {
		keysymTable = make(map[uint32]KeysymMapping)

		// Create xkb context
		ctx := C.xkb_context_new(C.XKB_CONTEXT_NO_FLAGS)
		if ctx == nil {
			xkbInitErr = nil // Not an error, just no xkbcommon
			return
		}
		defer C.xkb_context_unref(ctx)

		// Get keymap from environment variables (XKB_DEFAULT_RULES, XKB_DEFAULT_LAYOUT, etc.)
		// These are standard environment variables that Wayland compositors set
		var names C.struct_xkb_rule_names

		// Read from environment or use defaults
		if rules := os.Getenv("XKB_DEFAULT_RULES"); rules != "" {
			cRules := C.CString(rules)
			defer C.free(unsafe.Pointer(cRules))
			names.rules = cRules
		}
		if model := os.Getenv("XKB_DEFAULT_MODEL"); model != "" {
			cModel := C.CString(model)
			defer C.free(unsafe.Pointer(cModel))
			names.model = cModel
		}
		if layout := os.Getenv("XKB_DEFAULT_LAYOUT"); layout != "" {
			cLayout := C.CString(layout)
			defer C.free(unsafe.Pointer(cLayout))
			names.layout = cLayout
		}
		if variant := os.Getenv("XKB_DEFAULT_VARIANT"); variant != "" {
			cVariant := C.CString(variant)
			defer C.free(unsafe.Pointer(cVariant))
			names.variant = cVariant
		}
		if options := os.Getenv("XKB_DEFAULT_OPTIONS"); options != "" {
			cOptions := C.CString(options)
			defer C.free(unsafe.Pointer(cOptions))
			names.options = cOptions
		}

		// Create keymap from the rule names (or defaults if not set)
		keymap := C.xkb_keymap_new_from_names(ctx, &names, C.XKB_KEYMAP_COMPILE_NO_FLAGS)
		if keymap == nil {
			// Fall back to default US QWERTY
			keymap = C.xkb_keymap_new_from_names(ctx, nil, C.XKB_KEYMAP_COMPILE_NO_FLAGS)
			if keymap == nil {
				xkbInitErr = nil // Not an error, just no keymap
				return
			}
		}
		defer C.xkb_keymap_unref(keymap)

		// Build the reverse lookup table
		const maxEntries = 8192 // Should be enough for most keymaps
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
		// For duplicate keysyms, prefer the one without shift (level 0)
		for i := 0; i < int(count); i++ {
			ks := uint32(keysyms[i])
			kc := int(keycodes[i])
			shift := needsShift[i] != 0

			// Only add if not already present, or if this one doesn't need shift
			if existing, ok := keysymTable[ks]; !ok || (!shift && existing.NeedsShift) {
				keysymTable[ks] = KeysymMapping{
					EvdevCode:  kc,
					NeedsShift: shift,
				}
			}
		}

		xkbAvailable = true
	})
}

// XKBKeysymToEvdev converts an X11 keysym to a Linux evdev keycode using xkbcommon.
// This is layout-aware and works with non-QWERTY keyboards.
// Returns 0 if no mapping exists.
func XKBKeysymToEvdev(keysym uint32) int {
	initXKB()

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
	return xkbAvailable
}
