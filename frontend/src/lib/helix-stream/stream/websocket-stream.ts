/**
 * WebSocket-only streaming implementation
 *
 * Replaces WebRTC for environments with only L7 (HTTP/HTTPS) ingress.
 * Uses WebCodecs API for hardware-accelerated video/audio decoding.
 */

import { Api } from "../api"
import { StreamSettings } from "../component/settings_menu"
import { defaultStreamInputConfig, StreamInput } from "./input"
import { createSupportedVideoFormatsBits, VideoCodecSupport } from "./video"
import { StreamCapabilities } from "../api_bindings"

// ============================================================================
// Binary Protocol Types (matching Rust ws_protocol.rs)
// ============================================================================

const WsMessageType = {
  VideoFrame: 0x01,
  AudioFrame: 0x02,
  VideoBatch: 0x03,  // Multiple video frames in one message (congestion handling)
  KeyboardInput: 0x10,
  MouseClick: 0x11,
  MouseAbsolute: 0x12,
  MouseRelative: 0x13,
  TouchEvent: 0x14,
  ControllerEvent: 0x15,
  ControllerState: 0x16,
  ControlMessage: 0x20,
  StreamInit: 0x30,
  StreamError: 0x31,
  Ping: 0x40,
  Pong: 0x41,
} as const

// Exported for reuse in SSE video handling in DesktopStreamViewer.tsx
export const WsVideoCodec = {
  H264: 0x01,
  H264High444: 0x02,
  H265: 0x10,
  H265Main10: 0x11,
  H265Rext8_444: 0x12,
  H265Rext10_444: 0x13,
  Av1Main8: 0x20,
  Av1Main10: 0x21,
  Av1High8_444: 0x22,
  Av1High10_444: 0x23,
} as const

export type WsVideoCodecType = typeof WsVideoCodec[keyof typeof WsVideoCodec]

// Map codec byte to WebCodecs codec string
// Exported for reuse in SSE video handling
export function codecToWebCodecsString(codec: number): string {
  switch (codec) {
    case WsVideoCodec.H264: return "avc1.4d0033"
    case WsVideoCodec.H264High444: return "avc1.640032"
    case WsVideoCodec.H265: return "hvc1.1.6.L120.90"
    case WsVideoCodec.H265Main10: return "hvc1.2.4.L120.90"
    case WsVideoCodec.H265Rext8_444: return "hvc1.4.10.L120.90"
    case WsVideoCodec.H265Rext10_444: return "hvc1.4.10.L120.90"
    case WsVideoCodec.Av1Main8: return "av01.0.08M.08"
    case WsVideoCodec.Av1Main10: return "av01.0.08M.10"
    case WsVideoCodec.Av1High8_444: return "av01.1.08H.08"
    case WsVideoCodec.Av1High10_444: return "av01.1.08H.10"
    default: return "avc1.4d0033" // Default to H264
  }
}

// Map codec byte to human-readable display name for stats UI
// Exported for reuse in SSE video handling
export function codecToDisplayName(codec: number | null): string {
  if (codec === null) return "Unknown"
  switch (codec) {
    case WsVideoCodec.H264: return "H.264"
    case WsVideoCodec.H264High444: return "H.264 High 4:4:4"
    case WsVideoCodec.H265: return "HEVC"
    case WsVideoCodec.H265Main10: return "HEVC Main10"
    case WsVideoCodec.H265Rext8_444: return "HEVC RExt 4:4:4"
    case WsVideoCodec.H265Rext10_444: return "HEVC RExt 10bit 4:4:4"
    case WsVideoCodec.Av1Main8: return "AV1"
    case WsVideoCodec.Av1Main10: return "AV1 10bit"
    case WsVideoCodec.Av1High8_444: return "AV1 High 4:4:4"
    case WsVideoCodec.Av1High10_444: return "AV1 High 10bit 4:4:4"
    default: return `Unknown (0x${codec.toString(16)})`
  }
}

// ============================================================================
// Event Types
// ============================================================================

export type WsStreamInfoEvent = CustomEvent<
  | { type: "error"; message: string }
  | { type: "connecting" }
  | { type: "connected" }
  | { type: "disconnected" }
  | { type: "reconnecting"; attempt: number }
  | { type: "streamInit"; width: number; height: number; fps: number }
  | { type: "connectionComplete"; capabilities: StreamCapabilities }
  | { type: "addDebugLine"; line: string }
>
export type WsStreamInfoEventListener = (event: WsStreamInfoEvent) => void

// ============================================================================
// WebSocket Stream Class
// ============================================================================

export class WebSocketStream {
  private api: Api
  private hostId: number
  private appId: number
  private settings: StreamSettings
  private sessionId?: string
  private supportedVideoFormats: VideoCodecSupport

  private ws: WebSocket | null = null
  private eventTarget = new EventTarget()

  // Canvas for rendering
  private canvas: HTMLCanvasElement | null = null
  private canvasCtx: CanvasRenderingContext2D | null = null

  // WebCodecs decoders
  private videoDecoder: VideoDecoder | null = null
  private audioDecoder: AudioDecoder | null = null
  private audioContext: AudioContext | null = null

  // Input handling
  private input: StreamInput

  // Stream state
  private streamerSize: [number, number]
  private connected = false
  private reconnectAttempts = 0
  private maxReconnectAttempts = 10  // Increased from 5 for better reliability
  private reconnectDelay = 1000
  private reconnectTimeoutId: ReturnType<typeof setTimeout> | null = null
  private closed = false  // True when explicitly closed (prevents reconnection)

  // Connection timeout - if onOpen doesn't fire within this time, force reconnect
  private connectionTimeoutId: ReturnType<typeof setTimeout> | null = null
  private readonly CONNECTION_TIMEOUT_MS = 15000  // 15 seconds

  // Heartbeat for stale connection detection
  private heartbeatIntervalId: ReturnType<typeof setInterval> | null = null
  private lastMessageTime = 0
  private heartbeatTimeout = 10000  // 10 seconds without data = stale

  // Frame timing and stats
  private lastFrameTime = 0
  private frameCount = 0
  private currentFps = 0
  // Video payload bytes (H.264 data only, excluding protocol headers)
  private videoPayloadBytes = 0
  private lastVideoPayloadBytes = 0
  private currentVideoPayloadBitrateMbps = 0
  // Total WebSocket bytes received (video + audio + control + all headers)
  private totalBytesReceived = 0
  private lastTotalBytesReceived = 0
  private lastBytesTime = 0
  private currentTotalBitrateMbps = 0
  private framesDecoded = 0
  private framesDropped = 0

  // RTT (Round-Trip Time) measurement for latency tracking
  private pingSeq = 0
  private pendingPings = new Map<number, number>()  // seq → sendTime (performance.now())
  private rttSamples: number[] = []
  private currentRttMs = 0
  private encoderLatencyMs = 0  // Encoder pipeline latency from server (PTS to WebSocket send)
  private pingIntervalId: ReturnType<typeof setInterval> | null = null
  private readonly PING_INTERVAL_MS = 500   // Send ping every 500ms for faster RTT feedback
  private readonly MAX_RTT_SAMPLES = 10  // Keep last 10 samples for moving average
  private readonly HIGH_LATENCY_THRESHOLD_MS = 150  // Show warning above this

  // Batching stats for congestion visibility
  private batchesReceived = 0  // Total number of batch messages received
  private batchedFramesReceived = 0  // Total frames received in batches
  private individualFramesReceived = 0  // Total frames received individually
  private recentBatchSizes: number[] = []  // Last N batch sizes for avg calculation
  private readonly MAX_BATCH_SIZE_SAMPLES = 20

  // Frame latency tracking (arrival time vs expected based on PTS)
  // This measures actual frame delivery latency, not just Ping/Pong RTT
  private firstFramePtsUs: number | null = null       // PTS of first frame (microseconds)
  private firstFrameArrivalTime: number | null = null // performance.now() when first frame arrived
  private currentFrameLatencyMs = 0                   // How late current frame arrived (ms)
  private frameLatencySamples: number[] = []          // Recent samples for smoothing
  private readonly MAX_FRAME_LATENCY_SAMPLES = 30     // ~0.5 sec at 60fps

  // Decoder queue monitoring - tracks if decoder is backing up (for stats display)
  // When queue is high AND we receive a keyframe, we flush and skip to the keyframe
  private lastDecodeQueueSize = 0
  private maxDecodeQueueSize = 0                      // Peak queue size seen
  private readonly QUEUE_FLUSH_THRESHOLD = 10         // Flush queue when > 10 frames backed up
  private framesSkippedToKeyframe = 0                 // Count of frames flushed for stats
  private queueBackupLogged = false                   // Prevent log spam during queue backup

  // Adaptive input throttling based on RTT
  // Reduces mouse/scroll event rate when network latency is high to prevent frame queueing
  private adaptiveThrottleRatio = 1.0                 // 1.0 = full rate, 0.25 = 25% rate
  private manualThrottleRatio: number | null = null   // For debug override (null = use adaptive)

  // Mouse input throttling - prevents flooding WebSocket with high-polling-rate mice (500-1000 Hz)
  // Throttle rate is adaptive based on RTT - see getAdaptiveThrottleMs()
  private lastMouseSendTime = 0
  private pendingMousePosition: { x: number; y: number; refW: number; refH: number } | null = null
  private pendingMouseMove: { dx: number; dy: number } | null = null
  private mouseThrottleTimeoutId: ReturnType<typeof setTimeout> | null = null

  // Scroll input throttling - same principle as mouse, no point sending faster than frame rate
  // Scroll deltas accumulate during throttle period (like relative mouse movement)
  private lastScrollSendTime = 0
  private pendingScroll: { dx: number; dy: number; highRes: boolean } | null = null
  private scrollThrottleTimeoutId: ReturnType<typeof setTimeout> | null = null

  // Input send buffer congestion detection
  // Skip mouse moves immediately if buffer hasn't drained - prevents "ghost moves"
  // that arrive late and make it look like something else is controlling the cursor
  private lastBufferDrainTime = 0         // When we last saw bufferedAmount == 0
  private lastBufferedAmount = 0          // Current send buffer size
  private maxBufferedAmount = 0           // Peak buffer size seen
  private inputsDroppedDueToCongestion = 0 // Count of mouse moves skipped
  private inputsSent = 0                  // Total inputs sent
  private inputBufferSamples: number[] = [] // Recent buffer samples for averaging
  private readonly MAX_INPUT_BUFFER_SAMPLES = 30
  private bufferStaleMs = 0               // How long buffer has been non-empty

