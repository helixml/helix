/**
 * DirectInputWebSocket - Direct input to GNOME/PipeWire desktop sessions
 *
 * This module provides a WebSocket connection that bypasses Moonlight/Wolf
 * for input events, sending them directly to the Go screenshot-server which
 * then forwards them to GNOME's RemoteDesktop D-Bus API.
 *
 * This is specifically for Ubuntu/GNOME desktop sessions using PipeWire.
 * Sway sessions continue to use the Moonlight input path.
 *
 * Architecture:
 * Browser → DirectInputWebSocket → Helix API → RevDial → screenshot-server → D-Bus → GNOME
 */

// Message type constants (must match Go server)
const MSG_TYPE_KEYBOARD = 0x01;
const MSG_TYPE_MOUSE_BUTTON = 0x02;
const MSG_TYPE_MOUSE_ABSOLUTE = 0x03;
const MSG_TYPE_MOUSE_RELATIVE = 0x04;
const MSG_TYPE_SCROLL = 0x05;
const MSG_TYPE_TOUCH = 0x06;

// Scroll deltaMode values (from browser WheelEvent)
const DELTA_MODE_PIXEL = 0;
const DELTA_MODE_LINE = 1;
const DELTA_MODE_PAGE = 2;

export interface DirectInputWebSocketOptions {
  sessionId: string;
  token: string;  // Auth token for API authentication
  onConnected?: () => void;
  onDisconnected?: () => void;
  onError?: (error: Error) => void;
}

/**
 * DirectInputWebSocket provides direct input to GNOME/PipeWire sessions.
 *
 * Usage:
 * ```typescript
 * const directInput = new DirectInputWebSocket({
 *   sessionId: 'ses_xxx',
 *   onConnected: () => console.log('Direct input connected'),
 * });
 * await directInput.connect();
 *
 * // Send scroll events
 * element.addEventListener('wheel', (e) => {
 *   directInput.sendScroll(e);
 * });
 * ```
 */
export class DirectInputWebSocket {
  private ws: WebSocket | null = null;
  private sessionId: string;
  private token: string;
  private onConnected?: () => void;
  private onDisconnected?: () => void;
  private onError?: (error: Error) => void;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 3;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private isExplicitlyClosed = false;

  // Scroll tracking for trackpad detection
  private lastScrollTime = 0;
  private scrollEventCount = 0;

  constructor(options: DirectInputWebSocketOptions) {
    this.sessionId = options.sessionId;
    this.token = options.token;
    this.onConnected = options.onConnected;
    this.onDisconnected = options.onDisconnected;
    this.onError = options.onError;
  }

  /**
   * Connect to the direct input WebSocket.
   * Returns true if connection established, false otherwise.
   */
  async connect(): Promise<boolean> {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      console.log('[DirectInput] Already connected');
      return true;
    }

    this.isExplicitlyClosed = false;

    // Build WebSocket URL with token as query parameter (WebSocket handshake can't use headers)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/v1/external-agents/${this.sessionId}/ws/input?access_token=${encodeURIComponent(this.token)}`;

    console.log('[DirectInput] Connecting to:', wsUrl);

