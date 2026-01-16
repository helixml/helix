package desktop

import (
	"encoding/binary"
	"math"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocket input message types (matching frontend protocol)
const (
	MsgTypeKeyboard      = 0x01
	MsgTypeMouseButton   = 0x02
	MsgTypeMouseAbsolute = 0x03
	MsgTypeMouseRelative = 0x04
	MsgTypeScroll        = 0x05
	MsgTypeTouch         = 0x06
)

// Scroll deltaMode values (from browser WheelEvent)
const (
	DeltaModePixel = 0
	DeltaModeLine  = 1
	DeltaModePage  = 2
)

// wsInputState tracks scroll gesture state (reserved for future use)
type wsInputState struct {
	mu sync.Mutex
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// handleWSInput handles the direct WebSocket input connection.
// This sends input directly from browser to GNOME D-Bus.
func (s *Server) handleWSInput(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	s.logger.Info("WebSocket input client connected")

	state := &wsInputState{}

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Error("WebSocket read error", "err", err)
			}
			break
		}

		if messageType != websocket.BinaryMessage {
			s.logger.Warn("Ignoring non-binary WebSocket message")
			continue
		}

		if len(data) < 1 {
			continue
		}

		msgType := data[0]
		payload := data[1:]

		switch msgType {
		case MsgTypeKeyboard:
			s.handleWSKeyboard(payload)
		case MsgTypeMouseButton:
			s.handleWSMouseButton(payload)
		case MsgTypeMouseAbsolute:
			s.handleWSMouseAbsolute(payload)
		case MsgTypeMouseRelative:
			s.handleWSMouseRelative(payload)
		case MsgTypeScroll:
			s.handleWSScroll(payload, state)
		case MsgTypeTouch:
			s.handleWSTouch(payload)
		default:
			s.logger.Warn("Unknown WebSocket input message type", "type", msgType)
		}
	}

	s.logger.Info("WebSocket input client disconnected")
}

// handleWSKeyboard handles keyboard input messages.
//
// Supports three modes:
//   - subType=0: Evdev keycode [subType:1][isDown:1][modifiers:1][keycode:2 BE]
//   - subType=2: X11 keysym [subType:1][isDown:1][modifiers:1][keysym:4 BE]
//   - subType=3: X11 keysym tap [subType:1][modifiers:1][keysym:4 BE] (press+release)
//
// Keysym mode is used when event.code is unavailable (iPad/iOS).
// Keysym tap is used for Android virtual keyboards and swipe typing.
func (s *Server) handleWSKeyboard(data []byte) {
	if len(data) < 1 {
		return
	}

	subType := data[0]
	switch subType {
	case 0:
		s.handleWSKeyboardKeycode(data)
	case 2:
		s.handleWSKeyboardKeysym(data)
	case 3:
		s.handleWSKeyboardKeysymTap(data)
	default:
		s.logger.Warn("Unknown keyboard subType", "subType", subType)
	}
}

// handleWSKeyboardKeycode handles evdev keycode keyboard messages.
// Format: [subType:1][isDown:1][modifiers:1][keycode:2 BE]
func (s *Server) handleWSKeyboardKeycode(data []byte) {
	if len(data) < 5 {
		return
	}

	isDown := data[1] != 0
	// modifiers := data[2] // Currently unused, could be used for modifier sync
	evdevCode := int(binary.BigEndian.Uint16(data[3:5]))

	if evdevCode == 0 {
		return
	}

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeycode", 0, uint32(evdevCode), isDown).Err
		if err != nil {
			s.logger.Error("WebSocket keyboard D-Bus call failed", "evdev", evdevCode, "err", err)
		}
		return
	}

	// Fallback to Wayland-native virtual keyboard for Sway/wlroots
	if s.waylandInput != nil {
		var err error
		if isDown {
			err = s.waylandInput.KeyDownEvdev(evdevCode)
		} else {
			err = s.waylandInput.KeyUpEvdev(evdevCode)
		}
		if err != nil {
			s.logger.Debug("Wayland virtual keyboard failed", "evdev", evdevCode, "err", err)
		}
	}
}

