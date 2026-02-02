package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"
)

// VideoEncoder handles video encoding using GStreamer with VideoToolbox
// Architecture for macOS:
//   Dev containers capture frames via PipeWire → output via WebSocket
//   Host receives frames → VideoToolbox H.264 encode → stream to browser
//
// Current implementation: proxy H.264 from containers (containers encode with x264)
// Future: containers output JPEG, host encodes with VideoToolbox
type VideoEncoder struct {
	ctx       context.Context
	cancel    context.CancelFunc
	videoPort int
	width     int
	height    int
	framerate int
	running   bool
	mu        sync.Mutex
	stats     EncoderStats
	pipeline  *exec.Cmd
	server    *VideoServer
}

// EncoderStats holds encoding statistics
type EncoderStats struct {
	FramesEncoded  uint64  `json:"frames_encoded"`
	FramesDropped  uint64  `json:"frames_dropped"`
	CurrentFPS     float64 `json:"current_fps"`
	AverageBitrate float64 `json:"average_bitrate"`
	LastFrameTime  int64   `json:"last_frame_time"`
	PipelineState  string  `json:"pipeline_state"`
}

// NewVideoEncoder creates a new video encoder
func NewVideoEncoder(width, height, framerate int) *VideoEncoder {
	return &VideoEncoder{
		width:     width,
		height:    height,
		framerate: framerate,
		stats: EncoderStats{
			PipelineState: "initialized",
		},
	}
}

// SetVideoServer sets the WebSocket server for broadcasting encoded frames
func (ve *VideoEncoder) SetVideoServer(server *VideoServer) {
	ve.server = server
}

// SetVideoPort sets the video stream port to connect to
func (ve *VideoEncoder) SetVideoPort(port int) {
	ve.videoPort = port
}

// Start starts the video encoder
func (ve *VideoEncoder) Start() error {
	ve.mu.Lock()
	if ve.running {
		ve.mu.Unlock()
		return fmt.Errorf("encoder already running")
	}
	ve.running = true
	ve.mu.Unlock()

	ve.ctx, ve.cancel = context.WithCancel(context.Background())

	// Don't start until video port is set
	if ve.videoPort == 0 {
		log.Println("Video encoder initialized, waiting for video port...")
		ve.stats.PipelineState = "waiting_for_connection"
		return nil
	}

	// For now, we just track stats - the actual video proxying happens
	// in the WebSocket server which forwards streams from containers
	ve.stats.PipelineState = "ready"
	log.Printf("Video encoder ready, will connect to video stream on port %d", ve.videoPort)

	return nil
}

// StartWithVideoPort starts the encoder with a specific video port
func (ve *VideoEncoder) StartWithVideoPort(videoPort int) error {
	ve.videoPort = videoPort

	ve.mu.Lock()
	if ve.running {
		ve.mu.Unlock()
		ve.stats.PipelineState = "ready"
		return nil
	}
	ve.running = true
	ve.mu.Unlock()

	ve.ctx, ve.cancel = context.WithCancel(context.Background())
	ve.stats.PipelineState = "ready"
	log.Printf("Video encoder ready, video stream port: %d", ve.videoPort)

	return nil
}

// Stop stops the video encoder
func (ve *VideoEncoder) Stop() error {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	if !ve.running {
		return nil
	}

	ve.running = false
	if ve.cancel != nil {
		ve.cancel()
	}

	if ve.pipeline != nil && ve.pipeline.Process != nil {
		ve.pipeline.Process.Kill()
	}

	ve.stats.PipelineState = "stopped"
	return nil
}

// GetStats returns encoder statistics
func (ve *VideoEncoder) GetStats() EncoderStats {
	return ve.stats
}

// UpdateStats updates the encoder statistics (called by video server)
func (ve *VideoEncoder) UpdateStats(framesEncoded uint64, fps float64) {
	ve.stats.FramesEncoded = framesEncoded
	ve.stats.CurrentFPS = fps
	ve.stats.LastFrameTime = time.Now().UnixMilli()
}

// CheckGStreamerDeps checks if required GStreamer elements are available
func CheckGStreamerDeps() map[string]bool {
	deps := make(map[string]bool)

	// Check for gst-launch-1.0
	_, err := exec.LookPath("gst-launch-1.0")
	deps["gstreamer"] = err == nil

	if !deps["gstreamer"] {
		return deps
	}

	// Check for VideoToolbox encoder (for future JPEG→H.264 transcoding)
	elements := []string{
		"vtenc_h264_hw",  // VideoToolbox H.264 hardware encoder
		"jpegdec",       // JPEG decoder (for JPEG→H.264)
		"videoconvert",  // Pixel format conversion
		"h264parse",     // H.264 parser
	}

	for _, elem := range elements {
		cmd := exec.Command("gst-inspect-1.0", elem)
		err := cmd.Run()
		deps[elem] = err == nil
	}

	return deps
}
