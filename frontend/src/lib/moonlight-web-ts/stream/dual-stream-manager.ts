/**
 * DualStreamManager - Manages adaptive quality WebSocket streaming
 *
 * Starts with a high-quality (60fps) stream and switches to low-quality (15fps)
 * when latency exceeds the threshold. Switches are SEQUENTIAL - the old stream
 * is fully closed before the new one is opened to avoid parallel connections
 * which break the Moonlight protocol.
 *
 * Same resolution is used for both quality levels to avoid scaling issues in lobbies mode.
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
   * Start streaming (begins with primary/high-quality stream)
   */
  start() {
    console.log('[DualStream] Starting with primary (high-quality) stream')
    this.activeStream = 'primary'
    this.startPrimaryStream()

    // Start periodic check for stream switching
    this.startSwitchCheck()
  }

  /**
   * Create and start the primary (high-quality) stream
   */
  private startPrimaryStream() {
    const { api, hostId, appId, settings, supportedVideoFormats, viewerScreenSize, sessionId } = this.config

    const primarySettings = { ...settings, fps: HIGH_QUALITY.fps, bitrate: HIGH_QUALITY.bitrate }
    this.primaryStream = new WebSocketStream(
      api,
      hostId,
      appId,
      primarySettings,
      supportedVideoFormats,
      viewerScreenSize,
      sessionId
    )

    // Set canvas for rendering
    if (this.canvas) {
      this.primaryStream.setCanvas(this.canvas)
    }

    // Forward events from primary stream
    this.primaryStream.addInfoListener((event: WsStreamInfoEvent) => {
      this.handlePrimaryEvent(event)
    })
  }

  /**
   * Create and start the fallback (low-quality) stream
   */
  private startFallbackStream() {
    const { api, hostId, appId, settings, supportedVideoFormats, viewerScreenSize, sessionId } = this.config

    const fallbackSettings = { ...settings, fps: LOW_QUALITY.fps, bitrate: LOW_QUALITY.bitrate }
    this.fallbackStream = new WebSocketStream(
      api,
      hostId,
      appId,
      fallbackSettings,
      supportedVideoFormats,
      viewerScreenSize,
      sessionId
    )

    // Set canvas for rendering
    if (this.canvas) {
      this.fallbackStream.setCanvas(this.canvas)
    }

    // Forward events from fallback stream
    this.fallbackStream.addInfoListener((event: WsStreamInfoEvent) => {
      this.handleFallbackEvent(event)
    })
  }

  private handlePrimaryEvent(event: WsStreamInfoEvent) {
    const data = event.detail

    // Forward events from primary stream (only active when primary is the current stream)
    if (data.type === 'connected' || data.type === 'disconnected' ||
        data.type === 'error' || data.type === 'connectionComplete' ||
        data.type === 'streamInit' || data.type === 'reconnecting') {
      this.dispatchInfoEvent(data)
    }
  }

  private handleFallbackEvent(event: WsStreamInfoEvent) {
    const data = event.detail

    // Forward events from fallback stream (only active when fallback is the current stream)
    if (data.type === 'connected' || data.type === 'disconnected' ||
        data.type === 'error' || data.type === 'connectionComplete' ||
        data.type === 'streamInit' || data.type === 'reconnecting') {
      this.dispatchInfoEvent(data)
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
    const activeStream = this.getActiveStream()
    if (!activeStream) return

    const now = Date.now()
    if (now - this.lastSwitchTime < MIN_SWITCH_INTERVAL_MS) {
      return  // Don't switch too frequently
    }

    const stats = activeStream.getStats()
    const rtt = stats.rttMs

    if (this.activeStream === 'primary' && rtt > FALLBACK_RTT_THRESHOLD_MS) {
      this.switchToFallback()
    } else if (this.activeStream === 'fallback' && rtt < RECOVERY_RTT_THRESHOLD_MS) {
      this.switchToPrimary()
    }
  }

  private switchToFallback() {
    if (this.activeStream === 'fallback') return

    console.log('[DualStream] Switching to fallback stream (low quality) due to high latency')

    // SEQUENTIAL: Close primary stream first, then start fallback
    // This avoids parallel connections which break the Moonlight protocol
    if (this.primaryStream) {
      console.log('[DualStream] Closing primary stream before starting fallback...')
      this.primaryStream.close()
      this.primaryStream = null
    }

    this.activeStream = 'fallback'
    this.lastSwitchTime = Date.now()

    // Now start the fallback stream
    this.startFallbackStream()

    // Notify listeners
    this.dispatchInfoEvent({
      type: 'addDebugLine',
      line: 'Switched to low-quality stream due to network congestion'
    })
  }

  private switchToPrimary() {
    if (this.activeStream === 'primary') return

    console.log('[DualStream] Switching back to primary stream (high quality) - network recovered')

    // SEQUENTIAL: Close fallback stream first, then start primary
    // This avoids parallel connections which break the Moonlight protocol
    if (this.fallbackStream) {
      console.log('[DualStream] Closing fallback stream before starting primary...')
      this.fallbackStream.close()
      this.fallbackStream = null
    }

    this.activeStream = 'primary'
    this.lastSwitchTime = Date.now()

    // Now start the primary stream
    this.startPrimaryStream()

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
   * Get stats from the active stream
   */
  getStats(): DualStreamStats {
    const activeStream = this.getActiveStream()
    const activeStats = activeStream?.getStats()

    return {
      activeStream: this.activeStream,
      // Since we only have one stream active at a time, report its RTT in the appropriate field
      primaryRttMs: this.activeStream === 'primary' ? (activeStats?.rttMs ?? 0) : 0,
      fallbackRttMs: this.activeStream === 'fallback' ? (activeStats?.rttMs ?? 0) : 0,
      primaryFps: this.activeStream === 'primary' ? (activeStats?.fps ?? 0) : 0,
      fallbackFps: this.activeStream === 'fallback' ? (activeStats?.fps ?? 0) : 0,
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
   * Force reconnect the active stream
   */
  reconnect() {
    this.getActiveStream()?.reconnect()
  }

  /**
   * Close the active stream
   */
  close() {
    console.log('[DualStream] Closing active stream')
    this.stopSwitchCheck()
    if (this.primaryStream) {
      this.primaryStream.close()
      this.primaryStream = null
    }
    if (this.fallbackStream) {
      this.fallbackStream.close()
      this.fallbackStream = null
    }
  }
}
