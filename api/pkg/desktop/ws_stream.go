// Package desktop provides WebSocket video streaming using GStreamer and PipeWire.
package desktop

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// VideoMode controls the video capture pipeline mode
// Set via HELIX_VIDEO_MODE environment variable
type VideoMode string

const (
	// VideoModeSHM uses shared memory (current default, 2 CPU copies)
	// Pipeline: pipewiresrc → shmsink → shmsrc → cudaupload → nvh264enc
	VideoModeSHM VideoMode = "shm"

	// VideoModeNative uses native GStreamer DMA-BUF → CUDA (if supported)
	// Pipeline: pipewiresrc → video/x-raw(memory:DMABuf) → cudaupload → nvh264enc
	// Requires GStreamer 1.24+ with DMA-BUF CUDA support
	VideoModeNative VideoMode = "native"

	// VideoModeZeroCopy uses pipewirezerocopysrc plugin (true zero-copy)
	// Pipeline: pipewirezerocopysrc → video/x-raw(memory:CUDAMemory) → nvh264enc
	// Requires gst-plugin-pipewire-zerocopy to be installed
	VideoModeZeroCopy VideoMode = "zerocopy"
)

// getVideoMode returns the configured video mode
// If configOverride is provided (non-empty), it takes precedence over env var
func getVideoMode(configOverride string) VideoMode {
	mode := configOverride
	if mode == "" {
		mode = os.Getenv("HELIX_VIDEO_MODE")
	}
	switch strings.ToLower(mode) {
	case "native", "dmabuf":
		return VideoModeNative
	case "zerocopy", "zero-copy", "plugin":
		return VideoModeZeroCopy
	case "shm", "":
		return VideoModeSHM
	default:
		return VideoModeSHM
	}
}

// Binary message types for streaming protocol (matching frontend websocket-stream.ts)
const (
	StreamMsgVideoFrame  = 0x01
	StreamMsgAudioFrame  = 0x02
	StreamMsgVideoBatch  = 0x03
	StreamMsgStreamInit  = 0x30
	StreamMsgStreamError = 0x31
	StreamMsgPing        = 0x40
	StreamMsgPong        = 0x41
	// Input message types (for reference, handled separately)
	StreamMsgKeyboard      = 0x10
	StreamMsgMouseClick    = 0x11
	StreamMsgMouseAbsolute = 0x12
	StreamMsgMouseRelative  = 0x13
	StreamMsgTouch          = 0x14
	StreamMsgControllerEvent = 0x15
	StreamMsgControllerState = 0x16
	StreamMsgControlMessage  = 0x20
)

// Video codec types
const (
	StreamCodecH264     = 0x01
	StreamCodecH264High = 0x02
	StreamCodecH265     = 0x10
)

// StreamConfig holds the stream configuration received from client
type StreamConfig struct {
	Type                  string `json:"type"`
	HostID                int    `json:"host_id"`
	AppID                 int    `json:"app_id"`
	SessionID             string `json:"session_id"`
	Width                 int    `json:"width"`
	Height                int    `json:"height"`
	FPS                   int    `json:"fps"`
	Bitrate               int    `json:"bitrate"`
	PacketSize            int    `json:"packet_size"`
	PlayAudioLocal        bool   `json:"play_audio_local"`
	VideoSupportedFormats int    `json:"video_supported_formats"`
	ClientUniqueID        string `json:"client_unique_id,omitempty"`
	// VideoMode overrides the HELIX_VIDEO_MODE env var for this stream
	// Valid values: "shm", "native", "zerocopy" (default: from env or "shm")
	VideoMode string `json:"video_mode,omitempty"`
}

// VideoStreamer captures video from PipeWire and streams to WebSocket
// Uses RTP over UDP for reliable frame boundaries (matching Rust implementation).
type VideoStreamer struct {
	nodeID        uint32
	shmSocketPath string    // If set, use shmsrc instead of pipewiresrc
	videoMode     VideoMode // Video capture mode (shm, native, zerocopy)
	config        StreamConfig
	ws            *websocket.Conn
	logger        *slog.Logger
	cmd           *exec.Cmd
	running       atomic.Bool
	cancel        context.CancelFunc
	mu            sync.Mutex

	// RTP over UDP - provides natural packet framing like Rust WebRTC mode
	rtpPort  int           // UDP port for RTP packets
	rtpConn  *net.UDPConn  // UDP listener for RTP packets
	rtpDepack *H264Depacketizer // RTP H.264 depacketizer

	// Frame tracking
	frameCount uint64
	startTime  time.Time

	// Video pause control (for screenshot mode switching)
	videoEnabled atomic.Bool
}

