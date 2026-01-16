//go:build cgo

// Package desktop provides microphone audio playback from WebSocket to PipeWire/PulseAudio.
package desktop

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

// MicStreamer receives microphone audio from WebSocket and plays it on the desktop.
// Uses GStreamer with appsrc to inject audio into PulseAudio/PipeWire.
type MicStreamer struct {
	logger   *slog.Logger
	running  atomic.Bool
	cancel   context.CancelFunc
	mu       sync.Mutex

	// Audio config (from client)
	sampleRate int
	channels   int

	// GStreamer pipeline
	pipeline *gst.Pipeline
	appsrc   *app.Source
}

// MicConfig holds microphone streaming configuration
type MicConfig struct {
	SampleRate int // e.g., 48000
	Channels   int // e.g., 1 (mono) or 2 (stereo)
}

// DefaultMicConfig returns sensible defaults for microphone streaming
func DefaultMicConfig() MicConfig {
	return MicConfig{
		SampleRate: 48000,
		Channels:   1, // Mono mic is typical
	}
}

// NewMicStreamer creates a new microphone streamer
func NewMicStreamer(logger *slog.Logger, config MicConfig) *MicStreamer {
	return &MicStreamer{
		logger:     logger.With("component", "mic"),
		sampleRate: config.SampleRate,
		channels:   config.Channels,
	}
}

// Start begins the audio playback pipeline.
func (m *MicStreamer) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running.Load() {
		return nil
	}

	ctx, m.cancel = context.WithCancel(ctx)

	// Initialize GStreamer
	InitGStreamer()

	// Build pipeline: appsrc → audioconvert → audioresample → pulsesink
	// appsrc receives raw PCM from WebSocket
	// pulsesink plays to default audio output (PipeWire via pipewire-pulse)
	pipelineStr := m.buildPipelineString()
	m.logger.Info("starting mic playback pipeline", "pipeline", pipelineStr)

	pipeline, err := gst.NewPipelineFromString(pipelineStr)
	if err != nil {
		return err
	}
	m.pipeline = pipeline

	// Get appsrc element for pushing audio data
	srcElement, err := pipeline.GetElementByName("micsrc")
	if err != nil {
		return err
	}
	m.appsrc = app.SrcFromElement(srcElement)

	// Configure appsrc for streaming
	m.appsrc.SetProperty("format", gst.FormatTime)
	m.appsrc.SetProperty("is-live", true)
	m.appsrc.SetProperty("do-timestamp", true)

	// Set caps for raw audio
	caps := gst.NewCapsFromString(m.getCapsString())
	m.appsrc.SetProperty("caps", caps)

	// Start pipeline
	if err := pipeline.SetState(gst.StatePlaying); err != nil {
		return err
	}

	m.running.Store(true)
	m.logger.Info("mic playback started", "sampleRate", m.sampleRate, "channels", m.channels)

	// Monitor pipeline for errors
	go m.monitorPipeline(ctx)

	return nil
}

// Stop stops the mic playback pipeline
func (m *MicStreamer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running.Load() {
		return
	}

	if m.cancel != nil {
		m.cancel()
	}

	if m.appsrc != nil {
		m.appsrc.EndStream()
	}

	if m.pipeline != nil {
		m.pipeline.SetState(gst.StateNull)
		m.pipeline = nil
	}

	m.running.Store(false)
	m.logger.Info("mic playback stopped")
}

// PushAudio pushes raw PCM audio data to the playback pipeline.
// Data format: 16-bit signed little-endian PCM samples
func (m *MicStreamer) PushAudio(data []byte) {
	if !m.running.Load() || m.appsrc == nil {
		return
	}

	// Create GStreamer buffer from audio data
	buffer := gst.NewBufferFromBytes(data)

	// Push to appsrc
	ret := m.appsrc.PushBuffer(buffer)
	if ret != gst.FlowOK {
		m.logger.Debug("failed to push mic audio", "ret", ret)
	}
}

// buildPipelineString creates the GStreamer pipeline for mic playback
func (m *MicStreamer) buildPipelineString() string {
	// Pipeline: appsrc → audioconvert → audioresample → pulsesink
	// audioconvert handles format conversion if needed
	// audioresample handles sample rate conversion if needed
	// pulsesink plays to PulseAudio (or PipeWire via pipewire-pulse)
	return "appsrc name=micsrc ! audioconvert ! audioresample ! pulsesink sync=false"
}

// getCapsString returns the GStreamer caps string for the audio format
func (m *MicStreamer) getCapsString() string {
	// Raw PCM: 16-bit signed little-endian
	return "audio/x-raw,format=S16LE,rate=" + itoa(m.sampleRate) + ",channels=" + itoa(m.channels) + ",layout=interleaved"
}

// itoa converts int to string (avoiding strconv import for simple case)
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	idx := len(b)
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		idx--
		b[idx] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		idx--
		b[idx] = '-'
	}
	return string(b[idx:])
}

// monitorPipeline watches for pipeline errors
func (m *MicStreamer) monitorPipeline(ctx context.Context) {
	bus := m.pipeline.GetPipelineBus()
	defer bus.Unref()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg := bus.TimedPop(100 * 1000000) // 100ms timeout in nanoseconds
			if msg == nil {
				continue
			}

			switch msg.Type() {
			case gst.MessageError:
				err := msg.ParseError()
				m.logger.Error("mic pipeline error", "err", err)
				m.Stop()
				return
			case gst.MessageEOS:
				m.logger.Info("mic pipeline EOS")
				return
			}
		}
	}
}
