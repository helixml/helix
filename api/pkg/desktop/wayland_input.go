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

	// Screen dimensions for absolute positioning
	screenWidth  int
	screenHeight int

	// Track current mouse position for absolute->relative conversion
	// Wayland virtual pointer only supports relative movement
	currentX float64
	currentY float64
	positionInitialized bool
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
// x, y are in the range 0-1 representing screen position.
func (w *WaylandInput) MouseMoveAbsolute(x, y float64, screenWidth, screenHeight int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	// Calculate target absolute position
	targetX := x * float64(screenWidth)
	targetY := y * float64(screenHeight)

	// Calculate relative movement from current position
	dx := targetX - w.currentX
	dy := targetY - w.currentY

	// Update tracked position
	w.currentX = targetX
	w.currentY = targetY

	// If this is the first movement, we need to initialize position
	// by moving from center of screen
	if !w.positionInitialized {
		// Start from center of screen
		centerX := float64(screenWidth) / 2
		centerY := float64(screenHeight) / 2
		dx = targetX - centerX
		dy = targetY - centerY
		w.positionInitialized = true
	}

	// Use relative movement (Wayland virtual pointer doesn't support absolute)
	if dx != 0 || dy != 0 {
		w.pointer.MoveRelative(dx, dy)
	}

	return nil
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

// MouseWheel sends a scroll event.
// deltaY: positive = scroll down, negative = scroll up
func (w *WaylandInput) MouseWheel(deltaX, deltaY float64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.pointer == nil {
		return nil
	}

	if deltaY != 0 {
		w.pointer.ScrollVertical(deltaY)
	}
	if deltaX != 0 {
		w.pointer.ScrollHorizontal(deltaX)
	}
	w.pointer.Frame()

	return nil
}
