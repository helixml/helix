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
}

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

	return w.keyboard.Key(time.Now(), uint32(evdevCode), virtual_keyboard.KeyStatePressed)
}

// KeyUpEvdev sends a key release event with a Linux evdev keycode.
func (w *WaylandInput) KeyUpEvdev(evdevCode int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.keyboard == nil {
		return nil
	}

	return w.keyboard.Key(time.Now(), uint32(evdevCode), virtual_keyboard.KeyStateReleased)
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
// deltaY: positive = scroll down, negative = scroll up
// Uses AxisDiscrete for discrete scroll steps (required by apps like Zed)
// plus continuous Axis for smooth scrolling support.
func (w *WaylandInput) MouseWheel(deltaX, deltaY float64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	now := time.Now()

	// Set axis source to wheel (required for proper scroll handling)
	w.pointer.AxisSource(virtual_pointer.AxisSourceWheel)

	// Send vertical scroll with discrete steps
	// Discrete value is the number of 120ths of a wheel notch (Linux HID standard)
	// We calculate discrete from the delta: ~15 units = 1 wheel notch = 120 discrete
	if deltaY != 0 {
		discrete := int32(deltaY * 8) // Scale to discrete units
		w.pointer.AxisDiscrete(now, virtual_pointer.AxisVertical, deltaY, discrete)
	}

	// Send horizontal scroll with discrete steps
	if deltaX != 0 {
		discrete := int32(deltaX * 8)
		w.pointer.AxisDiscrete(now, virtual_pointer.AxisHorizontal, deltaX, discrete)
	}

	w.pointer.Frame()

	return nil
}
