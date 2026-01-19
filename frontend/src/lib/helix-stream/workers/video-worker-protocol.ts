/**
 * Video Worker Protocol
 *
 * Message types for communication between main thread and video worker.
 * The worker owns the WebSocket connection, VideoDecoder, and OffscreenCanvas.
 */

// ============================================================================
// Main Thread → Worker Messages
// ============================================================================

export interface InitMessage {
  type: 'init';
  canvas: OffscreenCanvas;
  wsUrl: string;
  sessionId: string;
  hostId: number;
  appId: number;
  userName: string;
  clientUniqueId: string;
  width: number;
  height: number;
  fps: number;
  bitrate: number;
}

export interface MouseMoveMessage {
  type: 'mouseMove';
  x: number;
  y: number;
  canvasWidth: number;
  canvasHeight: number;
}

export interface MouseButtonMessage {
  type: 'mouseButton';
  button: number;
  isDown: boolean;
  x: number;
  y: number;
  canvasWidth: number;
  canvasHeight: number;
}

export interface MouseWheelMessage {
  type: 'mouseWheel';
  deltaX: number;
  deltaY: number;
  x: number;
  y: number;
  canvasWidth: number;
  canvasHeight: number;
}

export interface KeyboardMessage {
  type: 'keyboard';
  key: string;
  code: string;
  isDown: boolean;
  modifiers: {
    ctrl: boolean;
    alt: boolean;
    shift: boolean;
    meta: boolean;
  };
}

export interface TouchMessage {
  type: 'touch';
  eventType: 'start' | 'move' | 'end' | 'cancel';
  touches: Array<{
    identifier: number;
    x: number;
    y: number;
    canvasWidth: number;
    canvasHeight: number;
  }>;
}

export interface SetVideoEnabledMessage {
  type: 'setVideoEnabled';
  enabled: boolean;
}

export interface SetAudioEnabledMessage {
  type: 'setAudioEnabled';
  enabled: boolean;
}

export interface SetBitrateMessage {
  type: 'setBitrate';
  bitrate: number;
}

export interface SetThrottleRatioMessage {
  type: 'setThrottleRatio';
  ratio: number | null;
}

export interface ReconnectMessage {
  type: 'reconnect';
}

export interface CloseMessage {
  type: 'close';
}

export type MainToWorkerMessage =
  | InitMessage
  | MouseMoveMessage
  | MouseButtonMessage
  | MouseWheelMessage
  | KeyboardMessage
  | TouchMessage
  | SetVideoEnabledMessage
  | SetAudioEnabledMessage
  | SetBitrateMessage
  | SetThrottleRatioMessage
  | ReconnectMessage
  | CloseMessage;

// ============================================================================
// Worker → Main Thread Messages
// ============================================================================

export interface ConnectedMessage {
  type: 'connected';
  width: number;
  height: number;
  fps: number;
}

export interface DisconnectedMessage {
  type: 'disconnected';
  reason: string;
  willReconnect: boolean;
}

export interface ErrorMessage {
  type: 'error';
  message: string;
  fatal: boolean;
}

export interface StatsMessage {
  type: 'stats';
  fps: number;
  framesDecoded: number;
  framesDropped: number;
  rttMs: number;
  encoderLatencyMs: number;
  isHighLatency: boolean;
  frameLatencyMs: number;
  adaptiveThrottleRatio: number;
  effectiveInputFps: number;
  isThrottled: boolean;
  decodeQueueSize: number;
  maxDecodeQueueSize: number;
  videoPayloadBitrateMbps: number;
  totalBitrateMbps: number;
  width: number;
  height: number;
  codecString: string;
  // Jitter stats
  receiveJitterMs: string;
  renderJitterMs: string;
  avgReceiveIntervalMs: number;
  avgRenderIntervalMs: number;
}

export interface CursorImageMessage {
  type: 'cursorImage';
  cursorId: number;
  hotspotX: number;
  hotspotY: number;
  width: number;
  height: number;
  imageData: ImageData; // Transferable
}

export interface CursorNameMessage {
  type: 'cursorName';
  name: string;
}

export interface RemoteCursorMessage {
  type: 'remoteCursor';
  clientId: number;
  x: number;
  y: number;
}

export interface RemoteUserMessage {
  type: 'remoteUser';
  clientId: number;
  action: 'joined' | 'left';
  name?: string;
  color?: string;
}

export interface SelfIdMessage {
  type: 'selfId';
  clientId: number;
}

export interface AgentCursorMessage {
  type: 'agentCursor';
  x: number;
  y: number;
  action: string;
  visible: boolean;
}

export interface VideoStartedMessage {
  type: 'videoStarted';
}

export interface ReconnectingMessage {
  type: 'reconnecting';
  attempt: number;
  maxAttempts: number;
  delayMs: number;
}

export type WorkerToMainMessage =
  | ConnectedMessage
  | DisconnectedMessage
  | ErrorMessage
  | StatsMessage
  | CursorImageMessage
  | CursorNameMessage
  | RemoteCursorMessage
  | RemoteUserMessage
  | SelfIdMessage
  | AgentCursorMessage
  | VideoStartedMessage
  | ReconnectingMessage;
