package desktop

import (
	"io"
	"log/slog"
	"testing"
)

// The remote caps lock follows the caps-lock bit of whoever presses a key: a
// toggle is injected only when the bit disagrees with the tracked remote state.
func TestReconcileCapsLock(t *testing.T) {
	tests := []struct {
		name       string
		remote     bool
		bit        bool
		wantToggle bool
	}{
		{"remote off, sender off", false, false, false},
		{"remote off, sender on", false, true, true},
		{"remote on, sender on", true, true, false},
		{"remote on, sender off", true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &sharedKeyboard{capsLocked: tt.remote}
			if got := k.reconcileCapsLock(tt.bit); got != tt.wantToggle {
				t.Errorf("toggle = %v, want %v", got, tt.wantToggle)
			}
			if k.capsLocked != tt.bit {
				t.Errorf("tracked caps lock = %v, want %v", k.capsLocked, tt.bit)
			}
		})
	}
}

// Two viewers with different local caps state, typing alternately, each get a
// toggle only when the lock last followed the other viewer.
func TestReconcileCapsLockAlternatingViewers(t *testing.T) {
	k := &sharedKeyboard{}
	steps := []struct {
		capsOn     bool
		wantToggle bool
	}{
		{true, true},   // Abi (caps on) types: lock turns on
		{true, false},  // Abi again: already on
		{false, true},  // Luke (caps off) types: lock turns off
		{false, false}, // Luke again
		{true, true},   // Abi again: back on
	}
	for i, st := range steps {
		if got := k.reconcileCapsLock(st.capsOn); got != st.wantToggle {
			t.Fatalf("step %d: toggle = %v, want %v", i, got, st.wantToggle)
		}
	}
}

func TestSharedKeyboardHeldKeys(t *testing.T) {
	t.Run("auto-repeat presses are not forwarded", func(t *testing.T) {
		k := &sharedKeyboard{}
		a := newKeyboardClient()
		if forward, repeat := k.press(a, KEY_LEFTCTRL); !forward || repeat {
			t.Fatal("first press must be forwarded and is not a repeat")
		}
		if forward, repeat := k.press(a, KEY_LEFTCTRL); forward || !repeat {
			t.Error("second press is a repeat and must not be forwarded")
		}
		if !k.release(a, KEY_LEFTCTRL) {
			t.Error("one release must release a key pressed once plus repeats")
		}
	})

	t.Run("release of a key never pressed is dropped", func(t *testing.T) {
		k := &sharedKeyboard{}
		a := newKeyboardClient()
		if k.release(a, KEY_LEFTCTRL) {
			t.Error("stray release must not be forwarded")
		}
	})

	t.Run("one viewer releasing does not release another viewer's key", func(t *testing.T) {
		k := &sharedKeyboard{}
		a, b := newKeyboardClient(), newKeyboardClient()
		if forward, _ := k.press(a, KEY_LEFTCTRL); !forward {
			t.Fatal("a's press must be forwarded")
		}
		if forward, repeat := k.press(b, KEY_LEFTCTRL); forward || repeat {
			t.Error("b's press of a key already down must not be forwarded")
		}
		if k.release(a, KEY_LEFTCTRL) {
			t.Error("a's release must not release Ctrl while b holds it")
		}
		if got := k.heldModifiers(); got != ModifierCtrl {
			t.Errorf("held modifiers = %d, want Ctrl", got)
		}
		if !k.release(b, KEY_LEFTCTRL) {
			t.Error("last holder's release must be forwarded")
		}
		if got := k.heldModifiers(); got != 0 {
			t.Errorf("held modifiers = %d, want none", got)
		}
	})

	t.Run("a stray release from one viewer cannot release another's key", func(t *testing.T) {
		k := &sharedKeyboard{}
		a, b := newKeyboardClient(), newKeyboardClient()
		k.press(a, KEY_LEFTSHIFT)
		if k.release(b, KEY_LEFTSHIFT) {
			t.Error("b never pressed Shift; its release must be dropped")
		}
		if !a.held[KEY_LEFTSHIFT] {
			t.Error("a must still hold Shift")
		}
	})
}

// A viewer that disconnects with keys held releases them, except keys another
// viewer still holds.
func TestReleaseKeyboardClientOnDisconnect(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	a, b := newKeyboardClient(), newKeyboardClient()

	s.keyboard.mu.Lock()
	s.keyboard.press(a, KEY_LEFTCTRL)
	s.keyboard.press(a, 30) // KEY_A
	s.keyboard.press(b, 30)
	s.keyboard.mu.Unlock()

	s.releaseKeyboardClient(a)

	if len(a.held) != 0 {
		t.Errorf("a still holds %v after disconnect", a.held)
	}
	if s.keyboard.holders[KEY_LEFTCTRL] != 0 {
		t.Error("Ctrl, held only by a, must be released")
	}
	if s.keyboard.holders[30] != 1 {
		t.Errorf("KEY_A holders = %d, want 1 (b still holds it)", s.keyboard.holders[30])
	}
}
