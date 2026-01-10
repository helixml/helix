package desktop

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/bendahl/uinput"
)

// VirtualInput provides uinput-based keyboard and mouse input injection.
// This is used for Sway/wlroots compositors where D-Bus RemoteDesktop isn't available.
type VirtualInput struct {
	keyboard uinput.Keyboard
	mouse    uinput.Mouse
	logger   *slog.Logger
	mu       sync.Mutex
	closed   bool
}

// NewVirtualInput creates virtual keyboard and mouse devices via uinput.
// Requires /dev/uinput access (--privileged or appropriate device permissions).
func NewVirtualInput(logger *slog.Logger) (*VirtualInput, error) {
	keyboard, err := uinput.CreateKeyboard("/dev/uinput", []byte("helix-keyboard"))
	if err != nil {
		return nil, fmt.Errorf("create virtual keyboard: %w", err)
	}

	mouse, err := uinput.CreateMouse("/dev/uinput", []byte("helix-mouse"))
	if err != nil {
		keyboard.Close()
		return nil, fmt.Errorf("create virtual mouse: %w", err)
	}

	logger.Info("virtual input devices created")
	return &VirtualInput{
		keyboard: keyboard,
		mouse:    mouse,
		logger:   logger,
	}, nil
}

// Close releases the virtual input devices.
func (v *VirtualInput) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}
	v.closed = true

	var errs []error
	if err := v.keyboard.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close keyboard: %w", err))
	}
	if err := v.mouse.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close mouse: %w", err))
	}

	if len(errs) > 0 {
		return errs[0]
	}
	v.logger.Info("virtual input devices closed")
	return nil
}

// KeyDown sends a key press event.
// vkCode is a Windows Virtual Key code which gets converted to evdev.
// Deprecated: Use KeyDownEvdev for direct evdev codes.
func (v *VirtualInput) KeyDown(vkCode uint16) error {
	evdevCode := VKToEvdev(vkCode)
	if evdevCode == 0 {
		v.logger.Debug("unknown VK code", "vk", vkCode)
		return nil
	}
	return v.KeyDownEvdev(evdevCode)
}

// KeyUp sends a key release event.
// Deprecated: Use KeyUpEvdev for direct evdev codes.
func (v *VirtualInput) KeyUp(vkCode uint16) error {
	evdevCode := VKToEvdev(vkCode)
	if evdevCode == 0 {
		return nil
	}
	return v.KeyUpEvdev(evdevCode)
}

// KeyDownEvdev sends a key press event with a Linux evdev keycode.
func (v *VirtualInput) KeyDownEvdev(evdevCode int) error {
	if evdevCode == 0 {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}

	return v.keyboard.KeyDown(evdevCode)
}

// KeyUpEvdev sends a key release event with a Linux evdev keycode.
func (v *VirtualInput) KeyUpEvdev(evdevCode int) error {
	if evdevCode == 0 {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}

	return v.keyboard.KeyUp(evdevCode)
}

// KeyPress sends a complete key press and release.
func (v *VirtualInput) KeyPress(vkCode uint16) error {
	if err := v.KeyDown(vkCode); err != nil {
		return err
	}
	return v.KeyUp(vkCode)
}

// MouseMove moves the mouse by relative amounts.
func (v *VirtualInput) MouseMove(dx, dy int32) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}

	return v.mouse.Move(dx, dy)
}

// MouseMoveAbsolute moves the mouse to absolute coordinates.
// x, y are in the range 0-1 representing screen position.
func (v *VirtualInput) MouseMoveAbsolute(x, y float64, screenWidth, screenHeight int) error {
	// uinput mouse doesn't support absolute positioning directly,
	// so we'd need to track current position and compute relative move.
	// For now, just log a warning - absolute mouse needs a touchscreen/tablet device.
	v.logger.Debug("absolute mouse move not fully supported in uinput mode",
		"x", x, "y", y)
	return nil
}

// MouseButtonDown presses a mouse button.
// button: 1=left, 2=middle, 3=right
func (v *VirtualInput) MouseButtonDown(button int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}

	switch button {
	case 1:
		return v.mouse.LeftPress()
	case 2:
		return v.mouse.MiddlePress()
	case 3:
		return v.mouse.RightPress()
	default:
		return nil
	}
}

// MouseButtonUp releases a mouse button.
func (v *VirtualInput) MouseButtonUp(button int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}

	switch button {
	case 1:
		return v.mouse.LeftRelease()
	case 2:
		return v.mouse.MiddleRelease()
	case 3:
		return v.mouse.RightRelease()
	default:
		return nil
	}
}

// MouseClick performs a complete click (down + up).
func (v *VirtualInput) MouseClick(button int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}

	switch button {
	case 1:
		return v.mouse.LeftClick()
	case 2:
		return v.mouse.MiddleClick()
	case 3:
		return v.mouse.RightClick()
	default:
		return nil
	}
}

// MouseWheel sends a scroll event.
// deltaY: positive = scroll up, negative = scroll down
func (v *VirtualInput) MouseWheel(deltaX, deltaY float64) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}

	// uinput wheel expects discrete steps
	if deltaY > 0 {
		return v.mouse.Wheel(false, int32(deltaY))
	} else if deltaY < 0 {
		return v.mouse.Wheel(true, int32(-deltaY))
	}
	return nil
}
