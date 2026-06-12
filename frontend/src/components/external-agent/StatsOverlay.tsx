/**
 * StatsOverlay - "Stats for Nerds" overlay for streaming diagnostics
 */

import React, { useState } from 'react';
import { Box, Typography } from '@mui/material';
import { StreamStats, ActiveConnection, QualityMode } from './DesktopStreamViewer.types';

// An inter-frame interval below this (ms) is physically impossible as a freshly
// rendered 60Hz frame — it means a queue piled up and then drained (a "burst").
const BURST_THRESHOLD_MS = 8;

const countBursts = (samples?: number[]): number =>
  samples ? samples.reduce((n, s) => (s < BURST_THRESHOLD_MS ? n + 1 : n), 0) : 0;

// Inline SVG sparkline of recent inter-frame intervals. Y auto-scales to the
// window max so a brief spike stands out next to the 16ms baseline. The 60fps
// reference (16.67ms) is a faint dashed line; sub-8ms "burst" samples are
// overdrawn as red dots so pileup-drains are visible at a glance.
const Sparkline: React.FC<{
  samples: number[];
  width?: number;
  height?: number;
  color?: string;
}> = ({ samples, width = 120, height = 22, color = '#00ff00' }) => {
  if (samples.length < 2) {
    return <svg width={width} height={height} />;
  }
  const max = Math.max(...samples, 16.67);
  const range = Math.max(max, 1);
  const step = width / Math.max(samples.length - 1, 1);
  const x = (i: number) => i * step;
  const y = (v: number) => height - (v / range) * height;
  const points = samples.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ');
  const refY = height - (16.67 / range) * height;
  return (
    <svg
      width={width}
      height={height}
      style={{ verticalAlign: 'middle', marginLeft: 6 }}
      aria-label={`${samples.length} samples, max ${Math.round(max)}ms`}
    >
      <line x1={0} y1={refY} x2={width} y2={refY} stroke="rgba(255,255,255,0.15)" strokeDasharray="2,2" strokeWidth={1} />
      <polyline points={points} fill="none" stroke={color} strokeWidth={1} opacity={0.9} />
      {samples.map((v, i) =>
        v < BURST_THRESHOLD_MS ? (
          <circle key={i} cx={x(i)} cy={y(v)} r={1.6} fill="#ff4d4d" />
        ) : null,
      )}
    </svg>
  );
};