  // Input send latency tracking
  // Measures time from ws.send() call to completion (should be ~0 if non-blocking)
  // and tracks bufferedAmount changes to detect TCP-level queueing
  private lastSendDurationMs = 0          // How long ws.send() took (should be ~0)
  private maxSendDurationMs = 0           // Peak send duration seen
  private sendDurationSamples: number[] = []
  private readonly MAX_SEND_DURATION_SAMPLES = 30
  private bufferedAmountBeforeSend = 0    // Buffer size before send
  private bufferedAmountAfterSend = 0     // Buffer size after send (shows what we added)

  // Event loop latency tracking
  // Uses periodic setTimeout(0) heartbeat to measure actual event loop responsiveness
  // If event loop is blocked (video decoding, DOM operations, etc.), setTimeout(0) is delayed
  private eventLoopCheckScheduledAt = 0    // When we scheduled setTimeout(0)
  private eventLoopLatencyMs = 0           // Current event loop latency (excess delay)
  private maxEventLoopLatencyMs = 0        // Peak latency seen
  private eventLoopLatencySamples: number[] = []
  private readonly MAX_EVENT_LOOP_SAMPLES = 30
  private readonly EVENT_LOOP_CHECK_INTERVAL_MS = 100  // Check every 100ms
  private eventLoopCheckTimeoutId: ReturnType<typeof setTimeout> | null = null

  // Unique client identifier for Wolf session matching
  // The frontend generates this and passes it to BOTH the Helix API (to pre-configure Wolf)
  // AND here (to send to moonlight-web). This enables immediate lobby attachment.
  private clientUniqueId?: string

  constructor(
    api: Api,
    hostId: number,
    appId: number,
    settings: StreamSettings,
    supportedVideoFormats: VideoCodecSupport,
    viewerScreenSize: [number, number],
    sessionId?: string,
    clientUniqueId?: string
  ) {
    this.api = api
    this.hostId = hostId
    this.appId = appId
    this.settings = settings
    this.supportedVideoFormats = supportedVideoFormats
    this.sessionId = sessionId
    this.clientUniqueId = clientUniqueId
    this.streamerSize = this.calculateStreamerSize(viewerScreenSize)

    // Initialize input handler
    // Use evdev keycodes for direct WebSocket mode - bypasses VK→evdev conversion on backend
    const streamInputConfig = defaultStreamInputConfig()
    Object.assign(streamInputConfig, {
      mouseScrollMode: this.settings.mouseScrollMode,
      controllerConfig: this.settings.controllerConfig,
      useEvdevCodes: true,  // Direct Linux evdev codes for WebSocket mode
    })
    this.input = new StreamInput(streamInputConfig)

    // Patch StreamInput's send methods to use WebSocket transport instead of DataChannels
    this.patchInputMethods()

    // Connect
    this.connect()
  }

  private patchInputMethods() {
    const wsStream = this
    // @ts-ignore - accessing private methods for patching
    this.input.sendKey = (isDown: boolean, key: number, modifiers: number) => {
      wsStream.sendKey(isDown, key, modifiers)
    }
    // @ts-ignore
    this.input.sendMouseMove = (movementX: number, movementY: number) => {
      wsStream.sendMouseMove(movementX, movementY)
    }
    // @ts-ignore
    this.input.sendMousePosition = (x: number, y: number, refW: number, refH: number) => {
      wsStream.sendMousePosition(x, y, refW, refH)
    }
    // @ts-ignore
    this.input.sendMouseButton = (isDown: boolean, button: number) => {
      wsStream.sendMouseButton(isDown, button)
    }
    // @ts-ignore
    this.input.sendMouseWheelHighRes = (deltaX: number, deltaY: number) => {
      wsStream.sendMouseWheelHighRes(deltaX, deltaY)
    }
    // @ts-ignore
    this.input.sendMouseWheel = (deltaX: number, deltaY: number) => {
      wsStream.sendMouseWheel(deltaX, deltaY)
    }
    // @ts-ignore - patch sendTouch to use WebSocket transport
    const origCalcNormalizedPosition = this.input['calcNormalizedPosition'].bind(this.input)
    // @ts-ignore
    this.input['sendTouch'] = (type: number, touch: Touch, rect: DOMRect) => {
      const position = origCalcNormalizedPosition(touch.clientX, touch.clientY, rect)
      if (position) {
        wsStream.sendTouch(type, touch.identifier, position[0], position[1])
      }
    }
  }

  private calculateStreamerSize(viewerScreenSize: [number, number]): [number, number] {
    let width: number, height: number
    if (this.settings.videoSize === "720p") {
      width = 1280
      height = 720
    } else if (this.settings.videoSize === "1080p") {
      width = 1920
      height = 1080
    } else if (this.settings.videoSize === "1440p") {
      width = 2560
      height = 1440
    } else if (this.settings.videoSize === "4k") {
      width = 3840
      height = 2160
    } else if (this.settings.videoSize === "5k") {
      width = 5120
      height = 2880
    } else if (this.settings.videoSize === "custom") {
      width = this.settings.videoSizeCustom.width
      height = this.settings.videoSizeCustom.height
    } else {
      // native
      width = viewerScreenSize[0]
      height = viewerScreenSize[1]
    }
    return [width, height]
  }

  private connect() {
    // Don't connect if explicitly closed
    if (this.closed) {
      console.log("[WebSocketStream] Not connecting - stream was explicitly closed")
      return
    }

    this.dispatchInfoEvent({ type: "connecting" })

    // Clean up old WebSocket if it exists
    if (this.ws) {
      try {
        this.ws.close()
      } catch (e) {
        // Ignore
      }
      this.ws = null
    }

    // Clean up decoders for fresh start
    this.cleanupDecoders()

    // Reset stream state for fresh connection
    this.resetStreamState()

    // Build WebSocket URL - direct endpoint (bypasses Wolf/Moonlight)
    // Uses /api/v1/external-agents/{sessionId}/ws/stream for direct GStreamer encoding
    // Auth is handled via cookies (same-origin WebSocket includes cookies automatically)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/v1/external-agents/${encodeURIComponent(this.sessionId || '')}/ws/stream`

    console.log("[WebSocketStream] Connecting to:", wsUrl)
    this.ws = new WebSocket(wsUrl)
    this.ws.binaryType = "arraybuffer"

    this.ws.addEventListener("open", this.onOpen.bind(this))
    this.ws.addEventListener("close", this.onClose.bind(this))
    this.ws.addEventListener("error", this.onError.bind(this))
    this.ws.addEventListener("message", this.onMessage.bind(this))

    // Start connection timeout - if onOpen doesn't fire, force reconnect
    this.clearConnectionTimeout()
    this.connectionTimeoutId = setTimeout(() => {
      console.warn(`[WebSocketStream] Connection timeout (${this.CONNECTION_TIMEOUT_MS}ms), forcing reconnect`)
      this.dispatchInfoEvent({ type: "error", message: "Connection timeout" })
      // Close the stuck WebSocket and trigger reconnection
      if (this.ws) {
        try {
          this.ws.close()
        } catch (e) {
          // Ignore
        }
      }
      // onClose will handle reconnection
    }, this.CONNECTION_TIMEOUT_MS)
  }

  private clearConnectionTimeout() {
    if (this.connectionTimeoutId) {
      clearTimeout(this.connectionTimeoutId)
      this.connectionTimeoutId = null
    }
  }

  private onOpen() {
    console.log("[WebSocketStream] Connected")
    this.connected = true
    this.reconnectAttempts = 0
    this.lastMessageTime = Date.now()

    // Clear connection timeout - we connected successfully
    this.clearConnectionTimeout()

    this.dispatchInfoEvent({ type: "connected" })

    // Start heartbeat monitoring for stale connections
    this.startHeartbeat()

    // Send initialization message FIRST - server expects this before any binary messages
    // The server reads the first message and parses it as JSON init config
    this.sendInit()

    // Start RTT measurement pings AFTER init is sent
    this.startPingInterval()

    // Start event loop latency tracking
    this.startEventLoopTracking()
  }

