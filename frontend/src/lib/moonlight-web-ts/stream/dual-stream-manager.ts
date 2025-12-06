/**
 * DualStreamManager - Manages two parallel WebSocket streams for congestion handling
 *
 * Opens a high-quality (60fps) and low-quality (15fps) stream simultaneously.
 * Monitors RTT on the primary stream and switches to the fallback stream
 * when latency exceeds the threshold.
 *
 * Same resolution is used for both streams to avoid scaling issues in lobbies mode.
 */

import { Api } from "../api"
import { StreamSettings } from "../component/settings_menu"
import { VideoCodecSupport } from "./video"
import { WebSocketStream, WsStreamInfoEvent, WsStreamInfoEventListener } from "./websocket-stream"

export interface DualStreamConfig {
  api: Api
  hostId: number
  appId: number
  settings: StreamSettings
  supportedVideoFormats: VideoCodecSupport
  viewerScreenSize: [number, number]
  sessionId?: string
}

export interface DualStreamStats {
  activeStream: 'primary' | 'fallback'
  primaryRttMs: number
  fallbackRttMs: number
  primaryFps: number
  fallbackFps: number
  isHighLatency: boolean
  // Forward stats from active stream
  fps: number
  videoPayloadBitrateMbps: number
  totalBitrateMbps: number
  framesDecoded: number
  framesDropped: number
  width: number
  height: number
  rttMs: number
}

// Quality presets
const HIGH_QUALITY = {
  fps: 60,
  bitrate: 20000,  // 20 Mbps
}

const LOW_QUALITY = {
  fps: 15,
  bitrate: 5000,  // 5 Mbps (lower fps = less data)
}

// Switching thresholds
const FALLBACK_RTT_THRESHOLD_MS = 150  // Switch to fallback when RTT > 150ms
const RECOVERY_RTT_THRESHOLD_MS = 80   // Switch back to primary when RTT < 80ms
const MIN_SWITCH_INTERVAL_MS = 5000    // Don't switch more than once per 5 seconds

export class DualStreamManager {
  private config: DualStreamConfig

  private primaryStream: WebSocketStream | null = null
  private fallbackStream: WebSocketStream | null = null

  private activeStream: 'primary' | 'fallback' = 'primary'
  private lastSwitchTime = 0

  private canvas: HTMLCanvasElement | null = null
  private eventTarget = new EventTarget()

  private checkIntervalId: ReturnType<typeof setInterval> | null = null

  constructor(config: DualStreamConfig) {
    this.config = config
  }

  /**
   * Start both streams
   */
  start() {
    const { api, hostId, appId, settings, supportedVideoFormats, viewerScreenSize, sessionId } = this.config

    // Create high-quality stream (primary)
    const primarySettings = { ...settings, fps: HIGH_QUALITY.fps, bitrate: HIGH_QUALITY.bitrate }
    this.primaryStream = new WebSocketStream(
      api,
      hostId,
      appId,
      primarySettings,
      supportedVideoFormats,
      viewerScreenSize,
      sessionId ? `${sessionId}-hq` : undefined  // Unique session ID for each stream
    )

    // Create low-quality stream (fallback)
    const fallbackSettings = { ...settings, fps: LOW_QUALITY.fps, bitrate: LOW_QUALITY.bitrate }
    this.fallbackStream = new WebSocketStream(
      api,
      hostId,
      appId,
      fallbackSettings,
      supportedVideoFormats,
      viewerScreenSize,
      sessionId ? `${sessionId}-lq` : undefined
    )

    // Set canvas on active stream
    if (this.canvas) {
      this.getActiveStream()?.setCanvas(this.canvas)
    }

    // Forward events from primary stream (it's the one we monitor)
    this.primaryStream.addInfoListener((event: WsStreamInfoEvent) => {
      this.handlePrimaryEvent(event)
    })

    // Forward connection events from fallback too
    this.fallbackStream.addInfoListener((event: WsStreamInfoEvent) => {
      this.handleFallbackEvent(event)
    })

    // Start periodic check for stream switching
    this.startSwitchCheck()
  }

  private handlePrimaryEvent(event: WsStreamInfoEvent) {
    const data = event.detail

    // Forward most events directly
    if (data.type === 'connected' || data.type === 'disconnected' ||
        data.type === 'error' || data.type === 'connectionComplete' ||
        data.type === 'streamInit' || data.type === 'reconnecting') {
      // Only forward if this is the active stream
      if (this.activeStream === 'primary') {
        this.dispatchInfoEvent(data)
      }
    }
  }

  private handleFallbackEvent(event: WsStreamInfoEvent) {
    const data = event.detail

    // Only forward events if fallback is active
    if (this.activeStream === 'fallback') {
      if (data.type === 'connected' || data.type === 'disconnected' ||
          data.type === 'error' || data.type === 'connectionComplete' ||
          data.type === 'streamInit' || data.type === 'reconnecting') {
        this.dispatchInfoEvent(data)
      }
    }
  }

  private startSwitchCheck() {
    this.stopSwitchCheck()

    this.checkIntervalId = setInterval(() => {
      this.checkAndSwitch()
    }, 1000)  // Check every second
  }