// handleWSKeyboardKeysym handles X11 keysym keyboard messages.
// Format: [subType:1][isDown:1][modifiers:1][keysym:4 BE]
//
// Keysyms provide layout-independent character input, used when
// event.code is unavailable (iPad/iOS keyboards).
func (s *Server) handleWSKeyboardKeysym(data []byte) {
	if len(data) < 7 {
		return
	}

	isDown := data[1] != 0
	// modifiers := data[2] // Currently unused
	keysym := binary.BigEndian.Uint32(data[3:7])

	if keysym == 0 {
		return
	}

	// Try D-Bus RemoteDesktop first (GNOME)
	// NotifyKeyboardKeysym sends the character directly, independent of layout
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, keysym, isDown).Err
		if err != nil {
			s.logger.Error("WebSocket keyboard keysym D-Bus call failed", "keysym", keysym, "err", err)
		}
		return
	}

	// Fallback to Wayland-native virtual keyboard for Sway/wlroots
	// Convert keysym to evdev keycode using xkbcommon (layout-aware) or static mapping
	if s.waylandInput != nil {
		// Try xkbcommon first for layout-aware mapping
		evdevCode := XKBKeysymToEvdev(keysym)
		if evdevCode == 0 {
			// Fall back to static QWERTY mapping
			evdevCode = keysymToEvdev(keysym)
		}
		if evdevCode == 0 {
			s.logger.Debug("No evdev mapping for keysym", "keysym", keysym, "xkbAvailable", IsXKBAvailable())
			return
		}
		var err error
		if isDown {
			err = s.waylandInput.KeyDownEvdev(evdevCode)
		} else {
			err = s.waylandInput.KeyUpEvdev(evdevCode)
		}
		if err != nil {
			s.logger.Debug("Wayland virtual keyboard keysym failed", "keysym", keysym, "evdev", evdevCode, "err", err)
		}
	}
}

// Modifier bit flags (matching frontend EvdevModifiers)
const (
	ModifierShift = 1 << 0
	ModifierCtrl  = 1 << 1
	ModifierAlt   = 1 << 2
	ModifierMeta  = 1 << 3
)

// Modifier keysyms (X11)
const (
	XK_Shift_L   = 0xffe1
	XK_Control_L = 0xffe3
	XK_Alt_L     = 0xffe9
	XK_Super_L   = 0xffeb
)

// Modifier evdev keycodes
const (
	KEY_LEFTSHIFT = 42
	KEY_LEFTCTRL  = 29
	KEY_LEFTALT   = 56
	KEY_LEFTMETA  = 125
)

// handleWSKeyboardKeysymTap handles X11 keysym "tap" messages (press + release).
// Format: [subType:1][modifiers:1][keysym:4 BE]
//
// Used for Android virtual keyboards and swipe typing where we only get
// the final character, not separate key down/up events. The backend
// synthesizes both key press and release.
//
// If modifiers are specified (e.g., Ctrl for deleteWordBackward), the backend
// sends modifier key down, keysym tap, modifier key up.
func (s *Server) handleWSKeyboardKeysymTap(data []byte) {
	if len(data) < 6 {
		return
	}

	modifiers := data[1]
	keysym := binary.BigEndian.Uint32(data[2:6])

	if keysym == 0 {
		return
	}

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

		// Press modifiers first
		if modifiers&ModifierShift != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Shift_L), true)
		}
		if modifiers&ModifierCtrl != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Control_L), true)
		}
		if modifiers&ModifierAlt != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Alt_L), true)
		}
		if modifiers&ModifierMeta != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Super_L), true)
		}

		// Key press
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, keysym, true).Err
		if err != nil {
			s.logger.Error("WebSocket keyboard keysym tap D-Bus call failed (press)", "keysym", keysym, "err", err)
		}
		// Key release
		rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, keysym, false)

		// Release modifiers (reverse order)
		if modifiers&ModifierMeta != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Super_L), false)
		}
		if modifiers&ModifierAlt != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Alt_L), false)
		}
		if modifiers&ModifierCtrl != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Control_L), false)
		}
		if modifiers&ModifierShift != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, uint32(XK_Shift_L), false)
		}
		return
	}

	// Fallback to Wayland-native virtual keyboard for Sway/wlroots
	// Convert keysym to evdev keycode using xkbcommon (layout-aware) or static mapping
	if s.waylandInput != nil {
		// Try xkbcommon first for layout-aware mapping
		evdevCode := XKBKeysymToEvdev(keysym)
		if evdevCode == 0 {
			// Fall back to static QWERTY mapping
			evdevCode = keysymToEvdev(keysym)
		}
		if evdevCode == 0 {
			s.logger.Debug("No evdev mapping for keysym tap", "keysym", keysym, "xkbAvailable", IsXKBAvailable())
			return
		}

		// Press modifiers first
		if modifiers&ModifierShift != 0 {
			s.waylandInput.KeyDownEvdev(KEY_LEFTSHIFT)
		}
		if modifiers&ModifierCtrl != 0 {
			s.waylandInput.KeyDownEvdev(KEY_LEFTCTRL)
		}
		if modifiers&ModifierAlt != 0 {
			s.waylandInput.KeyDownEvdev(KEY_LEFTALT)
		}
		if modifiers&ModifierMeta != 0 {
			s.waylandInput.KeyDownEvdev(KEY_LEFTMETA)
		}

		// Key press
		if err := s.waylandInput.KeyDownEvdev(evdevCode); err != nil {
			s.logger.Debug("Wayland virtual keyboard keysym tap failed (press)", "keysym", keysym, "evdev", evdevCode, "err", err)
		}
		// Key release
		s.waylandInput.KeyUpEvdev(evdevCode)

		// Release modifiers (reverse order)
		if modifiers&ModifierMeta != 0 {
			s.waylandInput.KeyUpEvdev(KEY_LEFTMETA)
		}
		if modifiers&ModifierAlt != 0 {
			s.waylandInput.KeyUpEvdev(KEY_LEFTALT)
		}
		if modifiers&ModifierCtrl != 0 {
			s.waylandInput.KeyUpEvdev(KEY_LEFTCTRL)
		}
		if modifiers&ModifierShift != 0 {
			s.waylandInput.KeyUpEvdev(KEY_LEFTSHIFT)
		}
	}
}

