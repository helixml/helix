// Package desktop provides WebSocket video streaming using GStreamer and PipeWire.
// Uses go-gst bindings with appsink for in-order frame delivery.
package desktop

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
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

// getVideoMode returns the configured video mode from the stream config
// Set via URL param: ?videoMode=native|zerocopy|shm
func getVideoMode(configOverride string) VideoMode {
	switch strings.ToLower(configOverride) {
	case "native", "dmabuf":
		return VideoModeNative
	case "zerocopy", "zero-copy", "plugin":
		return VideoModeZeroCopy
	case "shm":
		return VideoModeSHM
	default:
		// Default to zerocopy (custom pipewirezerocopysrc plugin)
		return VideoModeZeroCopy
	}
}

// getGOPSize returns the configured GOP (Group of Pictures) size.
// Set via HELIX_GOP_SIZE environment variable. Default is 120 frames (2 seconds at 60fps).
// Larger GOP = better compression, smaller GOP = faster seek/recovery after packet loss.
func getGOPSize() int {
	if val := os.Getenv("HELIX_GOP_SIZE"); val != "" {
		if gop, err := strconv.Atoi(val); err == nil && gop > 0 {
			return gop
		}
	}
	return 120 // Default: 2 seconds at 60fps
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

// VideoStreamer captures video from PipeWire and streams to WebSocket.
// Uses go-gst bindings with appsink for guaranteed in-order frame delivery.
type VideoStreamer struct {
	nodeID             uint32
	pipeWireFd         int       // PipeWire FD from portal (required for ScreenCast access)
	shmSocketPath      string    // If set, use shmsrc instead of pipewiresrc
	preEncodedH264Path string    // If set, read pre-encoded H.264 from FIFO (Sway/wf-recorder)
	videoMode          VideoMode // Video capture mode (shm, native, zerocopy)
	config             StreamConfig
	ws                 *websocket.Conn
	logger             *slog.Logger
	gstPipeline        *GstPipeline // GStreamer pipeline with appsink
	running            atomic.Bool
	cancel             context.CancelFunc
	mu                 sync.Mutex // Protects Start/Stop
	wsMu               sync.Mutex // Protects WebSocket writes (gorilla/websocket requires serialized writes)

	// Frame tracking
	frameCount uint64
	startTime  time.Time

	// Video pause control (for screenshot mode switching)
	videoEnabled atomic.Bool
}

// NewVideoStreamer creates a new video streamer
// pipeWireFd is the FD from OpenPipeWireRemote portal call - required for ScreenCast access
func NewVideoStreamer(nodeID uint32, pipeWireFd int, config StreamConfig, ws *websocket.Conn, logger *slog.Logger) *VideoStreamer {
	v := &VideoStreamer{
		nodeID:     nodeID,
		pipeWireFd: pipeWireFd,
		videoMode:  getVideoMode(config.VideoMode),
		config:     config,
		ws:         ws,
		logger:     logger,
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

// NewVideoStreamerWithPreEncodedH264 creates a video streamer that reads pre-encoded H.264
// from a FIFO. This is used for Sway where wf-recorder outputs encoded video directly.
// No additional encoding is performed - the H.264 is just parsed and RTP-packetized.
func NewVideoStreamerWithPreEncodedH264(h264Path string, config StreamConfig, ws *websocket.Conn, logger *slog.Logger) *VideoStreamer {
	v := &VideoStreamer{
		preEncodedH264Path: h264Path,
		config:             config,
		ws:                 ws,
		logger:             logger,
	}
	v.videoEnabled.Store(true)
	return v
}

// SetVideoEnabled controls video frame sending (for screenshot mode switching)
func (v *VideoStreamer) SetVideoEnabled(enabled bool) {
	v.videoEnabled.Store(enabled)
	v.logger.Info("video streaming", "enabled", enabled)
}

// Start begins capturing and streaming video.
// Uses go-gst bindings with appsink for guaranteed in-order frame delivery.
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

	// Build GStreamer pipeline string (outputs to appsink)
	pipelineStr := v.buildPipelineString(encoder)

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
		"pipeline", pipelineStr,
	)

	// Create GStreamer pipeline with appsink
	var err error
	v.gstPipeline, err = NewGstPipeline(pipelineStr)
	if err != nil {
		return fmt.Errorf("create GStreamer pipeline: %w", err)
	}

	// Start the pipeline
	if err := v.gstPipeline.Start(ctx); err != nil {
		return fmt.Errorf("start GStreamer pipeline: %w", err)
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

	// Read frames from appsink and send to WebSocket
	go v.readFramesAndSend(ctx)

	// Handle ping/pong
	go v.heartbeat(ctx)

	return nil
}

// selectEncoder chooses the best available encoder
// Priority order:
// 1. NVIDIA NVENC (nvh264enc) - fastest, lowest latency
// 2. Intel QSV (qsvh264enc) - Intel Quick Sync Video
// 3. VA-API (vah264enc) - Intel/AMD VA-API
// 4. VA-API Legacy (vaapih264enc) - older VA-API plugin
// 5. VA-API LP (vah264lpenc) - Intel/AMD VA-API Low Power mode
// 6. OpenH264 (openh264enc) - Cisco's software encoder (installed by default)
// 7. x264 (x264enc) - software fallback (requires gst-plugins-ugly)
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

	// Try VA-API (Intel/AMD) - check both new (vah264enc) and old (vaapih264enc) plugins
	if checkGstElement("vah264enc") {
		v.logger.Info("using VA-API encoder (gst-va plugin)")
		return "vaapi"
	}
	if checkGstElement("vaapih264enc") {
		v.logger.Info("using VA-API encoder (gst-vaapi plugin)")
		return "vaapi-legacy"
	}

	// Try VA-API Low Power mode (some Intel chips)
	if checkGstElement("vah264lpenc") {
		v.logger.Info("using VA-API Low Power encoder")
		return "vaapi-lp"
	}

	// Try OpenH264 software encoder (Cisco's implementation, commonly installed)
	if checkGstElement("openh264enc") {
		v.logger.Info("using OpenH264 software encoder")
		return "openh264"
	}

	// Fallback to x264 software encoder
	if checkGstElement("x264enc") {
		v.logger.Info("using x264 software encoder")
		return "x264"
	}

	// Last resort - try openh264 anyway (it's usually available)
	v.logger.Warn("no H.264 encoder found, trying openh264enc")
	return "openh264"
}

// checkGstElement checks if a GStreamer element is available.
// Uses go-gst bindings to query the element factory.
func checkGstElement(element string) bool {
	InitGStreamer()
	return CheckGstElement(element)
}

// buildPipelineString creates a GStreamer pipeline string ending with appsink.
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
func (v *VideoStreamer) buildPipelineString(encoder string) string {
	var parts []string

	// Pre-encoded H.264 path (Sway/wf-recorder) - skip all encoding, just parse
	if v.preEncodedH264Path != "" {
		// Read pre-encoded H.264 annex-b stream from FIFO
		// wf-recorder has already done the encoding, we just need to parse
		parts = []string{
			fmt.Sprintf("filesrc location=%s", v.preEncodedH264Path),
			"h264parse config-interval=-1",
			"video/x-h264,stream-format=byte-stream,alignment=au",
			"appsink name=videosink emit-signals=true max-buffers=2 drop=true sync=false",
		}
		return strings.Join(parts, " ! ")
	}

	// Step 1: Build source section based on video mode
	// Note: shmSocketPath (for video forwarder) takes precedence over videoMode
	if v.shmSocketPath != "" {
		// Use shmsrc to read from video forwarder's shared memory socket
		// This avoids PipeWire node conflicts when the forwarder is running
		parts = []string{
			fmt.Sprintf("shmsrc socket-path=%s is-live=true do-timestamp=true", v.shmSocketPath),
			fmt.Sprintf("video/x-raw,format=BGRx,width=%d,height=%d,framerate=0/1",
				v.config.Width, v.config.Height),
		}
	} else {
		// Choose source based on video mode
		// For ScreenCast nodes, pipeWireFd from OpenPipeWireRemote is REQUIRED
		// Without it, pipewiresrc gets "target not found" because the default
		// PipeWire connection doesn't have access to portal ScreenCast nodes.
		switch v.videoMode {
		case VideoModeZeroCopy:
			// pipewirezerocopysrc: Zero-copy via GPU memory sharing
			// Requires gst-plugin-pipewire-zerocopy to be installed
			// Note: framerate is handled internally via max_framerate in PipeWire negotiation
			//
			// Output mode selection based on encoder:
			// - nvenc: CUDA memory (auto mode tries CUDA first)
			// - vaapi/vaapi-legacy/vaapi-lp: DMA-BUF (VA-API accepts DmaBuf directly)
			// - qsv: auto (QSV handles its own memory)
			// - x264: system memory (CPU encoder)
			var outputMode string
			switch encoder {
			case "vaapi", "vaapi-legacy", "vaapi-lp":
				outputMode = "dmabuf" // VA-API encoders accept DMA-BUF directly
			case "x264":
				outputMode = "system" // CPU encoder needs system memory
			default:
				outputMode = "auto" // NVENC/QSV: auto-detect (CUDA for NVIDIA)
			}
			srcPart := fmt.Sprintf("pipewirezerocopysrc pipewire-node-id=%d output-mode=%s keepalive-time=100", v.nodeID, outputMode)
			// Add fd property if we have portal FD (required for ScreenCast access)
			if v.pipeWireFd > 0 {
				srcPart += fmt.Sprintf(" pipewire-fd=%d", v.pipeWireFd)
			}
			parts = []string{srcPart}

		case VideoModeNative:
			// Native DMA-BUF path: pipewiresrc negotiates DMA-BUF with compositor
			// Works on GStreamer 1.24+ with proper driver support
			// Falls back gracefully to system memory if DMA-BUF unavailable
			srcPart := fmt.Sprintf("pipewiresrc path=%d do-timestamp=true", v.nodeID)
			// Add fd property if we have portal FD (required for ScreenCast access)
			if v.pipeWireFd > 0 {
				srcPart += fmt.Sprintf(" fd=%d", v.pipeWireFd)
			}
			parts = []string{
				srcPart,
				// Let pipewiresrc negotiate best format - prefer DMA-BUF if available
				// Explicit framerate prevents Mutter from defaulting to lower rate
				fmt.Sprintf("video/x-raw,framerate=%d/1", v.config.FPS),
			}

		default: // VideoModeSHM
			// Standard pipewiresrc path - most compatible
			// Uses damage-based capture (only sends frames when screen changes)
			srcPart := fmt.Sprintf("pipewiresrc path=%d do-timestamp=true", v.nodeID)
			// Add fd property if we have portal FD (required for ScreenCast access)
			if v.pipeWireFd > 0 {
				srcPart += fmt.Sprintf(" fd=%d", v.pipeWireFd)
			}
			parts = []string{
				srcPart,
				// Explicit framerate prevents Mutter from defaulting to lower rate
				fmt.Sprintf("video/x-raw,format=BGRx,framerate=%d/1", v.config.FPS),
			}
		}
	}

	// Add leaky queue to decouple pipewiresrc from encoding pipeline
	// max-size-buffers=3 prevents buffer starvation - pipewiresrc needs buffers returned
	// quickly or it can't allocate new ones for the next frame
	// leaky=downstream drops oldest frames if encoding falls behind (low latency)
	parts = append(parts, "queue max-size-buffers=3 leaky=downstream")

	// Step 2: Add encoder-specific conversion and encoding pipeline
	// Each encoder type has its own GPU-optimized path
	// Note: zerocopy mode already provides GPU memory (CUDA or DMABuf)
	switch encoder {
	case "nvenc":
		// NVIDIA NVENC encoding
		// For zerocopy mode: frames already in CUDA memory, skip cudaupload
		// For other modes: use cudaupload to get frames into CUDA memory
		if v.videoMode == VideoModeZeroCopy && v.shmSocketPath == "" {
			// pipewirezerocopysrc outputs video/x-raw(memory:CUDAMemory) in BGRA/RGBA
			// nvh264enc accepts CUDA memory directly in these formats
			// No conversion needed - true zero-copy GPU pipeline
			parts = append(parts,
				fmt.Sprintf("nvh264enc preset=low-latency-hq zerolatency=true gop-size=%d rc-mode=cbr-ld-hq bitrate=%d aud=false", getGOPSize(), v.config.Bitrate),
			)
		} else {
			// Standard path: system memory → cudaupload → nvh264enc
			parts = append(parts,
				"videorate",
				"videoscale",
				fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				"cudaupload",
				fmt.Sprintf("nvh264enc preset=low-latency-hq zerolatency=true gop-size=%d rc-mode=cbr-ld-hq bitrate=%d aud=false", getGOPSize(), v.config.Bitrate),
			)
		}

	case "qsv":
		// Intel Quick Sync Video
		// QSV has its own memory system, uses CPU conversion for now
		parts = append(parts,
			"videoconvert",
			"videoscale",
			fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
				v.config.Width, v.config.Height, v.config.FPS),
			fmt.Sprintf("qsvh264enc b-frames=0 gop-size=%d idr-interval=1 ref-frames=1 bitrate=%d rate-control=cbr target-usage=6", getGOPSize(), v.config.Bitrate),
		)

	case "vaapi":
		// AMD/Intel VA-API pipeline
		// For zerocopy/native modes: source may provide DMABuf which VA-API can use directly
		// For shm mode: uses CPU-based videoconvert (vapostproc not widely available)
		if (v.videoMode == VideoModeZeroCopy || v.videoMode == VideoModeNative) && v.shmSocketPath == "" {
			// Try to use vapostproc for GPU-side processing (available on newer systems)
			// CRITICAL: format=NV12 is required for AMD VA-API to work correctly.
			// Without explicit format, vapostproc may output incompatible format for vah264enc.
			// This matches Wolf's working config.v6.toml pattern.
			parts = append(parts,
				"vapostproc",
				fmt.Sprintf("video/x-raw(memory:VAMemory),format=NV12,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				fmt.Sprintf("vah264enc aud=false b-frames=0 ref-frames=1 bitrate=%d cpb-size=%d key-int-max=%d rate-control=cqp target-usage=6",
					v.config.Bitrate, v.config.Bitrate, getGOPSize()),
			)
		} else {
			// Standard path: CPU videoconvert
			parts = append(parts,
				"videoconvert",
				"videoscale",
				fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				fmt.Sprintf("vah264enc aud=false b-frames=0 ref-frames=1 bitrate=%d cpb-size=%d key-int-max=%d rate-control=cqp target-usage=6",
					v.config.Bitrate, v.config.Bitrate, getGOPSize()),
			)
		}

	case "vaapi-lp":
		// VA-API Low Power mode (Intel-specific)
		if (v.videoMode == VideoModeZeroCopy || v.videoMode == VideoModeNative) && v.shmSocketPath == "" {
			// format=NV12 required for proper VA-API pipeline negotiation
			parts = append(parts,
				"vapostproc",
				fmt.Sprintf("video/x-raw(memory:VAMemory),format=NV12,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				fmt.Sprintf("vah264lpenc aud=false b-frames=0 ref-frames=1 bitrate=%d cpb-size=%d key-int-max=%d rate-control=cqp target-usage=6",
					v.config.Bitrate, v.config.Bitrate, getGOPSize()),
			)
		} else {
			parts = append(parts,
				"videoconvert",
				"videoscale",
				fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
					v.config.Width, v.config.Height, v.config.FPS),
				fmt.Sprintf("vah264lpenc aud=false b-frames=0 ref-frames=1 bitrate=%d cpb-size=%d key-int-max=%d rate-control=cqp target-usage=6",
					v.config.Bitrate, v.config.Bitrate, getGOPSize()),
			)
		}

	case "vaapi-legacy":
		// Legacy VA-API (gst-vaapi plugin) - wider compatibility for AMD/Intel
		// vaapih264enc accepts DMA-BUF directly, no need for vaapipostproc
		// Use vaapipostproc only if we need colorspace conversion from system memory
		if (v.videoMode == VideoModeZeroCopy || v.videoMode == VideoModeNative) && v.shmSocketPath == "" {
			// DMA-BUF path: vaapih264enc can accept DMA-BUF directly
			parts = append(parts,
				fmt.Sprintf("vaapih264enc tune=low-latency rate-control=cbr bitrate=%d keyframe-period=%d",
					v.config.Bitrate, getGOPSize()),
			)
		} else {
			// System memory path: use vaapipostproc for GPU upload
			parts = append(parts,
				"vaapipostproc",
				fmt.Sprintf("vaapih264enc tune=low-latency rate-control=cbr bitrate=%d keyframe-period=%d",
					v.config.Bitrate, getGOPSize()),
			)
		}

	case "openh264":
		// OpenH264 software encoder (Cisco's implementation)
		// Lower latency than x264 but may have lower quality
		// Commonly available as it's installed by default in many distros
		parts = append(parts,
			"videoconvert",
			"videoscale",
			fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
				v.config.Width, v.config.Height, v.config.FPS),
			fmt.Sprintf("openh264enc complexity=low bitrate=%d gop-size=%d", v.config.Bitrate*1000, getGOPSize()),
		)

	case "x264":
		// x264 software encoder - high quality but requires gst-plugins-ugly
		parts = append(parts,
			"videoconvert",
			"videoscale",
			fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
				v.config.Width, v.config.Height, v.config.FPS),
			fmt.Sprintf("x264enc pass=qual tune=zerolatency speed-preset=superfast b-adapt=false bframes=0 ref=1 key-int-max=%d bitrate=%d aud=false", getGOPSize(), v.config.Bitrate),
		)

	default:
		// Unknown encoder - try openh264 as last resort
		v.logger.Warn("unknown encoder, falling back to openh264", "encoder", encoder)
		parts = append(parts,
			"videoconvert",
			"videoscale",
			fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1",
				v.config.Width, v.config.Height, v.config.FPS),
			fmt.Sprintf("openh264enc complexity=low bitrate=%d gop-size=%d", v.config.Bitrate*1000, getGOPSize()),
		)
	}

	// Step 3: Add h264parse and appsink
	// h264parse with config-interval=-1 inserts SPS/PPS before every keyframe
	// appsink delivers complete H.264 access units to our Go callback
	parts = append(parts,
		"h264parse config-interval=-1",
		"video/x-h264,stream-format=byte-stream,alignment=au",
		"appsink name=videosink emit-signals=true max-buffers=2 drop=true sync=false",
	)

	return strings.Join(parts, " ! ")
}

