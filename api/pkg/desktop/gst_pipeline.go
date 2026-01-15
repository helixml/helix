//go:build cgo

// Package desktop provides GStreamer pipeline management using go-gst bindings.
// This replaces the UDP-based gst-launch subprocess approach with native Go bindings
// for in-order frame delivery from appsink.
package desktop

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

// gstInitOnce ensures GStreamer is initialized only once
var gstInitOnce sync.Once

// InitGStreamer initializes the GStreamer library. Safe to call multiple times.
func InitGStreamer() {
	gstInitOnce.Do(func() {
		gst.Init(nil)
	})
}

// VideoFrame represents a video frame from the GStreamer pipeline
type VideoFrame struct {
	Data       []byte    // H.264 NAL units (Annex B format with start codes)
	PTS        uint64    // Presentation timestamp in microseconds
	IsKeyframe bool      // True if this is an IDR frame
	Timestamp  time.Time // Wall clock time when frame was received
}

// GstPipelineOptions configures the GStreamer pipeline
type GstPipelineOptions struct {
	// UseRealtimeClock forces the pipeline to use a realtime (wall clock) based clock.
	// When enabled, do-timestamp=true on source elements will produce PTS values
	// that are relative to pipeline start but based on wall clock time.
	// This is useful for latency measurement when comparing PTS to time.Now().
	UseRealtimeClock bool
}

// GstPipeline wraps a GStreamer pipeline with appsink for video capture
type GstPipeline struct {
	pipeline      *gst.Pipeline
	appsink       *app.Sink
	frameCh       chan VideoFrame
	running       atomic.Bool
	stopOnce      sync.Once
	pipelineID    string     // For logging
	realtimeClock *gst.Clock // Kept to prevent GC if we create a custom clock

	// baseTimeNs is the pipeline's base_time in nanoseconds since epoch (only valid with realtime clock).
	// Used to convert PTS (running time) to wall clock: captureTime = baseTimeNs + PTS
	baseTimeNs uint64
	// useRealtimeClock indicates if the pipeline is using a realtime clock for latency calculation
	useRealtimeClock bool
}

// NewGstPipeline creates a new GStreamer pipeline from a pipeline string.
// The pipeline string must end with an appsink element named "videosink".
//
// Example pipeline:
//
//	pipewiresrc path=47 ! nvh264enc ! h264parse ! appsink name=videosink
func NewGstPipeline(pipelineStr string) (*GstPipeline, error) {
	return NewGstPipelineWithOptions(pipelineStr, GstPipelineOptions{})
}

// NewGstPipelineWithOptions creates a new GStreamer pipeline with custom options.
// The pipeline string must end with an appsink element named "videosink".
func NewGstPipelineWithOptions(pipelineStr string, opts GstPipelineOptions) (*GstPipeline, error) {
	InitGStreamer()

	// Parse the pipeline string
	pipeline, err := gst.NewPipelineFromString(pipelineStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pipeline: %w", err)
	}

	// Get the appsink element
	elem, err := pipeline.GetElementByName("videosink")
	if err != nil {
		pipeline.SetState(gst.StateNull)
		return nil, fmt.Errorf("failed to get videosink element: %w", err)
	}

	appsink := app.SinkFromElement(elem)
	if appsink == nil {
		pipeline.SetState(gst.StateNull)
		return nil, fmt.Errorf("videosink element is not an appsink")
	}

	g := &GstPipeline{
		pipeline:   pipeline,
		appsink:    appsink,
		frameCh:    make(chan VideoFrame, 8), // Buffer a few frames
		pipelineID: fmt.Sprintf("gst-%p", pipeline),
	}

	// Force the pipeline to use a realtime clock if requested.
	// This makes do-timestamp=true use wall clock time instead of monotonic time,
	// enabling accurate latency measurement by comparing PTS to time.Now().
	if opts.UseRealtimeClock {
		clock, err := gst.NewSystemClock(gst.ClockTypeRealtime)
		if err != nil {
			pipeline.SetState(gst.StateNull)
			return nil, fmt.Errorf("failed to create realtime clock: %w", err)
		}
		pipeline.ForceClock(clock.Clock)
		g.realtimeClock = clock.Clock // Keep reference to prevent GC
		g.useRealtimeClock = true
		fmt.Printf("[GST_PIPELINE] Using realtime clock for wall clock timestamps\n")
	}

	return g, nil
}

// Start begins the pipeline and frame delivery.
// Frames can be received from the Frames() channel.
func (g *GstPipeline) Start(ctx context.Context) error {
	if g.running.Load() {
		return nil
	}

	// Configure appsink properties
	g.appsink.SetProperty("emit-signals", true)
	g.appsink.SetProperty("max-buffers", uint(2))
	g.appsink.SetProperty("drop", true)
	g.appsink.SetProperty("sync", false)

	// Set up the new-sample callback
	g.appsink.SetCallbacks(&app.SinkCallbacks{
		NewSampleFunc: g.onNewSample,
	})

	// Start the pipeline
	if err := g.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to set pipeline to playing: %w", err)
	}

	// Capture base_time when using realtime clock for PTS→wall clock conversion
	// base_time is the clock time (nanoseconds since epoch for realtime clock) when pipeline started
	if g.useRealtimeClock {
		baseTime := g.pipeline.GetBaseTime()
		g.baseTimeNs = uint64(baseTime)
		fmt.Printf("[GST_PIPELINE] Captured base_time: %d ns (epoch: %s)\n",
			g.baseTimeNs, time.Unix(0, int64(g.baseTimeNs)).Format(time.RFC3339Nano))
	}

	g.running.Store(true)

	// Monitor for EOS and errors
	go g.watchBus(ctx)

	return nil
}

