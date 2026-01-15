//go:build cgo

// Package desktop provides audio streaming from PipeWire/PulseAudio to WebSocket.
// Uses Opus encoding for low-latency audio transmission.
package desktop

import (
	"context"
	"encoding/binary"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// AudioStreamer captures system audio and streams it via WebSocket.
// Uses GStreamer with pulsesrc (PipeWire PulseAudio compat) and opusenc.
type AudioStreamer struct {
	ws          *websocket.Conn
	wsMu        *sync.Mutex // Shared with VideoStreamer for WebSocket write serialization
	logger      *slog.Logger
	running     atomic.Bool
	cancel      context.CancelFunc
	mu          sync.Mutex

	// Audio config
	sampleRate  int
	channels    int

	// GStreamer pipeline
	gstPipeline *GstPipeline
}

// AudioConfig holds audio streaming configuration
type AudioConfig struct {
	SampleRate int // e.g., 48000
	Channels   int // e.g., 2 (stereo)
	Bitrate    int // Opus bitrate in kbps, e.g., 128
}

// DefaultAudioConfig returns sensible defaults for audio streaming
func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    128,
	}
}

// NewAudioStreamer creates a new audio streamer
func NewAudioStreamer(ws *websocket.Conn, wsMu *sync.Mutex, logger *slog.Logger, config AudioConfig) *AudioStreamer {
	return &AudioStreamer{
		ws:         ws,
		wsMu:       wsMu,
		logger:     logger.With("component", "audio"),
		sampleRate: config.SampleRate,
		channels:   config.Channels,
	}
}

// Start begins capturing and streaming audio.
func (a *AudioStreamer) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running.Load() {
		return nil
	}

	ctx, a.cancel = context.WithCancel(ctx)

	// Find the monitor device for capturing system audio output
	monitorDevice, err := a.findMonitorDevice()
	if err != nil {
		a.logger.Warn("failed to find audio monitor device, audio disabled", "err", err)
		return nil // Don't fail video streaming if audio isn't available
	}

	// Build GStreamer audio pipeline
	// pulsesrc captures from PulseAudio (or PipeWire via pipewire-pulse)
	// opusenc encodes to Opus (low-latency codec, widely supported)
	// appsink provides encoded frames to Go code
	pipelineStr := a.buildPipelineString(monitorDevice)
	a.logger.Info("starting audio pipeline", "pipeline", pipelineStr, "device", monitorDevice)

	pipeline, err := NewGstPipeline(pipelineStr)
	if err != nil {
		a.logger.Warn("failed to create audio pipeline", "err", err)
		return nil // Don't fail if audio isn't available
	}
	a.gstPipeline = pipeline

	if err := pipeline.Start(ctx); err != nil {
		a.logger.Warn("failed to start audio pipeline", "err", err)
		return nil
	}

	a.running.Store(true)

	// Start frame forwarding goroutine
	go a.forwardFrames(ctx)

	a.logger.Info("audio streaming started", "sampleRate", a.sampleRate, "channels", a.channels)
	return nil
}

// Stop stops audio streaming
func (a *AudioStreamer) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running.Load() {
		return
	}

	if a.cancel != nil {
		a.cancel()
	}

	if a.gstPipeline != nil {
		a.gstPipeline.Stop()
		a.gstPipeline = nil
	}

	a.running.Store(false)
	a.logger.Info("audio streaming stopped")
}

// buildPipelineString creates the GStreamer pipeline for audio capture
func (a *AudioStreamer) buildPipelineString(device string) string {
	// Pipeline: pulsesrc → audioconvert → audioresample → opusenc → appsink
	// opusenc produces Ogg Opus packets, but we want raw Opus for WebSocket
	// Use "application/x-rtp,media=audio,encoding-name=OPUS" for raw packets
	return strings.Join([]string{
		// Source: PulseAudio/PipeWire monitor device
		`pulsesrc device="` + device + `" do-timestamp=true`,
		// Convert to standard format
		"audioconvert",
		"audioresample",
		// Ensure consistent format for encoder
		`audio/x-raw,rate=48000,channels=2`,
		// Opus encoder - low latency settings
		// frame-size=10 gives 10ms frames (good for low latency)
		// bitrate in bits/sec
		"opusenc bitrate=128000 frame-size=10 audio-type=generic",
		// Parse Opus packets
		"opusparse",
		// Output to appsink (named "videosink" to match GstPipeline expectations)
		`appsink name=videosink emit-signals=true sync=false`,
	}, " ! ")
}

// findMonitorDevice finds the PulseAudio/PipeWire monitor device for capturing system audio
func (a *AudioStreamer) findMonitorDevice() (string, error) {
	// Use pactl to list sources and find the monitor
	// On PipeWire, this works via pipewire-pulse
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pactl", "list", "short", "sources")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Look for a monitor source (contains ".monitor")
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			sourceName := fields[1]
			if strings.Contains(sourceName, ".monitor") {
				a.logger.Debug("found audio monitor device", "device", sourceName)
				return sourceName, nil
			}
		}
	}

	// Fallback: try default monitor
	return "@DEFAULT_MONITOR@", nil
}

// forwardFrames reads audio frames from GStreamer and sends via WebSocket
func (a *AudioStreamer) forwardFrames(ctx context.Context) {
	frames := a.gstPipeline.Frames()

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			a.sendAudioFrame(frame.Data, frame.PTS)
		}
	}
}

// sendAudioFrame sends an audio frame via WebSocket
// Format: [msgType:1][channels:1][pts:8 BE][data:N]
func (a *AudioStreamer) sendAudioFrame(data []byte, pts uint64) {
	// Build message: type(1) + channels(1) + pts(8) + opus_data
	msg := make([]byte, 1+1+8+len(data))
	msg[0] = StreamMsgAudioFrame // 0x02
	msg[1] = uint8(a.channels)   // channels (2 for stereo)
	binary.BigEndian.PutUint64(msg[2:10], pts)
	copy(msg[10:], data)

	a.wsMu.Lock()
	err := a.ws.WriteMessage(websocket.BinaryMessage, msg)
	a.wsMu.Unlock()

	if err != nil {
		a.logger.Debug("failed to send audio frame", "err", err)
	}
}

// GetConfig returns the audio configuration for StreamInit message
func (a *AudioStreamer) GetConfig() (channels int, sampleRate int) {
	return a.channels, a.sampleRate
}
