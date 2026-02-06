//go:build !linux

package desktop

import (
	"errors"
	"log/slog"
)

type WaylandInput struct{}

func NewWaylandInput(logger *slog.Logger, screenWidth, screenHeight int) (*WaylandInput, error) {
	return nil, errors.New("wayland input requires linux")
}

func (w *WaylandInput) Close() error {
	return errors.New("wayland input requires linux")
}

func (w *WaylandInput) KeyDownEvdev(evdevCode int) error {
	return errors.New("wayland input requires linux")
}

func (w *WaylandInput) KeyUpEvdev(evdevCode int) error {
	return errors.New("wayland input requires linux")
}

func (w *WaylandInput) MouseButtonDown(button int) error {
	return errors.New("wayland input requires linux")
}

func (w *WaylandInput) MouseButtonUp(button int) error {
	return errors.New("wayland input requires linux")
}

func (w *WaylandInput) MouseWheel(deltaX, deltaY float64) error {
	return errors.New("wayland input requires linux")
}

func (w *WaylandInput) MouseMoveAbsolute(normX, normY float64, logicalW, logicalH int) error {
	return errors.New("wayland input requires linux")
}

func (w *WaylandInput) MouseMove(dx, dy int32) error {
	return errors.New("wayland input requires linux")
}