// NewVideoStreamer creates a new video streamer
func NewVideoStreamer(nodeID uint32, config StreamConfig, ws *websocket.Conn, logger *slog.Logger) *VideoStreamer {
	v := &VideoStreamer{
		nodeID:    nodeID,
		videoMode: getVideoMode(config.VideoMode),
		config:    config,
		ws:        ws,
		logger:    logger,
	}
	v.videoEnabled.Store(true) // Video enabled by default
	return v
}

// NewVideoStreamerWithSHM creates a video streamer that reads from shared memory
// This is used when a video forwarder is running to avoid PipeWire node conflicts
func NewVideoStreamerWithSHM(shmSocketPath string, config StreamConfig, ws *websocket.Conn, logger *slog.Logger) *VideoStreamer {
	v := &VideoStreamer{
		shmSocketPath: shmSocketPath,
		videoMode:     getVideoMode(config.VideoMode),
		config:        config,
		ws:            ws,
		logger:        logger,
	}
	v.videoEnabled.Store(true)
	return v
}

// SetVideoEnabled controls video frame sending (for screenshot mode switching)
func (v *VideoStreamer) SetVideoEnabled(enabled bool) {
	v.videoEnabled.Store(enabled)
	v.logger.Info("video streaming", "enabled", enabled)
}

// Start begins capturing and streaming video
// Uses RTP over UDP for reliable frame boundaries (matching Rust implementation).
func (v *VideoStreamer) Start(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running.Load() {
		return nil
	}

	ctx, v.cancel = context.WithCancel(ctx)
	v.startTime = time.Now()

	// Create UDP listener for RTP packets
	// Use port 0 to let OS assign an available port
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("resolve UDP address: %w", err)
	}
	v.rtpConn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("create UDP listener: %w", err)
	}
	v.rtpPort = v.rtpConn.LocalAddr().(*net.UDPAddr).Port
	v.rtpDepack = NewH264Depacketizer()

	// Determine encoder based on available hardware
	encoder := v.selectEncoder()

	// Build GStreamer pipeline args (now outputs RTP to UDP)
	pipelineArgs := v.buildPipelineArgs(encoder)

	// Log with appropriate source info
	sourceInfo := fmt.Sprintf("pipewire:%d", v.nodeID)
	if v.shmSocketPath != "" {
		sourceInfo = fmt.Sprintf("shm:%s", v.shmSocketPath)
	}
	v.logger.Info("starting video capture",
		"source", sourceInfo,
		"video_mode", string(v.videoMode),
		"encoder", encoder,
		"resolution", fmt.Sprintf("%dx%d", v.config.Width, v.config.Height),
		"fps", v.config.FPS,
		"bitrate", v.config.Bitrate,
		"rtp_port", v.rtpPort,
		"pipeline", strings.Join(pipelineArgs, " "),
	)

	// Start gst-launch with pipeline args
	args := append([]string{"-q"}, pipelineArgs...)
	v.cmd = exec.CommandContext(ctx, "gst-launch-1.0", args...)

	// Capture stderr for debugging
	v.cmd.Stderr = os.Stderr

	if err := v.cmd.Start(); err != nil {
		v.rtpConn.Close()
		return fmt.Errorf("start gst-launch: %w", err)
	}

	v.running.Store(true)

	// Send StreamInit to client (binary protocol)
	if err := v.sendStreamInit(); err != nil {
		v.logger.Error("failed to send StreamInit", "err", err)
	}

	// Send ConnectionComplete to signal frontend that connection is ready
	if err := v.sendConnectionComplete(); err != nil {
		v.logger.Error("failed to send ConnectionComplete", "err", err)
	}

	// Read RTP packets from UDP and send complete Access Units to WebSocket
	go v.readRTPAndSend(ctx)

	// Handle ping/pong
	go v.heartbeat(ctx)

	return nil
}

