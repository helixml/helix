//go:build !cgo || !linux

// Package desktop provides Wayland cursor stubs when CGO is disabled or not on Linux.
package desktop

import (
	"context"
	"errors"
)

// WaylandCursorData represents cursor information from Wayland
type WaylandCursorData struct {
	InArea       bool
	PositionX    int32
	PositionY    int32
	HotspotX     int32
	HotspotY     int32
	BitmapWidth  uint32
	BitmapHeight uint32
	BitmapStride int32
	BitmapFormat uint32
	BitmapData   []byte
}

// WaylandCursorCallback is called when cursor changes
type WaylandCursorCallback func(cursor *WaylandCursorData)

// WaylandCursorClient is a stub when CGO is disabled.
type WaylandCursorClient struct{}

// NewWaylandCursorClient returns an error when CGO is disabled.
func NewWaylandCursorClient() (*WaylandCursorClient, error) {
	return nil, errors.New("Wayland cursor client requires CGO")
}

// SetCallback is a no-op when CGO is disabled.
func (w *WaylandCursorClient) SetCallback(callback WaylandCursorCallback) {}

// Run returns an error when CGO is disabled.
func (w *WaylandCursorClient) Run(ctx context.Context) error {
	return errors.New("Wayland cursor client requires CGO")
}

// Stop is a no-op when CGO is disabled.
func (w *WaylandCursorClient) Stop() {}

// Close is a no-op when CGO is disabled.
func (w *WaylandCursorClient) Close() {}
