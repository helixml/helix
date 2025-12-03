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
  private maxReconnectAttempts = 5
  private reconnectDelay = 1000

  // Frame timing
  private lastFrameTime = 0
  private frameCount = 0

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
    this.streamerSize = this.getStreamerSize(viewerScreenSize)

    // Initialize input handler
    const streamInputConfig = defaultStreamInputConfig()
    Object.assign(streamInputConfig, {
      mouseScrollMode: this.settings.mouseScrollMode,
      controllerConfig: this.settings.controllerConfig,
    })
    this.input = new StreamInput(streamInputConfig)

    // Connect
    this.connect()
  }

  private getStreamerSize(viewerScreenSize: [number, number]): [number, number] {
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
    this.dispatchInfoEvent({ type: "connecting" })

    // Build WebSocket URL
    const queryParams = this.sessionId
      ? `?session_id=${encodeURIComponent(this.sessionId)}`
      : ""
    const wsUrl = `${this.api.host_url}/api/ws/stream${queryParams}`

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
    this.dispatchInfoEvent({ type: "connected" })

    // Send initialization message
    this.sendInit()
  }

  private onClose(event: CloseEvent) {
    console.log("[WebSocketStream] Disconnected:", event.code, event.reason)
    this.connected = false
    this.dispatchInfoEvent({ type: "disconnected" })

    // Attempt reconnection
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++
      this.dispatchInfoEvent({ type: "reconnecting", attempt: this.reconnectAttempts })

      setTimeout(() => {
        console.log(`[WebSocketStream] Reconnecting (attempt ${this.reconnectAttempts})...`)
        this.connect()
      }, this.reconnectDelay * this.reconnectAttempts)
    }
  }

  private onError(event: Event) {
    console.error("[WebSocketStream] Error:", event)
    this.dispatchInfoEvent({ type: "error", message: "WebSocket error" })
  }

  private async onMessage(event: MessageEvent) {
    if (!(event.data instanceof ArrayBuffer)) {
      // JSON control message
      try {
        const json = JSON.parse(event.data as string)
        this.handleControlMessage(json)
      } catch (e) {
        console.error("[WebSocketStream] Failed to parse JSON message:", e)
      }
      return
    }

    const data = new Uint8Array(event.data)
    if (data.length === 0) return

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

    this.videoDecoder.configure({
      codec: codecString,
      codedWidth: width,
      codedHeight: height,
      hardwareAcceleration: "prefer-hardware",
    })

    console.log("[WebSocketStream] Video decoder initialized")
  }

  private renderVideoFrame(frame: VideoFrame) {
    if (!this.canvas || !this.canvasCtx) {
      frame.close()
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

    // Track frame rate
    this.frameCount++
    const now = performance.now()
    if (now - this.lastFrameTime >= 1000) {
      console.log(`[WebSocketStream] FPS: ${this.frameCount}`)
      this.frameCount = 0
      this.lastFrameTime = now
    }
  }

  private async handleVideoFrame(data: Uint8Array) {
    if (!this.videoDecoder || this.videoDecoder.state !== "configured") {
      return
    }

    // Parse video frame header
    // Format: type(1) + codec(1) + flags(1) + pts(8) + width(2) + height(2) + data(...)
    if (data.length < 15) {
      console.error("[WebSocketStream] Video frame too short")
      return
    }

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const flags = data[2]
    const isKeyframe = (flags & 0x01) !== 0
    const ptsUs = view.getBigUint64(3, false) // big-endian
    // width at offset 11, height at offset 13 (already have from StreamInit)

    const frameData = data.slice(15)

    try {
      const chunk = new EncodedVideoChunk({
        type: isKeyframe ? "key" : "delta",
        timestamp: Number(ptsUs), // microseconds
        data: frameData,
      })

      this.videoDecoder.decode(chunk)
    } catch (e) {
      console.error("[WebSocketStream] Failed to decode video chunk:", e)
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

  private playAudioData(data: AudioData) {
    if (!this.audioContext) {
      data.close()
      return
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

    // Play audio
    const source = this.audioContext.createBufferSource()
    source.buffer = buffer
    source.connect(this.audioContext.destination)
    source.start()

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
  // Input Handling
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

  getInput(): StreamInput {
    // Patch input to send via WebSocket instead of DataChannel
    const originalInput = this.input
    const stream = this

    // Override the send methods to use WebSocket
    const wsInput = Object.create(originalInput)
    wsInput.sendRaw = (type: number, data: Uint8Array) => {
      stream.sendInputMessage(type, data)
    }

    return wsInput
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

  close() {
    console.log("[WebSocketStream] Closing")

    if (this.videoDecoder) {
      try {
        this.videoDecoder.close()
      } catch (e) {
        // Ignore
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

    if (this.ws) {
      this.maxReconnectAttempts = 0 // Prevent reconnection
      this.ws.close()
      this.ws = null
    }
  }
}