    return new Promise((resolve) => {
      try {
        this.ws = new WebSocket(wsUrl);
        this.ws.binaryType = 'arraybuffer';

        const timeout = setTimeout(() => {
          console.log('[DirectInput] Connection timeout');
          this.ws?.close();
          resolve(false);
        }, 5000);

        this.ws.onopen = () => {
          clearTimeout(timeout);
          console.log('[DirectInput] Connected');
          this.reconnectAttempts = 0;
          this.onConnected?.();
          resolve(true);
        };

        this.ws.onclose = () => {
          clearTimeout(timeout);
          console.log('[DirectInput] Disconnected');
          this.onDisconnected?.();

          // Auto-reconnect unless explicitly closed
          if (!this.isExplicitlyClosed && this.reconnectAttempts < this.maxReconnectAttempts) {
            this.scheduleReconnect();
          }

          resolve(false);
        };

        this.ws.onerror = (event) => {
          clearTimeout(timeout);
          console.error('[DirectInput] WebSocket error:', event);
          this.onError?.(new Error('WebSocket connection failed'));
          resolve(false);
        };

        // We don't expect to receive messages, but log them for debugging
        this.ws.onmessage = (event) => {
          console.log('[DirectInput] Received unexpected message:', event.data);
        };
      } catch (err) {
        console.error('[DirectInput] Failed to create WebSocket:', err);
        resolve(false);
      }
    });
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
    }

    this.reconnectAttempts++;
    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts - 1), 5000);

    console.log(`[DirectInput] Scheduling reconnect attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts} in ${delay}ms`);

    this.reconnectTimeout = setTimeout(() => {
      this.connect();
    }, delay);
  }

  /**
   * Close the WebSocket connection.
   */
  close(): void {
    this.isExplicitlyClosed = true;

    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }

    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    console.log('[DirectInput] Closed');
  }

  /**
   * Disconnect the WebSocket connection (alias for close).
   */
  disconnect(): void {
    this.close();
  }

  /**
   * Check if the WebSocket is connected and ready.
   */
  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  /**
   * Send a scroll event directly to GNOME.
   *
   * This converts browser WheelEvent values to the binary protocol
   * expected by the Go screenshot-server.
   */
  sendScroll(event: WheelEvent): boolean {
    if (!this.isConnected()) {
      return false;
    }

    // Detect if this is likely a trackpad gesture based on event frequency and magnitude
    const now = performance.now();
    const timeSinceLastScroll = now - this.lastScrollTime;
    this.lastScrollTime = now;

    // Trackpad detection heuristics:
    // - Trackpad events come at high frequency (< 50ms apart)
    // - Trackpad events have small deltas in pixel mode
    // - Mouse wheel events come less frequently and have larger deltas (~100-120px)
    let isTrackpad = false;
    if (event.deltaMode === DELTA_MODE_PIXEL) {
      const magnitude = Math.abs(event.deltaX) + Math.abs(event.deltaY);
      // High frequency + small magnitude = trackpad
      // OR recent burst of events = trackpad
      if ((timeSinceLastScroll < 50 && magnitude < 50) || this.scrollEventCount > 5) {
        isTrackpad = true;
      }
    }

    // Track event frequency
    if (timeSinceLastScroll < 100) {
      this.scrollEventCount++;
    } else {
      this.scrollEventCount = 1;
    }

    // Build binary message: [type:1][deltaMode:1][flags:1][deltaX:4][deltaY:4]
    const buffer = new ArrayBuffer(11);
    const view = new DataView(buffer);

    view.setUint8(0, MSG_TYPE_SCROLL);           // message type
    view.setUint8(1, event.deltaMode);            // deltaMode (0=pixel, 1=line, 2=page)
    view.setUint8(2, isTrackpad ? 0x01 : 0x00);   // flags (bit 0 = is_trackpad)
    view.setFloat32(3, event.deltaX, true);       // deltaX (little-endian)
    view.setFloat32(7, event.deltaY, true);       // deltaY (little-endian)

    try {
      this.ws!.send(buffer);

      // Log first few and then periodically for debugging
      if (this.scrollEventCount <= 3 || this.scrollEventCount % 100 === 0) {
        console.log(`[DirectInput] Scroll sent: deltaMode=${event.deltaMode} deltaX=${event.deltaX.toFixed(1)} deltaY=${event.deltaY.toFixed(1)} isTrackpad=${isTrackpad}`);
      }

      return true;
    } catch (err) {
      console.error('[DirectInput] Failed to send scroll:', err);
      return false;
    }
  }

  /**
   * Send a keyboard event directly to GNOME.
   *
   * Format: [type:1][isDown:1][modifiers:1][keycode:2]
   */
  sendKeyboard(keycode: number, isDown: boolean, modifiers: number = 0): boolean {
    if (!this.isConnected()) {
      return false;
    }

    const buffer = new ArrayBuffer(5);
    const view = new DataView(buffer);

    view.setUint8(0, MSG_TYPE_KEYBOARD);
    view.setUint8(1, isDown ? 1 : 0);
    view.setUint8(2, modifiers);
    view.setUint16(3, keycode, true); // little-endian

    try {
      this.ws!.send(buffer);
      return true;
    } catch (err) {
      console.error('[DirectInput] Failed to send keyboard:', err);
      return false;
    }
  }

  /**
   * Send a mouse button event directly to GNOME.
   *
   * Format: [type:1][isDown:1][button:1]
   */
  sendMouseButton(button: number, isDown: boolean): boolean {
    if (!this.isConnected()) {
      return false;
    }

    const buffer = new ArrayBuffer(3);
    const view = new DataView(buffer);

    view.setUint8(0, MSG_TYPE_MOUSE_BUTTON);
    view.setUint8(1, isDown ? 1 : 0);
    view.setUint8(2, button);

    try {
      this.ws!.send(buffer);
      return true;
    } catch (err) {
      console.error('[DirectInput] Failed to send mouse button:', err);
      return false;
    }
  }

  /**
   * Send an absolute mouse position event directly to GNOME.
   *
   * Format: [type:1][x:4][y:4][refWidth:2][refHeight:2]
   */
  sendMouseAbsolute(x: number, y: number, refWidth: number, refHeight: number): boolean {
    if (!this.isConnected()) {
      return false;
    }

    const buffer = new ArrayBuffer(13);
    const view = new DataView(buffer);

    view.setUint8(0, MSG_TYPE_MOUSE_ABSOLUTE);
    view.setFloat32(1, x, true);
    view.setFloat32(5, y, true);
    view.setUint16(9, refWidth, true);
    view.setUint16(11, refHeight, true);

    try {
      this.ws!.send(buffer);
      return true;
    } catch (err) {
      console.error('[DirectInput] Failed to send mouse absolute:', err);
      return false;
    }
  }

  /**
   * Send a relative mouse movement event directly to GNOME.
   *
   * Format: [type:1][dx:4][dy:4]
   */
  sendMouseRelative(dx: number, dy: number): boolean {
    if (!this.isConnected()) {
      return false;
    }

    const buffer = new ArrayBuffer(9);
    const view = new DataView(buffer);

    view.setUint8(0, MSG_TYPE_MOUSE_RELATIVE);
    view.setFloat32(1, dx, true);
    view.setFloat32(5, dy, true);

    try {
      this.ws!.send(buffer);
      return true;
    } catch (err) {
      console.error('[DirectInput] Failed to send mouse relative:', err);
      return false;
    }
  }
}

export default DirectInputWebSocket;