// Build the plain-text representation written to clipboard by the Copy button,
// so stats can be pasted into chat without screenshotting. Mirrors the panel,
// plus burst counts and the raw sample arrays for offline plotting.
function buildStatsClipboardText(
  stats: StreamStats | null,
  qualityMode: QualityMode,
  activeConnections: ActiveConnection[],
  requestedBitrate: number,
  screenshotFps: number,
  screenshotQuality: number,
  shouldPollScreenshots: boolean,
): string {
  const lines: string[] = [];
  lines.push('Stats for Nerds');
  lines.push('Transport: WebSocket');
  lines.push(
    `Active: ${activeConnections.length === 0 ? 'none' : activeConnections.map((c) => c.type.replace(/-/g, ' ')).join(', ')}`,
  );
  const v = stats?.video;
  if (v?.codec) {
    lines.push(`Codec: ${v.codec}${v.usingSoftwareDecoder ? ' (SW decode)' : ' (HW decode)'}`);
    lines.push(`Resolution: ${v.width}x${v.height}`);
    lines.push(`FPS: ${v.receiveFps} recv / ${v.fps} decoded`);
    lines.push(`Bitrate: ${v.totalBitrate} Mbps (req: ${requestedBitrate})`);
    lines.push(`Received: ${v.framesReceived} frames`);
    lines.push(`Decoded: ${v.framesDecoded} frames`);
    lines.push(`Dropped: ${v.framesDropped} frames`);
    if (v.rttMs !== undefined) {
      const parts = [`RTT: ${v.rttMs.toFixed(0)} ms`];
      if (v.encoderLatencyMs !== undefined && v.encoderLatencyMs > 0) {
        parts.push(`Encoder: ${v.encoderLatencyMs.toFixed(0)} ms`);
        parts.push(`Total: ${(v.encoderLatencyMs + v.rttMs).toFixed(0)} ms`);
      }
      lines.push(parts.join(' | '));
    }
    if (v.adaptiveThrottleRatio !== undefined) {
      lines.push(`Input Throttle: ${(v.adaptiveThrottleRatio * 100).toFixed(0)}% (${v.effectiveInputFps?.toFixed(0) || 0} Hz)`);
    }
    if (qualityMode !== 'screenshot' && v.frameLatencyMs !== undefined) {
      lines.push(`Frame Drift: ${v.frameLatencyMs > 0 ? '+' : ''}${v.frameLatencyMs.toFixed(0)} ms`);
    }
    if (v.decodeQueueSize !== undefined) {
      lines.push(`Decode Queue: ${v.decodeQueueSize}${(v.maxDecodeQueueSize ?? 0) > 3 ? ` (peak: ${v.maxDecodeQueueSize})` : ''}`);
    }
    if (v.framesSkippedToKeyframe !== undefined) {
      lines.push(`Skipped to KF: ${v.framesSkippedToKeyframe} frames`);
    }
    if (v.receiveJitterMs) {
      lines.push(
        `Receive Jitter: ${v.receiveJitterMs} ms (avg ${v.avgReceiveIntervalMs ?? 0} ms, burst<8ms ${countBursts(v.receiveIntervalSamples)})`,
      );
    }
    if (v.renderJitterMs) {
      lines.push(
        `Render Jitter: ${v.renderJitterMs} ms (avg ${v.avgRenderIntervalMs ?? 0} ms, burst<8ms ${countBursts(v.renderIntervalSamples)})`,
      );
    }
    if (
      v.schedulerJitterMaxMs !== undefined ||
      v.schedulerJitterP99Ms !== undefined ||
      v.schedulerJitterP50Ms !== undefined
    ) {
      lines.push(
        `Scheduler Jitter: ${v.schedulerJitterP50Ms ?? 0}/${v.schedulerJitterP99Ms ?? 0}/${v.schedulerJitterMaxMs ?? 0} ms (p50/p99/max)`,
      );
    }
  }
  if (stats?.input) {
    const i = stats.input;
    lines.push('--- Input ---');
    lines.push(`Send Buffer: ${i.bufferBytes} bytes`);
    lines.push(`Sent: ${i.inputsSent}${(i.inputsDropped ?? 0) > 0 ? ` (skipped: ${i.inputsDropped})` : ''}`);
    if ((i.maxSendMs ?? 0) > 1) {
      lines.push(`Send Latency: ${i.avgSendMs?.toFixed(2)}ms (peak: ${i.maxSendMs?.toFixed(1)}ms)`);
    }
    lines.push(`Event Loop: ${i.avgEventLoopLatencyMs?.toFixed(1) || 0}ms${(i.maxEventLoopLatencyMs ?? 0) > 10 ? ` (peak: ${i.maxEventLoopLatencyMs?.toFixed(0)}ms)` : ''}`);
  }
  if (shouldPollScreenshots) {
    lines.push('--- Screenshot Mode ---');
    lines.push(`FPS: ${screenshotFps}`);
    lines.push(`JPEG Quality: ${screenshotQuality}%`);
  }
  if (v?.receiveIntervalSamples && v.receiveIntervalSamples.length > 0) {
    lines.push(`Receive samples (ms, oldest→newest, n=${v.receiveIntervalSamples.length}): ${v.receiveIntervalSamples.map((s) => Math.round(s)).join(',')}`);
  }
  if (v?.renderIntervalSamples && v.renderIntervalSamples.length > 0) {
    lines.push(`Render samples (ms, oldest→newest, n=${v.renderIntervalSamples.length}): ${v.renderIntervalSamples.map((s) => Math.round(s)).join(',')}`);
  }
  return lines.join('\n');
}

export interface ConnectionLogEntry {
  time: string;
  msg: string;
}

// Two-finger gesture debug info for trackpad mode troubleshooting
export interface TwoFingerDebugInfo {
  gestureType: "undecided" | "pinch" | "scroll";
  distanceChange: number;
  centerMovement: number;
  lastScrollDelta: { dx: number; dy: number };
}

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
  // Connection log for debugging
  connectionLog: ConnectionLogEntry[];
  isConnected: boolean;
  isConnecting: boolean;
  // Two-finger gesture debug info for trackpad mode
  twoFingerDebug?: TwoFingerDebugInfo | null;
}