// handleWSMouseButton handles mouse click/wheel messages (message type 0x11).
// Dispatches based on subType:
//   - subType=2: Button click [subType:1][isDown:1][button:1]
//   - subType=3: High-res wheel [subType:1][deltaX:2 BE][deltaY:2 BE]
//   - subType=4: Normal wheel [subType:1][deltaX:1][deltaY:1]
func (s *Server) handleWSMouseButton(data []byte) {
	if len(data) < 1 {
		return
	}

	subType := data[0]
	switch subType {
	case 2: // Button click
		if len(data) < 3 {
			return
		}
		isDown := data[1] != 0
		button := int(data[2])
		s.handleMouseButtonClick(button, isDown)

	case 3: // High-res wheel (float32 deltas, little-endian)
		if len(data) < 9 {
			return
		}
		deltaX := math.Float32frombits(binary.LittleEndian.Uint32(data[1:5]))
		deltaY := math.Float32frombits(binary.LittleEndian.Uint32(data[5:9]))
		s.handleMouseWheel(float64(deltaX), float64(deltaY))

	case 4: // Normal wheel (int8 deltas)
		if len(data) < 3 {
			return
		}
		deltaX := int8(data[1])
		deltaY := int8(data[2])
		// Normal wheel has smaller values, scale up for consistency
		s.handleMouseWheel(float64(deltaX)*10, float64(deltaY)*10)

	default:
		s.logger.Debug("unknown mouse click subType", "subType", subType)
	}
}

// handleMouseButtonClick handles a mouse button press/release
func (s *Server) handleMouseButtonClick(button int, isDown bool) {
	// Convert button codes to evdev button codes
	// Input: 1=left, 2=middle, 3=right
	// Evdev: 272=BTN_LEFT, 273=BTN_RIGHT, 274=BTN_MIDDLE
	var evdevButton int32
	switch button {
	case 1:
		evdevButton = 272 // BTN_LEFT
	case 2:
		evdevButton = 274 // BTN_MIDDLE
	case 3:
		evdevButton = 273 // BTN_RIGHT
	case 4:
		evdevButton = 275 // BTN_SIDE
	default:
		evdevButton = 276 + int32(button-5) // BTN_EXTRA and beyond
	}

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerButton", 0, evdevButton, isDown).Err
		if err != nil {
			s.logger.Error("WebSocket mouse button D-Bus call failed", "button", evdevButton, "err", err)
		}
		return
	}

	// Fallback to Wayland-native virtual mouse for Sway/wlroots
	if s.waylandInput != nil {
		var err error
		if isDown {
			err = s.waylandInput.MouseButtonDown(button)
		} else {
			err = s.waylandInput.MouseButtonUp(button)
		}
		if err != nil {
			s.logger.Debug("Wayland virtual mouse button failed", "button", button, "err", err)
		}
	}
}

