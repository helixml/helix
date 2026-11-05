package desktop

import "sync"

// KEY_CAPSLOCK is the evdev keycode for Caps Lock.
const KEY_CAPSLOCK = 58

// sharedKeyboard is desktop-bridge's model of the one virtual keyboard that
// every viewer of this desktop types into. Mutter dedupes key presses per
// virtual device and per seat, so the compositor sees a key as simply "down" or
// "up" no matter how many viewers pressed it. Desktop-bridge is the only thing
// that injects keys into this compositor (there is no physical keyboard), so the
// state tracked here matches the compositor's.
//
// Policy for keys held by several viewers at once: a key goes down remotely
// when the first viewer presses it and up when the last viewer holding it
// releases it or disconnects. One viewer letting go of Ctrl therefore never
// releases it under another viewer who is still holding it.
//
// Caps Lock is state, not edges. Clients never forward the CapsLock key; every
// key press carries the sender's caps-lock state, and desktop-bridge taps
// KEY_CAPSLOCK first when the remote lock disagrees. The remote lock follows
// whoever is typing. See design/2026-10-11-shared-desktop-stuck-keys.md.
type sharedKeyboard struct {
	mu         sync.Mutex
	capsLocked bool
	// holders counts the connections holding each evdev key down.
	holders map[int]int
}

// keyboardClient is the keyboard state of one input connection (one viewer).
type keyboardClient struct {
	held        map[int]bool    // evdev keycodes this connection holds down
	heldKeysyms map[uint32]bool // keysyms this connection holds down (subType 2)
}

func newKeyboardClient() *keyboardClient {
	return &keyboardClient{held: map[int]bool{}, heldKeysyms: map[uint32]bool{}}
}

// reconcileCapsLock records that the remote caps lock must be wantLocked and
// reports whether a lock toggle has to be injected to get there.
// Caller holds k.mu.
func (k *sharedKeyboard) reconcileCapsLock(wantLocked bool) (toggle bool) {
	if k.capsLocked == wantLocked {
		return false
	}
	k.capsLocked = wantLocked
	return true
}

// press records that c pressed code. forward reports whether the press must
// be injected: false for a press of a key another client already holds (it is
// already down remotely). repeat reports a press from a client already holding
// the key, i.e. browser auto-repeat. Caller holds k.mu.
func (k *sharedKeyboard) press(c *keyboardClient, code int) (forward, repeat bool) {
	if c.held[code] {
		return false, true
	}
	if k.holders == nil {
		k.holders = map[int]int{}
	}
	c.held[code] = true
	k.holders[code]++
	return k.holders[code] == 1, false
}

// release records that c released code and reports whether the release must be
// forwarded. A release of a key c never pressed is dropped: it may be held by
// another viewer. Caller holds k.mu.
func (k *sharedKeyboard) release(c *keyboardClient, code int) (forward bool) {
	if !c.held[code] {
		return false
	}
	delete(c.held, code)
	k.holders[code]--
	if k.holders[code] > 0 {
		return false
	}
	delete(k.holders, code)
	return true
}

// heldModifiers returns the Modifier* bits of the left-hand modifier keys some
// viewer is holding down. Caller holds k.mu.
func (k *sharedKeyboard) heldModifiers() byte {
	var mods byte
	for mod, code := range map[byte]int{
		ModifierShift: KEY_LEFTSHIFT,
		ModifierCtrl:  KEY_LEFTCTRL,
		ModifierAlt:   KEY_LEFTALT,
		ModifierMeta:  KEY_LEFTMETA,
	} {
		if k.holders[code] > 0 {
			mods |= mod
		}
	}
	return mods
}

// waylandKeyboardOnly reports whether keys go to the Wayland virtual keyboard
// (Sway) rather than Mutter's RemoteDesktop session.
func (s *Server) waylandKeyboardOnly() bool {
	return (s.conn == nil || s.rdSessionPath == "") && s.waylandInput != nil
}

