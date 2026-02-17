/**
 * ChatStatsOverlay - "Stats for Nerds" overlay for chat/markdown rendering diagnostics
 *
 * Tracks and displays latency information for:
 * - MessageProcessor processing time
 * - React render timing
 * - Update frequency during streaming
 * - Content size metrics
 * - Throttle status
 */

import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Box, Typography, IconButton, Tooltip } from '@mui/material';
import BarChartIcon from '@mui/icons-material/BarChart';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';

// Stats data structure
export interface ChatStats {
  // MessageProcessor timing
  lastProcessTime: number; // ms
  avgProcessTime: number; // ms
  maxProcessTime: number; // ms
  processCount: number;

  // Render timing
  lastRenderTime: number; // ms
  avgRenderTime: number; // ms
  maxRenderTime: number; // ms
  renderCount: number;

  // Update frequency
  lastUpdateTime: number; // timestamp
  updateIntervalMs: number; // time between updates
  avgUpdateIntervalMs: number; // average time between updates
  updatesPerSecond: number; // calculated UPS

  // Content metrics
  contentLength: number; // characters
  codeBlockCount: number;
  lastContentDelta: number; // characters added since last update

  // Throttle status
  isThrottled: boolean;
  throttleMs: number;
  skippedUpdates: number; // updates skipped due to throttling

  // Streaming status
  isStreaming: boolean;
  streamStartTime: number | null;
  streamDurationMs: number;
}

// Create initial stats object
export const createInitialStats = (): ChatStats => ({
  lastProcessTime: 0,
  avgProcessTime: 0,
  maxProcessTime: 0,
  processCount: 0,
  lastRenderTime: 0,
  avgRenderTime: 0,
  maxRenderTime: 0,
  renderCount: 0,
  lastUpdateTime: 0,
  updateIntervalMs: 0,
  avgUpdateIntervalMs: 0,
  updatesPerSecond: 0,
  contentLength: 0,
  codeBlockCount: 0,
  lastContentDelta: 0,
  isThrottled: false,
  throttleMs: 150,
  skippedUpdates: 0,
  isStreaming: false,
  streamStartTime: null,
  streamDurationMs: 0,
});

// Stats collector class for tracking metrics
export class ChatStatsCollector {
  private stats: ChatStats = createInitialStats();
  private processTimeSamples: number[] = [];
  private renderTimeSamples: number[] = [];
  private updateIntervalSamples: number[] = [];
  private lastContentLength: number = 0;
  private listeners: Set<(stats: ChatStats) => void> = new Set();
  private upsWindow: number[] = []; // timestamps of recent updates for UPS calculation

  private readonly MAX_SAMPLES = 100;
  private readonly UPS_WINDOW_MS = 1000; // 1 second window for UPS calculation

  reset(): void {
    this.stats = createInitialStats();
    this.processTimeSamples = [];
    this.renderTimeSamples = [];
    this.updateIntervalSamples = [];
    this.lastContentLength = 0;
    this.upsWindow = [];
    this.notifyListeners();
  }

  recordProcessTime(ms: number): void {
    this.processTimeSamples.push(ms);
    if (this.processTimeSamples.length > this.MAX_SAMPLES) {
      this.processTimeSamples.shift();
    }

    this.stats.lastProcessTime = ms;
    this.stats.maxProcessTime = Math.max(this.stats.maxProcessTime, ms);
    this.stats.avgProcessTime =
      this.processTimeSamples.reduce((a, b) => a + b, 0) / this.processTimeSamples.length;
    this.stats.processCount++;

    this.notifyListeners();
  }

  recordRenderTime(ms: number): void {
    this.renderTimeSamples.push(ms);
    if (this.renderTimeSamples.length > this.MAX_SAMPLES) {
      this.renderTimeSamples.shift();
    }

    this.stats.lastRenderTime = ms;
    this.stats.maxRenderTime = Math.max(this.stats.maxRenderTime, ms);
    this.stats.avgRenderTime =
      this.renderTimeSamples.reduce((a, b) => a + b, 0) / this.renderTimeSamples.length;
    this.stats.renderCount++;

    this.notifyListeners();
  }