// handleMouseWheel handles scroll wheel events with smooth scrolling.
//
// Two paths with matching behavior:
// 1. GNOME RemoteDesktop D-Bus - uses NotifyPointerAxis with FINGER source
// 2. Wayland-native (Sway/wlroots) - uses zwlr_virtual_pointer with AxisSourceFinger
//
// Both paths scale browser pixels to scroll units (pixels * 0.15) and use
// FINGER source for smooth scrolling in apps like Zed.
//
// Note: We don't send scroll finish/axis_stop events. They were causing Sway
// crashes when mixed with other pointer events (assertion failures in wlr_seat),
// and scrolling works correctly without them.
func (s *Server) handleMouseWheel(deltaX, deltaY float64) {
	// Scale browser pixels to scroll units
	// Browser sends ~100-120 pixels per wheel notch
	// Scroll protocols expect ~10-15 units per notch
	// Scale factor 0.15: 100 pixels → 15 units
	scaledDX := deltaX * 0.15
	scaledDY := deltaY * 0.15

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		// GNOME RemoteDesktop flags:
		// - bits 0-1: finish flags (1=finish X, 2=finish Y, 3=finish both)
		// - bits 2-3: source (0=FINGER, 1=WHEEL, 2=CONTINUOUS)
		// Use FINGER source (0x00) for smooth scrolling - matches Wayland path
		gnomeFlags := uint32(0x00) // FINGER source

		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxis", 0, scaledDX, scaledDY, gnomeFlags).Err
		if err != nil {
			s.logger.Error("WebSocket scroll D-Bus call failed", "err", err)
		}
		return
	}

	// Fallback to Wayland-native input for Sway/wlroots
	// MouseWheel already scales by 0.15 internally, so pass raw pixels
	if s.waylandInput != nil {
		if err := s.waylandInput.MouseWheel(deltaX, deltaY); err != nil {
			s.logger.Debug("Wayland virtual mouse wheel failed", "err", err)
		}
	}
}

// handleWSMouseAbsolute handles absolute mouse position messages.
// Format: [subType:1][x:2 BE int16][y:2 BE int16][refWidth:2 BE int16][refHeight:2 BE int16]
// subType=1 for absolute position
func (s *Server) handleWSMouseAbsolute(data []byte) {
	if len(data) < 9 {
		return
	}

	// subType := data[0] // 1 for absolute
	x := int16(binary.BigEndian.Uint16(data[1:3]))
	y := int16(binary.BigEndian.Uint16(data[3:5]))
	refWidth := int16(binary.BigEndian.Uint16(data[5:7]))
	refHeight := int16(binary.BigEndian.Uint16(data[7:9]))

	// Scale from reference coordinates to actual screen dimensions
	// Screen dimensions are read from GAMESCOPE_WIDTH/HEIGHT env vars at startup
	absX := float64(x) / float64(refWidth) * float64(s.screenWidth)
	absY := float64(y) / float64(refHeight) * float64(s.screenHeight)

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		stream := string(s.scStreamPath)
		if stream == "" {
			return
		}
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotionAbsolute", 0, stream, absX, absY).Err
		if err != nil {
			s.logger.Error("WebSocket mouse absolute D-Bus call failed", "err", err)
		}
		return
	}

	// Fallback to Wayland-native input for Sway/wlroots
	// WaylandInput converts absolute to relative movement internally
	//
	// IMPORTANT: Use LOGICAL dimensions for Sway, not physical!
	// With scale 2.0, physical 3840x2160 → logical 1920x1080.
	// The video is captured at physical resolution, so frontend sends physical coords.
	// We must normalize to 0-1 using physical, then let WaylandInput convert to logical.
	if s.waylandInput != nil {
		// Normalize using physical dimensions (what the video is captured at)
		sw := float64(s.screenWidth)
		sh := float64(s.screenHeight)
		normX := absX / sw
		normY := absY / sh

		// WaylandInput was created with logical dimensions, so pass those
		logicalW := int(sw / s.displayScale)
		logicalH := int(sh / s.displayScale)
		if err := s.waylandInput.MouseMoveAbsolute(normX, normY, logicalW, logicalH); err != nil {
			s.logger.Debug("Wayland virtual mouse absolute failed", "err", err)
		}
	}

	// Broadcast cursor position to other connected clients for multi-player cursors
	// Note: This broadcasts the user's mouse position (in screen coords) to all other
	// clients viewing this session, enabling Figma-style collaborative cursors.
	if s.config.SessionID != "" {
		// Use original (unscaled) coordinates for broadcasting since all clients
		// receive video at the same resolution and scale independently
		GetSessionRegistry().BroadcastCursorPosition(s.config.SessionID, 0, int32(absX), int32(absY))
	}
}