// writeMessage is a thread-safe wrapper for WebSocket writes.
// gorilla/websocket requires that all writes to the same connection be serialized.
func (v *VideoStreamer) writeMessage(messageType int, data []byte) error {
	v.wsMu.Lock()
	defer v.wsMu.Unlock()
	return v.ws.WriteMessage(messageType, data)
}

// writeJSON is a thread-safe wrapper for WebSocket JSON writes.
func (v *VideoStreamer) writeJSON(data interface{}) error {
	v.wsMu.Lock()
	defer v.wsMu.Unlock()
	return v.ws.WriteJSON(data)
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

	return v.writeMessage(websocket.BinaryMessage, msg)
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
	return v.writeJSON(msg)
}

// readFramesAndSend reads video frames from appsink and sends to WebSocket.
// Frames are delivered in encode order via channel - no UDP reordering possible.
func (v *VideoStreamer) readFramesAndSend(ctx context.Context) {
	defer v.Stop()

	// Latency tracking
	var frameCount uint64
	var totalSendTime time.Duration
	var lastLogTime = time.Now()

	frameCh := v.gstPipeline.Frames()

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frameCh:
			if !ok {
				// Pipeline stopped
				v.logger.Info("appsink channel closed")
				return
			}

			// Measure WebSocket send time
			sendStart := time.Now()
			if err := v.sendVideoFrame(frame.Data, frame.IsKeyframe, frame.PTS); err != nil {
				v.logger.Error("send frame error", "err", err)
				return
			}
			sendTime := time.Since(sendStart)
			totalSendTime += sendTime
			frameCount++

			// Log latency stats every 5 seconds
			if time.Since(lastLogTime) >= 5*time.Second && frameCount > 0 {
				avgSend := totalSendTime / time.Duration(frameCount)
				v.logger.Info("VIDEO LATENCY STATS",
					"frames", frameCount,
					"avg_send_us", avgSend.Microseconds(),
					"frame_size_bytes", len(frame.Data),
					"is_keyframe", frame.IsKeyframe)
				// Reset counters
				frameCount = 0
				totalSendTime = 0
				lastLogTime = time.Now()
			}
		}
	}
}

