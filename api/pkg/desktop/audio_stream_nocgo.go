//go:build !cgo

// Package desktop provides audio streaming stubs when CGO is disabled.
package desktop

import (
	"context"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

// AudioStreamer is a stub when CGO is disabled.
type AudioStreamer struct{}

// AudioConfig holds audio streaming configuration
type AudioConfig struct {
	SampleRate int
	Channels   int
	Bitrate    int
}

// DefaultAudioConfig returns sensible defaults for audio streaming
func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    128,
	}
}

// NewAudioStreamer returns nil when CGO is disabled.
func NewAudioStreamer(ws *websocket.Conn, wsMu *sync.Mutex, logger *slog.Logger, config AudioConfig) *AudioStreamer {
	return nil
}

// Start is a no-op when CGO is disabled.
func (a *AudioStreamer) Start(ctx context.Context) error {
	return nil
}

// Stop is a no-op when CGO is disabled.
func (a *AudioStreamer) Stop() {}

// GetConfig returns 0 channels (no audio) when CGO is disabled.
func (a *AudioStreamer) GetConfig() (channels int, sampleRate int) {
	return 0, 0
}
