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
}

export const defaultStreamSettings = (): StreamSettings => ({
  videoSize: '1080p',
  videoSizeCustom: { width: 1920, height: 1080 },
  bitrate: 20000,
  packetSize: 1024,
  fps: 60,
  videoSampleQueueSize: 5,
  audioSampleQueueSize: 5,
  playAudioLocal: false,
  mouseScrollMode: 'normal',
  controllerConfig: {}
});