// injectKeycodeLocked sends one evdev key edge to the compositor.
// Caller holds s.keyboard.mu, which serializes all key injection so that
// decisions and the order the compositor sees them in cannot diverge.
func (s *Server) injectKeycodeLocked(code int, isDown bool) {
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeycode", 0, uint32(code), isDown).Err; err != nil {
			s.logger.Error("keyboard D-Bus call failed", "evdev", code, "down", isDown, "err", err)
		}
		return
	}
	if s.waylandInput != nil {
		var err error
		if isDown {
			err = s.waylandInput.KeyDownEvdev(code)
		} else {
			err = s.waylandInput.KeyUpEvdev(code)
		}
		if err != nil {
			s.logger.Debug("Wayland virtual keyboard failed", "evdev", code, "err", err)
		}
	}
}

// setCapsLockLocked makes the remote caps lock equal wantLocked.
// Caller holds s.keyboard.mu.
func (s *Server) setCapsLockLocked(wantLocked bool) {
	if !s.keyboard.reconcileCapsLock(wantLocked) {
		return
	}
	s.logger.Debug("syncing remote caps lock", "locked", wantLocked)
	if s.conn != nil && s.rdSessionPath != "" {
		s.injectKeycodeLocked(KEY_CAPSLOCK, true)
		s.injectKeycodeLocked(KEY_CAPSLOCK, false)
		return
	}
	// zwp_virtual_keyboard_v1 clients own the modifier state: a KEY_CAPSLOCK
	// tap would be overwritten by the next modifiers event, so set the lock
	// in the modifiers we send instead.
	if s.waylandInput != nil {
		if err := s.waylandInput.SetCapsLock(wantLocked); err != nil {
			s.logger.Debug("Wayland caps lock sync failed", "err", err)
		}
	}
}

// releaseKeyboardClient releases every key a disconnecting viewer still holds,
// except keys another viewer is also holding.
func (s *Server) releaseKeyboardClient(c *keyboardClient) {
	s.keyboard.mu.Lock()
	defer s.keyboard.mu.Unlock()

	for code := range c.held {
		if s.keyboard.release(c, code) {
			s.logger.Info("releasing key held by disconnected viewer", "evdev", code)
			s.injectKeycodeLocked(code, false)
		}
	}
	for keysym := range c.heldKeysyms {
		delete(c.heldKeysyms, keysym)
		s.logger.Info("releasing keysym held by disconnected viewer", "keysym", keysym)
		s.injectKeysymLocked(keysym, false)
	}
}

// injectKeysymLocked sends one keysym edge to the compositor.
// Caller holds s.keyboard.mu.
func (s *Server) injectKeysymLocked(keysym uint32, isDown bool) {
	// NotifyKeyboardKeysym resolves the shift level from the compositor's own
	// keymap, so Shift must NOT be synthesized here — that would double-apply it.
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, keysym, isDown).Err; err != nil {
			s.logger.Error("keyboard keysym D-Bus call failed", "keysym", keysym, "down", isDown, "err", err)
		}
		return
	}

	// Wayland virtual keyboard: convert keysym to evdev keycode using xkbcommon
	// (layout-aware) or the static mapping.
	if s.waylandInput != nil {
		evdevCode, needsShift := resolveKeysym(keysym)
		if evdevCode == 0 {
			s.logger.Debug("No evdev mapping for keysym", "keysym", keysym, "xkbAvailable", IsXKBAvailable())
			return
		}
		// Hold Shift around the key press for shift-level characters, otherwise
		// the bare keycode types the unshifted character ('@' becomes '2').
		var err error
		if isDown {
			if needsShift {
				s.waylandInput.KeyDownEvdev(KEY_LEFTSHIFT)
			}
			err = s.waylandInput.KeyDownEvdev(evdevCode)
		} else {
			err = s.waylandInput.KeyUpEvdev(evdevCode)
			if needsShift {
				s.waylandInput.KeyUpEvdev(KEY_LEFTSHIFT)
			}
		}
		if err != nil {
			s.logger.Debug("Wayland virtual keyboard keysym failed", "keysym", keysym, "evdev", evdevCode, "err", err)
		}
	}
}
