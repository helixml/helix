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

// GstPipeline wraps a GStreamer pipeline with appsink for video capture
type GstPipeline struct {
	pipeline   *gst.Pipeline
	appsink    *app.Sink
	frameCh    chan VideoFrame
	running    atomic.Bool
	stopOnce   sync.Once
	pipelineID string // For logging
}

// NewGstPipeline creates a new GStreamer pipeline from a pipeline string.
// The pipeline string must end with an appsink element named "videosink".
//
// Example pipeline:
//
//	pipewiresrc path=47 ! nvh264enc ! h264parse ! appsink name=videosink
func NewGstPipeline(pipelineStr string) (*GstPipeline, error) {
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
	// ClockTime.AsDuration() returns *time.Duration (nil if invalid)
	ptsDur := buffer.PresentationTimestamp().AsDuration()
	var pts uint64
	if ptsDur != nil {
		pts = uint64(ptsDur.Microseconds())
	}

	// Check if this is a keyframe
	// GST_BUFFER_FLAG_DELTA_UNIT is set for non-keyframes
	isKeyframe := !buffer.HasFlags(gst.BufferFlagDeltaUnit)

	frame := VideoFrame{
		Data:       data,
		PTS:        pts,
		IsKeyframe: isKeyframe,
		Timestamp:  time.Now(),
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
