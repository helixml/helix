// Stream transport mode - WebSocket only (WebRTC removed)
// WebSocket handles both video and input (L7-only, works everywhere)
export type StreamingMode = 'websocket';

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
  // Transport mode: WebSocket only (WebRTC removed)
  streamingMode: StreamingMode;
  // Video capture mode: 'native' (stock pipewiresrc) or 'zerocopy' (custom zero-copy plugin)
  videoMode?: VideoMode;
}

// Get default bitrate (in kbps) based on resolution
// 4K (3840x2160 or higher): 10 Mbps
// 1080p and below: 5 Mbps
export const getDefaultBitrateForResolution = (width: number, height: number): number => {
  // 4K is 3840x2160, so check if either dimension exceeds 1080p thresholds
  if (width >= 3840 || height >= 2160) {
    return 10000; // 10 Mbps for 4K
  }
  return 5000; // 5 Mbps for 1080p and below
};

export const defaultStreamSettings = (): StreamSettings => ({
  videoSize: '1080p',
  videoSizeCustom: { width: 1920, height: 1080 },
  bitrate: 5000, // 5 Mbps for 1080p default - use getDefaultBitrateForResolution() for resolution-aware default
  packetSize: 1024,
  fps: 60,
  videoSampleQueueSize: 50,  // Increased for 4K60 (was 5, too small for high bitrate)
  audioSampleQueueSize: 10,
  playAudioLocal: false,
  mouseScrollMode: 'normal',
  controllerConfig: {},
  streamingMode: 'websocket',  // Default to WebSocket-only (works through L7 ingress)
});