// handleWSMouseRelative handles relative mouse movement messages.
// Format: [subType:1][dx:2 BE int16][dy:2 BE int16]
// subType=0 for relative movement
func (s *Server) handleWSMouseRelative(data []byte) {
	if len(data) < 5 {
		return
	}

	// subType := data[0] // 0 for relative
	dx := int16(binary.BigEndian.Uint16(data[1:3]))
	dy := int16(binary.BigEndian.Uint16(data[3:5]))

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotionRelative", 0, float64(dx), float64(dy)).Err
		if err != nil {
			s.logger.Error("WebSocket mouse relative D-Bus call failed", "err", err)
		}
		return
	}

	// Fallback to Wayland-native virtual mouse for Sway/wlroots
	if s.waylandInput != nil {
		if err := s.waylandInput.MouseMove(int32(dx), int32(dy)); err != nil {
			s.logger.Debug("Wayland virtual mouse move failed", "err", err)
		}
	}
}

// handleWSScroll handles scroll input messages with proper browser→GNOME conversion.
// Format: [deltaMode:1][flags:1][deltaX:4][deltaY:4]
//
// Browser WheelEvent conventions:
//   - deltaY > 0 = scroll DOWN (content moves up)
//   - deltaMode: 0=pixel, 1=line, 2=page
//
// GNOME NotifyPointerAxis conventions:
//   - Positive values = content scrolls in that direction
//   - Values are in "scroll units" (~10-15 per wheel notch)
//   - Flags: bits 0-1 = finish flags, bits 2-3 = scroll source
func (s *Server) handleWSScroll(data []byte, state *wsInputState) {
	if len(data) < 10 {
		return
	}

	deltaMode := data[0]
	flags := data[1]
	isTrackpad := (flags & 0x01) != 0
	deltaX := math.Float32frombits(binary.LittleEndian.Uint32(data[2:6]))
	deltaY := math.Float32frombits(binary.LittleEndian.Uint32(data[6:10]))

	// Step 1: Normalize to pixels based on deltaMode
	var pixelX, pixelY float64
	switch deltaMode {
	case DeltaModePixel:
		pixelX, pixelY = float64(deltaX), float64(deltaY)
	case DeltaModeLine:
		// ~40 pixels per line (browser convention)
		pixelX, pixelY = float64(deltaX)*40, float64(deltaY)*40
	case DeltaModePage:
		// Approximate viewport size
		pixelX, pixelY = float64(deltaX)*800, float64(deltaY)*600
	default:
		pixelX, pixelY = float64(deltaX), float64(deltaY)
	}

	// Step 2: Convert browser pixels to GNOME scroll units
	// Browser sends ~100-120 pixels per wheel notch
	// GNOME expects ~10-15 scroll units per wheel notch
	// Ratio: divide by 10
	gnomeDX := pixelX / 10.0
	gnomeDY := pixelY / 10.0

	// Step 3: Direction mapping
	// Browser: +Y = scroll down (content moves up)
	// GNOME: +Y = content moves down
	// So we need to NEGATE to match expected behavior
	gnomeDY = -gnomeDY
	// Note: X axis typically doesn't need inversion

	// Step 4: Set scroll source flags
	// GNOME flags: bits 0-1 = finish, bits 2-3 = source
	// Source: 0x00 = FINGER, 0x04 = WHEEL, 0x08 = CONTINUOUS
	var gnomeFlags uint32
	if isTrackpad {
		gnomeFlags = 0x00 // FINGER source (enables kinetic scrolling)
	} else {
		gnomeFlags = 0x04 // WHEEL source (discrete clicks)
	}

	// Log for debugging (first few and then periodically)
	s.scrollLogCount++
	if s.scrollLogCount <= 5 || s.scrollLogCount%100 == 0 {
		s.logger.Debug("WebSocket scroll",
			"count", s.scrollLogCount,
			"deltaMode", deltaMode,
			"isTrackpad", isTrackpad,
			"browserDX", deltaX, "browserDY", deltaY,
			"gnomeDX", gnomeDX, "gnomeDY", gnomeDY,
			"flags", gnomeFlags)
	}

	// Try GNOME D-Bus first
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxis", 0, gnomeDX, gnomeDY, gnomeFlags).Err
		if err != nil {
			s.logger.Error("WebSocket scroll D-Bus call failed", "err", err)
		}
	} else if s.waylandInput != nil {
		// Fallback to Wayland-native input for Sway/wlroots
		// Pass raw pixel values - MouseWheel does its own scaling
		// Note: Negate Y for GNOME-like direction (matches gnomeDY behavior)
		if err := s.waylandInput.MouseWheel(pixelX, -pixelY); err != nil {
			s.logger.Debug("Wayland virtual scroll failed", "err", err)
		}
	}
	// Note: We don't send scroll finish events - they were causing crashes
	// when mixed with other pointer events, and scrolling works without them.
}