  recordUpdate(contentLength: number, codeBlockCount: number): void {
    const now = Date.now();

    // Calculate update interval
    if (this.stats.lastUpdateTime > 0) {
      const interval = now - this.stats.lastUpdateTime;
      this.updateIntervalSamples.push(interval);
      if (this.updateIntervalSamples.length > this.MAX_SAMPLES) {
        this.updateIntervalSamples.shift();
      }
      this.stats.updateIntervalMs = interval;
      this.stats.avgUpdateIntervalMs =
        this.updateIntervalSamples.reduce((a, b) => a + b, 0) / this.updateIntervalSamples.length;
    }

    // Calculate UPS (updates per second)
    this.upsWindow.push(now);
    // Remove timestamps outside the window
    const windowStart = now - this.UPS_WINDOW_MS;
    this.upsWindow = this.upsWindow.filter((t) => t > windowStart);
    this.stats.updatesPerSecond = this.upsWindow.length;

    // Content metrics
    this.stats.lastContentDelta = contentLength - this.lastContentLength;
    this.lastContentLength = contentLength;
    this.stats.contentLength = contentLength;
    this.stats.codeBlockCount = codeBlockCount;
    this.stats.lastUpdateTime = now;

    // Update stream duration
    if (this.stats.isStreaming && this.stats.streamStartTime) {
      this.stats.streamDurationMs = now - this.stats.streamStartTime;
    }

    this.notifyListeners();
  }

  recordThrottleSkip(): void {
    this.stats.skippedUpdates++;
    this.notifyListeners();
  }

  setThrottleStatus(isThrottled: boolean, throttleMs: number): void {
    this.stats.isThrottled = isThrottled;
    this.stats.throttleMs = throttleMs;
    this.notifyListeners();
  }

  setStreamingStatus(isStreaming: boolean): void {
    const wasStreaming = this.stats.isStreaming;
    this.stats.isStreaming = isStreaming;

    if (isStreaming && !wasStreaming) {
      // Stream started
      this.stats.streamStartTime = Date.now();
      this.stats.streamDurationMs = 0;
      this.stats.skippedUpdates = 0;
    } else if (!isStreaming && wasStreaming) {
      // Stream ended - keep final duration
      if (this.stats.streamStartTime) {
        this.stats.streamDurationMs = Date.now() - this.stats.streamStartTime;
      }
    }

    this.notifyListeners();
  }

  getStats(): ChatStats {
    return { ...this.stats };
  }

  subscribe(listener: (stats: ChatStats) => void): () => void {
    this.listeners.add(listener);
    // Immediately call with current stats
    listener(this.getStats());
    return () => {
      this.listeners.delete(listener);
    };
  }

  private notifyListeners(): void {
    const stats = this.getStats();
    this.listeners.forEach((listener) => listener(stats));
  }
}

// Global stats collector instance
let globalStatsCollector: ChatStatsCollector | null = null;

export const getGlobalStatsCollector = (): ChatStatsCollector => {
  if (!globalStatsCollector) {
    globalStatsCollector = new ChatStatsCollector();
  }
  return globalStatsCollector;
};

// Hook to use stats in components
export const useChatStats = (): ChatStats => {
  const [stats, setStats] = useState<ChatStats>(createInitialStats);

  useEffect(() => {
    const collector = getGlobalStatsCollector();
    return collector.subscribe(setStats);
  }, []);

  return stats;
};

// Props for the toggle button
export interface ChatStatsToggleProps {
  showStats: boolean;
  onToggle: () => void;
}

// Toggle button component to show/hide stats
export const ChatStatsToggle: React.FC<ChatStatsToggleProps> = ({ showStats, onToggle }) => {
  return (
    <Tooltip title="Stats for nerds - show chat performance metrics">
      <IconButton
        size="small"
        onClick={onToggle}
        sx={{
          p: 0.25,
          color: showStats ? 'primary.main' : 'text.secondary',
          '&:hover': {
            color: 'primary.main',
          },
        }}
      >
        <BarChartIcon sx={{ fontSize: 16 }} />
      </IconButton>
    </Tooltip>
  );
};

// Props for the overlay
export interface ChatStatsOverlayProps {
  onClose?: () => void;
}

