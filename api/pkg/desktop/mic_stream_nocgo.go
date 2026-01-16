//go:build !cgo

// Package desktop provides microphone streaming stubs when CGO is disabled.
package desktop

import (
	"context"
	"log/slog"
)

// MicStreamer is a stub when CGO is disabled.
type MicStreamer struct{}

// MicConfig holds microphone streaming configuration
type MicConfig struct {
	SampleRate int
	Channels   int
}

// DefaultMicConfig returns sensible defaults for microphone streaming
func DefaultMicConfig() MicConfig {
	return MicConfig{
		SampleRate: 48000,
		Channels:   1,
	}
}

// NewMicStreamer returns nil when CGO is disabled.
func NewMicStreamer(logger *slog.Logger, config MicConfig) *MicStreamer {
	return nil
}

// Start is a no-op when CGO is disabled.
func (m *MicStreamer) Start(ctx context.Context) error {
	return nil
}

// Stop is a no-op when CGO is disabled.
func (m *MicStreamer) Stop() {}

// PushAudio is a no-op when CGO is disabled.
func (m *MicStreamer) PushAudio(data []byte) {}