// selectEncoder chooses the best available encoder
// Priority order:
// 1. NVIDIA NVENC (nvh264enc) - fastest, lowest latency
// 2. Intel QSV (qsvh264enc) - Intel Quick Sync Video
// 3. VA-API (vah264enc) - Intel/AMD VA-API
// 4. VA-API LP (vah264lpenc) - Intel/AMD VA-API Low Power mode
// 5. x264 (x264enc) - software fallback
func (v *VideoStreamer) selectEncoder() string {
	// Try NVENC first (NVIDIA)
	if checkGstElement("nvh264enc") {
		v.logger.Info("using NVIDIA NVENC encoder")
		return "nvenc"
	}

	// Try Intel QSV (Quick Sync Video)
	if checkGstElement("qsvh264enc") {
		v.logger.Info("using Intel QSV encoder")
		return "qsv"
	}

	// Try VA-API (Intel/AMD)
	if checkGstElement("vah264enc") {
		v.logger.Info("using VA-API encoder")
		return "vaapi"
	}

	// Try VA-API Low Power mode (some Intel chips)
	if checkGstElement("vah264lpenc") {
		v.logger.Info("using VA-API Low Power encoder")
		return "vaapi-lp"
	}

	// Fallback to software
	v.logger.Info("using software x264 encoder (no hardware encoder found)")
	return "x264"
}

// checkGstElement checks if a GStreamer element is available
func checkGstElement(element string) bool {
	cmd := exec.Command("gst-inspect-1.0", element)
	return cmd.Run() == nil
}

