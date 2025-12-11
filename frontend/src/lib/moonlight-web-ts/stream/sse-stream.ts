/**
 * SSE-Based Video Stream
 *
 * Experimental alternative to WebSocket streaming.
 * Uses Server-Sent Events for video/audio frames (unidirectional, server→client)
 * Can be paired with WebSocket for input (bidirectional, client→server)
 *
 * This separation may help with latency issues observed on some network configurations
 * where WebSocket streaming exhibited high latency that went away with SSE.
 */

import { StreamSettings, VideoCodecSupport, WsVideoCodecType } from "./types"

interface SseStreamStats {
  fps: number
  framesReceived: number
  framesDecoded: number
  framesDropped: number
  width: number
  height: number
  bytesReceived: number
  connected: boolean
  transport: "sse"
}

interface SseVideoFrame {
  codec: number
  flags: number
  pts: number
  width: number
  height: number
  keyframe: boolean
  data: string  // base64 encoded
}

interface SseAudioFrame {
  channels: number
  pts: number
  data: string  // base64 encoded
}

interface SseStreamInit {
  video_codec: number
  width: number
  height: number
  fps: number
  audio_channels: number
  audio_sample_rate: number
  touch_supported: boolean
}

export class SseStream {
  private eventSource: EventSource | null = null
  private videoDecoder: VideoDecoder | null = null
  private audioContext: AudioContext | null = null

  // Canvas for rendering
  private canvas: HTMLCanvasElement | null = null
  private canvasCtx: CanvasRenderingContext2D | null = null

  // Stats tracking
  private framesReceived = 0
  private framesDecoded = 0
  private framesDropped = 0
  private bytesReceived = 0
  private lastFrameTime = 0
  private frameCount = 0
  private currentFps = 0

  // Stream info
  private streamWidth = 0
  private streamHeight = 0

  // Callbacks
  private onVideoFrame: ((frame: VideoFrame) => void) | null = null
  private onStreamInit: ((init: SseStreamInit) => void) | null = null
  private onError: ((error: string) => void) | null = null
  private onClose: (() => void) | null = null

  constructor(
    private readonly sseUrl: string,
    private readonly settings: StreamSettings
  ) {}

  async connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      this.eventSource = new EventSource(this.sseUrl)

      this.eventSource.onopen = () => {
        console.log("[SseStream] Connected to SSE endpoint")
        resolve()
      }

      this.eventSource.onerror = (event) => {
        console.error("[SseStream] Connection error:", event)
        if (this.eventSource?.readyState === EventSource.CLOSED) {
          this.onError?.("SSE connection closed")
          this.onClose?.()
        }
        reject(new Error("SSE connection failed"))
      }

      // Handle stream init
      this.eventSource.addEventListener("init", (event: MessageEvent) => {
        try {
          const init: SseStreamInit = JSON.parse(event.data)
          console.log("[SseStream] Stream init:", init)
          this.streamWidth = init.width
          this.streamHeight = init.height
          this.initVideoDecoder(init.video_codec, init.width, init.height)
          this.onStreamInit?.(init)
        } catch (e) {
          console.error("[SseStream] Failed to parse init:", e)
        }
      })

      // Handle video frames
      this.eventSource.addEventListener("video", (event: MessageEvent) => {
        try {
          const frame: SseVideoFrame = JSON.parse(event.data)
          this.handleVideoFrame(frame)
        } catch (e) {
          console.error("[SseStream] Failed to parse video frame:", e)
        }
      })

      // Handle audio frames
      this.eventSource.addEventListener("audio", (event: MessageEvent) => {
        try {
          const frame: SseAudioFrame = JSON.parse(event.data)
          this.handleAudioFrame(frame)
        } catch (e) {
          console.error("[SseStream] Failed to parse audio frame:", e)
        }
      })

      // Handle control messages
      this.eventSource.addEventListener("control", (event: MessageEvent) => {
        console.log("[SseStream] Control message:", event.data)
      })

