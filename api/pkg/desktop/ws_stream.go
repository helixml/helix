// Package desktop provides WebSocket video streaming using GStreamer and PipeWire.
// This implements the same binary protocol as Wolf/Moonlight-Web for frontend compatibility.
package desktop

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

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
}

// VideoStreamer captures video from PipeWire and streams to WebSocket
type VideoStreamer struct {
	nodeID        uint32
	shmSocketPath string // If set, use shmsrc instead of pipewiresrc
	config        StreamConfig
	ws            *websocket.Conn
	logger        *slog.Logger
	cmd           *exec.Cmd
	running       atomic.Bool
	cancel        context.CancelFunc
	mu            sync.Mutex

	// Frame tracking
	frameCount uint64
	startTime  time.Time

	// Video pause control (for screenshot mode switching)
	videoEnabled atomic.Bool
}

// NewVideoStreamer creates a new video streamer
func NewVideoStreamer(nodeID uint32, config StreamConfig, ws *websocket.Conn, logger *slog.Logger) *VideoStreamer {
	v := &VideoStreamer{
		nodeID: nodeID,
		config: config,
		ws:     ws,
		logger: logger,
	}
	v.videoEnabled.Store(true) // Video enabled by default
	return v
}

// NewVideoStreamerWithSHM creates a video streamer that reads from shared memory
// This is used when a video forwarder is running to avoid PipeWire node conflicts
func NewVideoStreamerWithSHM(shmSocketPath string, config StreamConfig, ws *websocket.Conn, logger *slog.Logger) *VideoStreamer {
	v := &VideoStreamer{
		shmSocketPath: shmSocketPath,
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
func (v *VideoStreamer) Start(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running.Load() {
		return nil
	}

	ctx, v.cancel = context.WithCancel(ctx)
	v.startTime = time.Now()

	// Determine encoder based on available hardware
	encoder := v.selectEncoder()

	// Build GStreamer pipeline args
	pipelineArgs := v.buildPipelineArgs(encoder)

	// Log with appropriate source info
	sourceInfo := fmt.Sprintf("pipewire:%d", v.nodeID)
	if v.shmSocketPath != "" {
		sourceInfo = fmt.Sprintf("shm:%s", v.shmSocketPath)
	}
	v.logger.Info("starting video capture",
		"source", sourceInfo,
		"encoder", encoder,
		"resolution", fmt.Sprintf("%dx%d", v.config.Width, v.config.Height),
		"fps", v.config.FPS,
		"bitrate", v.config.Bitrate,
		"pipeline", strings.Join(pipelineArgs, " "),
	)

	// Start gst-launch with pipeline args
	args := append([]string{"-q"}, pipelineArgs...)
	v.cmd = exec.CommandContext(ctx, "gst-launch-1.0", args...)
	stdout, err := v.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := v.cmd.Start(); err != nil {
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

	// Read H.264 NAL units and send to WebSocket
	go v.readAndSend(ctx, stdout)

	// Handle ping/pong
	go v.heartbeat(ctx)

	return nil
}

// selectEncoder chooses the best available encoder
// Priority order matches Wolf's tested configurations:
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
// Pipeline configurations match Wolf's tested settings from config.v6.toml
//
// GPU optimization notes:
// - NVIDIA: Uses cudaupload to get frames into CUDA memory, nvh264enc does
//   BGRx→NV12 colorspace conversion internally on NVENC hardware (zero-copy)
// - AMD/Intel VA-API: Uses CPU-based videoconvert since vapostproc isn't widely
//   available; vah264enc handles GPU upload. Not true zero-copy but still HW-accelerated.
// - Software: Uses CPU-based videoconvert + x264enc (slowest fallback)
func (v *VideoStreamer) buildPipelineArgs(encoder string) []string {
	var args []string

	// Step 1: Build source section (shmsrc or pipewiresrc)
	if v.shmSocketPath != "" {
		// Use shmsrc to read from video forwarder's shared memory socket
		// This avoids PipeWire node conflicts when the forwarder is running
		// IMPORTANT: shmsrc needs FULL caps (width, height, framerate) specified immediately
		// because shmsink doesn't embed caps metadata in the shared memory buffers
		args = []string{
			"shmsrc", fmt.Sprintf("socket-path=%s", v.shmSocketPath), "is-live=true", "do-timestamp=true",
			"!", fmt.Sprintf("video/x-raw,format=BGRx,width=%d,height=%d,framerate=0/1",
				v.config.Width, v.config.Height),
		}
	} else {
		// Use pipewiresrc to capture directly from PipeWire (legacy mode)
		args = []string{
			"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
			"!", "video/x-raw,format=BGRx",
		}
	}

	// Step 2: Add encoder-specific conversion and encoding pipeline
	// Each encoder type has its own GPU-optimized path
	switch encoder {
	case "nvenc":
		// NVIDIA zero-copy pipeline:
		// 1. videorate + videoscale: CPU-side scaling to target resolution/framerate
		// 2. cudaupload: Upload system memory to CUDA memory (one CPU→GPU copy)
		// 3. nvh264enc: NVENC encoding directly from CUDA memory
		//
		// nvh264enc handles BGRx→NV12 colorspace conversion internally on GPU.
		// This is faster than videoconvert (CPU) + nvh264enc because:
		// - Only one CPU→GPU copy (via cudaupload)
		// - Colorspace conversion happens on NVENC hardware
		args = append(args,
			"!", "videorate",
			"!", "videoscale",
			"!", fmt.Sprintf("video/x-raw,format=BGRx,width=%d,height=%d,framerate=%d/1",
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
		// AMD/Intel VA-API pipeline:
		// Uses CPU-based videoconvert for colorspace conversion (vapostproc not widely available)
		// vah264enc handles the upload to GPU memory internally
		//
		// This is slightly less optimal than true zero-copy (vapostproc → vah264enc)
		// but works on systems where vapostproc isn't available
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

	case "vaapi-lp":
		// VA-API Low Power mode (Intel-specific)
		// Same as vaapi but uses the low-power encoder variant
		// Uses CPU-based videoconvert (vapostproc not widely available)
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

	// Step 3: Add h264parse and output
	// h264parse with config-interval=-1 inserts SPS/PPS before every keyframe
	// This is required for WebCodecs which needs parameter sets for decoding
	args = append(args,
		"!", "h264parse", "config-interval=-1",
		"!", "video/x-h264,stream-format=byte-stream",
		"!", "fdsink", "fd=1",
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

// readAndSend reads H.264 data and sends to WebSocket
func (v *VideoStreamer) readAndSend(ctx context.Context, stdout io.Reader) {
	defer v.Stop()

	reader := bufio.NewReaderSize(stdout, 256*1024) // 256KB buffer

	// H.264 Annex B format: NAL units prefixed with 00 00 00 01 or 00 00 01
	// We accumulate complete Access Units (SPS+PPS+IDR or just slice) before sending
	nalBuffer := make([]byte, 0, 256*1024)

	// Access Unit accumulator - bundles SPS+PPS+IDR together
	accessUnit := make([]byte, 0, 256*1024)
	hasParameterSets := false // true if we've accumulated SPS or PPS

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read data
		buf := make([]byte, 32*1024)
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				v.logger.Error("read error", "err", err)
			}
			return
		}

		// Accumulate data
		nalBuffer = append(nalBuffer, buf[:n]...)

		// Find and process NAL units
		for {
			nalUnit, remaining, found := findNALUnit(nalBuffer)
			if !found {
				break
			}
			nalBuffer = remaining

			if len(nalUnit) == 0 {
				continue
			}

			// Get NAL type
			nalType := getNALType(nalUnit)

			switch nalType {
			case 7, 8: // SPS or PPS - accumulate, don't send yet
				accessUnit = append(accessUnit, nalUnit...)
				hasParameterSets = true

			case 5: // IDR slice - send with accumulated SPS/PPS as keyframe
				accessUnit = append(accessUnit, nalUnit...)
				if err := v.sendVideoFrame(accessUnit, true); err != nil {
					v.logger.Error("send keyframe error", "err", err)
					return
				}
				accessUnit = accessUnit[:0] // Reset accumulator
				hasParameterSets = false

			case 1, 2, 3, 4: // Non-IDR slice - send immediately as delta frame
				// If we have accumulated SPS/PPS without IDR, send them first (shouldn't happen normally)
				if hasParameterSets {
					if err := v.sendVideoFrame(accessUnit, true); err != nil {
						v.logger.Error("send params error", "err", err)
						return
					}
					accessUnit = accessUnit[:0]
					hasParameterSets = false
				}
				if err := v.sendVideoFrame(nalUnit, false); err != nil {
					v.logger.Error("send frame error", "err", err)
					return
				}

			case 9: // Access Unit Delimiter - discard (WebCodecs doesn't need them)
				// AUD NAL units are used for stream synchronization but WebCodecs
				// expects keyframes to start with SPS, not AUD
				continue

			case 6: // SEI - can be useful, accumulate with access unit
				accessUnit = append(accessUnit, nalUnit...)

			default:
				// Other NAL types - discard unknown types to avoid confusing decoder
				v.logger.Debug("discarding unknown NAL type", "type", nalType)
			}
		}
	}
}

// getNALType extracts the NAL unit type from a NAL unit with start code
func getNALType(nalUnit []byte) byte {
	if len(nalUnit) < 4 {
		return 0
	}
	// Find the byte after start code
	if len(nalUnit) > 2 && nalUnit[2] == 1 {
		// 3-byte start code: 00 00 01
		return nalUnit[3] & 0x1f
	} else if len(nalUnit) > 3 && nalUnit[3] == 1 {
		// 4-byte start code: 00 00 00 01
		return nalUnit[4] & 0x1f
	}
	return 0
}

// findNALUnit finds the next NAL unit in the buffer
// Returns the NAL unit data, remaining buffer, and whether a unit was found
func findNALUnit(buf []byte) (nalUnit, remaining []byte, found bool) {
	if len(buf) < 4 {
		return nil, buf, false
	}

	// Find start code (00 00 00 01 or 00 00 01)
	startIdx := -1
	startLen := 0
	for i := 0; i < len(buf)-3; i++ {
		if buf[i] == 0 && buf[i+1] == 0 {
			if buf[i+2] == 1 {
				startIdx = i
				startLen = 3
				break
			} else if i < len(buf)-3 && buf[i+2] == 0 && buf[i+3] == 1 {
				startIdx = i
				startLen = 4
				break
			}
		}
	}

	if startIdx == -1 {
		return nil, buf, false
	}

	// Find next start code
	dataStart := startIdx + startLen
	for i := dataStart; i < len(buf)-3; i++ {
		if buf[i] == 0 && buf[i+1] == 0 {
			if buf[i+2] == 1 || (i < len(buf)-3 && buf[i+2] == 0 && buf[i+3] == 1) {
				// Found next start code - return this NAL unit
				return buf[startIdx:i], buf[i:], true
			}
		}
	}

	// No complete NAL unit yet - need more data
	// But if buffer is getting large, send what we have
	if len(buf) > 128*1024 {
		return buf[startIdx:], nil, true
	}

	return nil, buf, false
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

// handleStreamInputMessage processes input messages from the combined stream WebSocket
// Uses the moonlight-web-stream protocol for compatibility with existing frontend.
// Note: Ping/Pong (0x40/0x41) are handled in the message loop, not here.
func (s *Server) handleStreamInputMessage(data []byte) {
	if len(data) < 1 {
		return
	}

	msgType := data[0]
	payload := data[1:]

	// Map moonlight-web-stream message types to our handlers
	// moonlight-web-stream uses 0x10-0x14 for input
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