// sendVideoFrame sends a video frame to the WebSocket
// isKeyframe should be true for Access Units containing SPS+PPS+IDR
// pts is the presentation timestamp in microseconds from GStreamer
func (v *VideoStreamer) sendVideoFrame(data []byte, isKeyframe bool, pts uint64) error {
	// Skip sending if video is paused (screenshot mode)
	if !v.videoEnabled.Load() {
		return nil
	}

	v.frameCount++

	// PTS is already in microseconds from GStreamer (converted in gst_pipeline.go)

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

	return v.writeMessage(websocket.BinaryMessage, msg)
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
			if err := v.writeMessage(websocket.PingMessage, nil); err != nil {
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

	// Stop GStreamer pipeline
	if v.gstPipeline != nil {
		v.gstPipeline.Stop()
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
// Uses standalone video session (s.videoNodeID) for DmaBuf zero-copy.
// Falls back to linked session (s.nodeID) if standalone isn't available (SHM path).
func (s *Server) handleStreamWebSocketWithServer(w http.ResponseWriter, r *http.Request) {
	// Prefer standalone video session (has DmaBuf modifiers for zero-copy)
	nodeID := s.videoNodeID
	if nodeID == 0 {
		nodeID = s.nodeID // Fallback to linked session (SHM path)
		if nodeID != 0 {
			s.logger.Warn("using linked session for video (SHM path, no DmaBuf)",
				"fallback_node_id", nodeID)
		}
	}
	handleStreamWebSocketInternal(w, r, nodeID, s.logger, s)
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

	// Create video streamer - select source based on compositor type
	var streamer *VideoStreamer
	if server != nil && server.compositorType == "sway" && server.videoForwarder != nil {
		// For Sway: Use wf-recorder which outputs pre-encoded H.264 to a FIFO
		// Restart forwarder for each connection to avoid stale FIFO state
		if err := server.videoForwarder.Restart(r.Context()); err != nil {
			logger.Error("failed to restart video forwarder", "err", err)
			// Fall back to direct PipeWire (may not work on Sway, but try anyway)
			logger.Info("falling back to PipeWire video source", "node_id", nodeID)
			streamer = NewVideoStreamer(nodeID, server.pipeWireFd, config, ws, logger)
		} else {
			forwarderPath := server.videoForwarder.ShmSocketPath()
			logger.Info("using pre-encoded H.264 source (Sway/wf-recorder)", "fifo", forwarderPath)
			streamer = NewVideoStreamerWithPreEncodedH264(forwarderPath, config, ws, logger)
		}
	} else {
		// For GNOME: Connect directly to pipewiresrc
		// The shmsink/shmsrc approach has reliability issues with reconnection,
		// so we skip the video forwarder and connect directly to PipeWire.
		// Pass the portal FD from OpenPipeWireRemote - required for ScreenCast access
		pipeWireFd := 0
		if server != nil {
			pipeWireFd = server.pipeWireFd
		}
		logger.Info("using PipeWire video source (direct)", "node_id", nodeID, "pipewire_fd", pipeWireFd)
		streamer = NewVideoStreamer(nodeID, pipeWireFd, config, ws, logger)
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
					// Use streamer's mutex-protected write to avoid concurrent write panic
					if err := streamer.writeMessage(websocket.BinaryMessage, pong); err != nil {
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
