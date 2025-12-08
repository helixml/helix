// Stream transport mode
export type StreamingMode = 'websocket' | 'webrtc';

// Stream settings interface
export interface StreamSettings {
  videoSize: '720p' | '1080p' | '1440p' | '4k' | 'native' | 'custom';
  videoSizeCustom: { width: number; height: number };
  bitrate: number;
  packetSize: number;
  fps: number;
  videoSampleQueueSize: number;
  audioSampleQueueSize: number;
  playAudioLocal: boolean;
  mouseScrollMode: string;
  controllerConfig: any;
  // Transport mode: 'websocket' (L7-only, works everywhere) or 'webrtc' (requires TURN)
  streamingMode: StreamingMode;
}

export const defaultStreamSettings = (): StreamSettings => ({
  videoSize: '1080p',
  videoSizeCustom: { width: 1920, height: 1080 },
  bitrate: 20000,
  packetSize: 1024,
  fps: 60,
  videoSampleQueueSize: 50,  // Increased for 4K60 (was 5, too small for high bitrate)
  audioSampleQueueSize: 10,
  playAudioLocal: false,
  mouseScrollMode: 'normal',
  controllerConfig: {},
  streamingMode: 'websocket',  // Default to WebSocket-only (works through L7 ingress)
});