type TabType = 'stats' | 'log';

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
  connectionLog,
  isConnected,
  isConnecting,
  twoFingerDebug,
}) => {
  const [activeTab, setActiveTab] = useState<TabType>('stats');
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>('idle');

  const handleCopy = async () => {
    const text = buildStatsClipboardText(
      stats,
      qualityMode,
      activeConnections,
      requestedBitrate,
      screenshotFps,
      screenshotQuality,
      shouldPollScreenshots,
    );
    try {
      await navigator.clipboard.writeText(text);
      setCopyState('copied');
    } catch {
      // Clipboard API is unavailable on insecure (http) origins; fall back to a
      // hidden textarea + execCommand so localhost still works.
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try {
        document.execCommand('copy');
        setCopyState('copied');
      } catch {
        setCopyState('failed');
      }
      document.body.removeChild(ta);
    }
    setTimeout(() => setCopyState('idle'), 1500);
  };

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
        maxWidth: 400,
        border: '1px solid rgba(0, 255, 0, 0.3)',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Typography variant="caption" sx={{ fontWeight: 'bold', color: '#00ff00' }}>
          Stats for Nerds
        </Typography>
        <button
          onClick={handleCopy}
          title="Copy stats to clipboard as plain text"
          style={{
            padding: '2px 8px',
            fontSize: '10px',
            background: copyState === 'copied' ? 'rgba(76, 175, 80, 0.25)' : 'rgba(0, 255, 0, 0.1)',
            border: copyState === 'copied' ? '1px solid #4caf50' : '1px solid rgba(0, 255, 0, 0.4)',
            borderRadius: 3,
            color: copyState === 'copied' ? '#4caf50' : '#00ff00',
            cursor: 'pointer',
            fontFamily: 'monospace',
          }}
        >
          {copyState === 'copied' ? '✓ Copied' : copyState === 'failed' ? 'Copy failed' : 'Copy'}
        </button>
      </Box>

      {/* Tab buttons */}
      <Box sx={{ display: 'flex', gap: 1, mb: 1.5, borderBottom: '1px solid rgba(0, 255, 0, 0.3)', pb: 1 }}>
        <button
          onClick={() => setActiveTab('stats')}
          style={{
            padding: '4px 12px',
            fontSize: '11px',
            background: activeTab === 'stats' ? 'rgba(0, 255, 0, 0.2)' : 'transparent',
            border: activeTab === 'stats' ? '1px solid #00ff00' : '1px solid rgba(255,255,255,0.2)',
            borderRadius: 3,
            color: activeTab === 'stats' ? '#00ff00' : '#888',
            cursor: 'pointer',
          }}
        >
          Stats
        </button>
        <button
          onClick={() => setActiveTab('log')}
          style={{
            padding: '4px 12px',
            fontSize: '11px',
            background: activeTab === 'log' ? 'rgba(0, 255, 0, 0.2)' : 'transparent',
            border: activeTab === 'log' ? '1px solid #00ff00' : '1px solid rgba(255,255,255,0.2)',
            borderRadius: 3,
            color: activeTab === 'log' ? '#00ff00' : '#888',
            cursor: 'pointer',
          }}
        >
          Log ({connectionLog.length})
        </button>
      </Box>

      {/* Stats Tab */}
      {activeTab === 'stats' && (
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
              {/* Frame jitter - shows timing variance. burst = intervals <8ms
                  (pileup drains), tinted red on the sparkline. */}
              {stats.video.receiveJitterMs && (
                <div>
                  <strong>Receive Jitter:</strong> {stats.video.receiveJitterMs} ms
                  {' '}(avg {stats.video.avgReceiveIntervalMs ?? 0}ms)
                  {(stats.video.avgReceiveIntervalMs ?? 0) > 0 && (stats.video.avgReceiveIntervalMs ?? 0) < 20 && (
                    <span style={{ color: '#4caf50' }}> {Math.round(1000 / (stats.video.avgReceiveIntervalMs ?? 16.7))}fps</span>
                  )}
                  {countBursts(stats.video.receiveIntervalSamples) > 0 && (
                    <span style={{ color: '#ff4d4d' }}> burst {countBursts(stats.video.receiveIntervalSamples)}</span>
                  )}
                  {stats.video.receiveIntervalSamples && stats.video.receiveIntervalSamples.length > 1 && (
                    <Sparkline samples={stats.video.receiveIntervalSamples} color="#00ff00" />
                  )}
                </div>
              )}
              {stats.video.renderJitterMs && (
                <div>
                  <strong>Render Jitter:</strong> {stats.video.renderJitterMs} ms
                  {' '}(avg {stats.video.avgRenderIntervalMs ?? 0}ms)
                  {countBursts(stats.video.renderIntervalSamples) > 0 && (
                    <span style={{ color: '#ff4d4d' }}> burst {countBursts(stats.video.renderIntervalSamples)}</span>
                  )}
                  {stats.video.renderIntervalSamples && stats.video.renderIntervalSamples.length > 1 && (
                    <Sparkline samples={stats.video.renderIntervalSamples} color="#4fc3f7" />
                  )}
                </div>
              )}
              {/* Adaptive playout buffer depth. Grows when idle to absorb
                  network/WiFi jitter; 0 either because we're interacting (kept
                  low for latency) or because there's no jitter worth buffering. */}
              {stats.video.playoutBufferMs !== undefined && (
                <div>
                  <strong>Playout Buffer:</strong> {stats.video.playoutBufferMs} ms
                  {stats.video.playoutBufferMs > 0 ? (
                    <span style={{ color: '#888' }}> (smoothing)</span>
                  ) : stats.video.playoutState === 'interactive' ? (
                    <span style={{ color: '#4caf50' }}> (interactive)</span>
                  ) : (
                    <span style={{ color: '#888' }}> (no jitter detected)</span>
                  )}
                </div>
              )}
              {/* Scheduler jitter — synthetic 60Hz canary in desktop-bridge.
                  High p99/max ms = the desktop container's kernel scheduler is
                  preempting userspace tasks (CPU contention from co-tenant work). */}
              {(stats.video.schedulerJitterMaxMs !== undefined ||
                stats.video.schedulerJitterP99Ms !== undefined ||
                stats.video.schedulerJitterP50Ms !== undefined) && (
                <div>
                  <strong>Scheduler Jitter:</strong>{' '}
                  {stats.video.schedulerJitterP50Ms ?? 0}/
                  {stats.video.schedulerJitterP99Ms ?? 0}/
                  {stats.video.schedulerJitterMaxMs ?? 0} ms
                  <span style={{ color: '#888' }}> (p50/p99/max)</span>
                  {(stats.video.schedulerJitterMaxMs ?? 0) > 50 && (
                    <span style={{ color: '#ff9800' }}> Contended</span>
                  )}
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
          {/* Two-finger gesture debug info for trackpad mode */}
          {twoFingerDebug && (
            <div style={{ marginTop: 8, borderTop: '1px solid rgba(0, 255, 0, 0.3)', paddingTop: 8 }}>
              <strong style={{ color: '#4fc3f7' }}>Two-Finger Gesture</strong>
              <div>
                <strong>Type:</strong>{' '}
                <span style={{ 
                  color: twoFingerDebug.gestureType === 'scroll' ? '#4caf50' : 
                         twoFingerDebug.gestureType === 'pinch' ? '#ff9800' : '#888'
                }}>
                  {twoFingerDebug.gestureType}
                </span>
              </div>
              <div><strong>Dist Change:</strong> {twoFingerDebug.distanceChange}px</div>
              <div><strong>Center Move:</strong> {twoFingerDebug.centerMovement}px</div>
              <div>
                <strong>Last Scroll:</strong> dx={twoFingerDebug.lastScrollDelta.dx} dy={twoFingerDebug.lastScrollDelta.dy}
              </div>
            </div>
          )}
        </Box>
      )}

      {/* Connection Log Tab */}
      {activeTab === 'log' && (
        <Box sx={{ '& > div': { mb: 0.3, lineHeight: 1.5 } }}>
          {/* Connection status header */}
          <Box sx={{ mb: 1, pb: 1, borderBottom: '1px solid rgba(0, 255, 0, 0.3)' }}>
            <strong>Status:</strong>{' '}
            {isConnected ? (
              <span style={{ color: '#4caf50' }}>✓ Connected</span>
            ) : isConnecting ? (
              <span style={{ color: '#ff9800' }}>⏳ Connecting...</span>
            ) : (
              <span style={{ color: '#ff6b6b' }}>✗ Disconnected</span>
            )}
            {stats?.video && (
              <span style={{ color: '#888' }}>
                {' '}| {stats.video.fps ?? 0}fps | {stats.video.framesDecoded ?? 0} dec | {stats.video.decoderState || '?'}
              </span>
            )}
          </Box>

          {/* Log entries */}
          <Box sx={{ maxHeight: 300, overflow: 'auto' }}>
            {connectionLog.length === 0 ? (
              <Box sx={{ color: '#666', fontStyle: 'italic' }}>No events yet</Box>
            ) : (
              connectionLog.map((entry, i) => (
                <Box key={i} sx={{ opacity: i === connectionLog.length - 1 ? 1 : 0.7 }}>
                  <span style={{ color: '#888' }}>{entry.time}</span> {entry.msg}
                </Box>
              ))
            )}
          </Box>
        </Box>
      )}
    </Box>
  );
};

export default StatsOverlay;
