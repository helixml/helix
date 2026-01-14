package desktop

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bnema/wayland-virtual-input-go/virtual_keyboard"
	"github.com/bnema/wayland-virtual-input-go/virtual_pointer"
)

// WaylandInput provides Wayland-native virtual input for Sway/wlroots compositors.
// Uses zwlr_virtual_pointer_v1 and zwp_virtual_keyboard_v1 protocols.
// No /dev/uinput or root privileges required.
type WaylandInput struct {
	pointerManager  *virtual_pointer.VirtualPointerManager
	pointer         *virtual_pointer.VirtualPointer
	keyboardManager *virtual_keyboard.VirtualKeyboardManager
	keyboard        *virtual_keyboard.VirtualKeyboard
	logger          *slog.Logger
	mu              sync.Mutex
	closed          bool

	// Screen dimensions for relative mouse movement bounds checking
	screenWidth  int
	screenHeight int

	// Track current mouse position for relative movement
	// (Only used by MouseMove for relative deltas, not for absolute positioning)
	currentX float64
	currentY float64

	// Modifier state tracking for zwp_virtual_keyboard_v1 protocol.
	// The protocol requires calling Modifiers() after modifier key events
	// to update the compositor's modifier state (XKB modifier masks).
	modsDepressed uint32 // Currently held modifiers (Shift=1, Ctrl=4, Alt=8, Meta=64)

}

// XKB modifier masks (from linux/input-event-codes.h and xkbcommon)
// These are the standard XKB modifier bits used by wlroots/Sway.
const (
	xkbModShift = 1 << 0  // Shift
	xkbModLock  = 1 << 1  // Caps Lock (in modsLocked, not modsDepressed)
	xkbModCtrl  = 1 << 2  // Control
	xkbModAlt   = 1 << 3  // Alt (Mod1)
	xkbModMod2  = 1 << 4  // Num Lock (in modsLocked)
	xkbModMod3  = 1 << 5  // (unused on most layouts)
	xkbModMod4  = 1 << 6  // Super/Meta/Win
	xkbModMod5  = 1 << 7  // (unused on most layouts)
)

// Evdev keycodes for modifier keys
const (
	evdevLeftShift  = 42
	evdevRightShift = 54
	evdevLeftCtrl   = 29
	evdevRightCtrl  = 97
	evdevLeftAlt    = 56
	evdevRightAlt   = 100
	evdevLeftMeta   = 125
	evdevRightMeta  = 126
)

// NewWaylandInput creates a new Wayland virtual input handler.
// Connects to the Wayland compositor and creates virtual pointer and keyboard devices.
func NewWaylandInput(logger *slog.Logger, screenWidth, screenHeight int) (*WaylandInput, error) {
	ctx := context.Background()

	// Create virtual pointer manager
	pointerManager, err := virtual_pointer.NewVirtualPointerManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("create virtual pointer manager: %w", err)
	}

	// Create virtual pointer device
	pointer, err := pointerManager.CreatePointer()
	if err != nil {
		pointerManager.Close()
		return nil, fmt.Errorf("create virtual pointer: %w", err)
	}

	// Create virtual keyboard manager
	keyboardManager, err := virtual_keyboard.NewVirtualKeyboardManager(ctx)
	if err != nil {
		pointer.Close()
		pointerManager.Close()
		return nil, fmt.Errorf("create virtual keyboard manager: %w", err)
	}

	// Create virtual keyboard device
	keyboard, err := keyboardManager.CreateKeyboard()
	if err != nil {
		keyboardManager.Close()
		pointer.Close()
		pointerManager.Close()
		return nil, fmt.Errorf("create virtual keyboard: %w", err)
	}

	logger.Info("wayland virtual input created",
		"screen_width", screenWidth,
		"screen_height", screenHeight)

	return &WaylandInput{
		pointerManager:  pointerManager,
		pointer:         pointer,
		keyboardManager: keyboardManager,
		keyboard:        keyboard,
		logger:          logger,
		screenWidth:     screenWidth,
		screenHeight:    screenHeight,
		currentX:        float64(screenWidth) / 2,
		currentY:        float64(screenHeight) / 2,
	}, nil
}

