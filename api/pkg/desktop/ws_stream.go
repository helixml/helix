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

	// VideoModePlugin uses our custom pipewirezerocopysrc GStreamer element
	// Output varies by compositor and GPU:
	// - GNOME + NVIDIA: true zero-copy (DMA-BUF → CUDAMemory → nvh264enc)
	// - Sway: SHM fallback (xdg-desktop-portal-wlr lacks NVIDIA modifier support)
	// - AMD/Intel: DMA-BUF via EGL for zero-copy
	// Requires gst-plugin-pipewire-zerocopy to be installed
	VideoModePlugin VideoMode = "zerocopy"
)

// getVideoMode returns the configured video mode from the stream config
// Set via URL param: ?videoMode=native|zerocopy|shm
func getVideoMode(configOverride string) VideoMode {
	switch strings.ToLower(configOverride) {
	case "native", "dmabuf":
		return VideoModeNative
	case "zerocopy", "zero-copy", "plugin":
		return VideoModePlugin
	case "shm":
		return VideoModeSHM
	default:
		// Default to zerocopy (custom pipewirezerocopysrc plugin)
		return VideoModePlugin
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
// Uses pipewirezerocopysrc for zero-copy DMA-BUF capture on both GNOME and Sway.
type VideoStreamer struct {
	nodeID     uint32
	pipeWireFd int       // PipeWire FD from portal (required for ScreenCast access)
	videoMode  VideoMode // Video capture mode (shm, native, zerocopy)
	config     StreamConfig
	ws         *websocket.Conn
	logger     *slog.Logger
	gstPipeline *GstPipeline // GStreamer pipeline with appsink
	running     atomic.Bool
	cancel      context.CancelFunc
	mu          sync.Mutex // Protects Start/Stop
	wsMu        sync.Mutex // Protects WebSocket writes (gorilla/websocket requires serialized writes)

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

	// Log with source info
	sourceInfo := fmt.Sprintf("pipewire:%d", v.nodeID)
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
// - shm: Uses pipewiresrc → system memory → encoder (1-2 CPU copies)
// - native: Uses pipewiresrc with DMA-BUF → encoder (GStreamer 1.24+, fewer copies)
// - zerocopy: Uses pipewirezerocopysrc plugin → CUDA/DMABuf memory (0 CPU copies, requires plugin)
//
// ============================================================================
// THE 4 CASES - Explicit properties for pipewirezerocopysrc (no auto-detection)
// ============================================================================
//
// Case 1: GNOME + NVIDIA (true zero-copy CUDA)
//   capture-source=pipewire buffer-type=dmabuf
//   Output: CUDAMemory → nvh264enc (0 CPU copies)
//
// Case 2: GNOME + AMD (SHM → VA encoder)
//   capture-source=pipewire buffer-type=shm
//   Output: System memory → vapostproc → vah264enc
//
// Case 3: Sway + NVIDIA
//   capture-source=wayland buffer-type=shm
//   Output: System memory → cudaupload → nvh264enc
//
// Case 4: Sway + AMD
//   capture-source=wayland buffer-type=shm
//   Output: System memory → vapostproc → vah264enc
//
// GPU optimization notes:
// - NVIDIA: cudaupload gets frames into CUDA memory, nvh264enc does colorspace on GPU
// - AMD/Intel VA-API: vah264enc handles GPU upload internally
// - Software: CPU-based videoconvert + x264enc (slowest fallback)
func (v *VideoStreamer) buildPipelineString(encoder string) string {
	var parts []string

	// Build source section based on video mode
	// pipewirezerocopysrc is the unified capture element for both GNOME and Sway.
	// For ScreenCast nodes, pipeWireFd from OpenPipeWireRemote is REQUIRED
	// Without it, pipewiresrc gets "target not found" because the default
	// PipeWire connection doesn't have access to portal ScreenCast nodes.

	switch v.videoMode {
		case VideoModePlugin:
			// pipewirezerocopysrc: Zero-copy via GPU memory sharing
			// Requires gst-plugin-pipewire-zerocopy to be installed
			//
			// EXPLICIT CONTROL: We tell the element exactly what to do:
			// - capture-source: "pipewire" (GNOME) or "wayland" (Sway ext-image-copy-capture)
			// - buffer-type: "dmabuf" (GNOME+NVIDIA CUDA) or "shm" (everything else)
			//
			// No probing, no fallbacks, no guessing. Go code knows the environment.

			// Detect compositor: Sway uses ext-image-copy-capture, GNOME uses PipeWire ScreenCast
			isSway := strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "sway") ||
				os.Getenv("SWAYSOCK") != ""

			// Detect GPU: Only GNOME+NVIDIA gets DmaBuf/CUDA, everything else uses SHM
			isNvidiaGnome := !isSway && (encoder == "nvenc" || checkGstElement("nvh264enc"))

			// GNOME + AMD/Intel: Fall back to native pipewiresrc
			// Our pipewirezerocopysrc requests MemFd but Mutter ONLY supports DmaBuf on AMD.
			// Mutter ignores MemFd request and allocates DmaBuf, which we can't mmap (tiled format).
			// Native pipewiresrc properly handles DmaBuf→vapostproc→GPU detiling.
			isAmdGnome := !isSway && !isNvidiaGnome

			if isAmdGnome {
				// Case 2: GNOME + AMD/Intel → use native pipewiresrc with always-copy=true
				// This forces SHM path (like Sway) instead of DmaBuf which has latency issues.
				// IMPORTANT: Do NOT add a capsfilter like "video/x-raw,framerate=60/1" here!
				// Mutter offers DmaBuf with modifiers, and the capsfilter breaks negotiation
				// by stripping the memory type. Let pipewiresrc negotiate directly with vapostproc.
				slog.Info("[STREAM] GNOME + AMD/Intel detected, using native pipewiresrc with always-copy=true")
				srcPart := fmt.Sprintf("pipewiresrc path=%d do-timestamp=true always-copy=true", v.nodeID)
				if v.pipeWireFd > 0 {
					srcPart += fmt.Sprintf(" fd=%d", v.pipeWireFd)
				}
				parts = []string{srcPart}
			} else {
				// Set explicit properties based on the remaining cases
				var captureSource, bufferType string
				if isSway {
					// Cases 3 & 4: Sway always uses ext-image-copy-capture (Wayland protocol)
					captureSource = "wayland"
					bufferType = "shm"
				} else {
					// Case 1: GNOME + NVIDIA → true zero-copy CUDA
					captureSource = "pipewire"
					bufferType = "dmabuf"
				}

				srcPart := fmt.Sprintf("pipewirezerocopysrc pipewire-node-id=%d capture-source=%s buffer-type=%s keepalive-time=100",
					v.nodeID, captureSource, bufferType)
				// Add fd property if we have portal FD (required for ScreenCast access)
				if v.pipeWireFd > 0 {
					srcPart += fmt.Sprintf(" pipewire-fd=%d", v.pipeWireFd)
				}
				parts = []string{srcPart}
			}

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

	// Add leaky queue to decouple pipewiresrc from encoding pipeline
	// max-size-buffers=1 keeps only the newest frame, dropping older ones immediately
	// This prevents frame buildup when Mutter drains buffered frames in bursts
	// leaky=downstream drops oldest frames if encoding falls behind (low latency)
	parts = append(parts, "queue max-size-buffers=1 leaky=downstream")

	// Step 2: Add encoder-specific conversion and encoding pipeline
	// Each encoder type has its own GPU-optimized path
	// Note: VideoModePlugin may provide GPU memory (CUDA/DMABuf) or SHM depending on compositor
	switch encoder {
	case "nvenc":
		// NVIDIA NVENC encoding
		// nvh264enc accepts BGRA/BGRx CUDAMemory directly and does conversion internally.
		//
		// Case 1: GNOME + NVIDIA (buffer-type=dmabuf)
		//   → pipewirezerocopysrc outputs CUDAMemory directly → nvh264enc (no cudaupload needed)
		//
		// Cases 2, 3, 4: Everything else (buffer-type=shm)
		//   → System memory → videoconvert → cudaupload → nvh264enc
		//
		// Sway ext-image-copy-capture outputs BGR888 (24-bit), so videoconvert is always
		// needed to convert to 32-bit RGBA for cudaupload.
		isSway := strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "sway") ||
			os.Getenv("SWAYSOCK") != ""
		isZeroCopyCuda := v.videoMode == VideoModePlugin && !isSway // Case 1 only

		if !isZeroCopyCuda {
			// SHM path: system memory → videoconvert → cudaupload → nvh264enc
			parts = append(parts,
				"videoconvert",
				"video/x-raw,format=RGBA", // Convert BGR→RGBA (32-bit) for cudaupload
				"cudaupload",
				fmt.Sprintf("nvh264enc preset=low-latency-hq zerolatency=true gop-size=%d rc-mode=cbr-ld-hq bitrate=%d aud=false", getGOPSize(), v.config.Bitrate),
			)
		} else {
			// Zero-copy GPU path: pipewirezerocopysrc outputs CUDAMemory → nvh264enc
			parts = append(parts,
				fmt.Sprintf("nvh264enc preset=low-latency-hq zerolatency=true gop-size=%d rc-mode=cbr-ld-hq bitrate=%d aud=false", getGOPSize(), v.config.Bitrate),
			)
		}

	case "qsv":
		// Intel Quick Sync Video
		// EXACTLY matches Wolf's approach: add-borders=true, system memory caps, rate-control=cqp
		parts = append(parts,
			"vapostproc add-borders=true",
			fmt.Sprintf("video/x-raw,format=NV12,width=%d,height=%d,pixel-aspect-ratio=1/1", v.config.Width, v.config.Height),
			fmt.Sprintf("qsvh264enc b-frames=0 gop-size=%d idr-interval=1 ref-frames=1 bitrate=%d rate-control=cqp target-usage=6", getGOPSize(), v.config.Bitrate),
			"h264parse",
			"video/x-h264,profile=main,stream-format=byte-stream",
		)

	case "vaapi":
		// AMD/Intel VA-API pipeline (gst-va plugin)
		// EXACTLY matches Wolf's "legacy pipeline" for AMD:
		// https://github.com/games-on-whales/wolf - uses system memory caps
		// Wolf pipeline: vapostproc add-borders=true ! video/x-raw,format=NV12 ! vah264enc rate-control=cqp
		// See design/2026-01-12-amd-vaapi-dmabuf-mystery.md for investigation.
		parts = append(parts,
			"vapostproc add-borders=true",
			fmt.Sprintf("video/x-raw,format=NV12,width=%d,height=%d,pixel-aspect-ratio=1/1", v.config.Width, v.config.Height),
			fmt.Sprintf("vah264enc aud=false b-frames=0 ref-frames=1 num-slices=1 bitrate=%d cpb-size=%d key-int-max=%d rate-control=cqp target-usage=6",
				v.config.Bitrate, v.config.Bitrate, getGOPSize()),
			"h264parse",
			"video/x-h264,profile=main,stream-format=byte-stream",
		)

	case "vaapi-lp":
		// VA-API Low Power mode (Intel-specific, gst-va plugin)
		// Matches Wolf's "legacy pipeline" - system memory caps, rate-control=cqp
		parts = append(parts,
			"vapostproc add-borders=true",
			fmt.Sprintf("video/x-raw,format=NV12,width=%d,height=%d,pixel-aspect-ratio=1/1", v.config.Width, v.config.Height),
			fmt.Sprintf("vah264lpenc aud=false b-frames=0 ref-frames=1 num-slices=1 bitrate=%d cpb-size=%d key-int-max=%d rate-control=cqp target-usage=6",
				v.config.Bitrate, v.config.Bitrate, getGOPSize()),
			"h264parse",
			"video/x-h264,profile=main,stream-format=byte-stream",
		)

	case "vaapi-legacy":
		// Legacy VA-API (gst-vaapi plugin) - wider compatibility for AMD/Intel
		// EXACTLY matches Wolf's approach: system memory caps, rate-control=cqp
		parts = append(parts,
			"vaapipostproc add-borders=true",
			fmt.Sprintf("video/x-raw,format=NV12,width=%d,height=%d,pixel-aspect-ratio=1/1", v.config.Width, v.config.Height),
			fmt.Sprintf("vaapih264enc tune=low-latency rate-control=cqp keyframe-period=%d",
				getGOPSize()),
		)

	case "openh264":
		// OpenH264 software encoder (Cisco's implementation)
		// Helix always matches desktop/client resolution, so no scaling needed
		parts = append(parts,
			"videoconvert",
			fmt.Sprintf("openh264enc complexity=low bitrate=%d gop-size=%d", v.config.Bitrate*1000, getGOPSize()),
		)

	case "x264":
		// x264 software encoder - high quality but requires gst-plugins-ugly
		// Helix always matches desktop/client resolution, so no scaling needed
		parts = append(parts,
			"videoconvert",
			fmt.Sprintf("x264enc pass=qual tune=zerolatency speed-preset=superfast b-adapt=false bframes=0 ref=1 key-int-max=%d bitrate=%d aud=false", getGOPSize(), v.config.Bitrate),
		)

	default:
		// Unknown encoder - try openh264 as last resort
		v.logger.Warn("unknown encoder, falling back to openh264", "encoder", encoder)
		parts = append(parts,
			"videoconvert",
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

	// Create video streamer - unified path for both GNOME and Sway
	// Both compositors now use pipewirezerocopysrc for zero-copy DMA-BUF capture.
	// The portal FD from OpenPipeWireRemote is required for ScreenCast access.
	pipeWireFd := 0
	if server != nil {
		pipeWireFd = server.pipeWireFd
	}
	logger.Info("using pipewirezerocopysrc (zero-copy)", "node_id", nodeID, "pipewire_fd", pipeWireFd)
	streamer := NewVideoStreamer(nodeID, pipeWireFd, config, ws, logger)

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