// buildPipelineArgs creates GStreamer pipeline arguments as a flat slice
//
// Video modes (HELIX_VIDEO_MODE env var):
// - shm: Current default, uses pipewiresrc → system memory → encoder (1-2 CPU copies)
// - native: Uses pipewiresrc with DMA-BUF → encoder (GStreamer 1.24+, fewer copies)
// - zerocopy: Uses pipewirezerocopysrc plugin → CUDA/DMABuf memory (0 CPU copies, requires plugin)
//
// GPU optimization notes:
// - NVIDIA: cudaupload gets frames into CUDA memory, nvh264enc does colorspace on GPU
// - AMD/Intel VA-API: vah264enc handles GPU upload internally
// - Software: CPU-based videoconvert + x264enc (slowest fallback)
func (v *VideoStreamer) buildPipelineArgs(encoder string) []string {
	var args []string

	// Step 1: Build source section based on video mode
	// Note: shmSocketPath (for video forwarder) takes precedence over videoMode
	if v.shmSocketPath != "" {
		// Use shmsrc to read from video forwarder's shared memory socket
		// This avoids PipeWire node conflicts when the forwarder is running
		args = []string{
			"shmsrc", fmt.Sprintf("socket-path=%s", v.shmSocketPath), "is-live=true", "do-timestamp=true",
			"!", fmt.Sprintf("video/x-raw,format=BGRx,width=%d,height=%d,framerate=0/1",
				v.config.Width, v.config.Height),
		}
	} else {
		// Choose source based on video mode
		switch v.videoMode {
		case VideoModeZeroCopy:
			// pipewirezerocopysrc: True zero-copy via EGL→CUDA interop
			// Outputs video/x-raw(memory:CUDAMemory) for NVIDIA or DMABuf for AMD/Intel
			// Requires gst-plugin-pipewire-zerocopy to be installed
			args = []string{
				"pipewirezerocopysrc",
				fmt.Sprintf("pipewire-node-id=%d", v.nodeID),
				"output-mode=auto", // auto-detect CUDA or DMABuf
				"keepalive-time=100", // GNOME 49+ damage-based ScreenCast support
			}
			// For NVIDIA, output is already in CUDA memory - no cudaupload needed
			// For AMD/Intel, output is DMABuf which VA-API can accept directly

		case VideoModeNative:
			// Native DMA-BUF path: pipewiresrc negotiates DMA-BUF with compositor
			// Works on GStreamer 1.24+ with proper driver support
			// Falls back gracefully to system memory if DMA-BUF unavailable
			args = []string{
				"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
				// Let pipewiresrc negotiate best format - prefer DMA-BUF if available
				"!", "video/x-raw",
			}

		default: // VideoModeSHM
			// Standard pipewiresrc path - most compatible
			args = []string{
				"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
				"!", "video/x-raw,format=BGRx",
			}
		}
	}

	// Step 2: Add encoder-specific conversion and encoding pipeline
	// Each encoder type has its own GPU-optimized path
	// Note: zerocopy mode already provides GPU memory (CUDA or DMABuf)
	switch encoder {
	case "nvenc":
		// NVIDIA NVENC encoding
		// For zerocopy mode: frames already in CUDA memory, skip cudaupload
		// For other modes: use cudaupload to get frames into CUDA memory
		if v.videoMode == VideoModeZeroCopy && v.shmSocketPath == "" {
			// pipewirezerocopysrc outputs CUDA memory directly
			args = append(args,
				"!", "cudaconvertscale", // GPU-side scaling + format conversion
				"!", fmt.Sprintf("video/x-raw(memory:CUDAMemory),width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				"!", "nvh264enc",
				"preset=low-latency-hq",
				"zerolatency=true",
				"gop-size=15",
				"rc-mode=cbr-ld-hq",
				fmt.Sprintf("bitrate=%d", v.config.Bitrate),
				"aud=false",
			)
		} else {
			// Standard path: system memory → cudaupload → nvh264enc
			args = append(args,
				"!", "videorate",
				"!", "videoscale",
				"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				"!", "cudaupload",
				"!", "nvh264enc",
				"preset=low-latency-hq",
				"zerolatency=true",
				"gop-size=15",
				"rc-mode=cbr-ld-hq",
				fmt.Sprintf("bitrate=%d", v.config.Bitrate),
				"aud=false",
			)
		}

	case "qsv":
		// Intel Quick Sync Video
		// QSV has its own memory system, uses CPU conversion for now
		// TODO: Use qsvvpp for GPU-side conversion when available
		args = append(args,
			"!", "videoconvert",
			"!", "videoscale",
			"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
				v.config.Width, v.config.Height, v.config.FPS),
			"!", "qsvh264enc",
			"b-frames=0",
			"gop-size=15",
			"idr-interval=1",
			"ref-frames=1",
			fmt.Sprintf("bitrate=%d", v.config.Bitrate),
			"rate-control=cbr",
			"target-usage=6",
		)

	case "vaapi":
		// AMD/Intel VA-API pipeline
		// For zerocopy/native modes: source may provide DMABuf which VA-API can use directly
		// For shm mode: uses CPU-based videoconvert (vapostproc not widely available)
		if (v.videoMode == VideoModeZeroCopy || v.videoMode == VideoModeNative) && v.shmSocketPath == "" {
			// Try to use vapostproc for GPU-side processing (available on newer systems)
			// Falls back gracefully if vapostproc not available
			args = append(args,
				"!", "vapostproc",
				"!", fmt.Sprintf("video/x-raw(memory:VAMemory),width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				"!", "vah264enc",
				"aud=false",
				"b-frames=0",
				"ref-frames=1",
				fmt.Sprintf("bitrate=%d", v.config.Bitrate),
				fmt.Sprintf("cpb-size=%d", v.config.Bitrate),
				"key-int-max=1024",
				"rate-control=cqp",
				"target-usage=6",
			)
		} else {
			// Standard path: CPU videoconvert
			args = append(args,
				"!", "videoconvert",
				"!", "videoscale",
				"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				"!", "vah264enc",
				"aud=false",
				"b-frames=0",
				"ref-frames=1",
				fmt.Sprintf("bitrate=%d", v.config.Bitrate),
				fmt.Sprintf("cpb-size=%d", v.config.Bitrate),
				"key-int-max=1024",
				"rate-control=cqp",
				"target-usage=6",
			)
		}

	case "vaapi-lp":
		// VA-API Low Power mode (Intel-specific)
		if (v.videoMode == VideoModeZeroCopy || v.videoMode == VideoModeNative) && v.shmSocketPath == "" {
			args = append(args,
				"!", "vapostproc",
				"!", fmt.Sprintf("video/x-raw(memory:VAMemory),width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				"!", "vah264lpenc",
				"aud=false",
				"b-frames=0",
				"ref-frames=1",
				fmt.Sprintf("bitrate=%d", v.config.Bitrate),
				fmt.Sprintf("cpb-size=%d", v.config.Bitrate),
				"key-int-max=1024",
				"rate-control=cqp",
				"target-usage=6",
			)
		} else {
			args = append(args,
				"!", "videoconvert",
				"!", "videoscale",
				"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				"!", "vah264lpenc",
				"aud=false",
				"b-frames=0",
				"ref-frames=1",
				fmt.Sprintf("bitrate=%d", v.config.Bitrate),
				fmt.Sprintf("cpb-size=%d", v.config.Bitrate),
				"key-int-max=1024",
				"rate-control=cqp",
				"target-usage=6",
			)
		}

	default:
		// Software x264 fallback - CPU-based conversion
		// This is the slowest path but works on any system
		args = append(args,
			"!", "videoconvert",
			"!", "videoscale",
			"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
				v.config.Width, v.config.Height, v.config.FPS),
			"!", "x264enc",
			"pass=qual",
			"tune=zerolatency",
			"speed-preset=superfast",
			"b-adapt=false",
			"bframes=0",
			"ref=1",
			fmt.Sprintf("bitrate=%d", v.config.Bitrate),
			"aud=false",
		)
	}

	// Step 3: Add h264parse and RTP output
	// h264parse with config-interval=-1 inserts SPS/PPS before every keyframe
	// rtph264pay creates RTP packets with marker bits for frame boundaries
	// udpsink sends to our local UDP listener for clean packet framing
	args = append(args,
		"!", "h264parse", "config-interval=-1",
		"!", "video/x-h264,stream-format=byte-stream,alignment=au",
		"!", "rtph264pay", "pt=96", "mtu=65000", // Large MTU to minimize fragmentation
		"!", "udpsink", "host=127.0.0.1", fmt.Sprintf("port=%d", v.rtpPort), "sync=false",
	)

	return args
}

// sendStreamInit sends the initialization message to client
func (v *VideoStreamer) sendStreamInit() error {
	// StreamInit format: type(1) + codec(1) + width(2) + height(2) + fps(1) + audio_channels(1) + sample_rate(4) + touch(1)
	msg := make([]byte, 13)
	msg[0] = StreamMsgStreamInit
	msg[1] = StreamCodecH264 // We always encode H.264 for now
	binary.BigEndian.PutUint16(msg[2:4], uint16(v.config.Width))
	binary.BigEndian.PutUint16(msg[4:6], uint16(v.config.Height))
	msg[6] = byte(v.config.FPS)
	msg[7] = 0                               // audio channels (not implemented yet)
	binary.BigEndian.PutUint32(msg[8:12], 0) // sample rate
	msg[12] = 0                              // touch supported

	return v.ws.WriteMessage(websocket.BinaryMessage, msg)
}

// connectionCompleteMsg is the JSON structure expected by frontend websocket-stream.ts
type connectionCompleteMsg struct {
	ConnectionComplete struct {
		Capabilities struct {
			Touch bool `json:"touch"`
		} `json:"capabilities"`
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"ConnectionComplete"`
}

// sendConnectionComplete sends the JSON control message to signal connection is ready
func (v *VideoStreamer) sendConnectionComplete() error {
	msg := connectionCompleteMsg{}
	msg.ConnectionComplete.Capabilities.Touch = false
	msg.ConnectionComplete.Width = v.config.Width
	msg.ConnectionComplete.Height = v.config.Height
	return v.ws.WriteJSON(msg)
}

// readRTPAndSend reads RTP packets from UDP and sends complete Access Units to WebSocket.
// This approach matches the Rust implementation where RTP marker bits indicate frame boundaries.
// Much more reliable than manual NAL parsing of raw byte streams.
func (v *VideoStreamer) readRTPAndSend(ctx context.Context) {
	defer v.Stop()

	// Large buffer for RTP packets - 65KB covers most cases
	buf := make([]byte, 65536)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline to allow checking context cancellation
		v.rtpConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, _, err := v.rtpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout - check context and try again
			}
			v.logger.Error("UDP read error", "err", err)
			return
		}

		if n == 0 {
			continue
		}

		// Process RTP packet through depacketizer
		accessUnit, isKeyframe, complete, err := v.rtpDepack.ProcessPacket(buf[:n])
		if err != nil {
			v.logger.Warn("RTP depacketization error", "err", err)
			continue
		}

		// When we have a complete Access Unit, send it
		if complete && len(accessUnit) > 0 {
			if err := v.sendVideoFrame(accessUnit, isKeyframe); err != nil {
				v.logger.Error("send frame error", "err", err)
				return
			}
		}
	}
}

// sendVideoFrame sends a video frame to the WebSocket
// isKeyframe should be true for Access Units containing SPS+PPS+IDR
func (v *VideoStreamer) sendVideoFrame(data []byte, isKeyframe bool) error {
	// Skip sending if video is paused (screenshot mode)
	if !v.videoEnabled.Load() {
		return nil
	}

	v.frameCount++
	pts := uint64(time.Since(v.startTime).Microseconds())

	// VideoFrame format: type(1) + codec(1) + flags(1) + pts(8) + width(2) + height(2) + data(...)
	header := make([]byte, 15)
	header[0] = StreamMsgVideoFrame
	header[1] = StreamCodecH264
	if isKeyframe {
		header[2] = 0x01 // keyframe flag
	}
	binary.BigEndian.PutUint64(header[3:11], pts)
	binary.BigEndian.PutUint16(header[11:13], uint16(v.config.Width))
	binary.BigEndian.PutUint16(header[13:15], uint16(v.config.Height))

	// Combine header and payload
	msg := make([]byte, 15+len(data))
	copy(msg[:15], header)
	copy(msg[15:], data)

	return v.ws.WriteMessage(websocket.BinaryMessage, msg)
}

// heartbeat sends periodic WebSocket pings to keep connection alive
// Uses WebSocket's built-in ping/pong mechanism, not binary protocol messages.
// The binary Ping (0x40) / Pong (0x41) messages are for client-initiated RTT measurement.
func (v *VideoStreamer) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Use WebSocket ping frame, not binary message
			if err := v.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				v.logger.Debug("ping failed", "err", err)
				return
			}
		}
	}
}

// Stop stops the video capture
func (v *VideoStreamer) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.running.Load() {
		return
	}

	v.running.Store(false)

	if v.cancel != nil {
		v.cancel()
	}

	// Close UDP listener for RTP
	if v.rtpConn != nil {
		v.rtpConn.Close()
		v.rtpConn = nil
	}

	if v.cmd != nil && v.cmd.Process != nil {
		v.cmd.Process.Kill()
	}

	v.logger.Info("video capture stopped",
		"frames", v.frameCount,
		"duration", time.Since(v.startTime),
	)
}

// HandleStreamWebSocket handles the /ws/stream endpoint (standalone version)
// This version is for backwards compatibility when Server is not available.
func HandleStreamWebSocket(w http.ResponseWriter, r *http.Request, nodeID uint32, logger *slog.Logger) {
	handleStreamWebSocketInternal(w, r, nodeID, logger, nil)
}

// handleStreamWebSocketWithServer handles the /ws/stream endpoint with Server access for input
func (s *Server) handleStreamWebSocketWithServer(w http.ResponseWriter, r *http.Request) {
	handleStreamWebSocketInternal(w, r, s.nodeID, s.logger, s)
}

// handleStreamWebSocketInternal is the shared implementation
func handleStreamWebSocketInternal(w http.ResponseWriter, r *http.Request, nodeID uint32, logger *slog.Logger, server *Server) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  16 * 1024,
		WriteBufferSize: 256 * 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("websocket upgrade failed", "err", err)
		return
	}
	defer ws.Close()

	logger.Info("stream WebSocket connected", "remote", r.RemoteAddr)

	// Wait for init message from client
	// Like the Rust implementation, we skip any binary messages that arrive before the JSON init
	// This handles clients that may send ping or other binary messages before init
	var config StreamConfig
	initReceived := false
	for !initReceived {
		messageType, msg, err := ws.ReadMessage()
		if err != nil {
			logger.Error("failed to read init message", "err", err)
			return
		}

		// Skip binary messages (client may send ping before init)
		if messageType == websocket.BinaryMessage {
			logger.Debug("skipping binary message while waiting for init", "len", len(msg))
			continue
		}

		// Parse JSON init message
		if err := json.Unmarshal(msg, &config); err != nil {
			logger.Error("failed to parse init message", "err", err, "msg", string(msg))
			return
		}

		if config.Type != "init" {
			logger.Error("expected init message", "got", config.Type)
			return
		}

		initReceived = true
	}

	logger.Info("stream init received",
		"width", config.Width,
		"height", config.Height,
		"fps", config.FPS,
		"bitrate", config.Bitrate,
	)

	// Create video streamer - prefer SHM source if video forwarder is running
	// This avoids PipeWire node conflicts (forwarder already claimed the ScreenCast node)
	var streamer *VideoStreamer
	if server != nil && server.videoForwarder != nil && server.videoForwarder.IsRunning() {
		shmSocketPath := server.videoForwarder.ShmSocketPath()
		logger.Info("using SHM video source (forwarder running)", "socket", shmSocketPath)
		streamer = NewVideoStreamerWithSHM(shmSocketPath, config, ws, logger)
	} else {
		logger.Info("using PipeWire video source", "node_id", nodeID)
		streamer = NewVideoStreamer(nodeID, config, ws, logger)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if err := streamer.Start(ctx); err != nil {
		logger.Error("failed to start streamer", "err", err)
		return
	}
	defer streamer.Stop()

	// Handle incoming messages (input events, ping/pong, etc.)
	for {
		messageType, msg, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("websocket read error", "err", err)
			}
			return
		}

		if messageType == websocket.BinaryMessage && len(msg) > 0 {
			msgType := msg[0]

			// Handle Ping/Pong at this level (needs ws access for response)
			if msgType == StreamMsgPing {
				// Client sent Ping for RTT measurement - respond with Pong
				// Pong format: type(1) + seq(4) + clientTime(8) + serverTime(8) = 21 bytes
				if len(msg) >= 13 {
					pong := make([]byte, 21)
					pong[0] = StreamMsgPong
					copy(pong[1:13], msg[1:13]) // Echo back seq + clientTime
					// Add server time (microseconds since epoch)
					serverTime := uint64(time.Now().UnixMicro())
					binary.BigEndian.PutUint64(pong[13:21], serverTime)
					if err := ws.WriteMessage(websocket.BinaryMessage, pong); err != nil {
						logger.Debug("failed to send pong", "err", err)
					}
				}
				continue
			}

			// Handle ControlMessage (0x20) for video pause/resume
			if msgType == StreamMsgControlMessage && len(msg) > 1 {
				var ctrl struct {
					SetVideoEnabled *bool `json:"set_video_enabled"`
				}
				if err := json.Unmarshal(msg[1:], &ctrl); err == nil && ctrl.SetVideoEnabled != nil {
					streamer.SetVideoEnabled(*ctrl.SetVideoEnabled)
				}
				continue
			}

			// Delegate other messages to input handler
			if server != nil {
				server.handleStreamInputMessage(msg)
			} else {
				logger.Debug("received input event (no server context)", "type", msgType)
			}
		}
	}
}

// handleStreamInputMessage processes input messages from the combined stream WebSocket.
// Note: Ping/Pong (0x40/0x41) are handled in the message loop, not here.
func (s *Server) handleStreamInputMessage(data []byte) {
	if len(data) < 1 {
		return
	}

	msgType := data[0]
	payload := data[1:]

	// Map stream message types to input handlers
	// Types 0x10-0x14 are reserved for input
	switch msgType {
	case StreamMsgKeyboard: // 0x10
		s.handleWSKeyboard(payload)
	case StreamMsgMouseClick: // 0x11
		s.handleWSMouseButton(payload)
	case StreamMsgMouseAbsolute: // 0x12
		s.handleWSMouseAbsolute(payload)
	case StreamMsgMouseRelative: // 0x13
		s.handleWSMouseRelative(payload)
	case StreamMsgTouch: // 0x14
		s.handleWSTouch(payload)
	case StreamMsgPong: // 0x41
		// Client responded to our WebSocket ping - no action needed
	default:
		s.logger.Debug("unknown stream message type", "type", msgType)
	}
}
