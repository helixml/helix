/**
 * Types for Desktop Stream Viewer components
 */

export interface DesktopStreamViewerProps {
  sessionId: string;
  sandboxId?: string;
  hostId?: number;
  appId?: number;
  onConnectionChange?: (isConnected: boolean) => void;
  onError?: (error: string) => void;
  onClientIdCalculated?: (clientId: string) => void;
  width?: number;
  height?: number;
  fps?: number;
  className?: string;
  // When true, suppress the connection overlay (parent component is showing its own overlay)
  suppressOverlay?: boolean;
}

export interface CursorPosition {
  x: number;
  y: number;
}

export interface CursorImageData {
  imageUrl: string;
  width: number;
  height: number;
  hotspotX: number;
  hotspotY: number;
}

export interface RemoteUserInfo {
  clientId: number;
  name: string;
  color: string;
}

export interface RemoteCursorPosition {
  clientId: number;
  x: number;
  y: number;
  visible: boolean;
}

export interface AgentCursorInfo {
  x: number;
  y: number;
  visible: boolean;
}

export interface RemoteTouchInfo {
  clientId: number;
  touchId: number;
  x: number;
  y: number;
}

export interface VideoStats {
  fps?: number;
  width?: number;
  height?: number;
  codec?: string;
  framesDecoded?: number;
  framesDropped?: number;
  effectiveInputFps?: number;
  throughputMbps?: number;
  rttMs?: number;
  frameDrift?: number;
}

export interface StreamStats {
  video?: VideoStats;
  audio?: {
    enabled: boolean;
    codec?: string;
  };
  connection?: {
    state: string;
    bytesReceived?: number;
    bytesSent?: number;
  };
}

export interface ClipboardToast {
  message: string;
  type: 'success' | 'error';
  visible: boolean;
}

// Quality mode: video streaming or screenshot polling
export type QualityMode = 'video' | 'screenshot';

// Connection registry for tracking active streams
export interface ActiveConnection {
  id: string;
  type: 'websocket-stream' | 'websocket-video-enabled' | 'screenshot-polling';
  createdAt: number;
}

// Chart history for bandwidth visualization
export interface ChartEvent {
  index: number;
  type: 'reduce' | 'increase' | 'reconnect' | 'rtt_spike' | 'saturation';
  reason: string;
}

// Bitrate recommendation
export interface BitrateRecommendation {
  type: 'decrease' | 'increase' | 'screenshot';
  targetBitrate: number;
  reason: string;
  frameDrift?: number;
  measuredThroughput?: number;
}
