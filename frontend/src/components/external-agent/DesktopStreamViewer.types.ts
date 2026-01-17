/**
 * Types for DesktopStreamViewer and related components
 */

export interface DesktopStreamViewerProps {
  sessionId: string;
  sandboxId?: string; // Sandbox ID for streaming connection
  hostId?: number;
  appId?: number;
  onConnectionChange?: (isConnected: boolean) => void;
  onError?: (error: string) => void;
  onClientIdCalculated?: (clientId: string) => void; // Callback when client unique ID is calculated
  width?: number;
  height?: number;
  fps?: number;
  className?: string;
  // When true, suppress the connection overlay (parent component is showing its own overlay)
  // This prevents multiple spinners stacking when container state changes
  suppressOverlay?: boolean;
}

// Stats for video streaming
export interface VideoStats {
  codec?: string;
  width?: number;
  height?: number;
  fps?: number;
  videoPayloadBitrate?: string;
  totalBitrate?: string;
  framesDecoded?: number;
  framesDropped?: number;
  rttMs?: number;
  encoderLatencyMs?: number;
  isHighLatency?: boolean;
  batchingRatio?: number;
  avgBatchSize?: number;
  batchesReceived?: number;
  frameLatencyMs?: number;
  adaptiveThrottleRatio?: number;
  effectiveInputFps?: number;
  isThrottled?: boolean;
  decodeQueueSize?: number;
  maxDecodeQueueSize?: number;
  framesSkippedToKeyframe?: number;
}

// Stats for input handling
export interface InputStats {
  bufferBytes?: number;
  maxBufferBytes?: number;
  avgBufferBytes?: number;
  inputsSent?: number;
  inputsDropped?: number;
  congested?: boolean;
  lastSendMs?: number;
  maxSendMs?: number;
  avgSendMs?: number;
  bufferBeforeSend?: number;
  bufferAfterSend?: number;
  bufferStaleMs?: number;
  eventLoopLatencyMs?: number;
  maxEventLoopLatencyMs?: number;
  avgEventLoopLatencyMs?: number;
}

// Combined stats object
export interface StreamStats {
  video?: VideoStats;
  input?: InputStats;
  connection?: {
    transport?: string;
  };
  timestamp?: string;
}

// Active connection info for display
export interface ActiveConnection {
  id: string;
  type: 'websocket-stream' | 'websocket-video-enabled' | 'screenshot-polling';
  createdAt: number;
}

// Quality mode for streaming
export type QualityMode = 'video' | 'screenshot';