// handleWSTouch handles touch input messages.
// Format: [eventType:1][slot:1][x:4 LE float32][y:4 LE float32]
// x and y are normalized coordinates (0.0-1.0) which we convert to screen coordinates.
func (s *Server) handleWSTouch(data []byte) {
	if len(data) < 10 {
		return
	}

	eventType := data[0]
	slot := uint32(data[1])
	// Normalized coordinates 0.0-1.0
	normX := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[2:6])))
	normY := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[6:10])))

	// Convert to screen coordinates
	screenX := normX * float64(s.screenWidth)
	screenY := normY * float64(s.screenHeight)

	if s.conn == nil || s.rdSessionPath == "" {
		return
	}

	stream := string(s.scStreamPath)
	if stream == "" {
		return
	}

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	var err error

	switch eventType {
	case 0: // Touch down
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchDown", 0, stream, slot, screenX, screenY).Err
	case 1: // Touch motion
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchMotion", 0, stream, slot, screenX, screenY).Err
	case 2: // Touch up
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchUp", 0, slot).Err
	}

	if err != nil {
		s.logger.Error("WebSocket touch D-Bus call failed", "eventType", eventType, "err", err)
	}

	// Broadcast touch event to other connected clients
	if s.config.SessionID != "" {
		var touchEventType TouchEventType
		switch eventType {
		case 0:
			touchEventType = TouchEventStart
		case 1:
			touchEventType = TouchEventMove
		case 2:
			touchEventType = TouchEventEnd
		default:
			touchEventType = TouchEventCancel
		}
		// Use client ID 0 since we don't track per-client connections here
		// Pressure is not available from the WebSocket message, default to 1.0
		GetSessionRegistry().BroadcastTouchEvent(s.config.SessionID, 0, slot, touchEventType, int32(screenX), int32(screenY), 1.0)
	}
}