// onNewSample is called when appsink has a new sample available
func (g *GstPipeline) onNewSample(sink *app.Sink) gst.FlowReturn {
	if !g.running.Load() {
		return gst.FlowEOS
	}

	sample := sink.PullSample()
	if sample == nil {
		return gst.FlowOK
	}

	buffer := sample.GetBuffer()
	if buffer == nil {
		return gst.FlowOK
	}

	// Map buffer to read data
	mapInfo := buffer.Map(gst.MapRead)
	if mapInfo == nil {
		return gst.FlowOK
	}
	defer buffer.Unmap()

	// Copy the data (buffer is only valid during this callback)
	data := make([]byte, len(mapInfo.Bytes()))
	copy(data, mapInfo.Bytes())

	// Get presentation timestamp (ClockTime is nanoseconds, convert to microseconds)
	// ClockTime.AsDuration() returns *time.Duration (nil if invalid/GST_CLOCK_TIME_NONE)
	// PTS = 0 is valid for the first frame, only nil is invalid
	ptsDur := buffer.PresentationTimestamp().AsDuration()
	var pts uint64
	var ptsNs int64
	if ptsDur != nil {
		pts = uint64(ptsDur.Microseconds())
		ptsNs = int64(*ptsDur) // Duration in nanoseconds
	}

	// Check if this is a keyframe
	// GST_BUFFER_FLAG_DELTA_UNIT is set for non-keyframes
	isKeyframe := !buffer.HasFlags(gst.BufferFlagDeltaUnit)

	// Calculate capture wall clock time for encoder latency measurement
	// There are two cases:
	// 1. pipewirezerocopysrc: PTS is wall clock nanoseconds since epoch (very large, ~1.7e18 for 2024)
	// 2. native pipewiresrc with realtime clock: baseTimeNs + PTS = wall clock
	// 3. Fallback: use time.Now() (appsink receive time)
	var captureTime time.Time
	// Check if PTS looks like wall clock (> year 2020 in nanoseconds = 1.577e18)
	const minWallClockNs = int64(1577836800000000000) // 2020-01-01 00:00:00 UTC
	if ptsNs > minWallClockNs {
		// PTS is already wall clock nanoseconds (from pipewirezerocopysrc)
		captureTime = time.Unix(0, ptsNs)
	} else if g.useRealtimeClock && g.baseTimeNs > 0 && ptsNs >= 0 {
		// PTS is running time, convert using base_time (from native pipewiresrc with realtime clock)
		captureTimeNs := int64(g.baseTimeNs) + ptsNs
		captureTime = time.Unix(0, captureTimeNs)
	} else {
		// Fallback: use current time (only measures Go processing time, not encoder latency)
		captureTime = time.Now()
	}

	frame := VideoFrame{
		Data:       data,
		PTS:        pts,
		IsKeyframe: isKeyframe,
		Timestamp:  captureTime,
	}

	// Non-blocking send to avoid blocking the GStreamer thread
	select {
	case g.frameCh <- frame:
	default:
		// Drop frame if channel is full (low latency preference)
	}

	return gst.FlowOK
}

// watchBus monitors the GStreamer bus for errors and EOS
func (g *GstPipeline) watchBus(ctx context.Context) {
	bus := g.pipeline.GetPipelineBus()
	if bus == nil {
		return
	}

	for g.running.Load() {
		select {
		case <-ctx.Done():
			g.Stop()
			return
		default:
		}

		// Poll with timeout to allow context checking
		// ClockTime is in nanoseconds, so 100ms = 100_000_000ns
		msg := bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
		if msg == nil {
			continue
		}

		switch msg.Type() {
		case gst.MessageEOS:
			g.Stop()
			return
		case gst.MessageError:
			gerr := msg.ParseError()
			if gerr != nil {
				// Log error but don't crash - let caller handle via Frames() closing
				fmt.Printf("[GST_PIPELINE] Error: %s\n", gerr.Error())
			}
			g.Stop()
			return
		case gst.MessageWarning:
			gwarn := msg.ParseWarning()
			if gwarn != nil {
				fmt.Printf("[GST_PIPELINE] Warning: %s\n", gwarn.Error())
			}
		case gst.MessageStateChanged:
			// Could log state changes if needed for debugging
		}
	}
}

// Frames returns a channel that receives video frames.
// The channel is closed when the pipeline stops.
func (g *GstPipeline) Frames() <-chan VideoFrame {
	return g.frameCh
}

// Stop stops the pipeline and closes the frame channel.
func (g *GstPipeline) Stop() {
	g.stopOnce.Do(func() {
		g.running.Store(false)

		if g.pipeline != nil {
			g.pipeline.SetState(gst.StateNull)
		}

		close(g.frameCh)
	})
}

// IsRunning returns whether the pipeline is currently running.
func (g *GstPipeline) IsRunning() bool {
	return g.running.Load()
}

// CheckGstElement checks if a GStreamer element is available.
// Returns true if the element factory exists.
func CheckGstElement(element string) bool {
	InitGStreamer()
	factory := gst.Find(element)
	return factory != nil
}
