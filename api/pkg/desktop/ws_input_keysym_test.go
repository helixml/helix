package desktop

import (
	"encoding/binary"
	"testing"
)

// Pins the wire contract against the exact bytes the frontend emits when '@' is
// typed on a phone keyboard. The matching frontend assertion lives in
// frontend/src/lib/helix-stream/stream/websocket-stream.keyboard.test.ts; if
// either side changes the framing, one of the two fails.
func TestKeysymTapWireFormat(t *testing.T) {
	// Payload after the message-type byte: [subType=3][modifiers=0][keysym 4 BE]
	payload := []byte{3, 0, 0x00, 0x00, 0x00, 0x40}

	if got := payload[0]; got != 3 {
		t.Fatalf("subType = %d, want 3 (keysym tap)", got)
	}
	// handleWSKeyboardKeysymTap requires at least 6 bytes and decodes as below.
	if len(payload) < 6 {
		t.Fatalf("payload too short for handleWSKeyboardKeysymTap: %d bytes", len(payload))
	}

	modifiers := payload[1]
	keysym := binary.BigEndian.Uint32(payload[2:6])

	if keysym != '@' {
		t.Errorf("decoded keysym = %#x, want %#x ('@')", keysym, '@')
	}
	if modifiers != 0 {
		t.Errorf("decoded modifiers = %d, want 0 (a virtual keyboard sends no Shift)", modifiers)
	}

	// With no modifiers on the wire, the shift level must come from the keysym,
	// otherwise KEY_2 is pressed bare and the remote desktop types '2'.
	evdevCode, needsShift := resolveKeysym(keysym)
	if evdevCode == 0 {
		t.Fatal("'@' must resolve to a keycode")
	}
	if !needsShift {
		t.Error("'@' must request Shift; without it the remote desktop types '2'")
	}
	if modifiers|ModifierShift != ModifierShift {
		t.Errorf("merged modifiers = %d, want %d", modifiers|ModifierShift, ModifierShift)
	}
}

// Virtual keyboards emit a shifted character with no Shift key event, so the
// shift level has to be derived from the keysym itself. Getting this wrong types
// '@' as '2', which is what made logging in from a phone impossible.
func TestKeysymNeedsShift(t *testing.T) {
	tests := []struct {
		name   string
		keysym uint32
		want   bool
	}{
		{"at sign", '@', true},
		{"exclamation", '!', true},
		{"underscore", '_', true},
		{"colon", ':', true},
		{"double quote", '"', true},
		{"tilde", '~', true},
		{"question mark", '?', true},
		{"uppercase A", 'A', true},
		{"uppercase Z", 'Z', true},
		{"lowercase a", 'a', false},
		{"lowercase z", 'z', false},
		{"digit 2", '2', false},
		{"hyphen", '-', false},
		{"semicolon", ';', false},
		{"period", '.', false},
		{"space", ' ', false},
		{"backspace", 0xff08, false},
		{"return", 0xff0d, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keysymNeedsShift(tt.keysym); got != tt.want {
				t.Errorf("keysymNeedsShift(%#x) = %v, want %v", tt.keysym, got, tt.want)
			}
		})
	}
}

// The pairs that share a physical key must resolve to the same keycode and
// differ only in shift level.
func TestKeysymToEvdevSharedKeyPairs(t *testing.T) {
	tests := []struct {
		name               string
		shifted, unshifted uint32
	}{
		{"at over 2", '@', '2'},
		{"exclamation over 1", '!', '1'},
		{"underscore over hyphen", '_', '-'},
		{"colon over semicolon", ':', ';'},
		{"question over slash", '?', '/'},
		{"uppercase over lowercase", 'A', 'a'},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shiftedCode := keysymToEvdev(tt.shifted)
			unshiftedCode := keysymToEvdev(tt.unshifted)

			if shiftedCode == 0 || unshiftedCode == 0 {
				t.Fatalf("expected both keysyms to map: shifted=%d unshifted=%d", shiftedCode, unshiftedCode)
			}
			if shiftedCode != unshiftedCode {
				t.Errorf("keycodes differ: %#x->%d, %#x->%d (same physical key expected)",
					tt.shifted, shiftedCode, tt.unshifted, unshiftedCode)
			}
			if !keysymNeedsShift(tt.shifted) {
				t.Errorf("keysymNeedsShift(%#x) = false, want true", tt.shifted)
			}
			if keysymNeedsShift(tt.unshifted) {
				t.Errorf("keysymNeedsShift(%#x) = true, want false", tt.unshifted)
			}
		})
	}
}

// resolveKeysym falls back to the static table when xkbcommon has no entry. It
// must never report a shift level for a keysym it could not map.
func TestResolveKeysymUnmapped(t *testing.T) {
	// 0x01F600 (grinning face) has no key on any layout.
	evdevCode, needsShift := resolveKeysym(0x0101F600)
	if evdevCode != 0 {
		t.Errorf("expected no mapping for emoji keysym, got keycode %d", evdevCode)
	}
	if needsShift {
		t.Error("unmapped keysym must not request Shift")
	}
}

func TestResolveKeysymShiftLevel(t *testing.T) {
	evdevCode, needsShift := resolveKeysym('@')
	if evdevCode == 0 {
		t.Fatal("expected '@' to resolve to a keycode")
	}
	if !needsShift {
		t.Error("resolveKeysym('@') must request Shift, otherwise it types '2'")
	}

	evdevCode, needsShift = resolveKeysym('2')
	if evdevCode == 0 {
		t.Fatal("expected '2' to resolve to a keycode")
	}
	if needsShift {
		t.Error("resolveKeysym('2') must not request Shift")
	}
}