// keysymToEvdev converts an X11 keysym to a Linux evdev keycode.
// This is a best-effort mapping for Sway/wlroots fallback when GNOME's
// NotifyKeyboardKeysym is not available.
//
// For QWERTY layouts, this maps ASCII characters to their physical key positions.
// Non-ASCII characters may not map correctly on non-QWERTY layouts.
//
// Returns 0 if no mapping exists.
func keysymToEvdev(keysym uint32) int {
	// X11 keysym constants
	const (
		XK_BackSpace = 0xff08
		XK_Tab       = 0xff09
		XK_Return    = 0xff0d
		XK_Escape    = 0xff1b
		XK_Delete    = 0xffff
		XK_Home      = 0xff50
		XK_Left      = 0xff51
		XK_Up        = 0xff52
		XK_Right     = 0xff53
		XK_Down      = 0xff54
		XK_Page_Up   = 0xff55
		XK_Page_Down = 0xff56
		XK_End       = 0xff57
		XK_Insert    = 0xff63
		XK_Num_Lock  = 0xff7f
		XK_KP_Enter  = 0xff8d
		XK_KP_0      = 0xffb0
		XK_F1        = 0xffbe
		XK_F12       = 0xffc9
		XK_Shift_L   = 0xffe1
		XK_Shift_R   = 0xffe2
		XK_Control_L = 0xffe3
		XK_Control_R = 0xffe4
		XK_Caps_Lock = 0xffe5
		XK_Alt_L     = 0xffe9
		XK_Alt_R     = 0xffea
		XK_Super_L   = 0xffeb
		XK_Super_R   = 0xffec
	)

	// Linux evdev keycodes
	const (
		KEY_ESC       = 1
		KEY_1         = 2
		KEY_2         = 3
		KEY_3         = 4
		KEY_4         = 5
		KEY_5         = 6
		KEY_6         = 7
		KEY_7         = 8
		KEY_8         = 9
		KEY_9         = 10
		KEY_0         = 11
		KEY_MINUS     = 12
		KEY_EQUAL     = 13
		KEY_BACKSPACE = 14
		KEY_TAB       = 15
		KEY_Q         = 16
		KEY_W         = 17
		KEY_E         = 18
		KEY_R         = 19
		KEY_T         = 20
		KEY_Y         = 21
		KEY_U         = 22
		KEY_I         = 23
		KEY_O         = 24
		KEY_P         = 25
		KEY_LEFTBRACE = 26
		KEY_RIGHTBRACE = 27
		KEY_ENTER     = 28
		KEY_LEFTCTRL  = 29
		KEY_A         = 30
		KEY_S         = 31
		KEY_D         = 32
		KEY_F         = 33
		KEY_G         = 34
		KEY_H         = 35
		KEY_J         = 36
		KEY_K         = 37
		KEY_L         = 38
		KEY_SEMICOLON = 39
		KEY_APOSTROPHE = 40
		KEY_GRAVE     = 41
		KEY_LEFTSHIFT = 42
		KEY_BACKSLASH = 43
		KEY_Z         = 44
		KEY_X         = 45
		KEY_C         = 46
		KEY_V         = 47
		KEY_B         = 48
		KEY_N         = 49
		KEY_M         = 50
		KEY_COMMA     = 51
		KEY_DOT       = 52
		KEY_SLASH     = 53
		KEY_RIGHTSHIFT = 54
		KEY_LEFTALT   = 56
		KEY_SPACE     = 57
		KEY_CAPSLOCK  = 58
		KEY_F1        = 59
		KEY_F12       = 70
		KEY_NUMLOCK   = 69
		KEY_HOME      = 102
		KEY_UP        = 103
		KEY_PAGEUP    = 104
		KEY_LEFT      = 105
		KEY_RIGHT     = 106
		KEY_END       = 107
		KEY_DOWN      = 108
		KEY_PAGEDOWN  = 109
		KEY_INSERT    = 110
		KEY_DELETE    = 111
		KEY_KPENTER   = 96
		KEY_RIGHTCTRL = 97
		KEY_RIGHTALT  = 100
		KEY_LEFTMETA  = 125
		KEY_RIGHTMETA = 126
	)

	// Special keys (0xFF00-0xFFFF range)
	switch keysym {
	case XK_BackSpace:
		return KEY_BACKSPACE
	case XK_Tab:
		return KEY_TAB
	case XK_Return:
		return KEY_ENTER
	case XK_Escape:
		return KEY_ESC
	case XK_Delete:
		return KEY_DELETE
	case XK_Home:
		return KEY_HOME
	case XK_Left:
		return KEY_LEFT
	case XK_Up:
		return KEY_UP
	case XK_Right:
		return KEY_RIGHT
	case XK_Down:
		return KEY_DOWN
	case XK_Page_Up:
		return KEY_PAGEUP
	case XK_Page_Down:
		return KEY_PAGEDOWN
	case XK_End:
		return KEY_END
	case XK_Insert:
		return KEY_INSERT
	case XK_Num_Lock:
		return KEY_NUMLOCK
	case XK_KP_Enter:
		return KEY_KPENTER
	case XK_Shift_L:
		return KEY_LEFTSHIFT
	case XK_Shift_R:
		return KEY_RIGHTSHIFT
	case XK_Control_L:
		return KEY_LEFTCTRL
	case XK_Control_R:
		return KEY_RIGHTCTRL
	case XK_Caps_Lock:
		return KEY_CAPSLOCK
	case XK_Alt_L:
		return KEY_LEFTALT
	case XK_Alt_R:
		return KEY_RIGHTALT
	case XK_Super_L:
		return KEY_LEFTMETA
	case XK_Super_R:
		return KEY_RIGHTMETA
	}

	// Function keys (F1-F12)
	if keysym >= XK_F1 && keysym <= XK_F12 {
		return KEY_F1 + int(keysym-XK_F1)
	}

	// Keypad numbers (0-9)
	if keysym >= XK_KP_0 && keysym <= XK_KP_0+9 {
		// KEY_KP0 = 82, KEY_KP1 = 79, KEY_KP2 = 80, ...
		// KP layout: 7,8,9 / 4,5,6 / 1,2,3 / 0
		kpNum := int(keysym - XK_KP_0)
		kpMap := []int{82, 79, 80, 81, 75, 76, 77, 71, 72, 73}
		return kpMap[kpNum]
	}

	// Latin-1 ASCII characters (keysym == Unicode code point for 0x20-0x7F)
	// Map to QWERTY layout physical key positions
	if keysym >= 0x20 && keysym <= 0x7F {
		switch keysym {
		case ' ':
			return KEY_SPACE
		// Numbers
		case '0':
			return KEY_0
		case '1':
			return KEY_1
		case '2':
			return KEY_2
		case '3':
			return KEY_3
		case '4':
			return KEY_4
		case '5':
			return KEY_5
		case '6':
			return KEY_6
		case '7':
			return KEY_7
		case '8':
			return KEY_8
		case '9':
			return KEY_9
		// Lowercase letters
		case 'a':
			return KEY_A
		case 'b':
			return KEY_B
		case 'c':
			return KEY_C
		case 'd':
			return KEY_D
		case 'e':
			return KEY_E
		case 'f':
			return KEY_F
		case 'g':
			return KEY_G
		case 'h':
			return KEY_H
		case 'i':
			return KEY_I
		case 'j':
			return KEY_J
		case 'k':
			return KEY_K
		case 'l':
			return KEY_L
		case 'm':
			return KEY_M
		case 'n':
			return KEY_N
		case 'o':
			return KEY_O
		case 'p':
			return KEY_P
		case 'q':
			return KEY_Q
		case 'r':
			return KEY_R
		case 's':
			return KEY_S
		case 't':
			return KEY_T
		case 'u':
			return KEY_U
		case 'v':
			return KEY_V
		case 'w':
			return KEY_W
		case 'x':
			return KEY_X
		case 'y':
			return KEY_Y
		case 'z':
			return KEY_Z
		// Uppercase letters (same keys as lowercase)
		case 'A':
			return KEY_A
		case 'B':
			return KEY_B
		case 'C':
			return KEY_C
		case 'D':
			return KEY_D
		case 'E':
			return KEY_E
		case 'F':
			return KEY_F
		case 'G':
			return KEY_G
		case 'H':
			return KEY_H
		case 'I':
			return KEY_I
		case 'J':
			return KEY_J
		case 'K':
			return KEY_K
		case 'L':
			return KEY_L
		case 'M':
			return KEY_M
		case 'N':
			return KEY_N
		case 'O':
			return KEY_O
		case 'P':
			return KEY_P
		case 'Q':
			return KEY_Q
		case 'R':
			return KEY_R
		case 'S':
			return KEY_S
		case 'T':
			return KEY_T
		case 'U':
			return KEY_U
		case 'V':
			return KEY_V
		case 'W':
			return KEY_W
		case 'X':
			return KEY_X
		case 'Y':
			return KEY_Y
		case 'Z':
			return KEY_Z
		// Punctuation (QWERTY layout)
		case '-', '_':
			return KEY_MINUS
		case '=', '+':
			return KEY_EQUAL
		case '[', '{':
			return KEY_LEFTBRACE
		case ']', '}':
			return KEY_RIGHTBRACE
		case ';', ':':
			return KEY_SEMICOLON
		case '\'', '"':
			return KEY_APOSTROPHE
		case '`', '~':
			return KEY_GRAVE
		case '\\', '|':
			return KEY_BACKSLASH
		case ',', '<':
			return KEY_COMMA
		case '.', '>':
			return KEY_DOT
		case '/', '?':
			return KEY_SLASH
		// Shift+number symbols (QWERTY US layout)
		case '!':
			return KEY_1
		case '@':
			return KEY_2
		case '#':
			return KEY_3
		case '$':
			return KEY_4
		case '%':
			return KEY_5
		case '^':
			return KEY_6
		case '&':
			return KEY_7
		case '*':
			return KEY_8
		case '(':
			return KEY_9
		case ')':
			return KEY_0
		}
	}

	// No mapping found
	return 0
}