// Close releases all virtual input devices.
func (w *WaylandInput) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	var errs []error

	if w.keyboard != nil {
		if err := w.keyboard.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close keyboard: %w", err))
		}
	}
	if w.keyboardManager != nil {
		if err := w.keyboardManager.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close keyboard manager: %w", err))
		}
	}
	if w.pointer != nil {
		if err := w.pointer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close pointer: %w", err))
		}
	}
	if w.pointerManager != nil {
		if err := w.pointerManager.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close pointer manager: %w", err))
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}

	w.logger.Info("wayland virtual input closed")
	return nil
}

// KeyDownEvdev sends a key press event with a Linux evdev keycode.
func (w *WaylandInput) KeyDownEvdev(evdevCode int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.keyboard == nil {
		return nil
	}

	// Send key event
	if err := w.keyboard.Key(time.Now(), uint32(evdevCode), virtual_keyboard.KeyStatePressed); err != nil {
		return err
	}

	// Update modifier state if this is a modifier key
	if mod := evdevToXkbMod(evdevCode); mod != 0 {
		w.modsDepressed |= mod
		// Send modifiers event to update compositor's modifier state
		// This is REQUIRED by zwp_virtual_keyboard_v1 protocol for modifiers to work
		if err := w.keyboard.Modifiers(w.modsDepressed, 0, 0, 0); err != nil {
			w.logger.Debug("failed to send modifiers", "err", err)
		}
	}

	return nil
}

// KeyUpEvdev sends a key release event with a Linux evdev keycode.
func (w *WaylandInput) KeyUpEvdev(evdevCode int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.keyboard == nil {
		return nil
	}

	// Send key event
	if err := w.keyboard.Key(time.Now(), uint32(evdevCode), virtual_keyboard.KeyStateReleased); err != nil {
		return err
	}

	// Update modifier state if this is a modifier key
	if mod := evdevToXkbMod(evdevCode); mod != 0 {
		w.modsDepressed &^= mod // Clear the modifier bit
		// Send modifiers event to update compositor's modifier state
		if err := w.keyboard.Modifiers(w.modsDepressed, 0, 0, 0); err != nil {
			w.logger.Debug("failed to send modifiers", "err", err)
		}
	}

	return nil
}

// evdevToXkbMod converts an evdev keycode to an XKB modifier mask.
// Returns 0 if the key is not a modifier.
func evdevToXkbMod(evdevCode int) uint32 {
	switch evdevCode {
	case evdevLeftShift, evdevRightShift:
		return xkbModShift
	case evdevLeftCtrl, evdevRightCtrl:
		return xkbModCtrl
	case evdevLeftAlt, evdevRightAlt:
		return xkbModAlt
	case evdevLeftMeta, evdevRightMeta:
		return xkbModMod4
	default:
		return 0
	}
}

// KeyDown sends a key press event (VK code converted to evdev).
func (w *WaylandInput) KeyDown(vkCode uint16) error {
	evdevCode := VKToEvdev(vkCode)
	if evdevCode == 0 {
		w.logger.Debug("unknown VK code", "vk", vkCode)
		return nil
	}
	return w.KeyDownEvdev(evdevCode)
}

// KeyUp sends a key release event (VK code converted to evdev).
func (w *WaylandInput) KeyUp(vkCode uint16) error {
	evdevCode := VKToEvdev(vkCode)
	if evdevCode == 0 {
		return nil
	}
	return w.KeyUpEvdev(evdevCode)
}

// MouseMove moves the mouse by relative amounts.
func (w *WaylandInput) MouseMove(dx, dy int32) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	w.currentX += float64(dx)
	w.currentY += float64(dy)

	// Clamp to screen bounds
	if w.currentX < 0 {
		w.currentX = 0
	}
	if w.currentX >= float64(w.screenWidth) {
		w.currentX = float64(w.screenWidth) - 1
	}
	if w.currentY < 0 {
		w.currentY = 0
	}
	if w.currentY >= float64(w.screenHeight) {
		w.currentY = float64(w.screenHeight) - 1
	}

	w.pointer.MoveRelative(float64(dx), float64(dy))
	return nil
}

