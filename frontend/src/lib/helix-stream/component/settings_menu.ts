// Stream transport mode
// - 'websocket': WebSocket for both video and input (L7-only, works everywhere)
// - 'webrtc': WebRTC for video/audio, WebSocket for signaling (requires TURN)
// Note: SSE video is now controlled by qualityMode='sse', not streamingMode
export type StreamingMode = 'websocket' | 'webrtc';

// Video capture mode for the backend
// - 'native': Stock GStreamer pipewiresrc (simpler, potentially lower latency)
// - 'zerocopy': Custom pipewirezerocopysrc plugin (zero-copy GPU path)
// - 'shm': Shared memory mode (fallback)
export type VideoMode = 'native' | 'zerocopy' | 'shm';

// Stream settings interface
export interface StreamSettings {
  videoSize: '720p' | '1080p' | '1440p' | '4k' | '5k' | 'native' | 'custom';
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
  // Video capture mode: 'native' (stock pipewiresrc) or 'zerocopy' (custom zero-copy plugin)
  videoMode?: VideoMode;
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
