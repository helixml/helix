/**
 * WebSocket Stream Types
 *
 * Type definitions for the WebSocket streaming protocol.
 */

import { StreamCapabilities } from "../api_bindings"

// ============================================================================
// Binary Protocol Types (matching Rust ws_protocol.rs)
// ============================================================================

export const WsMessageType = {
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
  MicAudio: 0x17,
  ControlMessage: 0x20,
  StreamInit: 0x30,
  StreamError: 0x31,
  Ping: 0x40,
  Pong: 0x41,
  // Cursor message types (server → client)
  CursorImage: 0x50,      // Cursor image data when cursor changes
  CursorName: 0x51,       // CSS cursor name for fallback rendering
  // Multi-user cursor message types (server → all clients)
  RemoteCursor: 0x53,     // Remote user cursor position
  RemoteUser: 0x54,       // Remote user joined/left
  AgentCursor: 0x55,      // AI agent cursor position/action
  RemoteTouch: 0x56,      // Remote user touch event
  SelfId: 0x58,           // Server tells client their own clientId
} as const

// ============================================================================
// Event Types
// ============================================================================

// Cursor image data from server
export interface CursorImageData {
  cursorId: number
  hotspotX: number
  hotspotY: number
  width: number
  height: number
  imageUrl: string  // data URL or blob URL for the cursor image
  cursorName?: string  // CSS cursor name for fallback rendering when pixels unavailable
}

// Remote user info for multi-player cursors
export interface RemoteUserInfo {
  userId: number
  userName: string
  color: string      // Hex color assigned to this user
  avatarUrl?: string // User's avatar URL if available
}

// Remote cursor position
export interface RemoteCursorPosition {
  userId: number
  x: number
  y: number
  color?: string
  lastSeen: number  // Timestamp for idle detection
  cursorImage?: CursorImageData  // Cursor shape for this remote user
}

// AI agent cursor info
export interface AgentCursorInfo {
  agentId: number
  x: number
  y: number
  action: 'idle' | 'moving' | 'clicking' | 'typing' | 'scrolling' | 'dragging'
  visible: boolean
  lastSeen: number  // Timestamp for idle detection
}

// Remote touch event
export interface RemoteTouchInfo {
  userId: number
  touchId: number
  eventType: 'start' | 'move' | 'end' | 'cancel'
  x: number
  y: number
  pressure: number
  color?: string  // User's assigned color for touch indicator
}

export type WsStreamInfoEvent = CustomEvent<
  | { type: "error"; message: string }
  | { type: "connecting" }
  | { type: "connected" }
  | { type: "disconnected" }
  | { type: "reconnecting"; attempt: number }
  | { type: "streamInit"; width: number; height: number; fps: number }
  | { type: "connectionComplete"; capabilities: StreamCapabilities }
  | { type: "addDebugLine"; line: string }
  // Cursor events
  | { type: "cursorImage"; cursor: CursorImageData; lastMoverID?: number }
  | { type: "cursorName"; cursorName: string; hotspotX: number; hotspotY: number; lastMoverID?: number }
  | { type: "cursorPosition"; x: number; y: number; hotspotX: number; hotspotY: number }
  // Multi-player cursor events
  | { type: "remoteCursor"; cursor: RemoteCursorPosition }
  | { type: "remoteUserJoined"; user: RemoteUserInfo }
  | { type: "remoteUserLeft"; userId: number }
  | { type: "agentCursor"; agent: AgentCursorInfo }
  | { type: "remoteTouch"; touch: RemoteTouchInfo }
  | { type: "selfId"; clientId: number }
>
export type WsStreamInfoEventListener = (event: WsStreamInfoEvent) => void
