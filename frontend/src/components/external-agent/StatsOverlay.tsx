/**
 * StatsOverlay - "Stats for Nerds" overlay for streaming diagnostics
 */

import React from 'react';
import { Box, Typography } from '@mui/material';
import { StreamStats, ActiveConnection, QualityMode } from './DesktopStreamViewer.types';

export interface StatsOverlayProps {
  stats: StreamStats | null;
  qualityMode: QualityMode;
  activeConnections: ActiveConnection[];
  requestedBitrate: number;
  debugThrottleRatio: number | null;
  onDebugThrottleRatioChange: (ratio: number | null) => void;
  shouldPollScreenshots: boolean;
  screenshotFps: number;
  screenshotQuality: number;
  // Keyboard debug event for iPad troubleshooting
  debugKeyEvent?: string | null;
}

const StatsOverlay: React.FC<StatsOverlayProps> = ({
  stats,
  qualityMode,
  activeConnections,
  requestedBitrate,
  debugThrottleRatio,
  onDebugThrottleRatioChange,
  shouldPollScreenshots,
  screenshotFps,
  screenshotQuality,
  debugKeyEvent,
}) => {
  return (
    <Box
      sx={{
        position: 'absolute',
        top: 60,
        right: 10,
        backgroundColor: 'rgba(0, 0, 0, 0.9)',
        color: '#00ff00',
        padding: 2,
        borderRadius: 1,
        fontFamily: 'monospace',
        fontSize: '0.75rem',
        zIndex: 1500,
        minWidth: 300,
        border: '1px solid rgba(0, 255, 0, 0.3)',
      }}
    >
      <Typography variant="caption" sx={{ fontWeight: 'bold', display: 'block', mb: 1, color: '#00ff00' }}>
        Stats for Nerds
      </Typography>

      <Box sx={{ '& > div': { mb: 0.3, lineHeight: 1.5 } }}>
        <div><strong>Transport:</strong> WebSocket</div>
        {/* Active Connections Registry */}
        <div>
          <strong>Active:</strong>{' '}
          {activeConnections.length === 0 ? (
            <span style={{ color: '#888' }}>none</span>
          ) : (
            activeConnections.map((c, i) => (
              <span key={c.id}>
                {i > 0 && ', '}
                <span style={{
                  color: activeConnections.length > 2 ? '#ff6b6b' : '#00ff00'
                }}>
                  {c.type.replace(/-/g, ' ')}
                </span>
              </span>
            ))
          )}
          {activeConnections.length > 2 && (
            <span style={{ color: '#ff6b6b' }}> TOO MANY!</span>
          )}
        </div>
        {stats?.video?.codec && (
          <>
            <div>
              <strong>Codec:</strong> {stats.video.codec}
              {stats.video.usingSoftwareDecoder ? (
                <span style={{ color: '#ff9800' }}> (SW decode - ?softdecode=1)</span>
              ) : (
                <span style={{ color: '#4caf50' }}> (HW decode)</span>
              )}
            </div>
            <div><strong>Resolution:</strong> {stats.video.width}x{stats.video.height}</div>
            <div>
              <strong>FPS:</strong> {stats.video.receiveFps} recv / {stats.video.fps} decoded
              {stats.video.fpsUpdatedAt && (
                <span style={{ color: '#888' }}>
                  {' '}({((Date.now() - stats.video.fpsUpdatedAt) / 1000).toFixed(1)}s ago)
                </span>
              )}
            </div>
            <div><strong>Bitrate:</strong> {stats.video.totalBitrate} Mbps <span style={{ color: '#888' }}>req: {requestedBitrate}</span></div>
            <div><strong>Received:</strong> {stats.video.framesReceived} frames</div>
            <div><strong>Decoded:</strong> {stats.video.framesDecoded} frames</div>
            <div>
              <strong>Dropped:</strong> {stats.video.framesDropped} frames
              {(stats.video.framesDropped ?? 0) > 0 && <span style={{ color: '#ff6b6b' }}> Warning</span>}
            </div>
            {/* Latency metrics */}
            {stats.video.rttMs !== undefined && (
              <div>
                <strong>RTT:</strong> {stats.video.rttMs.toFixed(0)} ms
                {stats.video.encoderLatencyMs !== undefined && stats.video.encoderLatencyMs > 0 && (
                  <>
                    <span style={{ color: '#888' }}> | Encoder: {stats.video.encoderLatencyMs.toFixed(0)} ms</span>
                    <span style={{ color: '#888' }}> | Total: {(stats.video.encoderLatencyMs + stats.video.rttMs).toFixed(0)} ms</span>
                  </>
                )}
                {stats.video.isHighLatency && <span style={{ color: '#ff9800' }}> Warning</span>}
              </div>
            )}
            {/* Adaptive input throttling */}
            {stats.video.adaptiveThrottleRatio !== undefined && (
              <div>
                <strong>Input Throttle:</strong> {(stats.video.adaptiveThrottleRatio * 100).toFixed(0)}%
                {' '}({stats.video.effectiveInputFps?.toFixed(0) || 0} Hz)
                {stats.video.isThrottled && <span style={{ color: '#ff9800' }}> Reduced due to latency</span>}
              </div>
            )}
            {/* Frame latency */}
            {qualityMode !== 'screenshot' && stats.video.frameLatencyMs !== undefined && (
              <div>
                <strong>Frame Drift:</strong> {stats.video.frameLatencyMs > 0 ? '+' : ''}{stats.video.frameLatencyMs.toFixed(0)} ms
                {stats.video.frameLatencyMs > 200 && <span style={{ color: '#ff6b6b' }}> Behind</span>}
                {stats.video.frameLatencyMs < -500 && <span style={{ color: '#4caf50' }}> (buffered)</span>}
              </div>
            )}
            {/* Decoder queue */}
            {stats.video.decodeQueueSize !== undefined && (
              <div>
                <strong>Decode Queue:</strong> {stats.video.decodeQueueSize}
                {(stats.video.maxDecodeQueueSize ?? 0) > 3 && (
                  <span style={{ color: '#888' }}> (peak: {stats.video.maxDecodeQueueSize})</span>
                )}
                {stats.video.decodeQueueSize > 3 && <span style={{ color: '#ff6b6b' }}> Backed up</span>}
              </div>
            )}
            {/* Frames skipped to keyframe */}
            {stats.video.framesSkippedToKeyframe !== undefined && (
              <div>
                <strong>Skipped to KF:</strong> {stats.video.framesSkippedToKeyframe} frames
                {stats.video.framesSkippedToKeyframe > 0 && <span style={{ color: '#ff9800' }}> Skip</span>}
              </div>
            )}
            {/* Frame jitter - shows timing variance */}
            {stats.video.receiveJitterMs && (
              <div>
                <strong>Receive Jitter:</strong> {stats.video.receiveJitterMs} ms
                {' '}(avg {stats.video.avgReceiveIntervalMs ?? 0}ms)
                {(stats.video.avgReceiveIntervalMs ?? 0) > 0 && (stats.video.avgReceiveIntervalMs ?? 0) < 20 && (
                  <span style={{ color: '#4caf50' }}> {Math.round(1000 / (stats.video.avgReceiveIntervalMs ?? 16.7))}fps</span>
                )}
              </div>
            )}
            {stats.video.renderJitterMs && (
              <div>
                <strong>Render Jitter:</strong> {stats.video.renderJitterMs} ms
                {' '}(avg {stats.video.avgRenderIntervalMs ?? 0}ms)
              </div>
            )}
          </>
        )}
        {/* Input stats */}
        {stats?.input && (
          <div style={{ marginTop: 8, borderTop: '1px solid rgba(0, 255, 0, 0.3)', paddingTop: 8 }}>
            <strong style={{ color: '#00ff00' }}>Input</strong>
            <div>
              <strong>Send Buffer:</strong> {stats.input.bufferBytes} bytes
              {(stats.input.maxBufferBytes ?? 0) > 1000 && (
                <span style={{ color: '#888' }}> (peak: {((stats.input.maxBufferBytes ?? 0) / 1024).toFixed(1)}KB)</span>
              )}
              {stats.input.congested && (
                <span style={{ color: '#ff6b6b' }}> Stale {stats.input.bufferStaleMs?.toFixed(0)}ms</span>
              )}
            </div>
            <div>
              <strong>Sent:</strong> {stats.input.inputsSent}
              {(stats.input.inputsDropped ?? 0) > 0 && (
                <span style={{ color: '#ff9800' }}> (skipped: {stats.input.inputsDropped})</span>
              )}
            </div>
            {(stats.input.maxSendMs ?? 0) > 1 && (
              <div>
                <strong>Send Latency:</strong> {stats.input.avgSendMs?.toFixed(2)}ms
                <span style={{ color: '#888' }}> (peak: {stats.input.maxSendMs?.toFixed(1)}ms)</span>
                {(stats.input.maxSendMs ?? 0) > 5 && <span style={{ color: '#ff6b6b' }}> Blocking</span>}
              </div>
            )}
            <div>
              <strong>Event Loop:</strong> {stats.input.avgEventLoopLatencyMs?.toFixed(1) || 0}ms
              {(stats.input.maxEventLoopLatencyMs ?? 0) > 10 && (
                <span style={{ color: '#888' }}> (peak: {stats.input.maxEventLoopLatencyMs?.toFixed(0)}ms)</span>
              )}
              {(stats.input.maxEventLoopLatencyMs ?? 0) > 50 && <span style={{ color: '#ff6b6b' }}> Main thread blocked</span>}
            </div>
            {debugKeyEvent && (
              <div>
                <strong>Last Key:</strong> <span style={{ color: '#4fc3f7' }}>{debugKeyEvent}</span>
              </div>
            )}
          </div>
        )}
        {/* Keyboard debug when no input stats available */}
        {!stats?.input && debugKeyEvent && (
          <div style={{ marginTop: 8, borderTop: '1px solid rgba(0, 255, 0, 0.3)', paddingTop: 8 }}>
            <strong style={{ color: '#00ff00' }}>Input</strong>
            <div>
              <strong>Last Key:</strong> <span style={{ color: '#4fc3f7' }}>{debugKeyEvent}</span>
            </div>
          </div>
        )}
        {!stats?.video?.codec && !shouldPollScreenshots && <div>Waiting for video data...</div>}
        {/* Debug: Throttle ratio override */}
        <div style={{ marginTop: 8, borderTop: '1px solid rgba(0, 255, 0, 0.3)', paddingTop: 8 }}>
          <strong>Debug: Throttle Override</strong>
          <div style={{ marginTop: 4, display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {[null, 1.0, 0.75, 0.5, 0.33, 0.25].map((ratio) => (
              <button
                key={ratio === null ? 'auto' : ratio}
                onClick={() => onDebugThrottleRatioChange(ratio)}
                style={{
                  padding: '2px 6px',
                  fontSize: '10px',
                  background: debugThrottleRatio === ratio ? '#4caf50' : 'rgba(255,255,255,0.1)',
                  border: '1px solid rgba(255,255,255,0.3)',
                  borderRadius: 3,
                  color: 'white',
                  cursor: 'pointer',
                }}
              >
                {ratio === null ? 'Auto' : `${(ratio * 100).toFixed(0)}%`}
              </button>
            ))}
          </div>
        </div>
        {/* Screenshot mode stats */}
        {shouldPollScreenshots && (
          <>
            <div style={{ marginTop: 8, borderTop: '1px solid rgba(0, 255, 0, 0.3)', paddingTop: 8 }}>
              <strong style={{ color: '#ff9800' }}>Screenshot Mode</strong>
            </div>
            <div><strong>FPS:</strong> {screenshotFps} <span style={{ color: '#888' }}>target: 2+</span></div>
            <div>
              <strong>JPEG Quality:</strong> {screenshotQuality}%
              <span style={{ color: '#888' }}> (adaptive 10-90)</span>
            </div>
          </>
        )}
      </Box>
    </Box>
  );
};

export default StatsOverlay;
