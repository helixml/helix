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
  videoSize: '720p' | '1080p' | '1440p' | '4k' | '5k' | 'iphone15pro' | 'ipadair11' | 'macbook13' | 'native' | 'custom';
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
// Scale ~4 bits/pixel for good quality H.264:
//   1080p (2M px): 8 Mbps
//   1440p (3.7M px): 15 Mbps
//   4K (8.3M px): 30 Mbps
//   5K (14.7M px): 50 Mbps
export const getDefaultBitrateForResolution = (width: number, height: number): number => {
  const pixels = width * height;
  // ~4 bits/pixel, converted to kbps (divide by 1000)
  const bitrate = Math.round(pixels * 4 / 1000);
  // Clamp to reasonable range: 5 Mbps minimum, 80 Mbps maximum
  return Math.max(5000, Math.min(80000, bitrate));
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
