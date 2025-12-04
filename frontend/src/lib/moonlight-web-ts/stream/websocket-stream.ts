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
} as const

const WsVideoCodec = {
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

type WsVideoCodecType = typeof WsVideoCodec[keyof typeof WsVideoCodec]

// Map codec byte to WebCodecs codec string
function codecToWebCodecsString(codec: WsVideoCodecType): string {
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

  // Heartbeat for stale connection detection
  private heartbeatIntervalId: ReturnType<typeof setInterval> | null = null
  private lastMessageTime = 0
  private heartbeatTimeout = 10000  // 10 seconds without data = stale

  // Frame timing and stats
  private lastFrameTime = 0
  private frameCount = 0
  private currentFps = 0
  // Video-only bytes (H.264 payload)
  private videoBytesReceived = 0
  private lastVideoBytesReceived = 0
  private currentVideoBitrateMbps = 0
  // Total WebSocket bytes (video + audio + control)
  private totalBytesReceived = 0
  private lastTotalBytesReceived = 0
  private lastBytesTime = 0
  private currentTotalBitrateMbps = 0
  private framesDecoded = 0
  private framesDropped = 0

  constructor(
    api: Api,
    hostId: number,
    appId: number,
    settings: StreamSettings,
    supportedVideoFormats: VideoCodecSupport,
    viewerScreenSize: [number, number],
    sessionId?: string
  ) {
    this.api = api
    this.hostId = hostId
    this.appId = appId
    this.settings = settings
    this.sessionId = sessionId
    this.streamerSize = this.calculateStreamerSize(viewerScreenSize)

    // Initialize input handler
    const streamInputConfig = defaultStreamInputConfig()
    Object.assign(streamInputConfig, {
      mouseScrollMode: this.settings.mouseScrollMode,
      controllerConfig: this.settings.controllerConfig,
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

    // Build WebSocket URL - must be absolute with ws:// or wss:// protocol
    const queryParams = this.sessionId
      ? `?session_id=${encodeURIComponent(this.sessionId)}`
      : ""

    // Convert relative URL to absolute WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}${this.api.host_url}/api/ws/stream${queryParams}`

    console.log("[WebSocketStream] Connecting to:", wsUrl)
    this.ws = new WebSocket(wsUrl)
    this.ws.binaryType = "arraybuffer"

    this.ws.addEventListener("open", this.onOpen.bind(this))
    this.ws.addEventListener("close", this.onClose.bind(this))
    this.ws.addEventListener("error", this.onError.bind(this))
    this.ws.addEventListener("message", this.onMessage.bind(this))
  }

  private onOpen() {
    console.log("[WebSocketStream] Connected")
    this.connected = true
    this.reconnectAttempts = 0
    this.lastMessageTime = Date.now()
    this.dispatchInfoEvent({ type: "connected" })

    // Start heartbeat monitoring for stale connections
    this.startHeartbeat()

    // Send initialization message
    this.sendInit()
  }

  private onClose(event: CloseEvent) {
    console.log("[WebSocketStream] Disconnected:", event.code, event.reason)
    this.connected = false

    // Stop heartbeat
    this.stopHeartbeat()

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
      default:
        console.warn("[WebSocketStream] Unknown message type:", msgType)
    }
  }

  private sendInit() {
    // Send initialization as JSON for simplicity
    const initMessage = {
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
      video_supported_formats: createSupportedVideoFormatsBits({
        H264: true,
        H264_HIGH8_444: false,
        H265: false,
        H265_MAIN10: false,
        H265_REXT8_444: false,
        H265_REXT10_444: false,
        AV1_MAIN8: false,
        AV1_MAIN10: false,
        AV1_HIGH8_444: false,
        AV1_HIGH10_444: false,
      }),
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

    // Initialize video decoder
    this.initVideoDecoder(codec, width, height)

    // Initialize audio decoder
    this.initAudioDecoder(audioChannels, sampleRate)
  }

  private async initVideoDecoder(codec: WsVideoCodecType, width: number, height: number) {
    if (!("VideoDecoder" in window)) {
      console.error("[WebSocketStream] WebCodecs VideoDecoder not supported")
      this.dispatchInfoEvent({ type: "error", message: "WebCodecs not supported in this browser" })
      return
    }

    // Store config for potential recovery
    this.lastVideoCodec = codec
    this.lastVideoWidth = width
    this.lastVideoHeight = height

    const codecString = codecToWebCodecsString(codec)
    console.log(`[WebSocketStream] Initializing video decoder: ${codecString} ${width}x${height}`)

    // Check if codec is supported
    try {
      const support = await VideoDecoder.isConfigSupported({
        codec: codecString,
        codedWidth: width,
        codedHeight: height,
        hardwareAcceleration: "prefer-hardware",
      })

      if (!support.supported) {
        console.error("[WebSocketStream] Video codec not supported:", codecString)
        this.dispatchInfoEvent({ type: "error", message: `Video codec ${codecString} not supported` })
        return
      }
    } catch (e) {
      console.error("[WebSocketStream] Failed to check codec support:", e)
    }

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
        console.error("[WebSocketStream] Video decoder error:", e)
      },
    })

    // Configure decoder with Annex B format for H264/H265 (in-band SPS/PPS)
    // This tells WebCodecs to expect NAL start codes and in-band parameter sets
    const config: VideoDecoderConfig = {
      codec: codecString,
      codedWidth: width,
      codedHeight: height,
      hardwareAcceleration: "prefer-hardware",
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
        hardwareAcceleration: "prefer-hardware",
      })
      console.log("[WebSocketStream] Video decoder configured (fallback mode)")
    }
  }

  private renderVideoFrame(frame: VideoFrame) {
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
          // Video-only bitrate
          const deltaVideoBytes = this.videoBytesReceived - this.lastVideoBytesReceived
          this.currentVideoBitrateMbps = (deltaVideoBytes * 8) / 1000000 / deltaTime
          // Total WebSocket bitrate (video + audio + control)
          const deltaTotalBytes = this.totalBytesReceived - this.lastTotalBytesReceived
          this.currentTotalBitrateMbps = (deltaTotalBytes * 8) / 1000000 / deltaTime
        }
      }
      this.lastVideoBytesReceived = this.videoBytesReceived
      this.lastTotalBytesReceived = this.totalBytesReceived
      this.lastBytesTime = now
    }
  }

  // Track if we've received the first keyframe (needed for decoder to work)
  private receivedFirstKeyframe = false

  // Track last video config for decoder recovery
  private lastVideoCodec: WsVideoCodecType | null = null
  private lastVideoWidth = 0
  private lastVideoHeight = 0

  private async handleVideoFrame(data: Uint8Array) {
    // Track video bytes received for bitrate calculation
    this.videoBytesReceived += data.length

    if (!this.videoDecoder || this.videoDecoder.state !== "configured") {
      // Queue frames or drop them if decoder isn't ready
      return
    }

    // Parse video frame header
    // Format: type(1) + codec(1) + flags(1) + pts(8) + width(2) + height(2) + data(...)
    if (data.length < 15) {
      console.error("[WebSocketStream] Video frame too short:", data.length)
      return
    }

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const flags = data[2]
    const isKeyframe = (flags & 0x01) !== 0
    const ptsUs = view.getBigUint64(3, false) // big-endian
    // width at offset 11, height at offset 13 (already have from StreamInit)

    const frameData = data.slice(15)

    // Skip delta frames until we receive the first keyframe
    // (keyframe should contain SPS/PPS needed for decoding)
    if (!this.receivedFirstKeyframe) {
      if (!isKeyframe) {
        console.log("[WebSocketStream] Waiting for first keyframe, skipping delta frame")
        return
      }
      console.log(`[WebSocketStream] First keyframe received (${frameData.length} bytes)`)
      this.receivedFirstKeyframe = true
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
  // Input Handling - WebSocket transport
  // ============================================================================

  private sendInputMessage(type: number, payload: Uint8Array) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return
    }

    const message = new Uint8Array(1 + payload.length)
    message[0] = type
    message.set(payload, 1)
    this.ws.send(message.buffer)
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
    // Format: subType(1) + dx(2) + dy(2)
    this.inputBuffer[0] = 0 // sub-type for relative
    this.inputView.setInt16(1, Math.round(movementX), false)
    this.inputView.setInt16(3, Math.round(movementY), false)
    this.sendInputMessage(WsMessageType.MouseRelative, this.inputBuffer.subarray(0, 5))
  }

  sendMousePosition(x: number, y: number, refWidth: number, refHeight: number) {
    // Format: subType(1) + x(2) + y(2) + refWidth(2) + refHeight(2)
    this.inputBuffer[0] = 1 // sub-type for absolute
    this.inputView.setInt16(1, Math.round(x), false)
    this.inputView.setInt16(3, Math.round(y), false)
    this.inputView.setInt16(5, Math.round(refWidth), false)
    this.inputView.setInt16(7, Math.round(refHeight), false)
    this.sendInputMessage(WsMessageType.MouseAbsolute, this.inputBuffer.subarray(0, 9))
  }

  sendMouseButton(isDown: boolean, button: number) {
    // Format: subType(1) + isDown(1) + button(1)
    this.inputBuffer[0] = 2 // sub-type for button
    this.inputBuffer[1] = isDown ? 1 : 0
    this.inputBuffer[2] = button
    this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 3))
  }

  sendMouseWheelHighRes(deltaX: number, deltaY: number) {
    // Format: subType(1) + deltaX(2) + deltaY(2)
    this.inputBuffer[0] = 3 // sub-type for high-res wheel
    this.inputView.setInt16(1, Math.round(deltaX), false)
    this.inputView.setInt16(3, Math.round(deltaY), false)
    this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 5))
  }

  sendMouseWheel(deltaX: number, deltaY: number) {
    // Format: subType(1) + deltaX(1) + deltaY(1)
    this.inputBuffer[0] = 4 // sub-type for normal wheel
    this.inputBuffer[1] = Math.round(deltaX) & 0xFF
    this.inputBuffer[2] = Math.round(deltaY) & 0xFF
    this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 3))
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
    videoBitrateMbps: number
    totalBitrateMbps: number
    framesDecoded: number
    framesDropped: number
    width: number
    height: number
  } {
    return {
      fps: this.currentFps,
      videoBitrateMbps: this.currentVideoBitrateMbps,
      totalBitrateMbps: this.currentTotalBitrateMbps,
      framesDecoded: this.framesDecoded,
      framesDropped: this.framesDropped,
      width: this.streamerSize[0],
      height: this.streamerSize[1],
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

    // Mark as explicitly closed to prevent reconnection
    this.closed = true

    // Cancel any pending reconnection
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId)
      this.reconnectTimeoutId = null
    }

    // Stop heartbeat
    this.stopHeartbeat()

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
