package desktop

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"sync"
	"time"

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

// wsInputState tracks scroll gesture state for finish detection
type wsInputState struct {
	mu              sync.Mutex
	scrollTimer     *time.Timer
	lastScrollTime  time.Time
	isTrackpadScroll bool
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// handleWSInput handles the direct WebSocket input connection.
// This bypasses Moonlight/Wolf for input, going directly from browser to GNOME D-Bus.
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
// Format: [isDown:1][modifiers:1][keycode:2]
func (s *Server) handleWSKeyboard(data []byte) {
	if len(data) < 4 {
		return
	}

	isDown := data[0] != 0
	// modifiers := data[1] // Currently unused, could be used for modifier sync
	keycode := binary.LittleEndian.Uint16(data[2:4])

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeycode", 0, uint32(keycode), isDown).Err
		if err != nil {
			s.logger.Error("WebSocket keyboard D-Bus call failed", "keycode", keycode, "err", err)
		}
		return
	}

	// Fallback to ydotool for Sway/wlroots
	// ydotool uses evdev keycodes with format: keycode:state (1=down, 0=up)
	state := 0
	if isDown {
		state = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ydotool", "key", fmt.Sprintf("%d:%d", keycode, state))
	if err := cmd.Run(); err != nil {
		s.logger.Debug("ydotool keyboard failed", "keycode", keycode, "err", err)
	}
}

// handleWSMouseButton handles mouse button input messages.
// Format: [isDown:1][button:1]
func (s *Server) handleWSMouseButton(data []byte) {
	if len(data) < 2 {
		return
	}

	isDown := data[0] != 0
	button := data[1]

	// Convert Moonlight button codes to evdev button codes
	// Moonlight: 1=left, 2=middle, 3=right
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

	// Fallback to ydotool for Sway/wlroots
	// ydotool click: 0=left, 1=right, 2=middle
	var ydoButton string
	switch button {
	case 1:
		ydoButton = "0xC0" // left down+up
	case 2:
		ydoButton = "0xC2" // middle down+up
	case 3:
		ydoButton = "0xC1" // right down+up
	default:
		ydoButton = "0xC0"
	}
	// Only send click on button up to avoid double-clicks
	if !isDown {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "ydotool", "click", ydoButton)
		if err := cmd.Run(); err != nil {
			s.logger.Debug("ydotool click failed", "button", button, "err", err)
		}
	}
}

// handleWSMouseAbsolute handles absolute mouse position messages.
// Format: [x:4][y:4][refWidth:2][refHeight:2]
func (s *Server) handleWSMouseAbsolute(data []byte) {
	if len(data) < 12 {
		return
	}

	x := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
	y := math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))
	refWidth := binary.LittleEndian.Uint16(data[8:10])
	refHeight := binary.LittleEndian.Uint16(data[10:12])

	// Scale from reference coordinates to absolute (use 1920x1080 as default)
	// TODO: Get actual screen size from compositor
	absX := float64(x) / float64(refWidth) * 1920
	absY := float64(y) / float64(refHeight) * 1080

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

	// Fallback to ydotool for Sway/wlroots
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ydotool", "mousemove", "--absolute",
		"-x", fmt.Sprintf("%.0f", absX),
		"-y", fmt.Sprintf("%.0f", absY))
	if err := cmd.Run(); err != nil {
		s.logger.Debug("ydotool mousemove failed", "err", err)
	}
}

// handleWSMouseRelative handles relative mouse movement messages.
// Format: [dx:4][dy:4]
func (s *Server) handleWSMouseRelative(data []byte) {
	if len(data) < 8 {
		return
	}

	dx := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
	dy := math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))

	// Try D-Bus RemoteDesktop first (GNOME)
	if s.conn != nil && s.rdSessionPath != "" {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotionRelative", 0, float64(dx), float64(dy)).Err
		if err != nil {
			s.logger.Error("WebSocket mouse relative D-Bus call failed", "err", err)
		}
		return
	}

	// Fallback to ydotool for Sway/wlroots
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ydotool", "mousemove",
		"-x", fmt.Sprintf("%.0f", dx),
		"-y", fmt.Sprintf("%.0f", dy))
	if err := cmd.Run(); err != nil {
		s.logger.Debug("ydotool mousemove relative failed", "err", err)
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

	if s.conn == nil || s.rdSessionPath == "" {
		return
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

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxis", 0, gnomeDX, gnomeDY, gnomeFlags).Err
	if err != nil {
		s.logger.Error("WebSocket scroll D-Bus call failed", "err", err)
	}

	// Step 5: Schedule scroll finish for trackpad (enables kinetic scrolling)
	if isTrackpad {
		state.mu.Lock()
		if state.scrollTimer != nil {
			state.scrollTimer.Stop()
		}
		state.scrollTimer = time.AfterFunc(150*time.Millisecond, func() {
			s.sendScrollFinish()
		})
		state.mu.Unlock()
	}
}

// sendScrollFinish sends the scroll gesture finished signal to GNOME.
func (s *Server) sendScrollFinish() {
	if s.conn == nil || s.rdSessionPath == "" {
		return
	}

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	// Finish flags: 3 = both axes finished (HORIZONTAL | VERTICAL)
	err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxis", 0, 0.0, 0.0, uint32(3)).Err
	if err != nil {
		s.logger.Debug("WebSocket scroll finish D-Bus call failed", "err", err)
	} else {
		s.logger.Debug("WebSocket scroll finish sent")
	}
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
