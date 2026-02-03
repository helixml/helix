//go:build !cgo || !linux

package desktop

import (
	"context"
	"errors"
)

type CursorData struct {
	ID           uint32
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

type CursorCallback func(cursor *CursorData)

type PipeWireCursorClient struct{}

func NewPipeWireCursorClient(pipeWireFd int) (*PipeWireCursorClient, error) {
	return nil, errors.New("PipeWire cursor client requires CGO on Linux")
}

func (p *PipeWireCursorClient) Connect(nodeID uint32, callback CursorCallback) error {
	return errors.New("PipeWire cursor client requires CGO on Linux")
}

func (p *PipeWireCursorClient) Run(ctx context.Context) {}

func (p *PipeWireCursorClient) Stop() {}

func (p *PipeWireCursorClient) Close() {}