// MouseMoveAbsolute moves the mouse to absolute coordinates.
// x, y are in the range 0-1 representing normalized screen position.
// extentX, extentY are the logical screen dimensions (accounting for display scaling).
func (w *WaylandInput) MouseMoveAbsolute(x, y float64, extentX, extentY int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	// Convert normalized (0-1) to absolute pixel coordinates
	// The wlr-virtual-pointer-v1 protocol uses absolute coordinates with extent
	absX := uint32(x * float64(extentX))
	absY := uint32(y * float64(extentY))

	// Use true absolute positioning via MotionAbsolute
	// This avoids drift issues from relative movement + position tracking
	if err := w.pointer.MotionAbsolute(time.Now(), absX, absY, uint32(extentX), uint32(extentY)); err != nil {
		return err
	}
	return w.pointer.Frame()
}

// MouseButtonDown presses a mouse button.
// button: 1=left, 2=middle, 3=right
func (w *WaylandInput) MouseButtonDown(button int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	var btn uint32
	switch button {
	case 1:
		btn = virtual_pointer.BTN_LEFT
	case 2:
		btn = virtual_pointer.BTN_MIDDLE
	case 3:
		btn = virtual_pointer.BTN_RIGHT
	default:
		return nil
	}

	w.pointer.Button(time.Now(), btn, virtual_pointer.BUTTON_STATE_PRESSED)
	w.pointer.Frame()
	return nil
}

// MouseButtonUp releases a mouse button.
func (w *WaylandInput) MouseButtonUp(button int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	var btn uint32
	switch button {
	case 1:
		btn = virtual_pointer.BTN_LEFT
	case 2:
		btn = virtual_pointer.BTN_MIDDLE
	case 3:
		btn = virtual_pointer.BTN_RIGHT
	default:
		return nil
	}

	w.pointer.Button(time.Now(), btn, virtual_pointer.BUTTON_STATE_RELEASED)
	w.pointer.Frame()
	return nil
}

// MouseClick performs a complete click (down + up).
func (w *WaylandInput) MouseClick(button int) error {
	if err := w.MouseButtonDown(button); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
	return w.MouseButtonUp(button)
}

// MouseWheel sends a scroll event using Wayland axis protocol.
// deltaX/deltaY: values in browser pixels (from frontend WheelEvent).
//
// Uses AxisSourceFinger for smooth continuous scrolling. This is critical
// because Zed ignores wl_pointer.axis events when source is Wheel, but
// processes them for Finger source (trackpad scrolling).
//
// Every scroll event is sent immediately with no accumulation, ensuring
// small finger movements always result in immediate scroll response.
//
// Note: We don't send axis_stop events. They were causing Sway crashes when
// mixed with other pointer events (assertion failures in wlr_seat), and
// scrolling works correctly without them.
func (w *WaylandInput) MouseWheel(deltaX, deltaY float64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	if deltaX == 0 && deltaY == 0 {
		return nil
	}

	now := time.Now()

	// Use Finger source for smooth scrolling - Zed ignores Axis events
	// when source is Wheel, but processes them for Finger (trackpad)
	w.pointer.AxisSource(virtual_pointer.AxisSourceFinger)

	// Send continuous axis events with immediate values
	// Scale browser pixels to Wayland scroll units
	// Browser sends ~100-120 pixels per wheel notch, Wayland expects ~10-15 units per notch
	// Use scale of 0.15 to convert: 100 pixels → 15 units
	// Values stay as floats throughout (no integer truncation)
	if deltaY != 0 {
		w.pointer.Axis(now, virtual_pointer.AxisVertical, deltaY*0.15)
	}

	if deltaX != 0 {
		w.pointer.Axis(now, virtual_pointer.AxisHorizontal, deltaX*0.15)
	}

	w.pointer.Frame()
	return nil
}
