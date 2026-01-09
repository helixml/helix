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
	StreamMsgMouseRelative = 0x13
	StreamMsgTouch         = 0x14
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
	nodeID  uint32
	config  StreamConfig
	ws      *websocket.Conn
	logger  *slog.Logger
	cmd     *exec.Cmd
	running atomic.Bool
	cancel  context.CancelFunc
	mu      sync.Mutex

	// Frame tracking
	frameCount uint64
	startTime  time.Time
}

// NewVideoStreamer creates a new video streamer
func NewVideoStreamer(nodeID uint32, config StreamConfig, ws *websocket.Conn, logger *slog.Logger) *VideoStreamer {
	return &VideoStreamer{
		nodeID: nodeID,
		config: config,
		ws:     ws,
		logger: logger,
	}
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

	v.logger.Info("starting video capture",
		"node_id", v.nodeID,
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

	// Send StreamInit to client
	if err := v.sendStreamInit(); err != nil {
		v.logger.Error("failed to send StreamInit", "err", err)
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
func (v *VideoStreamer) buildPipelineArgs(encoder string) []string {
	var encoderArgs []string

	switch encoder {
	case "nvenc":
		// NVIDIA hardware encoder - matches Wolf's nvcodec pipeline
		// Requires cudaconvertscale and cudaupload for optimal performance
		encoderArgs = []string{
			"nvh264enc",
			"preset=low-latency-hq",
			"zerolatency=true",
			"gop-size=15",
			"rc-mode=cbr-ld-hq",
			fmt.Sprintf("bitrate=%d", v.config.Bitrate),
			"aud=false",
		}

	case "qsv":
		// Intel Quick Sync Video - matches Wolf's qsv pipeline
		encoderArgs = []string{
			"qsvh264enc",
			"b-frames=0",
			"gop-size=15",
			"idr-interval=1",
			"ref-frames=1",
			fmt.Sprintf("bitrate=%d", v.config.Bitrate),
			"rate-control=cbr",
			"target-usage=6",
		}

	case "vaapi":
		// VA-API for Intel/AMD - matches Wolf's va pipeline
		encoderArgs = []string{
			"vah264enc",
			"aud=false",
			"b-frames=0",
			"ref-frames=1",
			fmt.Sprintf("bitrate=%d", v.config.Bitrate),
			fmt.Sprintf("cpb-size=%d", v.config.Bitrate),
			"key-int-max=1024",
			"rate-control=cqp",
			"target-usage=6",
		}

	case "vaapi-lp":
		// VA-API Low Power mode for Intel - matches Wolf's va LP pipeline
		encoderArgs = []string{
			"vah264lpenc",
			"aud=false",
			"b-frames=0",
			"ref-frames=1",
			fmt.Sprintf("bitrate=%d", v.config.Bitrate),
			fmt.Sprintf("cpb-size=%d", v.config.Bitrate),
			"key-int-max=1024",
			"rate-control=cqp",
			"target-usage=6",
		}

	default:
		// Software x264 fallback - matches Wolf's x264 pipeline
		encoderArgs = []string{
			"x264enc",
			"pass=qual",
			"tune=zerolatency",
			"speed-preset=superfast",
			"b-adapt=false",
			"bframes=0",
			"ref=1",
			fmt.Sprintf("bitrate=%d", v.config.Bitrate),
			"aud=false",
		}
	}

	// Build full pipeline as flat args
	args := []string{
		"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
		"!", "videoconvert",
		"!", "videoscale",
		"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
			v.config.Width, v.config.Height, v.config.FPS),
		"!",
	}
	args = append(args, encoderArgs...)
	args = append(args,
		"!", "h264parse",
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

// readAndSend reads H.264 data and sends to WebSocket
func (v *VideoStreamer) readAndSend(ctx context.Context, stdout io.Reader) {
	defer v.Stop()

	reader := bufio.NewReaderSize(stdout, 256*1024) // 256KB buffer

	// H.264 Annex B format: NAL units prefixed with 00 00 00 01 or 00 00 01
	// We need to find NAL unit boundaries and send each as a video frame
	nalBuffer := make([]byte, 0, 256*1024)

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

		// Find and send complete NAL units
		for {
			nalUnit, remaining, found := findNALUnit(nalBuffer)
			if !found {
				break
			}

			if len(nalUnit) > 0 {
				if err := v.sendVideoFrame(nalUnit); err != nil {
					v.logger.Error("send frame error", "err", err)
					return
				}
			}

			nalBuffer = remaining
		}
	}
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
func (v *VideoStreamer) sendVideoFrame(nalUnit []byte) error {
	v.frameCount++
	pts := uint64(time.Since(v.startTime).Microseconds())

	// Determine if this is a keyframe (IDR NAL unit type 5)
	isKeyframe := false
	if len(nalUnit) > 4 {
		// NAL unit type is in lower 5 bits of first byte after start code
		var nalType byte
		if len(nalUnit) > 2 && nalUnit[2] == 1 {
			nalType = nalUnit[3] & 0x1f
		} else if len(nalUnit) > 3 && nalUnit[3] == 1 {
			nalType = nalUnit[4] & 0x1f
		}
		isKeyframe = nalType == 5 // IDR slice
	}

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
	msg := make([]byte, 15+len(nalUnit))
	copy(msg[:15], header)
	copy(msg[15:], nalUnit)

	return v.ws.WriteMessage(websocket.BinaryMessage, msg)
}

// heartbeat sends periodic pings to keep connection alive
func (v *VideoStreamer) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := v.ws.WriteMessage(websocket.BinaryMessage, []byte{StreamMsgPing}); err != nil {
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
	_, msg, err := ws.ReadMessage()
	if err != nil {
		logger.Error("failed to read init message", "err", err)
		return
	}

	var config StreamConfig
	if err := json.Unmarshal(msg, &config); err != nil {
		logger.Error("failed to parse init message", "err", err)
		return
	}

	if config.Type != "init" {
		logger.Error("expected init message", "got", config.Type)
		return
	}

	logger.Info("stream init received",
		"width", config.Width,
		"height", config.Height,
		"fps", config.FPS,
		"bitrate", config.Bitrate,
	)

	// Create and start video streamer
	streamer := NewVideoStreamer(nodeID, config, ws, logger)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if err := streamer.Start(ctx); err != nil {
		logger.Error("failed to start streamer", "err", err)
		return
	}
	defer streamer.Stop()

	// Handle incoming messages (input events, pong, etc.)
	for {
		messageType, msg, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("websocket read error", "err", err)
			}
			return
		}

		if messageType == websocket.BinaryMessage && len(msg) > 0 {
			// Handle messages using moonlight-web-stream protocol
			if server != nil {
				server.handleStreamInputMessage(msg)
			} else {
				logger.Debug("received input event (no server context)", "type", msg[0])
			}
		}
	}
}

// handleStreamInputMessage processes input messages from the combined stream WebSocket
// Uses the moonlight-web-stream protocol for compatibility with existing frontend.
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
		// Client responded to ping - currently no action needed
	case StreamMsgPing: // 0x40
		// Client sent ping for RTT measurement - we should respond with pong
		// TODO: implement pong response with timing
		s.logger.Debug("received ping from client")
	default:
		s.logger.Debug("unknown stream message type", "type", msgType)
	}
}
