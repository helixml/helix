//go:build !cgo

// Package desktop provides GStreamer pipeline management stubs when CGO is disabled.
// The actual implementation in gst_pipeline.go requires CGO for go-gst bindings.
package desktop

import (
	"context"
	"errors"
	"time"
)

// ErrCGORequired is returned when GStreamer functions are called without CGO support.
var ErrCGORequired = errors.New("GStreamer support requires CGO")

// InitGStreamer is a no-op when CGO is disabled.
func InitGStreamer() {}

// VideoFrame represents a video frame from the GStreamer pipeline
type VideoFrame struct {
	Data       []byte    // H.264 NAL units (Annex B format with start codes)
	PTS        uint64    // Presentation timestamp in microseconds
	IsKeyframe bool      // True if this is an IDR frame
	Timestamp  time.Time // Wall clock time when frame was received
}

// GstPipelineOptions configures the GStreamer pipeline
type GstPipelineOptions struct {
	UseRealtimeClock bool
}

// GstPipeline wraps a GStreamer pipeline with appsink for video capture.
// This stub implementation always returns errors.
type GstPipeline struct {
	frameCh chan VideoFrame
}

// NewGstPipeline returns an error when CGO is disabled.
func NewGstPipeline(pipelineStr string) (*GstPipeline, error) {
	return nil, ErrCGORequired
}

// NewGstPipelineWithOptions returns an error when CGO is disabled.
func NewGstPipelineWithOptions(pipelineStr string, opts GstPipelineOptions) (*GstPipeline, error) {
	return nil, ErrCGORequired
}

// Start returns an error when CGO is disabled.
func (g *GstPipeline) Start(ctx context.Context) error {
	return ErrCGORequired
}

// Frames returns a nil channel when CGO is disabled.
func (g *GstPipeline) Frames() <-chan VideoFrame {
	return nil
}

// Stop is a no-op when CGO is disabled.
func (g *GstPipeline) Stop() {}

// IsRunning always returns false when CGO is disabled.
func (g *GstPipeline) IsRunning() bool {
	return false
}

// CheckGstElement always returns false when CGO is disabled.
func CheckGstElement(element string) bool {
	return false
}