  private stopSwitchCheck() {
    if (this.checkIntervalId) {
      clearInterval(this.checkIntervalId)
      this.checkIntervalId = null
    }
  }

  private checkAndSwitch() {
    if (!this.primaryStream) return

    const now = Date.now()
    if (now - this.lastSwitchTime < MIN_SWITCH_INTERVAL_MS) {
      return  // Don't switch too frequently
    }

    const primaryStats = this.primaryStream.getStats()
    const primaryRtt = primaryStats.rttMs

    if (this.activeStream === 'primary' && primaryRtt > FALLBACK_RTT_THRESHOLD_MS) {
      this.switchToFallback()
    } else if (this.activeStream === 'fallback' && primaryRtt < RECOVERY_RTT_THRESHOLD_MS) {
      this.switchToPrimary()
    }
  }

  private switchToFallback() {
    if (this.activeStream === 'fallback') return

    console.log('[DualStream] Switching to fallback stream (low quality) due to high latency')
    this.activeStream = 'fallback'
    this.lastSwitchTime = Date.now()

    // Switch canvas to fallback stream
    if (this.canvas && this.fallbackStream) {
      this.fallbackStream.setCanvas(this.canvas)
    }

    // Notify listeners
    this.dispatchInfoEvent({
      type: 'addDebugLine',
      line: 'Switched to low-quality stream due to network congestion'
    })
  }

  private switchToPrimary() {
    if (this.activeStream === 'primary') return

    console.log('[DualStream] Switching back to primary stream (high quality) - network recovered')
    this.activeStream = 'primary'
    this.lastSwitchTime = Date.now()

    // Switch canvas to primary stream
    if (this.canvas && this.primaryStream) {
      this.primaryStream.setCanvas(this.canvas)
    }

    // Notify listeners
    this.dispatchInfoEvent({
      type: 'addDebugLine',
      line: 'Switched back to high-quality stream - network recovered'
    })
  }

  /**
   * Set the canvas for rendering
   */
  setCanvas(canvas: HTMLCanvasElement) {
    this.canvas = canvas
    this.getActiveStream()?.setCanvas(canvas)
  }

  /**
   * Get the currently active stream
   */
  private getActiveStream(): WebSocketStream | null {
    return this.activeStream === 'primary' ? this.primaryStream : this.fallbackStream
  }

  /**
   * Get stream size from active stream
   */
  getStreamerSize(): [number, number] {
    return this.getActiveStream()?.getStreamerSize() ?? [1920, 1080]
  }

  /**
   * Get combined stats from both streams
   */
  getStats(): DualStreamStats {
    const primaryStats = this.primaryStream?.getStats()
    const fallbackStats = this.fallbackStream?.getStats()
    const activeStats = this.activeStream === 'primary' ? primaryStats : fallbackStats

    return {
      activeStream: this.activeStream,
      primaryRttMs: primaryStats?.rttMs ?? 0,
      fallbackRttMs: fallbackStats?.rttMs ?? 0,
      primaryFps: primaryStats?.fps ?? 0,
      fallbackFps: fallbackStats?.fps ?? 0,
      isHighLatency: this.activeStream === 'fallback',
      // Forward from active stream
      fps: activeStats?.fps ?? 0,
      videoPayloadBitrateMbps: activeStats?.videoPayloadBitrateMbps ?? 0,
      totalBitrateMbps: activeStats?.totalBitrateMbps ?? 0,
      framesDecoded: activeStats?.framesDecoded ?? 0,
      framesDropped: activeStats?.framesDropped ?? 0,
      width: activeStats?.width ?? 1920,
      height: activeStats?.height ?? 1080,
      rttMs: activeStats?.rttMs ?? 0,
    }
  }

  /**
   * Get input handler from active stream
   */
  getInput() {
    return this.getActiveStream()?.getInput()
  }

  /**
   * Check if on fallback (low quality) stream
   */
  isOnFallback(): boolean {
    return this.activeStream === 'fallback'
  }

  /**
   * Add event listener
   */
  addInfoListener(listener: WsStreamInfoEventListener) {
    this.eventTarget.addEventListener("stream-info", listener as EventListenerOrEventListenerObject)
  }

  /**
   * Remove event listener
   */
  removeInfoListener(listener: WsStreamInfoEventListener) {
    this.eventTarget.removeEventListener("stream-info", listener as EventListenerOrEventListenerObject)
  }

  private dispatchInfoEvent(detail: WsStreamInfoEvent["detail"]) {
    const event: WsStreamInfoEvent = new CustomEvent("stream-info", { detail })
    this.eventTarget.dispatchEvent(event)
  }

  /**
   * Force reconnect both streams
   */
  reconnect() {
    this.primaryStream?.reconnect()
    this.fallbackStream?.reconnect()
  }

  /**
   * Close both streams
   */
  close() {
    console.log('[DualStream] Closing both streams')
    this.stopSwitchCheck()
    this.primaryStream?.close()
    this.fallbackStream?.close()
    this.primaryStream = null
    this.fallbackStream = null
  }
}