// Main stats overlay component
const ChatStatsOverlay: React.FC<ChatStatsOverlayProps> = ({ onClose }) => {
  const stats = useChatStats();
  const [copied, setCopied] = useState(false);

  const formatMs = (ms: number): string => {
    if (ms < 0.01) return '<0.01';
    if (ms < 1) return ms.toFixed(2);
    if (ms < 10) return ms.toFixed(1);
    return ms.toFixed(0);
  };

  const formatBytes = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
  };

  const getStatusColor = (value: number, warn: number, critical: number): string => {
    if (value >= critical) return '#ff6b6b';
    if (value >= warn) return '#ff9800';
    return '#4caf50';
  };

  const handleCopyStats = useCallback(() => {
    const statsText = `Chat Performance Stats
========================
Timestamp: ${new Date().toISOString()}

MessageProcessor:
  Last: ${formatMs(stats.lastProcessTime)}ms
  Avg: ${formatMs(stats.avgProcessTime)}ms
  Max: ${formatMs(stats.maxProcessTime)}ms
  Count: ${stats.processCount}

React Render:
  Last: ${formatMs(stats.lastRenderTime)}ms
  Avg: ${formatMs(stats.avgRenderTime)}ms
  Max: ${formatMs(stats.maxRenderTime)}ms
  Count: ${stats.renderCount}

Updates:
  Interval: ${formatMs(stats.updateIntervalMs)}ms
  Avg Interval: ${formatMs(stats.avgUpdateIntervalMs)}ms
  Updates/sec: ${stats.updatesPerSecond}
  Skipped: ${stats.skippedUpdates}

Content:
  Length: ${stats.contentLength} chars (${formatBytes(stats.contentLength)})
  Code Blocks: ${stats.codeBlockCount}
  Last Delta: +${stats.lastContentDelta} chars

Streaming:
  Active: ${stats.isStreaming}
  Duration: ${(stats.streamDurationMs / 1000).toFixed(1)}s
  Throttle: ${stats.throttleMs}ms
`;

    navigator.clipboard.writeText(statsText).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [stats]);

  return (
    <Box
      sx={{
        position: 'absolute',
        top: 50,
        right: 10,
        backgroundColor: 'rgba(0, 0, 0, 0.9)',
        color: '#00ff00',
        padding: 2,
        borderRadius: 1,
        fontFamily: 'monospace',
        fontSize: '0.7rem',
        zIndex: 1500,
        minWidth: 280,
        maxWidth: 320,
        border: '1px solid rgba(0, 255, 0, 0.3)',
        boxShadow: '0 4px 20px rgba(0, 0, 0, 0.5)',
      }}
    >
      {/* Header */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
        <Typography
          variant="caption"
          sx={{ fontWeight: 'bold', color: '#00ff00', fontSize: '0.75rem' }}
        >
          Chat Stats for Nerds
        </Typography>
        <Tooltip title={copied ? 'Copied!' : 'Copy stats to clipboard'}>
          <IconButton
            size="small"
            onClick={handleCopyStats}
            sx={{ color: copied ? '#4caf50' : '#00ff00', p: 0.5 }}
          >
            <ContentCopyIcon sx={{ fontSize: 14 }} />
          </IconButton>
        </Tooltip>
      </Box>

      {/* Streaming Status Banner */}
      {stats.isStreaming && (
        <Box
          sx={{
            mb: 1,
            p: 0.5,
            backgroundColor: 'rgba(76, 175, 80, 0.2)',
            borderRadius: 0.5,
            border: '1px solid rgba(76, 175, 80, 0.5)',
          }}
        >
          <Typography sx={{ color: '#4caf50', fontSize: '0.7rem', fontWeight: 'bold' }}>
            ● STREAMING ({(stats.streamDurationMs / 1000).toFixed(1)}s)
          </Typography>
        </Box>
      )}

      {/* Stats Content */}
      <Box sx={{ '& > div': { mb: 0.5, lineHeight: 1.6 } }}>
        {/* MessageProcessor Section */}
        <Box sx={{ borderBottom: '1px solid rgba(0, 255, 0, 0.2)', pb: 0.5, mb: 0.5 }}>
          <Typography sx={{ color: '#00ff00', fontSize: '0.65rem', fontWeight: 'bold', mb: 0.25 }}>
            MESSAGE PROCESSOR
          </Typography>
          <div>
            <strong>Last:</strong>{' '}
            <span style={{ color: getStatusColor(stats.lastProcessTime, 2, 5) }}>
              {formatMs(stats.lastProcessTime)}ms
            </span>
          </div>
          <div>
            <strong>Avg:</strong>{' '}
            <span style={{ color: getStatusColor(stats.avgProcessTime, 2, 5) }}>
              {formatMs(stats.avgProcessTime)}ms
            </span>
            <span style={{ color: '#888' }}> | Max: {formatMs(stats.maxProcessTime)}ms</span>
          </div>
          <div>
            <strong>Count:</strong> {stats.processCount}
          </div>
        </Box>

        {/* React Render Section */}
        <Box sx={{ borderBottom: '1px solid rgba(0, 255, 0, 0.2)', pb: 0.5, mb: 0.5 }}>
          <Typography sx={{ color: '#00ff00', fontSize: '0.65rem', fontWeight: 'bold', mb: 0.25 }}>
            REACT RENDER
          </Typography>
          <div>
            <strong>Last:</strong>{' '}
            <span style={{ color: getStatusColor(stats.lastRenderTime, 16, 33) }}>
              {formatMs(stats.lastRenderTime)}ms
            </span>
            {stats.lastRenderTime > 16 && (
              <span style={{ color: '#ff9800' }}> {'>'} 60fps</span>
            )}
            {stats.lastRenderTime > 33 && (
              <span style={{ color: '#ff6b6b' }}> {'>'} 30fps</span>
            )}
          </div>
          <div>
            <strong>Avg:</strong>{' '}
            <span style={{ color: getStatusColor(stats.avgRenderTime, 16, 33) }}>
              {formatMs(stats.avgRenderTime)}ms
            </span>
            <span style={{ color: '#888' }}> | Max: {formatMs(stats.maxRenderTime)}ms</span>
          </div>
          <div>
            <strong>Count:</strong> {stats.renderCount}
          </div>
        </Box>

        {/* Update Frequency Section */}
        <Box sx={{ borderBottom: '1px solid rgba(0, 255, 0, 0.2)', pb: 0.5, mb: 0.5 }}>
          <Typography sx={{ color: '#00ff00', fontSize: '0.65rem', fontWeight: 'bold', mb: 0.25 }}>
            UPDATE FREQUENCY
          </Typography>
          <div>
            <strong>Interval:</strong> {formatMs(stats.updateIntervalMs)}ms
            <span style={{ color: '#888' }}> (avg: {formatMs(stats.avgUpdateIntervalMs)}ms)</span>
          </div>
          <div>
            <strong>Updates/sec:</strong>{' '}
            <span style={{ color: stats.updatesPerSecond > 10 ? '#ff9800' : '#4caf50' }}>
              {stats.updatesPerSecond}
            </span>
            {stats.updatesPerSecond > 10 && (
              <span style={{ color: '#ff9800' }}> High frequency</span>
            )}
          </div>
          <div>
            <strong>Throttle:</strong> {stats.throttleMs}ms
            {stats.skippedUpdates > 0 && (
              <span style={{ color: '#888' }}> (skipped: {stats.skippedUpdates})</span>
            )}
          </div>
        </Box>

        {/* Content Section */}
        <Box>
          <Typography sx={{ color: '#00ff00', fontSize: '0.65rem', fontWeight: 'bold', mb: 0.25 }}>
            CONTENT
          </Typography>
          <div>
            <strong>Length:</strong> {stats.contentLength.toLocaleString()} chars
            <span style={{ color: '#888' }}> ({formatBytes(stats.contentLength)})</span>
          </div>
          <div>
            <strong>Code Blocks:</strong> {stats.codeBlockCount}
            {stats.codeBlockCount > 5 && (
              <span style={{ color: '#ff9800' }}> Many blocks</span>
            )}
          </div>
          <div>
            <strong>Last Delta:</strong>{' '}
            <span style={{ color: stats.lastContentDelta > 0 ? '#4caf50' : '#888' }}>
              +{stats.lastContentDelta} chars
            </span>
          </div>
        </Box>
      </Box>

      {/* Performance Summary */}
      <Box
        sx={{
          mt: 1,
          pt: 1,
          borderTop: '1px solid rgba(0, 255, 0, 0.3)',
        }}
      >
        <Typography sx={{ fontSize: '0.65rem', color: '#888' }}>
          Target: Process {'<'}2ms, Render {'<'}16ms (60fps)
        </Typography>
        {stats.avgProcessTime > 2 && (
          <Typography sx={{ fontSize: '0.65rem', color: '#ff9800' }}>
            ⚠ MessageProcessor is slow ({formatMs(stats.avgProcessTime)}ms avg)
          </Typography>
        )}
        {stats.avgRenderTime > 16 && (
          <Typography sx={{ fontSize: '0.65rem', color: '#ff9800' }}>
            ⚠ React render is slow ({formatMs(stats.avgRenderTime)}ms avg)
          </Typography>
        )}
        {stats.avgProcessTime <= 2 && stats.avgRenderTime <= 16 && (
          <Typography sx={{ fontSize: '0.65rem', color: '#4caf50' }}>
            ✓ Performance looks good
          </Typography>
        )}
      </Box>
    </Box>
  );
};

export default ChatStatsOverlay;