      // Handle errors
      this.eventSource.addEventListener("error", (event: MessageEvent) => {
        console.error("[SseStream] Server error:", event.data)
        this.onError?.(event.data)
      })
    })
  }

  private async initVideoDecoder(codec: number, width: number, height: number): Promise<void> {
    // Map codec number to MIME type
    const codecString = this.getCodecString(codec)
    console.log(`[SseStream] Initializing video decoder: ${codecString} ${width}x${height}`)

    this.videoDecoder = new VideoDecoder({
      output: (frame: VideoFrame) => {
        this.framesDecoded++
        this.renderVideoFrame(frame)
        frame.close()
      },
      error: (error) => {
        console.error("[SseStream] Decoder error:", error)
        this.framesDropped++
      }
    })

    this.videoDecoder.configure({
      codec: codecString,
      codedWidth: width,
      codedHeight: height,
      hardwareAcceleration: "prefer-hardware",
    })
  }

  private getCodecString(codec: number): string {
    // Map server codec enum to WebCodecs codec string
    switch (codec) {
      case 0x01: return "avc1.640028"  // H.264 High
      case 0x02: return "avc1.f4001e"  // H.264 High 4:4:4
      case 0x10: return "hvc1.1.6.L120.90"  // HEVC Main
      case 0x11: return "hvc1.2.4.L120.90"  // HEVC Main 10
      default: return "avc1.640028"  // Default to H.264
    }
  }

  private handleVideoFrame(frame: SseVideoFrame): void {
    if (!this.videoDecoder || this.videoDecoder.state !== "configured") {
      return
    }

    this.framesReceived++

    // Decode base64 data
    const binaryString = atob(frame.data)
    const bytes = new Uint8Array(binaryString.length)
    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i)
    }

    this.bytesReceived += bytes.length

    // Update FPS counter
    const now = performance.now()
    this.frameCount++
    if (now - this.lastFrameTime > 1000) {
      this.currentFps = Math.round(this.frameCount * 1000 / (now - this.lastFrameTime))
      this.frameCount = 0
      this.lastFrameTime = now
    }

    try {
      const chunk = new EncodedVideoChunk({
        type: frame.keyframe ? "key" : "delta",
        timestamp: frame.pts,  // microseconds
        data: bytes,
      })

      this.videoDecoder.decode(chunk)
    } catch (e) {
      console.error("[SseStream] Failed to decode video chunk:", e)
      this.framesDropped++
    }
  }

  private handleAudioFrame(frame: SseAudioFrame): void {
    // Audio decoding would go here
    // For now, just track the data
    const binaryString = atob(frame.data)
    this.bytesReceived += binaryString.length
  }

  private renderVideoFrame(frame: VideoFrame): void {
    if (!this.canvasCtx) {
      return
    }

    // Draw frame to canvas
    this.canvasCtx.drawImage(frame, 0, 0, this.canvas!.width, this.canvas!.height)
  }

  setCanvas(canvas: HTMLCanvasElement): void {
    this.canvas = canvas
    this.canvasCtx = canvas.getContext("2d", {
      alpha: false,
      desynchronized: true,
    })
  }

  getStats(): SseStreamStats {
    return {
      fps: this.currentFps,
      framesReceived: this.framesReceived,
      framesDecoded: this.framesDecoded,
      framesDropped: this.framesDropped,
      width: this.streamWidth,
      height: this.streamHeight,
      bytesReceived: this.bytesReceived,
      connected: this.eventSource?.readyState === EventSource.OPEN,
      transport: "sse",
    }
  }

  getStreamerSize(): [number, number] {
    return [this.streamWidth, this.streamHeight]
  }

  close(): void {
    console.log("[SseStream] Closing connection")

    if (this.eventSource) {
      this.eventSource.close()
      this.eventSource = null
    }

    if (this.videoDecoder) {
      this.videoDecoder.close()
      this.videoDecoder = null
    }

    this.onClose?.()
  }

  // Callback setters
  setOnVideoFrame(callback: (frame: VideoFrame) => void): void {
    this.onVideoFrame = callback
  }

  setOnStreamInit(callback: (init: SseStreamInit) => void): void {
    this.onStreamInit = callback
  }

  setOnError(callback: (error: string) => void): void {
    this.onError = callback
  }

  setOnClose(callback: () => void): void {
    this.onClose = callback
  }
}

/**
 * Build SSE stream URL with query parameters
 */
export function buildSseStreamUrl(
  baseUrl: string,
  hostId: number,
  appId: number,
  sessionId: string,
  settings: StreamSettings
): string {
  const params = new URLSearchParams({
    host_id: hostId.toString(),
    app_id: appId.toString(),
    session_id: sessionId,
    width: settings.width.toString(),
    height: settings.height.toString(),
    fps: settings.fps.toString(),
    bitrate: settings.bitrate.toString(),
  })

  return `${baseUrl}/sse/stream?${params.toString()}`
}
