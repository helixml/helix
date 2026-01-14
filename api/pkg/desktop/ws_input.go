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
// Format: [subType:1][isDown:1][modifiers:1][keycode:2 BE]
// The keycode is a Linux evdev keycode sent directly by the frontend.
func (s *Server) handleWSKeyboard(data []byte) {
	if len(data) < 5 {
		return
	}

	// subType := data[0] // Always 0 for keyboard, unused
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
// Format: [eventType:1][slot:1][x:4][y:4]
func (s *Server) handleWSTouch(data []byte) {
	if len(data) < 10 {
		return
	}

	eventType := data[0]
	slot := uint32(data[1])
	x := math.Float32frombits(binary.LittleEndian.Uint32(data[2:6]))
	y := math.Float32frombits(binary.LittleEndian.Uint32(data[6:10]))

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
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchDown", 0, stream, slot, float64(x), float64(y)).Err
	case 1: // Touch motion
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchMotion", 0, stream, slot, float64(x), float64(y)).Err
	case 2: // Touch up
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchUp", 0, slot).Err
	}

	if err != nil {
		s.logger.Error("WebSocket touch D-Bus call failed", "eventType", eventType, "err", err)
	}
}