  private onClose(event: CloseEvent) {
    console.log("[WebSocketStream] Disconnected:", event.code, event.reason)
    this.connected = false

    // Clear connection timeout if it's still running
    this.clearConnectionTimeout()

    // Stop heartbeat
    this.stopHeartbeat()

    // Stop RTT pings
    this.stopPingInterval()

    // Stop event loop tracking
    this.stopEventLoopTracking()

    this.dispatchInfoEvent({ type: "disconnected" })

    // Don't reconnect if explicitly closed
    if (this.closed) {
      console.log("[WebSocketStream] Not reconnecting - stream was explicitly closed")
      return
    }

    // Attempt reconnection with exponential backoff (capped at 10 seconds)
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++
      const delay = Math.min(this.reconnectDelay * this.reconnectAttempts, 10000)
      this.dispatchInfoEvent({ type: "reconnecting", attempt: this.reconnectAttempts })

      console.log(`[WebSocketStream] Will reconnect in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`)

      // Cancel any pending reconnection
      if (this.reconnectTimeoutId) {
        clearTimeout(this.reconnectTimeoutId)
      }

      this.reconnectTimeoutId = setTimeout(() => {
        this.reconnectTimeoutId = null
        console.log(`[WebSocketStream] Reconnecting (attempt ${this.reconnectAttempts})...`)
        this.connect()
      }, delay)
    } else {
      console.error(`[WebSocketStream] Max reconnection attempts (${this.maxReconnectAttempts}) reached, giving up`)
      this.dispatchInfoEvent({ type: "error", message: "Connection lost - max reconnection attempts reached" })
    }
  }

  private onError(event: Event) {
    // Don't dispatch error events when explicitly closing - this is expected
    if (this.closed) {
      console.log("[WebSocketStream] Error event during explicit close (ignored):", event)
      return
    }
    console.error("[WebSocketStream] Error:", event)
    this.dispatchInfoEvent({ type: "error", message: "WebSocket error" })
  }

  private async onMessage(event: MessageEvent) {
    // Update heartbeat timestamp on any message
    this.lastMessageTime = Date.now()

    if (!(event.data instanceof ArrayBuffer)) {
      // JSON control message (text frame) - track string length as bytes
      const textData = event.data as string
      this.totalBytesReceived += textData.length
      try {
        const json = JSON.parse(textData)
        this.handleControlMessage(json)
      } catch (e) {
        console.error("[WebSocketStream] Failed to parse JSON message:", e)
      }
      return
    }

    const data = new Uint8Array(event.data)
    if (data.length === 0) return

    // Track total bytes received (all binary messages)
    this.totalBytesReceived += data.length

    const msgType = data[0]

    switch (msgType) {
      case WsMessageType.VideoFrame:
        await this.handleVideoFrame(data)
        break
      case WsMessageType.VideoBatch:
        await this.handleVideoBatch(data)
        break
      case WsMessageType.AudioFrame:
        await this.handleAudioFrame(data)
        break
      case WsMessageType.StreamInit:
        this.handleStreamInit(data)
        break
      case WsMessageType.ControlMessage:
        // JSON embedded in binary
        const json = new TextDecoder().decode(data.slice(1))
        this.handleControlMessage(JSON.parse(json))
        break
      case WsMessageType.TouchEvent:
      case WsMessageType.ControllerEvent:
        // Server → client events (rumble, etc.)
        this.input.handleServerMessage(msgType, data.slice(1))
        break
      case WsMessageType.Pong:
        this.handlePong(data)
        break
      default:
        console.warn("[WebSocketStream] Unknown message type:", msgType)
    }
  }

  private sendInit() {
    // Use actual browser codec support detection (from constructor)
    // This tells the server which codecs the browser can decode
    const supportBits = createSupportedVideoFormatsBits(this.supportedVideoFormats)

    console.log('[WebSocketStream] Sending init with supported formats:', {
      bits: supportBits,
      formats: this.supportedVideoFormats,
    })

    // Send initialization as JSON for simplicity
    // client_unique_id is passed to Wolf for immediate lobby attachment
    // The frontend generates this and calls Helix API to pre-configure Wolf BEFORE connecting
    const initMessage: Record<string, unknown> = {
      type: "init",
      host_id: this.hostId,
      app_id: this.appId,
      session_id: this.sessionId,
      width: this.streamerSize[0],
      height: this.streamerSize[1],
      fps: this.settings.fps,
      bitrate: this.settings.bitrate,
      packet_size: this.settings.packetSize,
      play_audio_local: this.settings.playAudioLocal,
      video_supported_formats: supportBits,
    }

    // Include client_unique_id if provided (enables immediate lobby attachment)
    if (this.clientUniqueId) {
      initMessage.client_unique_id = this.clientUniqueId
    }

    // Include video_mode if specified (controls backend capture pipeline)
    if (this.settings.videoMode) {
      initMessage.video_mode = this.settings.videoMode
    }

    this.ws?.send(JSON.stringify(initMessage))
  }

  private handleControlMessage(msg: any) {
    console.log("[WebSocketStream] Control message:", msg)

    if (msg.ConnectionComplete) {
      const { capabilities, width, height } = msg.ConnectionComplete
      this.dispatchInfoEvent({
        type: "connectionComplete",
        capabilities: capabilities || { touch: false },
      })
      this.input.onStreamStart(capabilities || { touch: false }, [width, height])
    } else if (msg.error) {
      this.dispatchInfoEvent({ type: "error", message: msg.error })
    }
  }

  private handleStreamInit(data: Uint8Array) {
    // Parse StreamInit message
    // Format: type(1) + codec(1) + width(2) + height(2) + fps(1) + audio_channels(1) + sample_rate(4) + touch(1)
    if (data.length < 13) {
      console.error("[WebSocketStream] StreamInit too short")
      return
    }

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const codec = data[1] as WsVideoCodecType
    const width = view.getUint16(2, false) // big-endian
    const height = view.getUint16(4, false)
    const fps = data[6]
    const audioChannels = data[7]
    const sampleRate = view.getUint32(8, false)
    const touchSupported = data[12] !== 0

    console.log(`[WebSocketStream] StreamInit: ${width}x${height}@${fps}fps, codec=${codec}, audio=${audioChannels}ch@${sampleRate}Hz, touch=${touchSupported}`)

    this.streamerSize = [width, height]
    this.dispatchInfoEvent({ type: "streamInit", width, height, fps })

    // Reset frame latency tracking - new stream has new PTS epoch
    // Without this, frame drift calculation goes haywire after reconnect/resolution change
    this.firstFramePtsUs = null
    this.firstFrameArrivalTime = null
    this.frameLatencySamples = []
    this.currentFrameLatencyMs = 0

    // Initialize video decoder
    this.initVideoDecoder(codec, width, height)

    // Initialize audio decoder (skip if no audio configured)
    if (audioChannels > 0 && sampleRate > 0) {
      this.initAudioDecoder(audioChannels, sampleRate)
    } else {
      console.log("[WebSocketStream] Audio disabled (no audio channels or sample rate)")
    }
  }

  // Decoder generation counter - incremented each time we create a new decoder
  // Used to ignore stale error callbacks from old decoders
  private decoderGeneration = 0

  private async initVideoDecoder(codec: WsVideoCodecType, width: number, height: number) {
    if (!("VideoDecoder" in window)) {
      console.error("[WebSocketStream] WebCodecs VideoDecoder not supported")
      this.dispatchInfoEvent({ type: "error", message: "WebCodecs not supported in this browser" })
      return
    }

    // Increment decoder generation to invalidate stale callbacks
    const thisGeneration = ++this.decoderGeneration
    console.log(`[WebSocketStream] Creating decoder generation ${thisGeneration}`)

    // Store config for potential recovery
    this.lastVideoCodec = codec
    this.lastVideoWidth = width
    this.lastVideoHeight = height

    const codecString = codecToWebCodecsString(codec)
    console.log(`[WebSocketStream] Initializing video decoder: ${codecString} ${width}x${height}`)

    // Check if codec is supported - try hardware first, then software fallback
    let useHardwareAcceleration: "prefer-hardware" | "prefer-software" | "no-preference" = "prefer-hardware"
    try {
      const hwSupport = await VideoDecoder.isConfigSupported({
        codec: codecString,
        codedWidth: width,
        codedHeight: height,
        hardwareAcceleration: "prefer-hardware",
      })

      if (!hwSupport.supported) {
        // Hardware not supported, try software decoding
        console.log("[WebSocketStream] Hardware decoding not supported, trying software fallback")
        const swSupport = await VideoDecoder.isConfigSupported({
          codec: codecString,
          codedWidth: width,
          codedHeight: height,
          // No hardwareAcceleration = allow any
        })

        if (!swSupport.supported) {
          console.error("[WebSocketStream] Video codec not supported (hardware or software):", codecString)
          this.dispatchInfoEvent({ type: "error", message: `Video codec ${codecString} not supported` })
          return
        }
        useHardwareAcceleration = "no-preference"
        console.log("[WebSocketStream] Using software video decoding")
      } else {
        console.log("[WebSocketStream] Using hardware video decoding")
      }
    } catch (e) {
      console.error("[WebSocketStream] Failed to check codec support:", e)
      // Continue anyway and let configure() fail if truly unsupported
    }

    // Store the working acceleration mode for recovery after reset
    this.lastVideoHwAccel = useHardwareAcceleration

    // Close existing decoder
    if (this.videoDecoder) {
      try {
        this.videoDecoder.close()
      } catch (e) {
        // Ignore
      }
    }

    this.videoDecoder = new VideoDecoder({
      output: (frame: VideoFrame) => {
        this.renderVideoFrame(frame)
      },
      error: (e: Error) => {
        // Check if this callback is from a stale decoder (already replaced)
        if (thisGeneration !== this.decoderGeneration) {
          console.log(`[WebSocketStream] Ignoring stale decoder error from generation ${thisGeneration} (current: ${this.decoderGeneration})`)
          return
        }

        console.error("[WebSocketStream] Video decoder error, will wait for next keyframe:", e)
        console.error("[WebSocketStream] Decoder state:", {
          generation: thisGeneration,
          framesDecoded: this.framesDecoded,
          queueSize: this.videoDecoder?.decodeQueueSize,
          state: this.videoDecoder?.state,
          codec: this.lastVideoCodec,
          resolution: `${this.lastVideoWidth}x${this.lastVideoHeight}`,
          hwAccel: this.lastVideoHwAccel,
        })
        // Reset keyframe flag so we wait for a fresh keyframe before decoding again
        this.receivedFirstKeyframe = false
        // Attempt decoder recovery if we have the codec info
        if (this.lastVideoCodec !== null && this.lastVideoWidth > 0 && this.lastVideoHeight > 0) {
          console.log("[WebSocketStream] Attempting decoder recovery...")
          this.initVideoDecoder(this.lastVideoCodec, this.lastVideoWidth, this.lastVideoHeight)
            .catch(err => console.error("[WebSocketStream] Failed to recover video decoder:", err))
        }
      },
    })

    // Configure decoder with Annex B format for H264/H265 (in-band SPS/PPS)
    // This tells WebCodecs to expect NAL start codes and in-band parameter sets
    const config: VideoDecoderConfig = {
      codec: codecString,
      codedWidth: width,
      codedHeight: height,
      hardwareAcceleration: useHardwareAcceleration,
    }

    // For H264, specify Annex B format to handle in-band SPS/PPS
    if (codecString.startsWith("avc1")) {
      // @ts-ignore - avc property is part of the spec but not in TypeScript types yet
      config.avc = { format: "annexb" }
    }
    // For HEVC, similar configuration
    if (codecString.startsWith("hvc1") || codecString.startsWith("hev1")) {
      // @ts-ignore - hevc property for Annex B format
      config.hevc = { format: "annexb" }
    }

    try {
      this.videoDecoder.configure(config)
      console.log("[WebSocketStream] Video decoder configured:", config)
    } catch (e) {
      console.error("[WebSocketStream] Failed to configure video decoder:", e)
      // Try without the format hint as fallback
      this.videoDecoder.configure({
        codec: codecString,
        codedWidth: width,
        codedHeight: height,
        hardwareAcceleration: useHardwareAcceleration,
      })
      console.log("[WebSocketStream] Video decoder configured (fallback mode)")
    }
  }

  private renderVideoFrame(frame: VideoFrame) {
    // CRITICAL: Prevent rendering after stream is closed
    // This prevents duplicate streams from writing to the same canvas
    if (this.closed) {
      frame.close()
      return
    }

    if (!this.canvas || !this.canvasCtx) {
      frame.close()
      this.framesDropped++
      return
    }

    // Resize canvas if needed
    if (this.canvas.width !== frame.displayWidth || this.canvas.height !== frame.displayHeight) {
      this.canvas.width = frame.displayWidth
      this.canvas.height = frame.displayHeight
    }

    // Draw frame to canvas
    this.canvasCtx.drawImage(frame, 0, 0)
    frame.close()
    this.framesDecoded++

    // Track frame rate (update every second)
    this.frameCount++
    const now = performance.now()
    if (now - this.lastFrameTime >= 1000) {
      this.currentFps = this.frameCount
      this.frameCount = 0
      this.lastFrameTime = now

      // Calculate bitrates
      if (this.lastBytesTime > 0) {
        const deltaTime = (now - this.lastBytesTime) / 1000 // seconds
        if (deltaTime > 0) {
          // Video payload bitrate (H.264 data only, excluding protocol headers)
          const deltaVideoPayload = this.videoPayloadBytes - this.lastVideoPayloadBytes
          this.currentVideoPayloadBitrateMbps = (deltaVideoPayload * 8) / 1000000 / deltaTime
          // Total WebSocket bitrate (everything received: video + audio + control + headers)
          const deltaTotalBytes = this.totalBytesReceived - this.lastTotalBytesReceived
          this.currentTotalBitrateMbps = (deltaTotalBytes * 8) / 1000000 / deltaTime
        }
      }
      this.lastVideoPayloadBytes = this.videoPayloadBytes
      this.lastTotalBytesReceived = this.totalBytesReceived
      this.lastBytesTime = now
    }
  }

  // Track if we've received the first keyframe (needed for decoder to work)
  private receivedFirstKeyframe = false

  // Track video enabled state to make setVideoEnabled idempotent
  private _videoEnabled = true

  // Track audio enabled state to make setAudioEnabled idempotent
  // Audio is disabled by default - user must explicitly enable via toolbar
  private _audioEnabled = false

  // Track last video config for decoder recovery
  private lastVideoCodec: WsVideoCodecType | null = null
  private lastVideoWidth = 0
  private lastVideoHeight = 0
  private lastVideoHwAccel: "prefer-hardware" | "prefer-software" | "no-preference" = "prefer-hardware"

  private async handleVideoFrame(data: Uint8Array, fromBatch = false) {
    if (!this.videoDecoder || this.videoDecoder.state !== "configured") {
      // Queue frames or drop them if decoder isn't ready
      return
    }

    const arrivalTime = performance.now()

    // Track individual vs batched frames for stats
    if (!fromBatch) {
      this.individualFramesReceived++
    }

    // Parse video frame header
    // Format: type(1) + codec(1) + flags(1) + pts(8) + width(2) + height(2) + data(...)
    if (data.length < 15) {
      console.error("[WebSocketStream] Video frame too short:", data.length)
      return
    }

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const msgType = data[0]
    const codec = data[1]
    const flags = data[2]
    const isKeyframe = (flags & 0x01) !== 0
    const ptsUs = view.getBigUint64(3, false) // big-endian

    // DEBUG: Log first 10 frames received (before decode)
    // Use a class counter since framesDecoded only increments on successful decode
    this.framesReceived = (this.framesReceived || 0) + 1
    if (this.framesReceived <= 10) {
      // Log header bytes to debug PTS issues
      const ptsBytes = Array.from(data.slice(3, 11)).map(b => b.toString(16).padStart(2, "0")).join(" ")
      console.log(`[WebSocketStream] Frame ${this.framesReceived}: type=${msgType} codec=${codec} flags=0x${flags.toString(16)} isKeyframe=${isKeyframe} size=${data.length} pts=${ptsUs} (bytes: ${ptsBytes})`)
    }
    // width at offset 11, height at offset 13 (already have from StreamInit)

    const frameData = data.slice(15)

    // Track video PAYLOAD bytes only (H.264 data, excluding 15-byte protocol header)
    this.videoPayloadBytes += frameData.length

    // === Frame Latency Tracking ===
    // Measure how late frames arrive compared to when they should based on PTS
    // Skip frames with invalid PTS (0 or negative) to avoid bogus drift calculations
    const ptsUsNum = Number(ptsUs)
    if (ptsUsNum <= 0) {
      // Invalid PTS, skip drift tracking for this frame
    } else if (this.firstFramePtsUs === null || this.firstFramePtsUs <= 0) {
      // First valid frame: establish baseline
      this.firstFramePtsUs = ptsUsNum
      this.firstFrameArrivalTime = arrivalTime
      this.currentFrameLatencyMs = 0
    } else {
      // Calculate expected arrival time based on PTS delta from first frame
      const ptsDeltaMs = (ptsUsNum - this.firstFramePtsUs) / 1000
      const expectedArrivalTime = this.firstFrameArrivalTime! + ptsDeltaMs

      // Latency = how much later than expected the frame arrived
      // Positive = frames arriving late, negative = frames arriving early
      const latencyMs = arrivalTime - expectedArrivalTime

      // Detect PTS discontinuity (pipeline restart without StreamInit)
      // If latency is absurdly large (> 60 seconds), reset baseline
      if (Math.abs(latencyMs) > 60000) {
        // Log detailed info to debug oscillating PTS issue
        console.warn(`[WebSocketStream] PTS discontinuity: drift=${latencyMs.toFixed(0)}ms, pts=${ptsUsNum}, firstPts=${this.firstFramePtsUs}, ptsDelta=${ptsDeltaMs.toFixed(0)}ms`)
        this.firstFramePtsUs = ptsUsNum
        this.firstFrameArrivalTime = arrivalTime
        this.frameLatencySamples = []
        this.currentFrameLatencyMs = 0
      } else {
        // Keep a moving average for stability
        this.frameLatencySamples.push(latencyMs)
        if (this.frameLatencySamples.length > this.MAX_FRAME_LATENCY_SAMPLES) {
          this.frameLatencySamples.shift()
        }
        this.currentFrameLatencyMs = this.frameLatencySamples.reduce((a, b) => a + b, 0) / this.frameLatencySamples.length
      }
    }

    // === Decoder Queue Monitoring ===
    // Track decoder queue size for stats/debugging
    const queueSize = this.videoDecoder.decodeQueueSize
    this.lastDecodeQueueSize = queueSize
    if (queueSize > this.maxDecodeQueueSize) {
      this.maxDecodeQueueSize = queueSize
    }

    // Skip delta frames until we receive the first keyframe
    // (keyframe should contain SPS/PPS needed for decoding)
    if (!this.receivedFirstKeyframe) {
      if (!isKeyframe) {
        console.log("[WebSocketStream] Waiting for first keyframe, skipping delta frame")
        return
      }
      // Debug: hexdump first 32 bytes to see NAL structure
      // Helps diagnose HEVC description issues - compare with SSE mode
      if (frameData.length >= 32) {
        const hexBytes = Array.from(frameData.slice(0, 32))
          .map(b => b.toString(16).padStart(2, "0"))
          .join(" ")
        console.log(`[WebSocketStream] Keyframe first 32 bytes: ${hexBytes}`)
        // Check NAL type after start code
        const hasStartCode4 = frameData[0] === 0 && frameData[1] === 0 && frameData[2] === 0 && frameData[3] === 1
        if (hasStartCode4) {
          const nalTypeByte = frameData[4]
          const isH264 = (nalTypeByte & 0x80) === 0 && (nalTypeByte & 0x60) !== 0
          if (isH264) {
            const h264NalType = nalTypeByte & 0x1F
            console.log(`[WebSocketStream] H.264 NAL type: ${h264NalType}`)
          } else {
            const hevcNalType = (nalTypeByte >> 1) & 0x3F
            console.log(`[WebSocketStream] HEVC NAL type: ${hevcNalType} (VPS=32, SPS=33, PPS=34, IDR=19/20)`)
          }
        }
      }
      console.log(`[WebSocketStream] First keyframe received (${frameData.length} bytes)`)
      this.receivedFirstKeyframe = true
      // Notify that video is starting (first frame is being decoded)
      this.dispatchInfoEvent({ type: "videoStarted" })
    }

    // === Queue Monitoring (flush disabled) ===
    // Queue flush on keyframe is disabled because it causes decoder errors on software H.264
    // (CPU contention between decode and fetch processing leads to decode failures after reset)
    // Instead, just log when queue backs up and let it naturally catch up
    if (queueSize > this.QUEUE_FLUSH_THRESHOLD) {
      // Only log once per backup event (not every frame)
      if (!this.queueBackupLogged) {
        console.warn(`[WebSocketStream] Queue backed up (${queueSize} frames), waiting for catchup`)
        this.queueBackupLogged = true
      }
    } else if (this.queueBackupLogged && queueSize <= 3) {
      console.log(`[WebSocketStream] Queue recovered (${queueSize} frames)`)
      this.queueBackupLogged = false
    }

    try {
      const chunk = new EncodedVideoChunk({
        type: isKeyframe ? "key" : "delta",
        timestamp: Number(ptsUs), // microseconds
        data: frameData,
      })

      this.videoDecoder.decode(chunk)
    } catch (e) {
      console.error("[WebSocketStream] Failed to decode video chunk:", e, "isKeyframe:", isKeyframe)

      // If decoding fails, reset state and wait for next keyframe
      this.receivedFirstKeyframe = false

      if (isKeyframe) {
        console.warn("[WebSocketStream] Keyframe decode failed, attempting decoder recovery")

        // Try to reconfigure decoder for recovery
        if (this.lastVideoCodec !== null && this.lastVideoWidth > 0 && this.lastVideoHeight > 0) {
          this.initVideoDecoder(this.lastVideoCodec, this.lastVideoWidth, this.lastVideoHeight)
            .catch(err => console.error("[WebSocketStream] Failed to recover video decoder:", err))
        }
      }
    }
  }

  /**
   * Get adaptive throttle interval based on RTT
   * Returns the effective throttle interval in ms, accounting for RTT-based reduction
   */
  private getAdaptiveThrottleMs(): number {
    const ratio = this.manualThrottleRatio ?? this.adaptiveThrottleRatio
    const baseThrottleMs = 1000 / this.settings.fps
    // When ratio < 1, we send LESS frequently, so interval INCREASES
    return baseThrottleMs / ratio
  }

  /**
   * Update adaptive throttle ratio based on current RTT
   * Called from handlePong() whenever RTT measurement is updated
   */
  private updateAdaptiveThrottle() {
    // Don't update if manually overridden
    if (this.manualThrottleRatio !== null) {
      return
    }

    const rtt = this.currentRttMs

    // Calculate ratio based on RTT thresholds
    let ratio: number
    if (rtt < 50) {
      ratio = 1.0     // 100% - full configured rate
    } else if (rtt < 100) {
      ratio = 0.75    // 75%
    } else if (rtt < 150) {
      ratio = 0.5     // 50%
    } else if (rtt < 250) {
      ratio = 0.33    // 33%
    } else {
      ratio = 0.25    // 25% - minimum
    }

    // Smooth transitions with exponential moving average
    this.adaptiveThrottleRatio = this.adaptiveThrottleRatio * 0.7 + ratio * 0.3
  }

  /**
   * Set manual throttle ratio override for debugging
   * Pass null to return to automatic adaptive throttling
   */
  setThrottleRatio(ratio: number | null) {
    this.manualThrottleRatio = ratio
    if (ratio !== null) {
      console.log(`[WebSocketStream] Manual throttle ratio set to ${ratio * 100}%`)
    } else {
      console.log(`[WebSocketStream] Returned to adaptive throttle (current: ${(this.adaptiveThrottleRatio * 100).toFixed(0)}%)`)
    }
  }

  /**
   * Handle a batched video frames message (type 0x03)
   * Format: type(1) + count(2) + [length(4) + frame_data]...
   */
  private async handleVideoBatch(data: Uint8Array) {
    if (data.length < 3) {
      console.error("[WebSocketStream] VideoBatch too short:", data.length)
      return
    }

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const frameCount = view.getUint16(1, false)  // big-endian

    // Track batch stats
    this.batchesReceived++
    this.batchedFramesReceived += frameCount
    this.recentBatchSizes.push(frameCount)
    if (this.recentBatchSizes.length > this.MAX_BATCH_SIZE_SAMPLES) {
      this.recentBatchSizes.shift()
    }

    // Parse and process each frame
    let offset = 3  // After type + count
    for (let i = 0; i < frameCount; i++) {
      if (offset + 4 > data.length) {
        console.error("[WebSocketStream] VideoBatch truncated at frame", i)
        break
      }

      const frameLen = view.getUint32(offset, false)  // big-endian
      offset += 4

      if (offset + frameLen > data.length) {
        console.error("[WebSocketStream] VideoBatch frame data truncated at frame", i)
        break
      }

      const frameData = data.slice(offset, offset + frameLen)
      offset += frameLen

      // Process each frame (pass fromBatch=true to skip individual frame counting)
      await this.handleVideoFrame(frameData, true)
    }
  }

  private async initAudioDecoder(channels: number, sampleRate: number) {
    if (!("AudioDecoder" in window)) {
      console.warn("[WebSocketStream] WebCodecs AudioDecoder not supported, trying Web Audio API")
      // Fallback to opus-decoder library or Web Audio API decoding
      return
    }

    console.log(`[WebSocketStream] Initializing audio decoder: Opus ${channels}ch@${sampleRate}Hz`)

    // Initialize AudioContext
    this.audioContext = new AudioContext({ sampleRate })

    // Close existing decoder
    if (this.audioDecoder) {
      try {
        this.audioDecoder.close()
      } catch (e) {
        // Ignore
      }
    }

    this.audioDecoder = new AudioDecoder({
      output: (data: AudioData) => {
        this.playAudioData(data)
      },
      error: (e: Error) => {
        console.error("[WebSocketStream] Audio decoder error:", e)
      },
    })

    // Configure for Opus
    this.audioDecoder.configure({
      codec: "opus",
      numberOfChannels: channels,
      sampleRate: sampleRate,
    })

    console.log("[WebSocketStream] Audio decoder initialized")
  }

  // Audio scheduling state
  private audioStartTime = 0 // AudioContext.currentTime when first audio was played
  private audioPtsBase = 0 // PTS of first audio frame (microseconds)
  private audioInitialized = false

  private playAudioData(data: AudioData) {
    if (!this.audioContext) {
      data.close()
      return
    }

    // Resume AudioContext if suspended (browser autoplay policy)
    if (this.audioContext.state === "suspended") {
      this.audioContext.resume().catch(e => {
        console.warn("[WebSocketStream] Failed to resume AudioContext:", e)
      })
    }

    // Create audio buffer from AudioData
    const buffer = this.audioContext.createBuffer(
      data.numberOfChannels,
      data.numberOfFrames,
      data.sampleRate
    )

    // Copy data to buffer
    for (let i = 0; i < data.numberOfChannels; i++) {
      const channelData = new Float32Array(data.numberOfFrames)
      data.copyTo(channelData, { planeIndex: i, format: "f32-planar" })
      buffer.copyToChannel(channelData, i)
    }

    // Schedule audio based on PTS timestamp
    // data.timestamp is in microseconds
    const ptsUs = data.timestamp

    if (!this.audioInitialized) {
      // First audio frame - establish timing baseline
      this.audioStartTime = this.audioContext.currentTime
      this.audioPtsBase = ptsUs
      this.audioInitialized = true
      console.log(`[WebSocketStream] Audio initialized: baseTime=${this.audioStartTime}, basePTS=${ptsUs}`)
    }

    // Calculate when this frame should play
    // scheduledTime = audioStartTime + (framePTS - basePTS) / 1000000
    const ptsDelta = (ptsUs - this.audioPtsBase) / 1_000_000 // Convert to seconds
    const scheduledTime = this.audioStartTime + ptsDelta

    // If we're too far behind, skip or catch up
    const now = this.audioContext.currentTime
    if (scheduledTime < now - 0.1) {
      // Frame is more than 100ms in the past, skip it
      data.close()
      return
    }

    // Play audio at scheduled time
    const source = this.audioContext.createBufferSource()
    source.buffer = buffer
    source.connect(this.audioContext.destination)

    // Schedule for the correct time (or now if it should have already played)
    const playTime = Math.max(scheduledTime, now)
    source.start(playTime)

    data.close()
  }

  private async handleAudioFrame(data: Uint8Array) {
    if (!this.audioDecoder || this.audioDecoder.state !== "configured") {
      return
    }

    // Parse audio frame header
    // Format: type(1) + channels(1) + pts(8) + data(...)
    if (data.length < 10) {
      console.error("[WebSocketStream] Audio frame too short")
      return
    }

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const ptsUs = view.getBigUint64(2, false) // big-endian

    const frameData = data.slice(10)

    try {
      const chunk = new EncodedAudioChunk({
        type: "key", // Opus frames are always keyframes
        timestamp: Number(ptsUs), // microseconds
        data: frameData,
      })

      this.audioDecoder.decode(chunk)
    } catch (e) {
      console.error("[WebSocketStream] Failed to decode audio chunk:", e)
    }
  }

  // ============================================================================
  // RTT (Latency) Measurement
  // ============================================================================

  private sendPing() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return
    }

    const seq = this.pingSeq++
    const sendTime = performance.now()
    this.pendingPings.set(seq, sendTime)

    // Ping format: type(1) + seq(4) + clientTime(8) = 13 bytes
    const buffer = new ArrayBuffer(13)
    const view = new DataView(buffer)
    view.setUint8(0, WsMessageType.Ping)
    view.setUint32(1, seq, false)  // big-endian
    // We use performance.now() * 1000 for microseconds, but we only need
    // the send time locally - the server echoes it back for calculation
    view.setBigUint64(5, BigInt(Math.floor(sendTime * 1000)), false)

    this.ws.send(buffer)
  }

  private handlePong(data: Uint8Array) {
    // Extended Pong format: type(1) + seq(4) + clientTime(8) + serverTime(8) + encoderLatencyMs(2) = 23 bytes
    // Backward compatible: old servers send 21 bytes without encoder latency
    if (data.length < 21) {
      console.warn("[WebSocketStream] Pong too short:", data.length)
      return
    }

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const seq = view.getUint32(1, false)  // big-endian

    const sendTime = this.pendingPings.get(seq)
    if (sendTime === undefined) {
      console.warn("[WebSocketStream] Received pong for unknown seq:", seq)
      return
    }

    this.pendingPings.delete(seq)

    // Calculate RTT
    const receiveTime = performance.now()
    const rtt = receiveTime - sendTime

    // Add to samples, keep only the most recent
    this.rttSamples.push(rtt)
    if (this.rttSamples.length > this.MAX_RTT_SAMPLES) {
      this.rttSamples.shift()
    }

    // Calculate moving average
    const sum = this.rttSamples.reduce((a, b) => a + b, 0)
    this.currentRttMs = sum / this.rttSamples.length

    // Extract encoder latency if present (extended Pong format: 23 bytes)
    if (data.length >= 23) {
      this.encoderLatencyMs = view.getUint16(21, false)  // big-endian
      console.debug(`[WebSocketStream] Pong: RTT=${this.currentRttMs.toFixed(0)}ms, Encoder=${this.encoderLatencyMs}ms, pongSize=${data.length}`)
    } else {
      console.debug(`[WebSocketStream] Pong: RTT=${this.currentRttMs.toFixed(0)}ms, pongSize=${data.length} (no encoder latency - old backend?)`)
    }

    // Update adaptive input throttling based on new RTT
    this.updateAdaptiveThrottle()

    // Dispatch event if latency is high
    if (this.currentRttMs > this.HIGH_LATENCY_THRESHOLD_MS) {
      this.dispatchInfoEvent({
        type: "addDebugLine",
        line: `High latency detected: ${this.currentRttMs.toFixed(0)}ms RTT`
      })
    }
  }

  private startPingInterval() {
    this.stopPingInterval()

    // Send first ping immediately
    this.sendPing()

    // Then send periodically
    this.pingIntervalId = setInterval(() => {
      this.sendPing()

      // Clean up old pending pings (older than 5 seconds = lost)
      const now = performance.now()
      for (const [seq, sendTime] of this.pendingPings.entries()) {
        if (now - sendTime > 5000) {
          this.pendingPings.delete(seq)
        }
      }
    }, this.PING_INTERVAL_MS)
  }

  private stopPingInterval() {
    if (this.pingIntervalId) {
      clearInterval(this.pingIntervalId)
      this.pingIntervalId = null
    }
    this.pendingPings.clear()
  }

  // ============================================================================
  // Input Handling - WebSocket transport
  // ============================================================================

  /**
   * Check if the input send buffer is congested
   * Returns true if we should skip non-critical inputs (mouse moves)
   *
   * Strategy: Skip mouse moves if buffer hasn't drained since last send.
   * This prevents "ghost moves" - stale positions that arrive late and make
   * it look like something else is controlling the cursor.
   *
   * We allow ONE mouse move to queue (to detect congestion), then skip
   * all subsequent moves until the buffer drains completely.
   */
  private isInputBufferCongested(): boolean {
    if (!this.ws) return false

    const now = performance.now()
    const buffered = this.ws.bufferedAmount

    if (buffered === 0) {
      // Buffer is empty - network is keeping up, safe to send
      this.lastBufferDrainTime = now
      this.bufferStaleMs = 0
      return false
    }

    // Buffer has data - track how long
    if (this.lastBufferDrainTime === 0) {
      this.lastBufferDrainTime = now
    }
    this.bufferStaleMs = now - this.lastBufferDrainTime

    // Skip immediately if buffer hasn't drained - don't pile up stale moves
    // The one move already in the buffer will transmit; we'll send fresh
    // position when buffer drains
    return true
  }

  /**
   * Track WebSocket send buffer stats for input latency monitoring
   */
  private trackInputBuffer() {
    if (!this.ws) return

    const buffered = this.ws.bufferedAmount
    this.lastBufferedAmount = buffered

    // Track peak
    if (buffered > this.maxBufferedAmount) {
      this.maxBufferedAmount = buffered
    }

    // Keep recent samples for averaging
    this.inputBufferSamples.push(buffered)
    if (this.inputBufferSamples.length > this.MAX_INPUT_BUFFER_SAMPLES) {
      this.inputBufferSamples.shift()
    }
  }

  private sendInputMessage(type: number, payload: Uint8Array) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn(`[WebSocketStream] sendInputMessage: WS not ready (ws=${!!this.ws}, state=${this.ws?.readyState}), dropping input type=0x${type.toString(16)}`)
      return
    }

    // Track buffer stats before sending
    this.trackInputBuffer()
    this.bufferedAmountBeforeSend = this.ws.bufferedAmount

    const message = new Uint8Array(1 + payload.length)
    message[0] = type
    message.set(payload, 1)

    // Measure how long ws.send() takes (should be ~0 if truly non-blocking)
    const sendStart = performance.now()
    this.ws.send(message.buffer)
    const sendDuration = performance.now() - sendStart

    // Track send duration
    this.lastSendDurationMs = sendDuration
    if (sendDuration > this.maxSendDurationMs) {
      this.maxSendDurationMs = sendDuration
    }
    this.sendDurationSamples.push(sendDuration)
    if (this.sendDurationSamples.length > this.MAX_SEND_DURATION_SAMPLES) {
      this.sendDurationSamples.shift()
    }

    // Track buffer after send
    this.bufferedAmountAfterSend = this.ws.bufferedAmount

    this.inputsSent++
  }

  // WebSocket-specific input methods that mirror StreamInput API
  // These construct the same binary format as the RTCDataChannel version

  private inputBuffer = new Uint8Array(64)
  private inputView = new DataView(this.inputBuffer.buffer)

  sendKey(isDown: boolean, key: number, modifiers: number) {
    // Format: subType(1) + isDown(1) + modifiers(1) + keyCode(2)
    this.inputBuffer[0] = 0 // sub-type for key
    this.inputBuffer[1] = isDown ? 1 : 0
    this.inputBuffer[2] = modifiers
    this.inputView.setUint16(3, key, false) // big-endian
    this.sendInputMessage(WsMessageType.KeyboardInput, this.inputBuffer.subarray(0, 5))
  }

  sendMouseMove(movementX: number, movementY: number) {
    const now = performance.now()
    const elapsed = now - this.lastMouseSendTime

    // Check for input buffer congestion - if buffer is backing up, accumulate instead of sending
    // This prevents input queueing that causes mouse lag even when video latency is low
    if (this.isInputBufferCongested()) {
      // Accumulate movement for when buffer clears
      if (this.pendingMouseMove) {
        this.pendingMouseMove.dx += movementX
        this.pendingMouseMove.dy += movementY
      } else {
        this.pendingMouseMove = { dx: movementX, dy: movementY }
      }
      this.inputsDroppedDueToCongestion++
      // Schedule flush when buffer might be clearer
      this.scheduleMouseFlush(this.getAdaptiveThrottleMs())
      return
    }

    if (elapsed >= this.getAdaptiveThrottleMs()) {
      // Enough time has passed - send immediately
      this.sendMouseMoveImmediate(movementX, movementY)
      this.lastMouseSendTime = now
      this.pendingMouseMove = null
    } else {
      // Throttled - accumulate movement (relative moves add up)
      if (this.pendingMouseMove) {
        this.pendingMouseMove.dx += movementX
        this.pendingMouseMove.dy += movementY
      } else {
        this.pendingMouseMove = { dx: movementX, dy: movementY }
      }
      // Schedule flush after throttle period
      this.scheduleMouseFlush(this.getAdaptiveThrottleMs() - elapsed)
    }
  }

  // ============================================================================
  // Event Loop Latency Tracking
  // ============================================================================

  /**
   * Start periodic event loop latency measurement using setTimeout(0)
   * The idea: schedule a callback for "immediate" execution and measure actual delay
   * If event loop is blocked, the callback is delayed proportionally
   */
  private startEventLoopTracking() {
    this.stopEventLoopTracking()
    this.scheduleEventLoopCheck()
  }

  private stopEventLoopTracking() {
    if (this.eventLoopCheckTimeoutId) {
      clearTimeout(this.eventLoopCheckTimeoutId)
      this.eventLoopCheckTimeoutId = null
    }
  }

  private scheduleEventLoopCheck() {
    if (this.closed) return

    this.eventLoopCheckScheduledAt = performance.now()

    // Use setTimeout(0) which should fire "immediately" - any excess delay is event loop latency
    this.eventLoopCheckTimeoutId = setTimeout(() => {
      const actualTime = performance.now()
      const elapsed = actualTime - this.eventLoopCheckScheduledAt

      // setTimeout(0) has ~4ms minimum delay in browsers, so only count excess beyond that
      // Also account for timer coalescing which can add a few more ms
      const baselineDelay = 8  // Expected delay for setTimeout(0) with coalescing
      const excessLatency = Math.max(0, elapsed - baselineDelay)

      this.eventLoopLatencyMs = excessLatency

      if (excessLatency > this.maxEventLoopLatencyMs) {
        this.maxEventLoopLatencyMs = excessLatency
      }

      this.eventLoopLatencySamples.push(excessLatency)
      if (this.eventLoopLatencySamples.length > this.MAX_EVENT_LOOP_SAMPLES) {
        this.eventLoopLatencySamples.shift()
      }

      // Schedule next check after interval
      if (!this.closed) {
        this.eventLoopCheckTimeoutId = setTimeout(
          () => this.scheduleEventLoopCheck(),
          this.EVENT_LOOP_CHECK_INTERVAL_MS
        )
      }
    }, 0)
  }

  private sendMouseMoveImmediate(movementX: number, movementY: number) {
    // Format: subType(1) + dx(2) + dy(2)
    this.inputBuffer[0] = 0 // sub-type for relative
    this.inputView.setInt16(1, Math.round(movementX), false)
    this.inputView.setInt16(3, Math.round(movementY), false)
    this.sendInputMessage(WsMessageType.MouseRelative, this.inputBuffer.subarray(0, 5))
  }

  sendMousePosition(x: number, y: number, refWidth: number, refHeight: number) {
    const now = performance.now()
    const elapsed = now - this.lastMouseSendTime

    // Check for input buffer congestion - if buffer is backing up, just store latest position
    // Absolute positions replace each other so we just keep the newest one
    if (this.isInputBufferCongested()) {
      this.pendingMousePosition = { x, y, refW: refWidth, refH: refHeight }
      this.inputsDroppedDueToCongestion++
      // Schedule flush when buffer might be clearer
      this.scheduleMouseFlush(this.getAdaptiveThrottleMs())
      return
    }

    if (elapsed >= this.getAdaptiveThrottleMs()) {
      // Enough time has passed - send immediately
      this.sendMousePositionImmediate(x, y, refWidth, refHeight)
      this.lastMouseSendTime = now
      this.pendingMousePosition = null
    } else {
      // Throttled - store latest position (absolute positions replace, not accumulate)
      this.pendingMousePosition = { x, y, refW: refWidth, refH: refHeight }
      // Schedule flush after throttle period
      this.scheduleMouseFlush(this.getAdaptiveThrottleMs() - elapsed)
    }
  }

  private sendMousePositionImmediate(x: number, y: number, refWidth: number, refHeight: number) {
    // Format: subType(1) + x(2) + y(2) + refWidth(2) + refHeight(2)
    this.inputBuffer[0] = 1 // sub-type for absolute
    this.inputView.setInt16(1, Math.round(x), false)
    this.inputView.setInt16(3, Math.round(y), false)
    this.inputView.setInt16(5, Math.round(refWidth), false)
    this.inputView.setInt16(7, Math.round(refHeight), false)
    this.sendInputMessage(WsMessageType.MouseAbsolute, this.inputBuffer.subarray(0, 9))
  }

  private scheduleMouseFlush(delayMs: number) {
    // Only schedule if not already scheduled
    if (this.mouseThrottleTimeoutId) return

    this.mouseThrottleTimeoutId = setTimeout(() => {
      this.mouseThrottleTimeoutId = null

      // If still congested, reschedule for later
      if (this.isInputBufferCongested()) {
        this.scheduleMouseFlush(this.getAdaptiveThrottleMs())
        return
      }

      this.lastMouseSendTime = performance.now()

      // Send any pending mouse data
      if (this.pendingMouseMove) {
        const { dx, dy } = this.pendingMouseMove
        this.pendingMouseMove = null
        this.sendMouseMoveImmediate(dx, dy)
      }
      if (this.pendingMousePosition) {
        const { x, y, refW, refH } = this.pendingMousePosition
        this.pendingMousePosition = null
        this.sendMousePositionImmediate(x, y, refW, refH)
      }
    }, delayMs)
  }

  sendMouseButton(isDown: boolean, button: number) {
    console.log(`[WebSocketStream] sendMouseButton: isDown=${isDown} button=${button} (1=left, 2=middle, 3=right)`)
    // Format: subType(1) + isDown(1) + button(1)
    this.inputBuffer[0] = 2 // sub-type for button
    this.inputBuffer[1] = isDown ? 1 : 0
    this.inputBuffer[2] = button
    this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 3))
  }

  sendMouseWheelHighRes(deltaX: number, deltaY: number) {
    this.sendScrollThrottled(deltaX, deltaY, true)
  }

  sendMouseWheel(deltaX: number, deltaY: number) {
    this.sendScrollThrottled(deltaX, deltaY, false)
  }

  private sendScrollThrottled(deltaX: number, deltaY: number, highRes: boolean) {
    const now = performance.now()
    const elapsed = now - this.lastScrollSendTime

    // Accumulate scroll deltas (like relative mouse movement)
    if (this.pendingScroll) {
      this.pendingScroll.dx += deltaX
      this.pendingScroll.dy += deltaY
      // Keep highRes if any event in batch was highRes
      this.pendingScroll.highRes = this.pendingScroll.highRes || highRes
    } else {
      this.pendingScroll = { dx: deltaX, dy: deltaY, highRes }
    }

    // If enough time has passed, send immediately
    if (elapsed >= this.getAdaptiveThrottleMs()) {
      this.flushPendingScroll()
      this.lastScrollSendTime = now
    } else {
      // Schedule flush after throttle period
      this.scheduleScrollFlush(this.getAdaptiveThrottleMs() - elapsed)
    }
  }

  private scheduleScrollFlush(delayMs: number) {
    if (this.scrollThrottleTimeoutId) return // Already scheduled

    this.scrollThrottleTimeoutId = setTimeout(() => {
      this.scrollThrottleTimeoutId = null
      if (this.pendingScroll) {
        this.flushPendingScroll()
        this.lastScrollSendTime = performance.now()
      }
    }, delayMs)
  }

  private flushPendingScroll() {
    if (!this.pendingScroll) return

    const { dx, dy, highRes } = this.pendingScroll
    this.pendingScroll = null

    if (dx === 0 && dy === 0) return

    if (highRes) {
      // Format: subType(1) + deltaX(4 float32) + deltaY(4 float32)
      this.inputBuffer[0] = 3 // sub-type for high-res wheel
      this.inputView.setFloat32(1, dx, true) // little-endian
      this.inputView.setFloat32(5, dy, true) // little-endian
      this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 9))
    } else {
      // Format: subType(1) + deltaX(1) + deltaY(1)
      this.inputBuffer[0] = 4 // sub-type for normal wheel
      this.inputBuffer[1] = Math.round(dx) & 0xFF
      this.inputBuffer[2] = Math.round(dy) & 0xFF
      this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 3))
    }
  }

  // ============================================================================
  // Touch Input (WebSocket transport)
  // ============================================================================

  // Touch throttling - motion events are throttled like mouse/scroll, down/up are immediate
  private lastTouchSendTime = 0
  private pendingTouchMotion: Map<number, { normX: number; normY: number }> = new Map()
  private touchThrottleTimeoutId: ReturnType<typeof setTimeout> | null = null
  // Map browser touch identifiers to slot numbers (0-9)
  private touchSlotMap: Map<number, number> = new Map()
  private nextTouchSlot = 0

  /**
   * Send touch event via WebSocket.
   * Format: [eventType:1][slot:1][x:4 LE float32][y:4 LE float32]
   * x/y are normalized 0.0-1.0 coordinates.
   */
  sendTouch(eventType: number, identifier: number, normX: number, normY: number) {
    // Map browser identifier to slot (0-9)
    let slot: number
    if (eventType === 0) {
      // Touch down - assign new slot
      slot = this.nextTouchSlot++ % 10
      this.touchSlotMap.set(identifier, slot)
    } else {
      // Touch motion or up - use existing slot
      const existingSlot = this.touchSlotMap.get(identifier)
      if (existingSlot === undefined) {
        console.warn("[WebSocketStream] Touch event for unknown identifier:", identifier)
        return
      }
      slot = existingSlot
    }

    if (eventType === 0 || eventType === 2) {
      // Touch down/up - send immediately (discrete events)
      this.sendTouchImmediate(eventType, slot, normX, normY)

      // Clean up slot on touch up
      if (eventType === 2) {
        this.touchSlotMap.delete(identifier)
      }
    } else {
      // Touch motion - throttle like mouse movement
      this.sendTouchMotionThrottled(slot, normX, normY)
    }
  }

  private sendTouchImmediate(eventType: number, slot: number, normX: number, normY: number) {
    // Format: [eventType:1][slot:1][x:4 LE float32][y:4 LE float32]
    this.inputBuffer[0] = eventType
    this.inputBuffer[1] = slot
    this.inputView.setFloat32(2, normX, true) // little-endian
    this.inputView.setFloat32(6, normY, true) // little-endian
    this.sendInputMessage(WsMessageType.TouchEvent, this.inputBuffer.subarray(0, 10))
  }

  private sendTouchMotionThrottled(slot: number, normX: number, normY: number) {
    const now = performance.now()
    const elapsed = now - this.lastTouchSendTime

    // Store latest position for this slot (overwrites previous - we only care about latest position)
    this.pendingTouchMotion.set(slot, { normX, normY })

    // If enough time has passed, send immediately
    if (elapsed >= this.getAdaptiveThrottleMs()) {
      this.flushPendingTouchMotion()
      this.lastTouchSendTime = now
    } else {
      // Schedule flush after throttle period
      this.scheduleTouchFlush(this.getAdaptiveThrottleMs() - elapsed)
    }
  }

  private scheduleTouchFlush(delayMs: number) {
    if (this.touchThrottleTimeoutId) return // Already scheduled

    this.touchThrottleTimeoutId = setTimeout(() => {
      this.touchThrottleTimeoutId = null
      if (this.pendingTouchMotion.size > 0) {
        this.flushPendingTouchMotion()
        this.lastTouchSendTime = performance.now()
      }
    }, delayMs)
  }

  private flushPendingTouchMotion() {
    for (const [slot, pos] of this.pendingTouchMotion) {
      this.sendTouchImmediate(1, slot, pos.normX, pos.normY) // 1 = motion
    }
    this.pendingTouchMotion.clear()
  }

  // ============================================================================
  // Public API
  // ============================================================================

  setCanvas(canvas: HTMLCanvasElement) {
    this.canvas = canvas
    this.canvasCtx = canvas.getContext("2d", {
      alpha: false,
      desynchronized: true, // Lower latency
    })
  }

  getStreamerSize(): [number, number] {
    return this.streamerSize
  }

  getStats(): {
    fps: number
    videoPayloadBitrateMbps: number  // H.264 data only
    totalBitrateMbps: number         // Everything over WebSocket
    framesDecoded: number
    framesDropped: number
    width: number
    height: number
    rttMs: number                    // Round-trip time in milliseconds
    encoderLatencyMs: number         // Server-side encoder latency (PTS to WebSocket send)
    isHighLatency: boolean           // True if RTT exceeds threshold
    // Batching stats for congestion visibility
    batchesReceived: number          // Total batch messages received
    batchedFramesReceived: number    // Total frames received in batches
    individualFramesReceived: number // Total frames received individually
    avgBatchSize: number             // Average frames per batch (0 = no batching)
    batchingRatio: number            // Percent of frames that arrived batched (0-100)
    // Frame latency (measures actual delivery delay, not just RTT)
    frameLatencyMs: number           // How late frames are arriving based on PTS
    // Adaptive input throttling stats
    adaptiveThrottleRatio: number    // Current throttle ratio (1.0 = full, 0.25 = 25%)
    effectiveInputFps: number        // Actual input rate after throttling
    isThrottled: boolean             // True if throttle ratio < 1.0
    // Decoder queue stats (detects if decoder can't keep up)
    decodeQueueSize: number          // Current decoder queue depth
    maxDecodeQueueSize: number       // Peak queue size seen
    framesSkippedToKeyframe: number  // Frames flushed when skipping to keyframe
    // Codec info
    codecString: string              // Human-readable codec name (H.264, HEVC, AV1, etc.)
    // Input buffer stats (detects if input is queueing up)
    inputBufferBytes: number         // Current WebSocket send buffer size
    maxInputBufferBytes: number      // Peak send buffer size seen
    avgInputBufferBytes: number      // Average send buffer size
    inputsSent: number               // Total inputs sent
    inputsDroppedDueToCongestion: number  // Mouse moves skipped due to buffer congestion
    inputCongested: boolean          // True if input buffer is currently congested
    bufferStaleMs: number            // How long buffer has been non-empty (0 = draining fine)
    // Send latency stats (should be ~0 if ws.send is truly non-blocking)
    lastSendDurationMs: number       // How long last ws.send() took
    maxSendDurationMs: number        // Peak send duration seen
    avgSendDurationMs: number        // Average send duration
    bufferedAmountBeforeSend: number // Buffer size before last send
    bufferedAmountAfterSend: number  // Buffer size after last send
    // Event loop latency (detects if main thread is blocked)
    eventLoopLatencyMs: number       // Current excess delay for setTimeout(0)
    maxEventLoopLatencyMs: number    // Peak event loop latency seen
    avgEventLoopLatencyMs: number    // Average event loop latency
  } {
    // Calculate batching metrics
    const totalFrames = this.batchedFramesReceived + this.individualFramesReceived
    const batchingRatio = totalFrames > 0
      ? Math.round((this.batchedFramesReceived / totalFrames) * 100)
      : 0
    const avgBatchSize = this.recentBatchSizes.length > 0
      ? this.recentBatchSizes.reduce((a, b) => a + b, 0) / this.recentBatchSizes.length
      : 0

    return {
      fps: this.currentFps,
      videoPayloadBitrateMbps: this.currentVideoPayloadBitrateMbps,
      totalBitrateMbps: this.currentTotalBitrateMbps,
      framesDecoded: this.framesDecoded,
      framesDropped: this.framesDropped,
      width: this.streamerSize[0],
      height: this.streamerSize[1],
      rttMs: this.currentRttMs,
      encoderLatencyMs: this.encoderLatencyMs,
      isHighLatency: this.currentRttMs > this.HIGH_LATENCY_THRESHOLD_MS,
      // Batching stats
      batchesReceived: this.batchesReceived,
      batchedFramesReceived: this.batchedFramesReceived,
      individualFramesReceived: this.individualFramesReceived,
      avgBatchSize,
      batchingRatio,
      // Frame latency (the real measure of how delayed frames are)
      frameLatencyMs: this.currentFrameLatencyMs,
      // Adaptive input throttling
      adaptiveThrottleRatio: this.manualThrottleRatio ?? this.adaptiveThrottleRatio,
      effectiveInputFps: this.settings.fps * (this.manualThrottleRatio ?? this.adaptiveThrottleRatio),
      isThrottled: (this.manualThrottleRatio ?? this.adaptiveThrottleRatio) < 0.99,
      // Decoder queue
      decodeQueueSize: this.lastDecodeQueueSize,
      maxDecodeQueueSize: this.maxDecodeQueueSize,
      framesSkippedToKeyframe: this.framesSkippedToKeyframe,
      // Codec info
      codecString: codecToDisplayName(this.lastVideoCodec),
      // Input buffer stats
      inputBufferBytes: this.lastBufferedAmount,
      maxInputBufferBytes: this.maxBufferedAmount,
      avgInputBufferBytes: this.inputBufferSamples.length > 0
        ? Math.round(this.inputBufferSamples.reduce((a, b) => a + b, 0) / this.inputBufferSamples.length)
        : 0,
      inputsSent: this.inputsSent,
      inputsDroppedDueToCongestion: this.inputsDroppedDueToCongestion,
      inputCongested: this.isInputBufferCongested(),
      bufferStaleMs: this.bufferStaleMs,
      // Send latency stats (should be ~0 if ws.send is truly non-blocking)
      lastSendDurationMs: this.lastSendDurationMs,
      maxSendDurationMs: this.maxSendDurationMs,
      avgSendDurationMs: this.sendDurationSamples.length > 0
        ? this.sendDurationSamples.reduce((a, b) => a + b, 0) / this.sendDurationSamples.length
        : 0,
      bufferedAmountBeforeSend: this.bufferedAmountBeforeSend,
      bufferedAmountAfterSend: this.bufferedAmountAfterSend,
      // Event loop latency
      eventLoopLatencyMs: this.eventLoopLatencyMs,
      maxEventLoopLatencyMs: this.maxEventLoopLatencyMs,
      avgEventLoopLatencyMs: this.eventLoopLatencySamples.length > 0
        ? this.eventLoopLatencySamples.reduce((a, b) => a + b, 0) / this.eventLoopLatencySamples.length
        : 0,
    }
  }

  getInput(): StreamInput {
    // Return the underlying StreamInput that's been configured
    // The caller will use onKeyDown, onMouseMove, etc. which internally
    // call sendKey, sendMouseMove, etc.
    // We need to patch the send methods to use our WebSocket transport
    const wsStream = this
    const patchedInput = this.input

    // Override the send methods on the StreamInput instance
    // @ts-ignore - accessing private methods for patching
    patchedInput.sendKey = (isDown: boolean, key: number, modifiers: number) => {
      wsStream.sendKey(isDown, key, modifiers)
    }
    // @ts-ignore
    patchedInput.sendMouseMove = (movementX: number, movementY: number) => {
      wsStream.sendMouseMove(movementX, movementY)
    }
    // @ts-ignore
    patchedInput.sendMousePosition = (x: number, y: number, refW: number, refH: number) => {
      wsStream.sendMousePosition(x, y, refW, refH)
    }
    // @ts-ignore
    patchedInput.sendMouseButton = (isDown: boolean, button: number) => {
      wsStream.sendMouseButton(isDown, button)
    }
    // @ts-ignore
    patchedInput.sendMouseWheelHighRes = (deltaX: number, deltaY: number) => {
      wsStream.sendMouseWheelHighRes(deltaX, deltaY)
    }
    // @ts-ignore
    patchedInput.sendMouseWheel = (deltaX: number, deltaY: number) => {
      wsStream.sendMouseWheel(deltaX, deltaY)
    }

    return patchedInput
  }

  addInfoListener(listener: WsStreamInfoEventListener) {
    this.eventTarget.addEventListener("stream-info", listener as EventListenerOrEventListenerObject)
  }

  removeInfoListener(listener: WsStreamInfoEventListener) {
    this.eventTarget.removeEventListener("stream-info", listener as EventListenerOrEventListenerObject)
  }

  private dispatchInfoEvent(detail: WsStreamInfoEvent["detail"]) {
    const event: WsStreamInfoEvent = new CustomEvent("stream-info", { detail })
    this.eventTarget.dispatchEvent(event)
  }

  private resetStreamState() {
    // Reset video state
    this.receivedFirstKeyframe = false

    // Reset audio state
    this.audioInitialized = false
    this.audioStartTime = 0
    this.audioPtsBase = 0
  }

  private cleanupDecoders() {
    if (this.videoDecoder) {
      try {
        this.videoDecoder.close()
      } catch (e) {
        // Ignore - decoder may already be closed
      }
      this.videoDecoder = null
    }

    if (this.audioDecoder) {
      try {
        this.audioDecoder.close()
      } catch (e) {
        // Ignore
      }
      this.audioDecoder = null
    }

    if (this.audioContext) {
      try {
        this.audioContext.close()
      } catch (e) {
        // Ignore
      }
      this.audioContext = null
    }
  }

  private startHeartbeat() {
    this.stopHeartbeat()

    this.heartbeatIntervalId = setInterval(() => {
      if (!this.connected) return

      const now = Date.now()
      const elapsed = now - this.lastMessageTime

      if (elapsed > this.heartbeatTimeout) {
        console.warn(`[WebSocketStream] Stale connection detected (${elapsed}ms since last message), forcing reconnect`)
        this.dispatchInfoEvent({ type: "error", message: "Connection stale - no data received" })

        // Force close and trigger reconnection
        if (this.ws) {
          try {
            this.ws.close()
          } catch (e) {
            // Ignore
          }
        }
      }
    }, 5000) // Check every 5 seconds
  }

  private stopHeartbeat() {
    if (this.heartbeatIntervalId) {
      clearInterval(this.heartbeatIntervalId)
      this.heartbeatIntervalId = null
    }
  }

  /**
   * Enable or disable video frame transmission from the server
   * When disabled, server stops sending video frames (saves bandwidth in screenshot mode)
   *
   * @param enabled - true to enable video, false to disable (screenshot mode)
   */
  setVideoEnabled(enabled: boolean) {
    // Idempotent check - don't send duplicate messages
    if (this._videoEnabled === enabled) {
      console.log(`[WebSocketStream] Video already ${enabled ? 'enabled' : 'disabled'}, skipping`)
      return
    }

    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn("[WebSocketStream] Cannot set video enabled - WebSocket not connected")
      // Still update local state so we don't spam when connection is restored
      this._videoEnabled = enabled
      return
    }

    console.log(`[WebSocketStream] Setting video enabled: ${enabled}`)
    this._videoEnabled = enabled

    // When re-enabling video, reset keyframe flag so we wait for a fresh keyframe
    // The decoder needs VPS/SPS/PPS parameter sets from a keyframe after being paused
    if (enabled) {
      this.receivedFirstKeyframe = false
      console.log("[WebSocketStream] Reset keyframe flag - will wait for fresh keyframe")
    }

    // Send control message to server
    // Format: type(1) + JSON payload
    const json = JSON.stringify({ set_video_enabled: enabled })
    const encoder = new TextEncoder()
    const jsonBytes = encoder.encode(json)

    const message = new Uint8Array(1 + jsonBytes.length)
    message[0] = WsMessageType.ControlMessage
    message.set(jsonBytes, 1)

    this.ws.send(message.buffer)
  }

  /**
   * Enable or disable audio streaming from the server.
   * Audio is disabled by default to avoid autoplay restrictions and save bandwidth.
   * @param enabled - true to start audio streaming, false to stop
   */
  setAudioEnabled(enabled: boolean) {
    // Idempotent check - don't send duplicate messages
    if (this._audioEnabled === enabled) {
      console.log(`[WebSocketStream] Audio already ${enabled ? 'enabled' : 'disabled'}, skipping`)
      return
    }

    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn("[WebSocketStream] Cannot set audio enabled - WebSocket not connected")
      // Still update local state so we don't spam when connection is restored
      this._audioEnabled = enabled
      return
    }

    console.log(`[WebSocketStream] Setting audio enabled: ${enabled}`)
    this._audioEnabled = enabled

    // Send control message to server
    // Format: type(1) + JSON payload
    const json = JSON.stringify({ set_audio_enabled: enabled })
    const encoder = new TextEncoder()
    const jsonBytes = encoder.encode(json)

    const message = new Uint8Array(1 + jsonBytes.length)
    message[0] = WsMessageType.ControlMessage
    message.set(jsonBytes, 1)

    this.ws.send(message.buffer)
  }

  /**
   * Public method to force reconnection
   * Resets the attempt counter and initiates a fresh connection
   */
  reconnect() {
    console.log("[WebSocketStream] Manual reconnect requested")

    // Reset state for fresh connection
    this.closed = false
    this.reconnectAttempts = 0

    // Cancel any pending reconnection
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId)
      this.reconnectTimeoutId = null
    }

    // Close current connection and reconnect
    if (this.ws) {
      try {
        this.ws.close()
      } catch (e) {
        // Ignore
      }
    }

    // Connect immediately
    this.connect()
  }

  close() {
    console.log("[WebSocketStream] Closing")

    // Mark as explicitly closed to prevent reconnection and rendering
    this.closed = true

    // CRITICAL: Clear canvas references FIRST to prevent any further rendering
    // This must happen before decoder cleanup to ensure no frames are drawn
    // after close() is called (even if decoder has frames in queue)
    this.canvas = null
    this.canvasCtx = null

    // Cancel any pending reconnection
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId)
      this.reconnectTimeoutId = null
    }

    // Clear connection timeout
    this.clearConnectionTimeout()

    // Stop heartbeat
    this.stopHeartbeat()

    // Stop RTT pings
    this.stopPingInterval()

    // Stop event loop tracking
    this.stopEventLoopTracking()

    // Cancel pending mouse throttle flush
    if (this.mouseThrottleTimeoutId) {
      clearTimeout(this.mouseThrottleTimeoutId)
      this.mouseThrottleTimeoutId = null
    }

    // Cancel pending scroll throttle flush
    if (this.scrollThrottleTimeoutId) {
      clearTimeout(this.scrollThrottleTimeoutId)
      this.scrollThrottleTimeoutId = null
    }
    this.pendingScroll = null

    // Cancel pending touch throttle flush
    if (this.touchThrottleTimeoutId) {
      clearTimeout(this.touchThrottleTimeoutId)
      this.touchThrottleTimeoutId = null
    }
    this.pendingTouchMotion.clear()
    this.touchSlotMap.clear()

    // Reset stream state
    this.resetStreamState()

    // Clean up decoders
    this.cleanupDecoders()

    // Close WebSocket
    if (this.ws) {
      try {
        this.ws.close()
      } catch (e) {
        // Ignore
      }
      this.ws = null
    }

    this.connected = false
  }
}
