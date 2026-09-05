package desktop

import "testing"

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
