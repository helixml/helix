import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton, Button, Tooltip, Menu, MenuItem } from '@mui/material';
import {
  Fullscreen,
  FullscreenExit,
  Refresh,
  VolumeUp,
  VolumeOff,
  BarChart,
  Keyboard,
  Wifi,
  SignalCellularAlt,
  Speed,
  Stream as StreamIcon,
  Timeline,
} from '@mui/icons-material';
import KeyboardObservabilityPanel from './KeyboardObservabilityPanel';
import { LineChart } from '@mui/x-charts';
import {
  darkChartStyles,
  chartContainerStyles,
  chartLegendProps,
  axisLabelStyle,
} from '../wolf/chartStyles';
import { getApi, apiGetApps } from '../../lib/moonlight-web-ts/api';
import { Stream } from '../../lib/moonlight-web-ts/stream/index';
import { WebSocketStream } from '../../lib/moonlight-web-ts/stream/websocket-stream';
import { DualStreamManager } from '../../lib/moonlight-web-ts/stream/dual-stream-manager';
import { SseStream, buildSseStreamUrl } from '../../lib/moonlight-web-ts/stream/sse-stream';
import { defaultStreamSettings, StreamingMode } from '../../lib/moonlight-web-ts/component/settings_menu';
import { getSupportedVideoFormats, getWebCodecsSupportedVideoFormats, getStandardVideoFormats } from '../../lib/moonlight-web-ts/stream/video';
import useApi from '../../hooks/useApi';
import { useAccount } from '../../contexts/account';
import { TypesClipboardData } from '../../api/api';

interface MoonlightStreamViewerProps {
  sessionId: string;
  wolfLobbyId?: string;
  hostId?: number;
  appId?: number;
  isPersonalDevEnvironment?: boolean;
  showLoadingOverlay?: boolean; // Show loading overlay (for restart/reconnect scenarios)
  isRestart?: boolean; // Whether this is a restart (vs first start)
  onConnectionChange?: (isConnected: boolean) => void;
  onError?: (error: string) => void;
  onClientIdCalculated?: (clientId: string) => void; // Callback when client unique ID is calculated
  width?: number;
  height?: number;
  fps?: number;
  className?: string;
}

/**
 * MoonlightStreamViewer - Native React component using moonlight-web-stream modules
 *
 * This component embeds the moonlight-web-stream JavaScript modules directly
 * without using an iframe, providing seamless integration with Helix RBAC.
 *
 * Architecture:
 * - Uses compiled moonlight-web-stream JS modules from /moonlight-static/
 * - Stream class manages WebSocket → WebRTC signaling
 * - StreamInput handles mouse/keyboard/gamepad/touch
 * - Direct MediaStream attachment to <video> element
 */
const MoonlightStreamViewer: React.FC<MoonlightStreamViewerProps> = ({
  sessionId,
  wolfLobbyId,
  hostId = 0,
  appId = 1,
  isPersonalDevEnvironment = false,
  showLoadingOverlay = false,
  isRestart = false,
  onConnectionChange,
  onError,
  onClientIdCalculated,
  width = 1920,
  height = 1080,
  fps = 60,
  className = '',
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null); // Canvas for WebSocket-only mode
  const containerRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<Stream | WebSocketStream | DualStreamManager | SseStream | null>(null); // Stream instance from moonlight-web
  const sseInputWsRef = useRef<WebSocketStream | null>(null); // WebSocket for input when using SSE mode
  const retryAttemptRef = useRef(0); // Use ref to avoid closure issues
  const previousLobbyIdRef = useRef<string | undefined>(undefined); // Track lobby changes

  // Generate unique UUID for this component instance (persists across re-renders)
  // This ensures multiple floating windows get different Moonlight client IDs
  const componentInstanceIdRef = useRef<string>(
    'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = Math.random() * 16 | 0;
      const v = c === 'x' ? r : (r & 0x3 | 0x8);
      return v.toString(16);
    })
  )

  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState('Initializing...');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [audioEnabled, setAudioEnabled] = useState(true);
  const [pendingAutoJoin, setPendingAutoJoin] = useState(false); // Wait for video before auto-join
  const [cursorPosition, setCursorPosition] = useState({ x: 0, y: 0 });
  const [hasMouseMoved, setHasMouseMoved] = useState(false);
  const [retryCountdown, setRetryCountdown] = useState<number | null>(null);
  const [retryAttemptDisplay, setRetryAttemptDisplay] = useState(0);
  const [showStats, setShowStats] = useState(false);
  const [showKeyboardPanel, setShowKeyboardPanel] = useState(false);
  const [requestedBitrate, setRequestedBitrate] = useState<number>(10); // Mbps (from backend config)
  const [userBitrate, setUserBitrate] = useState<number | null>(null); // User-selected bitrate (overrides backend)
  const [bitrateMenuAnchor, setBitrateMenuAnchor] = useState<null | HTMLElement>(null);
  const manualBitrateSelectionTimeRef = useRef<number>(0); // Track when user manually selected bitrate (20s cooldown before auto-reduce)
  const [streamingMode, setStreamingMode] = useState<StreamingMode>('websocket'); // Default to WebSocket-only
  const [canvasDisplaySize, setCanvasDisplaySize] = useState<{ width: number; height: number } | null>(null);
  const [containerSize, setContainerSize] = useState<{ width: number; height: number } | null>(null);
  const [isHighLatency, setIsHighLatency] = useState(false); // Show warning when RTT > 150ms
  // Quality mode: 'adaptive' (auto-switch), 'high' (force 60fps), 'low' (screenshot-based for low bandwidth)
  // Low mode uses rapid screenshot polling for video while keeping input via the stream
  // This provides a working low-bandwidth fallback without the keyframes-only streaming bugs
  // Default to 'high' (60fps video) - screenshot mode has app_id issues in lobbies mode
  const [qualityMode, setQualityMode] = useState<'adaptive' | 'high' | 'low'>('high');
  const [isOnFallback, setIsOnFallback] = useState(false); // True when on low-quality fallback stream
  const [modeSwitchCooldown, setModeSwitchCooldown] = useState(false); // Prevent rapid mode switching (causes Wolf deadlock)

  // Screenshot-based low-quality mode state
  const [screenshotUrl, setScreenshotUrl] = useState<string | null>(null);
  const screenshotIntervalRef = useRef<NodeJS.Timeout | null>(null);
  // Adaptive JPEG quality control - targets 2 FPS (500ms max per frame)
  const [screenshotQuality, setScreenshotQuality] = useState(70); // JPEG quality 10-90
  const [screenshotFps, setScreenshotFps] = useState(0); // Current FPS for display
  const screenshotQualityRef = useRef(70); // Ref for use in async callback
  // Adaptive mode: auto-enable screenshot overlay when stream latency is high
  const [adaptiveScreenshotEnabled, setAdaptiveScreenshotEnabled] = useState(false);
  // Lock-in behavior: once adaptive mode falls back to screenshots, stay there until user explicitly changes mode
  // This prevents oscillation: when we stop sending video, latency drops, which would cause us to switch back
  const [adaptiveLockedToScreenshots, setAdaptiveLockedToScreenshots] = useState(false);

  // Clipboard sync state
  const lastRemoteClipboardHash = useRef<string>(''); // Track changes to avoid unnecessary writes
  const [stats, setStats] = useState<any>(null);

  // Chart history for visualizing adaptive bitrate behavior (60 seconds of data)
  // Uses refs to persist across reconnects - only reset when component unmounts
  const CHART_HISTORY_LENGTH = 60;
  const throughputHistoryRef = useRef<number[]>([]);
  const rttHistoryRef = useRef<number[]>([]);
  const bitrateHistoryRef = useRef<number[]>([]);
  // Events: track when and why bitrate changed (for chart annotations)
  const chartEventsRef = useRef<Array<{
    index: number;
    type: 'reduce' | 'increase' | 'reconnect' | 'rtt_spike' | 'saturation';
    reason: string;
  }>>([]);
  const [chartUpdateTrigger, setChartUpdateTrigger] = useState(0); // Force re-render when refs change
  const [showCharts, setShowCharts] = useState(false);

  // Helper to add chart event
  const addChartEvent = useCallback((type: 'reduce' | 'increase' | 'reconnect' | 'rtt_spike' | 'saturation', reason: string) => {
    const index = throughputHistoryRef.current.length;
    chartEventsRef.current.push({ index, type, reason });
    // Keep only events within the visible window
    const minIndex = Math.max(0, index - CHART_HISTORY_LENGTH);
    chartEventsRef.current = chartEventsRef.current.filter(e => e.index >= minIndex);
  }, []);

  // Clipboard toast state
  const [clipboardToast, setClipboardToast] = useState<{
    message: string;
    type: 'success' | 'error';
    visible: boolean;
  }>({ message: '', type: 'success', visible: false });
  const clipboardToastTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Show clipboard toast notification
  const showClipboardToast = useCallback((message: string, type: 'success' | 'error') => {
    // Clear any existing timeout
    if (clipboardToastTimeoutRef.current) {
      clearTimeout(clipboardToastTimeoutRef.current);
    }

    setClipboardToast({ message, type, visible: true });

    // Auto-hide after delay (longer for errors so user can read the reason)
    const hideDelay = type === 'error' ? 4000 : 2000;
    clipboardToastTimeoutRef.current = setTimeout(() => {
      setClipboardToast(prev => ({ ...prev, visible: false }));
    }, hideDelay);
  }, []);
  const lastBytesRef = useRef<{ bytes: number; timestamp: number } | null>(null);

  const helixApi = useApi();
  const account = useAccount();

  // Connect to stream
  const connect = useCallback(async () => {
    // Generate fresh UUID for EVERY connection attempt
    // This prevents Wolf session ID conflicts when reconnecting to the same Helix session
    // (Wolf requires unique client_unique_id per connection to avoid stale state corruption)
    componentInstanceIdRef.current = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = Math.random() * 16 | 0;
      const v = c === 'x' ? r : (r & 0x3 | 0x8);
      return v.toString(16);
    });

    setIsConnecting(true);
    setError(null);
    setStatus('Connecting to streaming server...');

    try {
      // Fetch Helix config to determine moonlight-web mode
      const apiClient = helixApi.getApiClient();
      const configResponse = await apiClient.v1ConfigList();
      const moonlightWebMode = configResponse.data.moonlight_web_mode || 'single';

      console.log(`MoonlightStreamViewer: Using moonlight-web mode: ${moonlightWebMode}`);

      // Determine app ID based on mode
      let actualAppId = appId;

      if (wolfLobbyId) {
        // Lobbies mode: Fetch Wolf UI app ID dynamically from Wolf
        // Pass session_id to identify which Wolf instance to query
        try {
          const response = await fetch(`/api/v1/wolf/ui-app-id?session_id=${encodeURIComponent(sessionId)}`, {
            headers: {
              'Authorization': `Bearer ${account.user?.token || ''}`,
            },
          });
          if (response.ok) {
            const data = await response.json();
            actualAppId = parseInt(data.placeholder_app_id, 10);
            console.log(`MoonlightStreamViewer: Using placeholder app ID ${actualAppId} for lobbies mode, lobby ${wolfLobbyId}`);
          } else {
            const errorText = await response.text();
            console.warn(`MoonlightStreamViewer: Failed to fetch Wolf UI app ID: ${errorText}`);
            actualAppId = 0;
          }
        } catch (err) {
          console.warn('Failed to fetch Wolf UI app ID, using default 0:', err);
          actualAppId = 0;
        }
      } else if (sessionId && !isPersonalDevEnvironment) {
        // Apps mode: Fetch the specific Wolf app ID for this session
        try {
          const wolfStateResponse = await apiClient.v1SessionsWolfAppStateDetail(sessionId);
          if (wolfStateResponse.data?.wolf_app_id) {
            actualAppId = parseInt(wolfStateResponse.data.wolf_app_id, 10);
            console.log(`MoonlightStreamViewer: Using Wolf app ID ${actualAppId} for session ${sessionId}`);
          }
        } catch (err) {
          console.warn('Failed to fetch Wolf app ID, using default:', err);
        }
      }

      // Get Helix JWT from account context (HttpOnly cookie not readable by JS)
      const helixToken = account.user?.token || '';

      console.log('[MoonlightStreamViewer] Auth check:', {
        hasAccount: !!account,
        hasUser: !!account.user,
        hasToken: !!helixToken,
        tokenLength: helixToken.length,
      });

      if (!helixToken) {
        console.error('[MoonlightStreamViewer] No token available:', { account, user: account.user });
        throw new Error('Not authenticated - please log in');
      }

      console.log('[MoonlightStreamViewer] Using Helix token for streaming auth');

      // Create API instance directly (don't use getApi() - it caches globally)
      // Pointing to moonlight-web via Helix proxy at /moonlight
      // Proxy validates Helix auth via HttpOnly cookie (sent automatically by browser)
      // and injects moonlight-web credentials
      console.log('[MoonlightStreamViewer] Creating fresh moonlight API instance');
      const api = {
        host_url: `/moonlight`,
        credentials: helixToken,  // For HTTP fetch requests (Authorization header)
      };
      console.log('[MoonlightStreamViewer] API instance created (WebSocket will use HttpOnly cookie auth)');

      // Get streaming bitrate: user-selected > backend config > default
      let streamingBitrateMbps = 10; // Default: 10 Mbps (conservative for low-bandwidth)

      if (userBitrate !== null) {
        // User explicitly selected a bitrate - use it
        streamingBitrateMbps = userBitrate;
        console.log(`[MoonlightStreamViewer] Using user-selected bitrate: ${streamingBitrateMbps} Mbps`);
      } else {
        // Try to get from backend config
        try {
          const configResponse = await apiClient.v1ConfigList();
          if (configResponse.data.streaming_bitrate_mbps) {
            streamingBitrateMbps = configResponse.data.streaming_bitrate_mbps;
            console.log(`[MoonlightStreamViewer] Using configured bitrate: ${streamingBitrateMbps} Mbps`);
          }
        } catch (err) {
          console.warn('[MoonlightStreamViewer] Failed to fetch streaming bitrate config, using default:', err);
        }
      }

      // Store for stats display
      setRequestedBitrate(streamingBitrateMbps);

      // Get default stream settings and customize
      const settings = defaultStreamSettings();
      settings.videoSize = 'custom';
      settings.videoSizeCustom = { width, height };  // Use configured resolution from props
      settings.bitrate = streamingBitrateMbps * 1000;  // Convert to kbps - Configured bitrate (P-frames more efficient than all I-frames)
      settings.packetSize = 1024;
      settings.fps = fps;  // Use configured fps from props
      settings.videoSampleQueueSize = 50;  // Queue size for 1080p60 streaming
      settings.audioSampleQueueSize = 20;
      settings.playAudioLocal = !audioEnabled;

      // Detect actual browser codec support
      // For WebSocket mode: use WebCodecs detection (VideoDecoder.isConfigSupported)
      // For WebRTC mode: use RTCRtpReceiver detection (default behavior)
      let supportedFormats;
      if (streamingMode === 'websocket') {
        // WebSocket mode uses WebCodecs for decoding - detect actual hardware decoder support
        console.log('[MoonlightStreamViewer] Detecting WebCodecs supported codecs...');
        supportedFormats = await getWebCodecsSupportedVideoFormats();
        console.log('[MoonlightStreamViewer] WebCodecs supported formats:', supportedFormats);
      } else {
        // WebRTC mode - use standard video format detection
        supportedFormats = getStandardVideoFormats();
      }

      // Create Stream instance with mode-aware parameters
      console.log('[MoonlightStreamViewer] Creating Stream instance', {
        mode: moonlightWebMode,
        streamingMode,
        hostId,
        actualAppId,
        sessionId,
      });

      let stream: Stream | WebSocketStream | DualStreamManager | SseStream;

      // Check if using SSE mode (experimental - WebSocket for session/input, SSE for video)
      // Architecture: ONE WebSocket creates Wolf session, SSE subscribes to video from it
      if (streamingMode === 'sse') {
        console.log('[MoonlightStreamViewer] Using SSE streaming mode (WebSocket for input, SSE for video)');

        // Use same session ID format as websocket mode for consistency
        // This allows hot-switching between modes without confusion
        const qualitySessionId = sessionId ? `${sessionId}-hq` : undefined;

        // Create WebSocket stream for session creation and input
        // This is the ONLY connection to Wolf - it creates the session
        const wsStream = new WebSocketStream(
          api,
          hostId,
          actualAppId,
          settings,
          supportedFormats,
          [width, height],
          qualitySessionId // Use -hq suffix like websocket mode for consistency
        );

        // Store for input handling
        sseInputWsRef.current = wsStream;

        // Listen for WebSocket events to know when session is ready
        wsStream.addInfoListener((event: any) => {
          const data = event.detail;

          if (data.type === 'streamInit') {
            // Session is initialized - NOW open SSE for video
            console.log('[MoonlightStreamViewer] WebSocket session ready, opening SSE for video');

            // Disable video over WebSocket - we'll use SSE instead
            wsStream.setVideoEnabled(false);

            // Open SSE connection for video frames - use same session ID as WebSocket
            const sseUrl = `/moonlight/api/sse/video?session_id=${encodeURIComponent(qualitySessionId || '')}`;
            console.log('[MoonlightStreamViewer] SSE video URL:', sseUrl);

            // Create EventSource for video
            const eventSource = new EventSource(sseUrl, { withCredentials: true });

            // Set canvas for rendering SSE video frames
            if (canvasRef.current) {
              const canvas = canvasRef.current;
              const ctx = canvas.getContext('2d', { alpha: false, desynchronized: true });

              // Initialize video decoder for SSE frames
              let videoDecoder: VideoDecoder | null = null;

              eventSource.addEventListener('init', (e: MessageEvent) => {
                try {
                  const init = JSON.parse(e.data);
                  console.log('[MoonlightStreamViewer] SSE video init:', init);

                  // Configure video decoder - use same codec strings as websocket-stream.ts
                  let codecString: string;
                  if (init.video_codec === 0x01) {
                    codecString = 'avc1.4d0033';  // H.264 Main Profile Level 5.1
                  } else if (init.video_codec === 0x02) {
                    codecString = 'avc1.640032';  // H.264 High Profile Level 5.0
                  } else if (init.video_codec === 0x10) {
                    codecString = 'hvc1.1.6.L120.90';  // HEVC Main
                  } else if (init.video_codec === 0x11) {
                    codecString = 'hvc1.2.4.L120.90';  // HEVC Main 10
                  } else {
                    codecString = 'avc1.4d0033';  // Default to H.264
                  }
                  console.log(`[SSE Video] Using codec string: ${codecString} for ${init.width}x${init.height}@${init.fps}`);
                  videoDecoder = new VideoDecoder({
                    output: (frame: VideoFrame) => {
                      if (ctx && canvas) {
                        canvas.width = frame.displayWidth;
                        canvas.height = frame.displayHeight;
                        ctx.drawImage(frame, 0, 0);
                      }
                      frame.close();
                    },
                    error: (err) => console.error('[SSE Video] Decoder error:', err),
                  });
                  videoDecoder.configure({
                    codec: codecString,
                    codedWidth: init.width,
                    codedHeight: init.height,
                    hardwareAcceleration: 'prefer-hardware',
                  });
                } catch (err) {
                  console.error('[MoonlightStreamViewer] Failed to parse SSE init:', err);
                }
              });

              eventSource.addEventListener('video', (e: MessageEvent) => {
                if (!videoDecoder || videoDecoder.state !== 'configured') return;

                try {
                  const frame = JSON.parse(e.data);
                  // Decode base64 video data
                  const binaryString = atob(frame.data);
                  const bytes = new Uint8Array(binaryString.length);
                  for (let i = 0; i < binaryString.length; i++) {
                    bytes[i] = binaryString.charCodeAt(i);
                  }

                  const chunk = new EncodedVideoChunk({
                    type: frame.keyframe ? 'key' : 'delta',
                    timestamp: frame.pts,
                    data: bytes,
                  });
                  videoDecoder.decode(chunk);
                } catch (err) {
                  console.error('[MoonlightStreamViewer] Failed to decode SSE video frame:', err);
                }
              });

              eventSource.addEventListener('stop', () => {
                console.log('[MoonlightStreamViewer] SSE video stopped');
                if (videoDecoder) {
                  videoDecoder.close();
                  videoDecoder = null;
                }
              });

              eventSource.onerror = (err) => {
                console.error('[MoonlightStreamViewer] SSE video error:', err);
              };

              // Store eventSource for cleanup
              (wsStream as any)._sseEventSource = eventSource;
            }
          } else if (data.type === 'connectionComplete') {
            setIsConnected(true);
            setIsConnecting(false);
            setStatus('Streaming active (SSE + WebSocket)');
            onConnectionChange?.(true);
          } else if (data.type === 'error') {
            setError(data.message);
            setIsConnected(false);
            onError?.(data.message);
          } else if (data.type === 'disconnected') {
            setIsConnected(false);
            setStatus('Disconnected');
            onConnectionChange?.(false);
          }
        });

        stream = wsStream;
      } else if (streamingMode === 'websocket') {
        // Check if using WebSocket-only mode
        // WebSocket-only mode: bypass WebRTC entirely, use WebCodecs for decoding
        console.log('[MoonlightStreamViewer] Using WebSocket-only streaming mode, qualityMode:', qualityMode);

        // Adaptive and High modes: use high-quality 60fps stream
        // Low mode: still uses stream for input, but screenshot overlay provides video
        // In adaptive mode, screenshot overlay auto-enables when RTT exceeds threshold
        const streamSettings = { ...settings };
        const qualitySessionId = sessionId ? `${sessionId}-hq` : undefined;

        if (qualityMode === 'adaptive') {
          console.log('[MoonlightStreamViewer] Adaptive mode: 60fps stream + auto screenshot fallback');
        } else if (qualityMode === 'low') {
          console.log('[MoonlightStreamViewer] Low mode: 60fps stream for input + screenshot overlay');
        } else {
          console.log('[MoonlightStreamViewer] High mode: 60fps stream only');
        }

        stream = new WebSocketStream(
          api,
          hostId,
          actualAppId,
          streamSettings,
          supportedFormats,
          [width, height],
          qualitySessionId
        );

        // Set canvas for WebSocket stream rendering
        if (canvasRef.current) {
          if (qualityMode !== 'low') {
            // Normal mode: stream renders frames to canvas
            stream.setCanvas(canvasRef.current);
          } else {
            // Low/screenshot mode: stream is only used for input, not video rendering
            // But we still need to set canvas dimensions for proper mouse coordinate mapping
            // (getStreamRect uses canvas.width/height to calculate aspect ratio)
            canvasRef.current.width = 1920;
            canvasRef.current.height = 1080;
          }
        }
      } else if (moonlightWebMode === 'multi') {
        // Multi-WebRTC architecture: backend created streamer via POST /api/streamers
        // Connect to persistent streamer via peer endpoint
        // Include instance ID for multi-tab support
        const streamerID = `agent-${sessionId}-${componentInstanceIdRef.current}`;
        stream = new Stream(
          api,
          hostId, // Wolf host ID (always 0 for local)
          actualAppId, // App ID (backend already knows it)
          settings,
          supportedFormats,
          [width, height],
          "peer", // Peer mode - connects to existing streamer
          undefined, // No session ID needed
          streamerID // Streamer ID - unique per component instance
        );
      } else {
        // Single mode (kickoff approach): Fresh "create" connection with explicit client_unique_id
        // CRITICAL: Include lobby ID in uniqueid to prevent stale session conflicts
        // When session restarts (test startup script), lobby ID changes but session ID doesn't
        // - Kickoff used: session="agent-{sessionId}-kickoff", client_unique_id="helix-agent-{sessionId}"
        // - Browser uses: session="agent-{sessionId}-{lobbyId}-{instanceId}", client_unique_id="helix-agent-{sessionId}-{lobbyId}-{instanceId}"
        // Different lobby_id → Fresh Moonlight session (prevents "AlreadyStreaming" conflicts)
        // Unique client_unique_id per browser tab → Multiple tabs can stream simultaneously!

        const lobbyIdPart = wolfLobbyId ? `-${wolfLobbyId}` : '';
        const uniqueClientId = `helix-agent-${sessionId}${lobbyIdPart}-${componentInstanceIdRef.current}`;

        console.log(`[MoonlightStream] Creating stream with uniqueClientId: ${uniqueClientId}`, {
          sessionId,
          wolfLobbyId,
          componentInstanceId: componentInstanceIdRef.current,
        });

        // Notify parent component of calculated client ID
        onClientIdCalculated?.(uniqueClientId);

        stream = new Stream(
          api,
          hostId, // Wolf host ID (always 0 for local)
          actualAppId, // Moonlight app ID from Wolf
          settings,
          supportedFormats,
          [width, height],
          "create", // Create mode - fresh session/streamer (kickoff already terminated)
          `agent-${sessionId}-${componentInstanceIdRef.current}`, // Unique per component instance
          undefined, // No streamer ID
          uniqueClientId // Unique per lobby+component → prevents conflicts
        );
      }

      streamRef.current = stream;

      // Listen for stream events (SSE mode uses callbacks instead of addInfoListener)
      if (streamingMode !== 'sse' && 'addInfoListener' in stream) {
        stream.addInfoListener((event: any) => {
        const data = event.detail;

        if (data.type === 'connected') {
          // WebSocket opened - show initializing status (still waiting for connectionComplete)
          setStatus('Initializing stream...');
        } else if (data.type === 'streamInit') {
          // Stream parameters received - decoding about to start
          setStatus('Starting video decoder...');
        } else if (data.type === 'connectionComplete') {
          setIsConnected(true);
          setIsConnecting(false);
          setStatus('Streaming active');
          setError(null); // Clear any previous errors on successful connection
          retryAttemptRef.current = 0; // Reset retry counter on successful connection
          setRetryAttemptDisplay(0);
          onConnectionChange?.(true);

          // Auto-join lobby if in lobbies mode (after video starts playing)
          // Set pending flag - actual join triggered by onCanPlay handler
          if (wolfLobbyId && sessionId) {
            console.log('[AUTO-JOIN] Connection established, waiting for video to start before auto-join');
            setPendingAutoJoin(true);
          }
        } else if (data.type === 'error') {
          const errorMsg = data.message || 'Stream error';

          // Check if error is AlreadyStreaming - retry instead of permanent failure
          if (errorMsg.includes('AlreadyStreaming') || errorMsg.includes('already streaming')) {
            setIsConnecting(false);

            // Progressive retry: 2s, 3s, 4s, 5s... (capped at 10s)
            // Use ref to avoid closure issues with event listeners
            retryAttemptRef.current += 1;
            const nextAttempt = retryAttemptRef.current;
            const retryDelaySeconds = Math.min(nextAttempt + 1, 10); // +1 to start at 2s

            console.warn(`[MoonlightStreamViewer] AlreadyStreaming error from stream (attempt ${nextAttempt}), will retry in ${retryDelaySeconds} seconds...`);

            setRetryAttemptDisplay(nextAttempt);
            setRetryCountdown(retryDelaySeconds);

            // Update countdown every second
            const countdownInterval = setInterval(() => {
              setRetryCountdown((prev) => {
                if (prev === null || prev <= 1) {
                  clearInterval(countdownInterval);
                  return null;
                }
                return prev - 1;
              });
            }, 1000);

            // Retry after delay
            setTimeout(() => {
              console.log(`[MoonlightStreamViewer] Retrying connection after AlreadyStreaming stream error (attempt ${nextAttempt})`);
              setRetryCountdown(null);
              reconnect();
            }, retryDelaySeconds * 1000);
            return;
          }

          // Permanent error - not AlreadyStreaming
          setError(errorMsg);
          setIsConnected(false);  // Important: mark as disconnected on error
          setIsConnecting(false);
          retryAttemptRef.current = 0; // Reset retry counter on different error
          setRetryAttemptDisplay(0);
          onError?.(errorMsg);
        } else if (data.type === 'connectionStatus') {
          setIsConnected(data.status === 'Connected');
        } else if (data.type === 'connectionTerminated') {
          setError(`Stream terminated (code: ${data.errorCode})`);
          setIsConnected(false);
        } else if (data.type === 'stageStarting') {
          setStatus(data.stage);
        } else if (data.type === 'disconnected') {
          // WebSocket disconnected - show reconnecting status
          console.log('[MoonlightStreamViewer] Stream disconnected');
          setIsConnected(false);
          setStatus('Disconnected - reconnecting...');
          onConnectionChange?.(false);
        } else if (data.type === 'reconnecting') {
          // Show reconnection attempt in status
          console.log(`[MoonlightStreamViewer] Reconnecting attempt ${data.attempt}`);
          setIsConnecting(true);
          setStatus(`Reconnecting (attempt ${data.attempt})...`);
        }
        });
      }

      // Attach media stream to video element (WebRTC mode only)
      // WebSocket mode renders directly to canvas via WebCodecs
      if (streamingMode === 'webrtc' && videoRef.current && stream instanceof Stream) {
        videoRef.current.srcObject = stream.getMediaStream();
        videoRef.current.play().catch((err) => {
          console.warn('Autoplay blocked, user interaction required:', err);
        });
      }

      setStatus('Stream connected');
    } catch (err: any) {
      const errorMsg = err.message || 'Failed to initialize stream';
      console.error('Stream connection error:', errorMsg);

      // Check if error is AlreadyStreaming - retry instead of permanent failure
      if (errorMsg.includes('AlreadyStreaming') || errorMsg.includes('already streaming')) {
        setIsConnecting(false);

        // Progressive retry: 2s, 3s, 4s, 5s... (capped at 10s)
        // Use ref to avoid closure issues
        retryAttemptRef.current += 1;
        const nextAttempt = retryAttemptRef.current;
        const retryDelaySeconds = Math.min(nextAttempt + 1, 10); // +1 to start at 2s

        console.warn(`[MoonlightStreamViewer] AlreadyStreaming error detected (attempt ${nextAttempt}), will retry in ${retryDelaySeconds} seconds...`);

        setRetryAttemptDisplay(nextAttempt);
        setRetryCountdown(retryDelaySeconds);

        // Update countdown every second
        const countdownInterval = setInterval(() => {
          setRetryCountdown((prev) => {
            if (prev === null || prev <= 1) {
              clearInterval(countdownInterval);
              return null;
            }
            return prev - 1;
          });
        }, 1000);

        // Retry after delay
        setTimeout(() => {
          console.log(`[MoonlightStreamViewer] Retrying connection after AlreadyStreaming error (attempt ${nextAttempt})`);
          setRetryCountdown(null);
          connect();
        }, retryDelaySeconds * 1000);
        return;
      }

      // Permanent error - not AlreadyStreaming
      setError(errorMsg);
      setIsConnected(false);  // Important: mark as disconnected on error
      setIsConnecting(false);
      retryAttemptRef.current = 0; // Reset retry counter on different error
      setRetryAttemptDisplay(0);
      onError?.(errorMsg);
    }
  }, [sessionId, hostId, appId, width, height, audioEnabled, onConnectionChange, onError, helixApi, account, isPersonalDevEnvironment, streamingMode, wolfLobbyId, onClientIdCalculated, qualityMode, userBitrate]);

  // Disconnect
  const disconnect = useCallback(() => {
    console.log('[MoonlightStreamViewer] disconnect() called, cleaning up stream resources');

    // Close SSE EventSource if it exists (from hot-switch or initial SSE mode)
    if (sseEventSourceRef.current) {
      console.log('[MoonlightStreamViewer] Closing SSE EventSource...');
      try {
        sseEventSourceRef.current.close();
        sseEventSourceRef.current = null;
      } catch (err) {
        console.warn('[MoonlightStreamViewer] Error closing SSE EventSource:', err);
      }
    }
    if (sseVideoDecoderRef.current) {
      console.log('[MoonlightStreamViewer] Closing SSE VideoDecoder...');
      try {
        sseVideoDecoderRef.current.close();
        sseVideoDecoderRef.current = null;
      } catch (err) {
        console.warn('[MoonlightStreamViewer] Error closing SSE VideoDecoder:', err);
      }
    }
    // Also check for legacy SSE EventSource stored on stream object
    if (streamRef.current && (streamRef.current as any)._sseEventSource) {
      console.log('[MoonlightStreamViewer] Closing legacy SSE EventSource...');
      try {
        (streamRef.current as any)._sseEventSource.close();
        (streamRef.current as any)._sseEventSource = null;
      } catch (err) {
        console.warn('[MoonlightStreamViewer] Error closing legacy SSE EventSource:', err);
      }
    }

    // Close SSE input WebSocket if it exists
    if (sseInputWsRef.current) {
      console.log('[MoonlightStreamViewer] Closing SSE input WebSocket...');
      try {
        sseInputWsRef.current.close();
      } catch (err) {
        console.warn('[MoonlightStreamViewer] Error closing SSE input WebSocket:', err);
      }
      sseInputWsRef.current = null;
    }

    if (streamRef.current) {
      // Properly close the stream to prevent "AlreadyStreaming" errors
      try {
        // Check if it's an SseStream
        if (streamRef.current instanceof SseStream) {
          console.log('[MoonlightStreamViewer] Closing SseStream...');
          streamRef.current.close();
        } else if (streamRef.current instanceof DualStreamManager) {
          // Check if it's a DualStreamManager
          console.log('[MoonlightStreamViewer] Closing DualStreamManager...');
          streamRef.current.close();
        } else if (streamRef.current instanceof WebSocketStream) {
          // Check if it's a WebSocketStream (has close() method)
          console.log('[MoonlightStreamViewer] Closing WebSocketStream...');
          streamRef.current.close();
        } else {
          // WebRTC Stream - close WebSocket and RTCPeerConnection
          console.log('[MoonlightStreamViewer] Closing WebSocket and RTCPeerConnection...');

          // Close WebSocket connection if it exists
          if ((streamRef.current as any).ws) {
            console.log('[MoonlightStreamViewer] Closing WebSocket, readyState:', (streamRef.current as any).ws.readyState);
            (streamRef.current as any).ws.close();
          }

          // Close RTCPeerConnection if it exists
          if ((streamRef.current as any).peer) {
            console.log('[MoonlightStreamViewer] Closing RTCPeerConnection');
            (streamRef.current as any).peer.close();
          }

          // Stop all media stream tracks
          const mediaStream = (streamRef.current as Stream).getMediaStream();
          if (mediaStream) {
            const tracks = mediaStream.getTracks();
            console.log('[MoonlightStreamViewer] Stopping', tracks.length, 'media tracks');
            tracks.forEach((track: MediaStreamTrack) => track.stop());
          }
        }
      } catch (err) {
        console.warn('[MoonlightStreamViewer] Error during stream cleanup:', err);
      }

      streamRef.current = null;
      console.log('[MoonlightStreamViewer] Stream reference cleared');
    } else {
      console.log('[MoonlightStreamViewer] No active stream to disconnect');
    }

    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }

    setIsConnected(false);
    setIsConnecting(false);
    setStatus('Disconnected');
    setPendingAutoJoin(false); // Reset auto-join state on disconnect
    setIsHighLatency(false); // Reset latency warning on disconnect
    setIsOnFallback(false); // Reset fallback state on disconnect
    console.log('[MoonlightStreamViewer] disconnect() completed');
  }, []);

  // Reconnect with configurable delay
  // Mode switches need longer delay to wait for moonlight-web cleanup (up to 15s)
  // Normal reconnects (errors, user-initiated) can be faster
  const reconnect = useCallback((delayMs = 1000) => {
    disconnect();
    setTimeout(connect, delayMs);
  }, [disconnect, connect]);

  // Toggle fullscreen
  const toggleFullscreen = useCallback(() => {
    if (!containerRef.current) return;

    if (!isFullscreen) {
      containerRef.current.requestFullscreen?.();
    } else {
      document.exitFullscreen?.();
    }
  }, [isFullscreen]);

  // Handle fullscreen events
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange);
  }, []);

  // Track previous streaming mode for hot-switching
  const previousStreamingModeRef = useRef<StreamingMode>(streamingMode);
  const sseEventSourceRef = useRef<EventSource | null>(null);
  const sseVideoDecoderRef = useRef<VideoDecoder | null>(null);
  const sseReceivedFirstKeyframeRef = useRef(false);

  // Handle streaming mode changes - hot-switch between websocket/sse when possible
  // Only reconnect when switching to/from webrtc (different protocol)
  useEffect(() => {
    if (previousStreamingModeRef.current === streamingMode) return;

    const prevMode = previousStreamingModeRef.current;
    const newMode = streamingMode;
    console.log('[MoonlightStreamViewer] Streaming mode changed from', prevMode, 'to', newMode);
    previousStreamingModeRef.current = newMode;

    // Hot-switch between websocket and sse modes (same WebSocket connection)
    const canHotSwitch = (prevMode === 'websocket' || prevMode === 'sse') &&
                         (newMode === 'websocket' || newMode === 'sse');

    if (canHotSwitch && isConnected && streamRef.current instanceof WebSocketStream) {
      const wsStream = streamRef.current as WebSocketStream;

      if (newMode === 'sse') {
        // Switching to SSE mode: disable WS video, open SSE for video
        console.log('[MoonlightStreamViewer] Hot-switching to SSE mode (no reconnect)');
        wsStream.setVideoEnabled(false);

        // Open SSE connection for video - use same session ID format as WebSocket
        // WebSocket uses `{sessionId}-hq` format, so SSE must match
        const qualitySessionId = sessionId ? `${sessionId}-hq` : sessionId;
        const sseUrl = `/moonlight/api/sse/video?session_id=${encodeURIComponent(qualitySessionId || '')}`;
        console.log('[MoonlightStreamViewer] Opening SSE video connection:', sseUrl);
        const eventSource = new EventSource(sseUrl, { withCredentials: true });
        sseEventSourceRef.current = eventSource;
        sseReceivedFirstKeyframeRef.current = false;  // Reset for new SSE connection

        // Set up video decoder for SSE frames
        if (canvasRef.current) {
          const canvas = canvasRef.current;
          const ctx = canvas.getContext('2d', { alpha: false, desynchronized: true });

          eventSource.addEventListener('init', async (e: MessageEvent) => {
            try {
              const init = JSON.parse(e.data);
              console.log('[MoonlightStreamViewer] SSE video init:', init);

              // Get fresh canvas reference - the canvas should now be visible in SSE mode
              const sseCanvas = canvasRef.current;
              if (!sseCanvas) {
                console.error('[SSE Video] Canvas ref is null!');
                return;
              }
              const sseCtx = sseCanvas.getContext('2d', { alpha: false, desynchronized: true });
              if (!sseCtx) {
                console.error('[SSE Video] Failed to get canvas context!');
                return;
              }
              console.log('[SSE Video] Got canvas:', sseCanvas.width, 'x', sseCanvas.height, 'context:', !!sseCtx);

              // Configure video decoder - use same codec strings as websocket-stream.ts
              let codecString: string;
              if (init.video_codec === 0x01) {
                codecString = 'avc1.4d0033';  // H.264 Main Profile Level 5.1
              } else if (init.video_codec === 0x02) {
                codecString = 'avc1.640032';  // H.264 High Profile Level 5.0
              } else if (init.video_codec === 0x10) {
                codecString = 'hvc1.1.6.L120.90';  // HEVC Main
              } else if (init.video_codec === 0x11) {
                codecString = 'hvc1.2.4.L120.90';  // HEVC Main 10
              } else {
                codecString = 'avc1.4d0033';  // Default to H.264
              }
              console.log(`[SSE Video] Using codec string: ${codecString} for ${init.width}x${init.height}@${init.fps}`);

              // Check if codec is supported - try hardware first, then software fallback
              let useHardwareAcceleration: HardwareAcceleration = 'prefer-hardware';
              try {
                const hwSupport = await VideoDecoder.isConfigSupported({
                  codec: codecString,
                  codedWidth: init.width,
                  codedHeight: init.height,
                  hardwareAcceleration: 'prefer-hardware',
                });

                if (!hwSupport.supported) {
                  console.log('[SSE Video] Hardware decoding not supported, trying software fallback');
                  const swSupport = await VideoDecoder.isConfigSupported({
                    codec: codecString,
                    codedWidth: init.width,
                    codedHeight: init.height,
                  });

                  if (!swSupport.supported) {
                    console.error('[SSE Video] Video codec not supported (hardware or software):', codecString);
                    return;
                  }
                  useHardwareAcceleration = 'no-preference';
                  console.log('[SSE Video] Using software video decoding');
                } else {
                  console.log('[SSE Video] Using hardware video decoding');
                }
              } catch (e) {
                console.error('[SSE Video] Failed to check codec support:', e);
              }

              // Create decoder with output callback
              // IMPORTANT: Store canvas context NOW to avoid closure issues
              const capturedCanvas = canvasRef.current;
              const capturedCtx = capturedCanvas?.getContext('2d', { alpha: false, desynchronized: true });
              console.log('[SSE Video] Captured canvas for decoder:', capturedCanvas?.width, 'x', capturedCanvas?.height, 'ctx:', !!capturedCtx);

              const decoder = new VideoDecoder({
                output: (frame: VideoFrame) => {
                  console.log('[SSE Video] === OUTPUT CALLBACK FIRED ===');
                  console.log('[SSE Video] Frame decoded:', frame.displayWidth, 'x', frame.displayHeight);
                  // Use captured canvas context from init time
                  if (capturedCtx && capturedCanvas) {
                    if (capturedCanvas.width !== frame.displayWidth || capturedCanvas.height !== frame.displayHeight) {
                      capturedCanvas.width = frame.displayWidth;
                      capturedCanvas.height = frame.displayHeight;
                    }
                    capturedCtx.drawImage(frame, 0, 0);
                    console.log('[SSE Video] Frame drawn to canvas');
                  } else {
                    console.error('[SSE Video] Canvas or context is null!', { ctx: !!capturedCtx, canvas: !!capturedCanvas });
                  }
                  frame.close();
                },
                error: (err) => {
                  console.error('[SSE Video] === DECODER ERROR ===');
                  console.error('[SSE Video] Decoder error:', err);
                },
              });

              // Add dequeue event listener for debugging
              decoder.addEventListener('dequeue', () => {
                console.log('[SSE Video] Dequeue event - decodeQueueSize:', decoder.decodeQueueSize);
              });
              // Configure decoder with Annex B format for H264/H265 (in-band SPS/PPS/VPS)
              // This tells WebCodecs to expect NAL start codes and in-band parameter sets
              const decoderConfig: VideoDecoderConfig = {
                codec: codecString,
                codedWidth: init.width,
                codedHeight: init.height,
                hardwareAcceleration: useHardwareAcceleration,
              };

              // For H264, specify Annex B format to handle in-band SPS/PPS
              if (codecString.startsWith('avc1')) {
                // @ts-ignore - avc property is part of the spec but not in TypeScript types yet
                decoderConfig.avc = { format: 'annexb' };
              }
              // For HEVC, specify Annex B format to handle in-band VPS/SPS/PPS
              if (codecString.startsWith('hvc1') || codecString.startsWith('hev1')) {
                // @ts-ignore - hevc property for Annex B format
                decoderConfig.hevc = { format: 'annexb' };
              }

              decoder.configure(decoderConfig);
              console.log('[SSE Video] Decoder configured:', decoderConfig);
              sseVideoDecoderRef.current = decoder;
            } catch (err) {
              console.error('[MoonlightStreamViewer] Failed to parse SSE init:', err);
            }
          });

          eventSource.addEventListener('video', (e: MessageEvent) => {
            const decoder = sseVideoDecoderRef.current;
            if (!decoder || decoder.state !== 'configured') return;

            try {
              const frame = JSON.parse(e.data);

              // Skip delta frames until we receive the first keyframe
              // (keyframe contains in-band VPS/SPS/PPS needed for HEVC decoding)
              if (!sseReceivedFirstKeyframeRef.current) {
                if (!frame.keyframe) {
                  console.log('[SSE Video] Waiting for first keyframe, skipping delta frame');
                  return;
                }
                console.log('[SSE Video] First keyframe received');
                sseReceivedFirstKeyframeRef.current = true;
              }

              // Debug: check incoming data size
              const base64Length = frame.data?.length || 0;
              if (base64Length < 2000) {
                console.warn('[SSE Video] Suspiciously small frame data:', base64Length, 'base64 chars, keyframe:', frame.keyframe);
              }

              const binaryString = atob(frame.data);
              const bytes = new Uint8Array(binaryString.length);
              for (let i = 0; i < binaryString.length; i++) {
                bytes[i] = binaryString.charCodeAt(i);
              }

              const chunk = new EncodedVideoChunk({
                type: frame.keyframe ? 'key' : 'delta',
                timestamp: frame.pts,
                data: bytes,
              });
              console.log('[SSE Video] BEFORE decode - state:', decoder.state, 'queueSize:', decoder.decodeQueueSize);
              console.log('[SSE Video] Decoding chunk:', chunk.type, 'size:', bytes.length, 'pts:', frame.pts);

              try {
                decoder.decode(chunk);
                console.log('[SSE Video] AFTER decode - state:', decoder.state, 'queueSize:', decoder.decodeQueueSize);
              } catch (decodeErr) {
                console.error('[SSE Video] decode() threw exception:', decodeErr);
              }

              // Check decoder state after decode
              if (decoder.decodeQueueSize > 10) {
                console.warn('[SSE Video] Decoder queue backing up:', decoder.decodeQueueSize);
              }

              // Try flush every 60 frames to force output (debugging)
              // Use a closure variable since this is not a class context
              const currentFrameCount = (window as any).__sseFrameCount || 0;
              (window as any).__sseFrameCount = currentFrameCount + 1;
              if ((currentFrameCount + 1) % 60 === 0) {
                console.log('[SSE Video] Calling flush() after 60 frames');
                decoder.flush().then(() => {
                  console.log('[SSE Video] flush() completed, queueSize:', decoder.decodeQueueSize);
                }).catch((flushErr) => {
                  console.error('[SSE Video] flush() failed:', flushErr);
                });
              }
            } catch (err) {
              console.error('[MoonlightStreamViewer] Failed to decode SSE video frame:', err);
            }
          });

          eventSource.addEventListener('stop', () => {
            console.log('[MoonlightStreamViewer] SSE video stopped');
            if (sseVideoDecoderRef.current) {
              sseVideoDecoderRef.current.close();
              sseVideoDecoderRef.current = null;
            }
          });

          eventSource.onerror = (err) => {
            console.error('[MoonlightStreamViewer] SSE video error:', err);
          };
        }
      } else if (newMode === 'websocket') {
        // Switching to WebSocket mode: close SSE, enable WS video
        console.log('[MoonlightStreamViewer] Hot-switching to WebSocket mode (no reconnect)');

        // Close SSE connection
        if (sseEventSourceRef.current) {
          sseEventSourceRef.current.close();
          sseEventSourceRef.current = null;
        }
        if (sseVideoDecoderRef.current) {
          sseVideoDecoderRef.current.close();
          sseVideoDecoderRef.current = null;
        }

        // Re-enable video on WebSocket
        wsStream.setVideoEnabled(true);

        // Re-attach canvas to stream for rendering
        if (canvasRef.current) {
          wsStream.setCanvas(canvasRef.current);
        }
      }
    } else {
      // Full reconnect needed (switching to/from webrtc or not connected)
      console.log('[MoonlightStreamViewer] Full reconnect needed for mode switch');
      console.log('[MoonlightStreamViewer] Using 5s delay to wait for moonlight-web cleanup');
      reconnect(5000);
    }
  }, [streamingMode, reconnect, isConnected, sessionId]);

  // Track previous quality mode for reconnection
  const previousQualityModeRef = useRef<'adaptive' | 'high' | 'low'>(qualityMode);

  // Reconnect when quality mode changes (user toggled fps/quality)
  // Uses 5-second delay to wait for moonlight-web session cleanup (prevents Wolf conflicts)
  useEffect(() => {
    if (previousQualityModeRef.current !== qualityMode) {
      console.log('[MoonlightStreamViewer] Quality mode changed from', previousQualityModeRef.current, 'to', qualityMode);
      console.log('[MoonlightStreamViewer] Using 5s delay to wait for moonlight-web cleanup');
      previousQualityModeRef.current = qualityMode;
      // Update fallback state immediately for UI feedback
      setIsOnFallback(qualityMode === 'low');
      // Reset adaptive lock when user manually changes mode
      // This allows adaptive mode to start fresh and evaluate latency again
      setAdaptiveLockedToScreenshots(false);
      setAdaptiveScreenshotEnabled(false);
      reconnect(5000); // 5 seconds for mode switches to allow cleanup
    }
  }, [qualityMode, reconnect]);

  // Track previous user bitrate for reconnection
  // Initialize to a sentinel value (-1) to distinguish "not yet set" from "set to null"
  const previousUserBitrateRef = useRef<number | null | undefined>(undefined);

  // Reconnect when user bitrate changes (user selected new bitrate or adaptive reduction)
  // IMPORTANT: Skip reconnect during initial connection (hasConnectedRef guards this)
  // The initial bandwidth probe sets userBitrate BEFORE calling connect(), so we must not
  // trigger a reconnect on that first bitrate change or we'll get double-connections
  useEffect(() => {
    // Skip on first render (previousUserBitrateRef is undefined)
    if (previousUserBitrateRef.current === undefined) {
      previousUserBitrateRef.current = userBitrate;
      return;
    }
    // Skip if we're in the middle of initial connection (started but not yet connected)
    // hasConnectedRef.current = true means we started initial connection
    // isConnected = false means connection not complete yet
    // In this state, the bitrate change came from the initial bandwidth probe, not user action
    if (hasConnectedRef.current && !isConnected) {
      console.log('[MoonlightStreamViewer] Skipping bitrate-change reconnect (initial connection in progress)');
      previousUserBitrateRef.current = userBitrate;
      return;
    }
    // Reconnect if bitrate actually changed (including from null to a value)
    if (previousUserBitrateRef.current !== userBitrate) {
      console.log('[MoonlightStreamViewer] Bitrate changed from', previousUserBitrateRef.current, 'to', userBitrate);
      console.log('[MoonlightStreamViewer] Reconnecting with new bitrate (5s delay)');
      reconnect(5000); // 5 seconds for bitrate switches to allow cleanup
    }
    previousUserBitrateRef.current = userBitrate;
  }, [userBitrate, reconnect, isConnected]);

  // Detect lobby changes and reconnect (for test script restart scenarios)
  // Uses 5-second delay to wait for moonlight-web cleanup before connecting to new lobby
  useEffect(() => {
    if (wolfLobbyId && previousLobbyIdRef.current && previousLobbyIdRef.current !== wolfLobbyId) {
      console.log('[MoonlightStreamViewer] Lobby changed from', previousLobbyIdRef.current, 'to', wolfLobbyId);
      console.log('[MoonlightStreamViewer] Disconnecting old stream and reconnecting to new lobby (5s delay)');
      reconnect(5000); // 5 seconds for lobby switches to allow cleanup
    }
    previousLobbyIdRef.current = wolfLobbyId;
  }, [wolfLobbyId, reconnect]);

  // Initial bandwidth probe - runs BEFORE session exists to determine optimal starting bitrate
  // Uses /api/v1/bandwidth-probe which only requires auth (no session ownership)
  // Returns measured throughput in Mbps (0 on failure)
  const runInitialBandwidthProbe = useCallback(async (): Promise<number> => {
    console.log(`[AdaptiveBitrate] Running INITIAL bandwidth probe (before session creation)...`);

    try {
      // Fire parallel requests to fill the TCP pipe (same logic as session probe)
      const probeCount = 5;
      const probeSize = 524288; // 512KB per request (max 2MB for initial probe on server)
      const startTime = performance.now();

      const probePromises = Array.from({ length: probeCount }, (_, i) =>
        fetch(`/api/v1/bandwidth-probe?size=${probeSize}`)
          .then(response => {
            if (!response.ok) {
              console.warn(`[AdaptiveBitrate] Initial probe request ${i + 1} failed: ${response.status}`);
              return 0;
            }
            return response.arrayBuffer().then(buf => buf.byteLength);
          })
          .catch(err => {
            console.warn(`[AdaptiveBitrate] Initial probe request ${i + 1} error:`, err);
            return 0;
          })
      );

      const sizes = await Promise.all(probePromises);
      const totalBytes = sizes.reduce((a, b) => a + b, 0);

      const elapsedMs = performance.now() - startTime;
      const elapsedSec = elapsedMs / 1000;
      const throughputMbps = (totalBytes * 8) / (1000000 * elapsedSec);

      console.log(`[AdaptiveBitrate] Initial probe complete: ${(totalBytes / 1024).toFixed(0)} KB in ${elapsedMs.toFixed(0)}ms = ${throughputMbps.toFixed(1)} Mbps`);

      return throughputMbps;
    } catch (err) {
      console.warn('[AdaptiveBitrate] Initial probe failed:', err);
      return 0;
    }
  }, []);

  // Calculate optimal bitrate from measured throughput (with 25% headroom)
  const calculateOptimalBitrate = useCallback((throughputMbps: number): number => {
    const BITRATE_OPTIONS = [5, 10, 20, 40, 80];
    const maxSustainableBitrate = throughputMbps / 1.25;

    // Find highest bitrate option that fits
    for (let i = BITRATE_OPTIONS.length - 1; i >= 0; i--) {
      if (BITRATE_OPTIONS[i] <= maxSustainableBitrate) {
        return BITRATE_OPTIONS[i];
      }
    }
    return BITRATE_OPTIONS[0]; // Default to lowest
  }, []);

  // Auto-connect when wolfLobbyId becomes available
  // wolfLobbyId is fetched asynchronously from session data, so it's undefined on initial render
  // If we connect before it's available, we use the wrong app_id (apps mode instead of lobbies mode)
  // NEW: Probe bandwidth FIRST, then connect at optimal bitrate (avoids reconnect on startup)
  const hasConnectedRef = useRef(false);
  useEffect(() => {
    // Only auto-connect once
    if (hasConnectedRef.current) return;

    // If wolfLobbyId prop is expected but not yet loaded, wait for it
    // We detect this by checking if sessionId is provided (external agent mode)
    // In this mode, wolfLobbyId should be provided by the parent once session data loads
    if (sessionId && !isPersonalDevEnvironment && !wolfLobbyId) {
      console.log('[MoonlightStreamViewer] Waiting for wolfLobbyId to load before connecting...');
      return;
    }

    // Probe bandwidth BEFORE connecting to start at optimal bitrate
    const probeAndConnect = async () => {
      hasConnectedRef.current = true;

      // Show connecting overlay so user sees the probe status
      setIsConnecting(true);
      console.log('[MoonlightStreamViewer] Probing bandwidth before initial connection...');
      setStatus('Measuring bandwidth...');

      const throughput = await runInitialBandwidthProbe();

      if (throughput > 0) {
        const optimalBitrate = calculateOptimalBitrate(throughput);
        console.log(`[MoonlightStreamViewer] Initial probe: ${throughput.toFixed(1)} Mbps → starting at ${optimalBitrate} Mbps`);
        setUserBitrate(optimalBitrate);
        setRequestedBitrate(optimalBitrate);
      } else {
        console.log('[MoonlightStreamViewer] Initial probe failed, using default 10 Mbps');
      }

      console.log('[MoonlightStreamViewer] Auto-connecting with wolfLobbyId:', wolfLobbyId);
      connect();
    };

    probeAndConnect();
  }, [wolfLobbyId, sessionId, isPersonalDevEnvironment, connect, runInitialBandwidthProbe, calculateOptimalBitrate]);

  // Cleanup on unmount
  useEffect(() => {
    console.log('[MoonlightStreamViewer] Component mounted, setting up cleanup handler');
    return () => {
      console.log('[MoonlightStreamViewer] Component unmounting, calling disconnect()');
      disconnect();
    };
  }, [disconnect]);

  // Auto-focus container when stream connects for keyboard input
  useEffect(() => {
    if (isConnected && containerRef.current) {
      containerRef.current.focus();
    }
  }, [isConnected]);

  // Screenshot polling for low-quality mode OR adaptive mode with high latency
  // Targets 2 FPS minimum (500ms max per frame)
  // Dynamically adjusts JPEG quality based on fetch time
  const shouldPollScreenshots = qualityMode === 'low' || (qualityMode === 'adaptive' && adaptiveScreenshotEnabled);

  // Notify server to pause/resume video when entering/exiting screenshot mode
  // This saves bandwidth by not sending video frames we won't display
  useEffect(() => {
    const stream = streamRef.current;
    if (!stream || !(stream instanceof WebSocketStream) || !isConnected) {
      return;
    }

    if (shouldPollScreenshots) {
      console.log('[MoonlightStreamViewer] Entering screenshot mode - pausing video stream');
      stream.setVideoEnabled(false);
    } else {
      console.log('[MoonlightStreamViewer] Exiting screenshot mode - resuming video stream');
      stream.setVideoEnabled(true);
    }
  }, [shouldPollScreenshots, isConnected]);

  useEffect(() => {
    // Only poll screenshots when needed
    if (!shouldPollScreenshots || !isConnected || !sessionId) {
      // Clean up old screenshot URL when exiting screenshot mode
      if (screenshotUrl) {
        URL.revokeObjectURL(screenshotUrl);
        setScreenshotUrl(null);
      }
      // Reset quality to default when exiting
      screenshotQualityRef.current = 70;
      setScreenshotQuality(70);
      setScreenshotFps(0);
      return;
    }

    console.log('[MoonlightStreamViewer] Starting screenshot polling:', qualityMode === 'low' ? 'low mode' : 'adaptive fallback');

    let isPolling = true;
    let lastFrameTime = Date.now();
    let frameCount = 0;
    let fpsStartTime = Date.now();

    const fetchScreenshot = async () => {
      if (!isPolling) return;

      const startTime = Date.now();
      const currentQuality = screenshotQualityRef.current;

      try {
        // Pass quality parameter to screenshot endpoint
        const endpoint = `/api/v1/external-agents/${sessionId}/screenshot?format=jpeg&quality=${currentQuality}`;
        const response = await fetch(endpoint);

        if (!response.ok) {
          console.warn('[MoonlightStreamViewer] Screenshot fetch failed:', response.status);
          // Schedule next fetch after a short delay on error
          if (isPolling) setTimeout(fetchScreenshot, 200);
          return;
        }

        const blob = await response.blob();
        const fetchTime = Date.now() - startTime;
        const newUrl = URL.createObjectURL(blob);

        // Update FPS counter
        frameCount++;
        const elapsedSinceStart = Date.now() - fpsStartTime;
        if (elapsedSinceStart >= 1000) {
          const fps = frameCount / (elapsedSinceStart / 1000);
          setScreenshotFps(Math.round(fps * 10) / 10);
          frameCount = 0;
          fpsStartTime = Date.now();
        }

        // Adaptive quality control: target 500ms max per frame (2 FPS minimum)
        // - If fetch took > 500ms: decrease quality to speed up (min 10)
        // - If fetch took < 300ms: increase quality for better image (max 90)
        // - Between 300-500ms: keep current quality (sweet spot)
        let newQuality = currentQuality;
        if (fetchTime > 500) {
          // Too slow - decrease quality aggressively
          newQuality = Math.max(10, currentQuality - 10);
          console.log(`[Screenshot] Slow fetch (${fetchTime}ms), decreasing quality: ${currentQuality} → ${newQuality}`);
        } else if (fetchTime < 300 && currentQuality < 90) {
          // Fast enough - increase quality slightly
          newQuality = Math.min(90, currentQuality + 5);
          // Only log quality increases occasionally to reduce spam
          if (newQuality % 10 === 0) {
            console.log(`[Screenshot] Fast fetch (${fetchTime}ms), increasing quality: ${currentQuality} → ${newQuality}`);
          }
        }

        if (newQuality !== currentQuality) {
          screenshotQualityRef.current = newQuality;
          setScreenshotQuality(newQuality);
        }

        // Preload image before displaying
        const img = new Image();
        img.onload = () => {
          setScreenshotUrl((oldUrl) => {
            if (oldUrl) URL.revokeObjectURL(oldUrl);
            return newUrl;
          });
          // Schedule next frame with rate limiting
          if (isPolling) {
            // Cap at 10 FPS max (100ms minimum interval) to prevent CPU hammering from forking grim
            // If fetch took less than 100ms, wait the remainder; otherwise fetch immediately
            const minInterval = 100; // 10 FPS max
            const delay = Math.max(0, minInterval - fetchTime);
            setTimeout(fetchScreenshot, delay);
          }
        };
        img.onerror = () => {
          URL.revokeObjectURL(newUrl);
          // Retry on error
          if (isPolling) setTimeout(fetchScreenshot, 200);
        };
        img.src = newUrl;
      } catch (err) {
        console.warn('[MoonlightStreamViewer] Screenshot fetch error:', err);
        // Schedule next fetch after a short delay on error
        if (isPolling) setTimeout(fetchScreenshot, 200);
      }
    };

    // Start continuous polling
    fetchScreenshot();

    return () => {
      isPolling = false;
    };
  }, [shouldPollScreenshots, isConnected, sessionId]);

  // Cleanup screenshot URL on unmount
  useEffect(() => {
    return () => {
      if (screenshotUrl) {
        URL.revokeObjectURL(screenshotUrl);
      }
    };
  }, [screenshotUrl]);

  // Adaptive mode: monitor RTT and auto-enable screenshot overlay when latency is high
  // Once locked to screenshots, stay there until user manually changes mode (prevents oscillation)
  // When we stop sending video, latency drops, which would otherwise trigger switching back
  useEffect(() => {
    if (qualityMode !== 'adaptive' || !isConnected || !streamRef.current) {
      setAdaptiveScreenshotEnabled(false);
      return;
    }

    // If already locked to screenshots, stay there (prevent oscillation)
    if (adaptiveLockedToScreenshots) {
      if (!adaptiveScreenshotEnabled) {
        setAdaptiveScreenshotEnabled(true);
      }
      return;
    }

    const ENABLE_THRESHOLD_MS = 150;  // Enable screenshot overlay when RTT > 150ms
    const CHECK_INTERVAL_MS = 1000;   // Check every second

    const checkRtt = () => {
      const stream = streamRef.current;
      if (!stream || !(stream instanceof WebSocketStream)) return;

      const wsStats = stream.getStats();
      const rtt = wsStats.rttMs;

      // Only check for enabling - once enabled, we lock in (user must manually switch back)
      if (rtt > ENABLE_THRESHOLD_MS && !adaptiveScreenshotEnabled) {
        console.log(`[Adaptive] High latency detected (${rtt.toFixed(0)}ms > ${ENABLE_THRESHOLD_MS}ms), locking to screenshot mode`);
        console.log(`[Adaptive] To try video again, manually switch to High mode then back to Adaptive`);
        setAdaptiveScreenshotEnabled(true);
        setAdaptiveLockedToScreenshots(true);  // Lock in - prevent oscillation
      }
    };

    const intervalId = setInterval(checkRtt, CHECK_INTERVAL_MS);
    checkRtt(); // Initial check

    return () => clearInterval(intervalId);
  }, [qualityMode, isConnected, adaptiveScreenshotEnabled, adaptiveLockedToScreenshots]);

  // Adaptive bitrate: reduce based on frame drift, increase based on bandwidth probe
  // Frame drift = how late frames are arriving - reliable indicator of congestion
  // Bandwidth probe = active test before increasing to verify headroom exists
  const stableCheckCountRef = useRef(0); // Count of checks with low frame drift
  const congestionCheckCountRef = useRef(0); // Count of consecutive checks with high frame drift (dampening)
  const lastBitrateChangeRef = useRef(0);
  const bandwidthProbeInProgressRef = useRef(false); // Prevent concurrent probes

  // Bandwidth probe: actively test available bandwidth before increasing bitrate
  // Fetches random data and measures throughput to verify headroom exists
  // Uses PARALLEL requests to fill high-BDP pipes (critical for high-latency links like satellite/VPN)
  // NOTE: Uses dedicated bandwidth-probe endpoint that returns random bytes immediately,
  // unlike screenshot which has capture latency before bytes start flowing
  // Returns measured throughput in Mbps (0 on failure)
  const runBandwidthProbe = useCallback(async (): Promise<number> => {
    if (!sessionId || bandwidthProbeInProgressRef.current) {
      return 0;
    }

    bandwidthProbeInProgressRef.current = true;
    console.log(`[AdaptiveBitrate] Running bandwidth probe...`);

    try {
      // Fetch random data IN PARALLEL to fill the TCP pipe faster
      // Sequential requests on high-RTT links never reach steady-state throughput
      // Each request fetches 512KB of random incompressible data
      const probeCount = 5; // 5 parallel requests = 2.5MB total
      const probeSize = 524288; // 512KB per request
      const startTime = performance.now();

      // Fire all requests simultaneously
      const probePromises = Array.from({ length: probeCount }, (_, i) =>
        fetch(`/api/v1/external-agents/${sessionId}/bandwidth-probe?size=${probeSize}`)
          .then(response => {
            if (!response.ok) {
              console.warn(`[AdaptiveBitrate] Probe request ${i + 1} failed: ${response.status}`);
              return 0;
            }
            return response.arrayBuffer().then(buf => buf.byteLength);
          })
          .catch(err => {
            console.warn(`[AdaptiveBitrate] Probe request ${i + 1} error:`, err);
            return 0;
          })
      );

      const sizes = await Promise.all(probePromises);
      const totalBytes = sizes.reduce((a, b) => a + b, 0);

      const elapsedMs = performance.now() - startTime;
      const elapsedSec = elapsedMs / 1000;
      const throughputMbps = (totalBytes * 8) / (1000000 * elapsedSec);

      console.log(`[AdaptiveBitrate] Probe complete: ${(totalBytes / 1024).toFixed(0)} KB in ${elapsedMs.toFixed(0)}ms = ${throughputMbps.toFixed(1)} Mbps`);

      return throughputMbps;
    } catch (err) {
      console.warn('[AdaptiveBitrate] Probe failed:', err);
      return 0;
    } finally {
      bandwidthProbeInProgressRef.current = false;
    }
  }, [sessionId]);

  useEffect(() => {
    // Support both WebSocket and SSE modes for adaptive bitrate
    if (!isConnected || !streamRef.current) {
      stableCheckCountRef.current = 0;
      return;
    }

    // Only WebSocket and SSE modes support adaptive bitrate (WebRTC has its own congestion control)
    if (streamingMode !== 'websocket' && streamingMode !== 'sse') {
      return;
    }

    const CHECK_INTERVAL_MS = 1000;       // Check every second
    const REDUCE_COOLDOWN_MS = 10000;     // Don't reduce again within 10 seconds
    const INCREASE_COOLDOWN_MS = 30000;   // Don't increase again within 30 seconds
    const MANUAL_SELECTION_COOLDOWN_MS = 20000;  // Don't auto-reduce within 20s of user manually selecting bitrate
    const BITRATE_OPTIONS = [5, 10, 20, 40, 80]; // Available bitrates in ascending order
    const MIN_BITRATE = 5;
    const STABLE_CHECKS_FOR_INCREASE = 20; // Need 20 seconds of low frame drift before trying increase
    const CONGESTION_CHECKS_FOR_REDUCE = 3; // Need 3 consecutive high drift samples before reducing (dampening)
    const FRAME_DRIFT_THRESHOLD = 200;    // Reduce if frames arriving > 200ms late (positive drift = behind)

    const checkBandwidth = () => {
      const stream = streamRef.current;
      if (!stream) return;

      // Get frame drift from stream stats (the reliable metric for congestion detection)
      // Frame drift = how late frames are arriving compared to their PTS
      // Positive = frames arriving late (congestion), Negative = frames arriving early (buffered)
      let frameDrift = 0;

      if (stream instanceof WebSocketStream) {
        const stats = stream.getStats();
        frameDrift = stats.frameLatencyMs;
      } else if (stream instanceof SseStream) {
        // SSE doesn't have frame drift, skip adaptive bitrate for SSE
        return;
      } else {
        return; // Unsupported stream type
      }

      const currentBitrate = userBitrate || requestedBitrate;
      const now = Date.now();

      // Skip auto-changes if user manually selected bitrate within cooldown period
      // This lets the stream settle after user explicitly chooses a bitrate
      const timeSinceManualSelection = now - manualBitrateSelectionTimeRef.current;
      if (timeSinceManualSelection < MANUAL_SELECTION_COOLDOWN_MS) {
        return; // Don't make any bitrate changes during cooldown
      }

      // Frame drift congestion detection:
      // - Positive drift means frames are arriving late (we're falling behind)
      // - This is a reliable indicator of network congestion or encoder overload
      // - Unlike throughput, this isn't affected by H.264 compression efficiency
      const congestionDetected = frameDrift > FRAME_DRIFT_THRESHOLD;

      // Reduce bitrate on sustained high frame drift (dampening prevents single-spike reductions)
      if (congestionDetected && currentBitrate > MIN_BITRATE) {
        // Increment congestion counter - require multiple consecutive high drift samples
        congestionCheckCountRef.current++;
        stableCheckCountRef.current = 0; // Reset stable counter on any congestion

        // Only reduce if we've seen sustained congestion
        if (congestionCheckCountRef.current >= CONGESTION_CHECKS_FOR_REDUCE) {
          const timeSinceLastChange = now - lastBitrateChangeRef.current;

          if (timeSinceLastChange > REDUCE_COOLDOWN_MS) {
            // Step down one tier
            const currentIndex = BITRATE_OPTIONS.indexOf(currentBitrate);
            if (currentIndex > 0) {
              const newBitrate = BITRATE_OPTIONS[currentIndex - 1];
              console.log(`[AdaptiveBitrate] Sustained high frame drift (${congestionCheckCountRef.current} samples, ${frameDrift.toFixed(0)}ms), reducing: ${currentBitrate} -> ${newBitrate} Mbps`);
              addChartEvent('rtt_spike', `Sustained drift ${frameDrift.toFixed(0)}ms, reducing ${currentBitrate}→${newBitrate} Mbps`);

              lastBitrateChangeRef.current = now;
              stableCheckCountRef.current = 0;
              congestionCheckCountRef.current = 0; // Reset after reduction
              setUserBitrate(newBitrate);
              return;
            }
          }
        }
      } else {
        // Low frame drift - connection is stable at current bitrate
        congestionCheckCountRef.current = 0; // Reset congestion counter on good sample
        stableCheckCountRef.current++;

        // Try to increase if stable for a while
        if (stableCheckCountRef.current >= STABLE_CHECKS_FOR_INCREASE) {
          const timeSinceLastChange = now - lastBitrateChangeRef.current;

          if (timeSinceLastChange > INCREASE_COOLDOWN_MS) {
            const currentIndex = BITRATE_OPTIONS.indexOf(currentBitrate);

            if (currentIndex !== -1 && currentIndex < BITRATE_OPTIONS.length - 1) {
              // Run bandwidth probe to measure actual throughput
              // Then jump directly to the highest sustainable bitrate (not just +1 tier)
              console.log(`[AdaptiveBitrate] Stable for ${stableCheckCountRef.current}s, probing to find optimal bitrate...`);

              // Mark that we're attempting an increase (prevent re-triggering during probe)
              stableCheckCountRef.current = 0;

              runBandwidthProbe().then((measuredThroughputMbps) => {
                if (measuredThroughputMbps <= 0) {
                  console.log(`[AdaptiveBitrate] Probe failed, staying at ${currentBitrate} Mbps`);
                  lastBitrateChangeRef.current = Date.now();
                  return;
                }

                // Calculate max sustainable bitrate with 25% headroom
                // If we measure 100 Mbps, we can sustain 100/1.25 = 80 Mbps
                const maxSustainableBitrate = measuredThroughputMbps / 1.25;

                // Find the highest BITRATE_OPTIONS that fits
                let targetBitrate = currentBitrate;
                for (let i = BITRATE_OPTIONS.length - 1; i >= 0; i--) {
                  if (BITRATE_OPTIONS[i] <= maxSustainableBitrate && BITRATE_OPTIONS[i] > currentBitrate) {
                    targetBitrate = BITRATE_OPTIONS[i];
                    break;
                  }
                }

                if (targetBitrate > currentBitrate) {
                  console.log(`[AdaptiveBitrate] Probe measured ${measuredThroughputMbps.toFixed(1)} Mbps → max sustainable ${maxSustainableBitrate.toFixed(1)} Mbps`);
                  console.log(`[AdaptiveBitrate] Jumping directly: ${currentBitrate} → ${targetBitrate} Mbps`);
                  addChartEvent('increase', `Probe: ${measuredThroughputMbps.toFixed(0)} Mbps, jumping ${currentBitrate}→${targetBitrate} Mbps`);
                  lastBitrateChangeRef.current = Date.now();
                  setUserBitrate(targetBitrate);
                } else {
                  console.log(`[AdaptiveBitrate] Probe measured ${measuredThroughputMbps.toFixed(1)} Mbps → max sustainable ${maxSustainableBitrate.toFixed(1)} Mbps (not enough for next tier)`);
                  lastBitrateChangeRef.current = Date.now();
                }
              });
            }
          }
        }
      }
    };

    const intervalId = setInterval(checkBandwidth, CHECK_INTERVAL_MS);

    return () => clearInterval(intervalId);
  }, [isConnected, streamingMode, userBitrate, requestedBitrate, runBandwidthProbe, addChartEvent]);

  // Auto-join lobby after video starts playing
  // Backend API polls Wolf sessions to wait for pipeline switch to complete before returning
  useEffect(() => {
    if (!pendingAutoJoin || !sessionId) return;

    const doAutoJoin = async () => {
      try {
        console.log('[AUTO-JOIN] Triggering auto-join (backend waits for pipeline switch)');
        const apiClient = helixApi.getApiClient();
        const response = await apiClient.v1ExternalAgentsAutoJoinLobbyCreate(sessionId);

        if (response.status === 200) {
          console.log('[AUTO-JOIN] ✅ Successfully auto-joined lobby:', response.data);
        } else {
          console.warn('[AUTO-JOIN] Failed to auto-join lobby. Status:', response.status);
        }
      } catch (err: any) {
        console.error('[AUTO-JOIN] Error calling auto-join endpoint:', err);
        console.error('[AUTO-JOIN] User can still manually join lobby via Wolf UI');
      } finally {
        setPendingAutoJoin(false);
      }
    };

    doAutoJoin();
  }, [pendingAutoJoin, sessionId, streamingMode]);

  // Track container size for canvas aspect ratio calculation
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        setContainerSize({ width, height });
      }
    });

    resizeObserver.observe(container);
    return () => resizeObserver.disconnect();
  }, []);

  // Calculate proper canvas display size to maintain aspect ratio
  useEffect(() => {
    if (!containerSize || !canvasRef.current) return;

    // Get the actual canvas internal dimensions (set by WebCodecs when frames are rendered)
    const canvas = canvasRef.current;
    const canvasWidth = canvas.width || 1920;  // Default to 1080p if not yet set
    const canvasHeight = canvas.height || 1080;

    if (canvasWidth === 0 || canvasHeight === 0) return;

    const containerWidth = containerSize.width;
    const containerHeight = containerSize.height;

    const canvasAspect = canvasWidth / canvasHeight;
    const containerAspect = containerWidth / containerHeight;

    let displayWidth: number;
    let displayHeight: number;

    if (containerAspect > canvasAspect) {
      // Container is wider than canvas aspect - height is the limiting factor
      displayHeight = containerHeight;
      displayWidth = displayHeight * canvasAspect;
    } else {
      // Container is taller than canvas aspect - width is the limiting factor
      displayWidth = containerWidth;
      displayHeight = displayWidth / canvasAspect;
    }

    setCanvasDisplaySize({ width: displayWidth, height: displayHeight });
  }, [containerSize]);

  // Update canvas display size when canvas dimensions change (after first frame is rendered)
  useEffect(() => {
    if (!containerSize || !canvasRef.current || streamingMode !== 'websocket') return;

    const checkCanvasDimensions = () => {
      const canvas = canvasRef.current;
      if (!canvas || canvas.width === 0 || canvas.height === 0) return;

      const containerWidth = containerSize.width;
      const containerHeight = containerSize.height;
      const canvasAspect = canvas.width / canvas.height;
      const containerAspect = containerWidth / containerHeight;

      let displayWidth: number;
      let displayHeight: number;

      if (containerAspect > canvasAspect) {
        displayHeight = containerHeight;
        displayWidth = displayHeight * canvasAspect;
      } else {
        displayWidth = containerWidth;
        displayHeight = displayWidth / canvasAspect;
      }

      setCanvasDisplaySize({ width: displayWidth, height: displayHeight });
    };

    // Check periodically until canvas has dimensions
    const interval = setInterval(checkCanvasDimensions, 100);
    checkCanvasDimensions();

    return () => clearInterval(interval);
  }, [containerSize, streamingMode, isConnected]);

  // Auto-sync clipboard from remote → local every 2 seconds
  useEffect(() => {
    if (!isConnected || !sessionId) return;

    const syncClipboard = async () => {
      try {
        const apiClient = helixApi.getApiClient();
        const response = await apiClient.v1ExternalAgentsClipboardDetail(sessionId);
        const clipboardData: TypesClipboardData = response.data;

        // Skip if clipboard is empty or malformed
        if (!clipboardData || !clipboardData.type || !clipboardData.data) {
          return;
        }

        // Hash the clipboard data to detect changes
        const hash = `${clipboardData.type}:${clipboardData.data.substring(0, 100)}`;
        if (hash === lastRemoteClipboardHash.current) {
          return; // No change, skip update
        }

        if (clipboardData.type === 'image') {
          // Decode base64 image and write to browser clipboard
          const base64Data = clipboardData.data;
          const byteCharacters = atob(base64Data);
          const byteNumbers = new Array(byteCharacters.length);
          for (let i = 0; i < byteCharacters.length; i++) {
            byteNumbers[i] = byteCharacters.charCodeAt(i);
          }
          const byteArray = new Uint8Array(byteNumbers);
          const blob = new Blob([byteArray], { type: 'image/png' });

          await navigator.clipboard.write([
            new ClipboardItem({ 'image/png': blob })
          ]);

          console.log(`[Clipboard] Auto-synced image from remote (${byteArray.length} bytes)`);
        } else if (clipboardData.type === 'text') {
          // Write text to browser clipboard
          await navigator.clipboard.writeText(clipboardData.data);
          console.log(`[Clipboard] Auto-synced text from remote (${clipboardData.data.length} chars)`);
        }

        lastRemoteClipboardHash.current = hash;
      } catch (err) {
        // Silent failure - don't spam console with clipboard sync errors
        // Only log if not a 404 (container might not be ready yet)
        if (err && !String(err).includes('404')) {
          console.warn('[Clipboard] Auto-sync failed:', err);
        }
      }
    };

    // Initial sync
    syncClipboard();

    // Poll every 2 seconds
    const syncInterval = setInterval(syncClipboard, 2000);

    return () => clearInterval(syncInterval);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isConnected, sessionId]); // Don't include helixApi - it's not reactive

  // Prevent page scroll on wheel events inside viewer (native listener with passive: false)
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const wheelHandler = (event: WheelEvent) => {
      event.preventDefault();
      event.stopPropagation();
      // Send to stream - use SSE input WebSocket if in SSE mode, otherwise main stream
      const input = sseInputWsRef.current?.getInput() ??
        (streamRef.current && 'getInput' in streamRef.current
          ? (streamRef.current as WebSocketStream | Stream | DualStreamManager).getInput()
          : null);
      input?.onMouseWheel(event);
    };

    // CRITICAL: Use { passive: false } to allow preventDefault() on wheel events
    // Chrome makes wheel events passive by default, which prevents preventDefault()
    container.addEventListener('wheel', wheelHandler, { passive: false });

    return () => {
      container.removeEventListener('wheel', wheelHandler);
    };
  }, []);

  // Handle audio toggle
  useEffect(() => {
    if (videoRef.current) {
      // Mute/unmute video element
      videoRef.current.muted = !audioEnabled;
    }
  }, [audioEnabled]);

  // Poll WebRTC stats when stats overlay is visible
  useEffect(() => {
    if (!showStats || !streamRef.current) {
      return;
    }

    // WebSocket and SSE modes - poll stats from WebSocketStream or DualStreamManager
    // SSE mode also uses WebSocketStream for input and session management
    if (streamingMode === 'websocket' || streamingMode === 'sse') {
      const pollWsStats = () => {
        const currentStream = streamRef.current;
        if (!currentStream) return;

        // Handle DualStreamManager
        if (currentStream instanceof DualStreamManager) {
          const dualStats = currentStream.getStats();
          setStats({
            video: {
              codec: `H264 (WebSocket${dualStats.activeStream === 'fallback' ? ' - Low Quality' : ''})`,
              width: dualStats.width,
              height: dualStats.height,
              fps: dualStats.fps,
              videoPayloadBitrate: dualStats.videoPayloadBitrateMbps.toFixed(2),
              totalBitrate: dualStats.totalBitrateMbps.toFixed(2),
              framesDecoded: dualStats.framesDecoded,
              framesDropped: dualStats.framesDropped,
              rttMs: dualStats.rttMs,
              isHighLatency: dualStats.isHighLatency,
              // Additional dual-stream info
              activeStream: dualStats.activeStream,
              primaryRttMs: dualStats.primaryRttMs,
              fallbackRttMs: dualStats.fallbackRttMs,
            },
            connection: {
              transport: `WebSocket (L7) - ${dualStats.activeStream === 'primary' ? '60fps' : '~1fps'}`,
            },
            timestamp: new Date().toISOString(),
          });
          setIsHighLatency(dualStats.isHighLatency);
          setIsOnFallback(dualStats.activeStream === 'fallback');
          return;
        }

        // Handle regular WebSocketStream
        const wsStream = currentStream as WebSocketStream;
        const wsStats = wsStream.getStats();
        const isForcedLow = qualityMode === 'low';
        setStats({
          video: {
            codec: `H264 (WebSocket${isForcedLow ? ' - ~1fps' : ''})`,
            width: wsStats.width,
            height: wsStats.height,
            fps: wsStats.fps,
            videoPayloadBitrate: wsStats.videoPayloadBitrateMbps.toFixed(2),  // H.264 only
            totalBitrate: wsStats.totalBitrateMbps.toFixed(2),                 // Everything
            framesDecoded: wsStats.framesDecoded,
            framesDropped: wsStats.framesDropped,
            rttMs: wsStats.rttMs,                                              // RTT in ms
            isHighLatency: wsStats.isHighLatency,                              // High latency flag
            // Batching stats for congestion visibility
            batchingRatio: wsStats.batchingRatio,                              // % of frames batched
            avgBatchSize: wsStats.avgBatchSize,                                // Avg frames per batch
            batchesReceived: wsStats.batchesReceived,                          // Total batches
            // Frame latency and decoder queue (new metrics for debugging)
            frameLatencyMs: wsStats.frameLatencyMs,                            // Actual frame delivery delay
            batchingRequested: wsStats.batchingRequested,                      // Client requested batching
            decodeQueueSize: wsStats.decodeQueueSize,                          // Decoder queue depth
            maxDecodeQueueSize: wsStats.maxDecodeQueueSize,                    // Peak queue size
            framesSkippedToKeyframe: wsStats.framesSkippedToKeyframe,          // Frames flushed when skipping to keyframe
          },
          connection: {
            transport: streamingMode === 'sse'
              ? 'SSE Video + WebSocket Input'
              : `WebSocket (L7)${isForcedLow ? ' - Force ~1fps' : qualityMode === 'high' ? ' - Force 60fps' : ''}`,
          },
          timestamp: new Date().toISOString(),
        });
        // Update high latency state for warning banner
        setIsHighLatency(wsStats.isHighLatency);
        // Show orange border for forced low quality mode
        setIsOnFallback(isForcedLow);

        // Update chart history (60 seconds of data) - use refs to persist across reconnects
        throughputHistoryRef.current = [...throughputHistoryRef.current, wsStats.totalBitrateMbps].slice(-CHART_HISTORY_LENGTH);
        rttHistoryRef.current = [...rttHistoryRef.current, wsStats.rttMs].slice(-CHART_HISTORY_LENGTH);
        bitrateHistoryRef.current = [...bitrateHistoryRef.current, requestedBitrate].slice(-CHART_HISTORY_LENGTH);
        // Trigger re-render for charts
        if (showCharts) {
          setChartUpdateTrigger(prev => prev + 1);
        }
      };

      // Poll every second
      const interval = setInterval(pollWsStats, 1000);
      pollWsStats(); // Initial call

      return () => clearInterval(interval);
    }

    const pollStats = async () => {
      const peer = (streamRef.current as any)?.getPeer?.();
      if (!peer) {
        console.warn('[Stats] getPeer not available yet');
        return;
      }

      try {
        const statsReport = await peer.getStats();
        const parsedStats: any = {
          video: {},
          connection: {},
          timestamp: new Date().toISOString(),
        };

        let codecInfo = 'unknown';

        statsReport.forEach((report: any) => {
          // Get codec details
          if (report.type === 'codec' && report.mimeType?.includes('video')) {
            const profile = report.sdpFmtpLine?.match(/profile-level-id=([0-9a-fA-F]+)/)?.[1];
            codecInfo = `${report.mimeType}${profile ? ` (${profile})` : ''}`;
          }

          if (report.type === 'inbound-rtp' && report.kind === 'video') {
            // Calculate bitrate from delta
            const now = Date.now();
            const bytes = report.bytesReceived || 0;
            let bitrateMbps = 0;

            if (lastBytesRef.current) {
              const deltaBytes = bytes - lastBytesRef.current.bytes;
              const deltaTime = (now - lastBytesRef.current.timestamp) / 1000; // seconds
              if (deltaTime > 0) {
                bitrateMbps = ((deltaBytes * 8) / 1000000) / deltaTime;
              }
            }

            lastBytesRef.current = { bytes, timestamp: now };

            parsedStats.video = {
              codec: codecInfo,
              width: report.frameWidth,
              height: report.frameHeight,
              fps: report.framesPerSecond?.toFixed(1) || 0,
              bitrate: bitrateMbps.toFixed(2),
              packetsLost: report.packetsLost || 0,
              packetsReceived: report.packetsReceived || 0,
              framesDecoded: report.framesDecoded || 0,
              framesDropped: report.framesDropped || 0,
              jitter: report.jitter ? (report.jitter * 1000).toFixed(2) : 0,
            };
          }
          if (report.type === 'candidate-pair' && report.state === 'succeeded') {
            parsedStats.connection = {
              rtt: report.currentRoundTripTime ? (report.currentRoundTripTime * 1000).toFixed(1) : 0,
              bytesSent: report.bytesSent,
              bytesReceived: report.bytesReceived,
            };
          }
        });

        setStats(parsedStats);
      } catch (err) {
        console.error('[Stats] Failed to get WebRTC stats:', err);
      }
    };

    // Poll every second
    const interval = setInterval(pollStats, 1000);
    pollStats(); // Initial call

    return () => {
      clearInterval(interval);
      lastBytesRef.current = null; // Reset for next time
    };
  }, [showStats, streamingMode, width, height, qualityMode]);

  // Calculate stream rectangle for mouse coordinate mapping
  const getStreamRect = useCallback((): DOMRect => {
    // Check if we're in screenshot mode (screenshot overlay is visible)
    const inScreenshotMode = shouldPollScreenshots && screenshotUrl && streamingMode === 'websocket';

    // In screenshot mode, the img uses containerRef with objectFit: contain
    // In normal WebSocket mode, use canvas; in WebRTC mode, use video
    if (inScreenshotMode) {
      // Screenshot mode: calculate letterboxed content rect within container
      if (!containerRef.current) {
        return new DOMRect(0, 0, width, height);
      }

      const containerRect = containerRef.current.getBoundingClientRect();
      const contentAspect = width / height; // Remote desktop aspect ratio
      const containerAspect = containerRect.width / containerRect.height;

      let contentX = containerRect.x;
      let contentY = containerRect.y;
      let contentWidth: number;
      let contentHeight: number;

      if (containerAspect > contentAspect) {
        // Container is wider than content - letterbox on sides
        contentHeight = containerRect.height;
        contentWidth = contentHeight * contentAspect;
        contentX += (containerRect.width - contentWidth) / 2;
      } else {
        // Container is taller than content - letterbox on top/bottom
        contentWidth = containerRect.width;
        contentHeight = contentWidth / contentAspect;
        contentY += (containerRect.height - contentHeight) / 2;
      }

      return new DOMRect(contentX, contentY, contentWidth, contentHeight);
    }

    // Use canvas for WebSocket mode, video for WebRTC mode
    const element = streamingMode === 'websocket' ? canvasRef.current : videoRef.current;
    if (!element || !streamRef.current) {
      return new DOMRect(0, 0, width, height);
    }

    const boundingRect = element.getBoundingClientRect();

    // For WebSocket mode: canvas is already sized to maintain aspect ratio,
    // so bounding rect IS the video content area (no letterboxing)
    if (streamingMode === 'websocket') {
      return new DOMRect(
        boundingRect.x,
        boundingRect.y,
        boundingRect.width,
        boundingRect.height
      );
    }

    // For WebRTC mode: video element uses objectFit: contain, so we need to
    // calculate where the actual video content appears within the element
    const videoSize = streamRef.current.getStreamerSize() || [width, height];
    const videoAspect = videoSize[0] / videoSize[1];
    const boundingRectAspect = boundingRect.width / boundingRect.height;

    let x = boundingRect.x;
    let y = boundingRect.y;
    let videoMultiplier;

    if (boundingRectAspect > videoAspect) {
      videoMultiplier = boundingRect.height / videoSize[1];
      const boundingRectHalfWidth = boundingRect.width / 2;
      const videoHalfWidth = videoSize[0] * videoMultiplier / 2;
      x += boundingRectHalfWidth - videoHalfWidth;
    } else {
      videoMultiplier = boundingRect.width / videoSize[0];
      const boundingRectHalfHeight = boundingRect.height / 2;
      const videoHalfHeight = videoSize[1] * videoMultiplier / 2;
      y += boundingRectHalfHeight - videoHalfHeight;
    }

    return new DOMRect(
      x,
      y,
      videoSize[0] * videoMultiplier,
      videoSize[1] * videoMultiplier
    );
  }, [width, height, streamingMode, shouldPollScreenshots, screenshotUrl]);

  // Get input handler - for SSE mode, use the separate input WebSocket
  const getInputHandler = useCallback(() => {
    if (streamingMode === 'sse' && sseInputWsRef.current) {
      return sseInputWsRef.current.getInput();
    }
    // For WebSocket and WebRTC modes, get input from the main stream
    if (streamRef.current && 'getInput' in streamRef.current) {
      return (streamRef.current as WebSocketStream | Stream | DualStreamManager).getInput();
    }
    return null;
  }, [streamingMode]);

  // Input event handlers
  const handleMouseDown = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
    getInputHandler()?.onMouseDown(event.nativeEvent, getStreamRect());
  }, [getStreamRect, getInputHandler]);

  const handleMouseUp = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
    getInputHandler()?.onMouseUp(event.nativeEvent);
  }, [getInputHandler]);

  const handleMouseMove = useCallback((event: React.MouseEvent) => {
    event.preventDefault();

    // Update custom cursor position
    if (containerRef.current) {
      const rect = containerRef.current.getBoundingClientRect();
      setCursorPosition({
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      });

      // Mark that mouse has moved at least once
      if (!hasMouseMoved) {
        setHasMouseMoved(true);
      }
    }

    getInputHandler()?.onMouseMove(event.nativeEvent, getStreamRect());
  }, [getStreamRect, hasMouseMoved, getInputHandler]);

  const handleContextMenu = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
  }, []);

  // Reset all input state - clears stuck modifiers and mouse buttons
  const resetInputState = useCallback(() => {
    const input = getInputHandler();
    if (!input) return;

    console.log('[MoonlightStreamViewer] Resetting stuck input state (modifiers + mouse buttons)');

    // Send key-up events for all modifier keys to clear stuck state
    const modifierKeys = [
      { code: 'ShiftLeft', key: 'Shift' },
      { code: 'ShiftRight', key: 'Shift' },
      { code: 'ControlLeft', key: 'Control' },
      { code: 'ControlRight', key: 'Control' },
      { code: 'AltLeft', key: 'Alt' },
      { code: 'AltRight', key: 'Alt' },
      { code: 'MetaLeft', key: 'Meta' },
      { code: 'MetaRight', key: 'Meta' },
      { code: 'CapsLock', key: 'CapsLock' },
    ];

    modifierKeys.forEach(({ code, key }) => {
      const fakeEvent = new KeyboardEvent('keyup', {
        code,
        key,
        bubbles: true,
        cancelable: true,
      });
      input.onKeyUp(fakeEvent);
    });

    // Send mouse-up events for all buttons to clear stuck mouse state
    for (let button = 0; button < 5; button++) {
      const fakeMouseEvent = new MouseEvent('mouseup', {
        button,
        bubbles: true,
        cancelable: true,
      });
      input.onMouseUp(fakeMouseEvent);
    }

    console.log('[MoonlightStreamViewer] Input state reset complete');
  }, []);

  // Attach native keyboard event listeners when connected
  useEffect(() => {
    if (!isConnected || !containerRef.current) return;

    const container = containerRef.current;

    // Helper to get input handler - works for SSE mode (separate input WS) or normal modes
    const getInput = () => {
      return sseInputWsRef.current?.getInput() ??
        (streamRef.current && 'getInput' in streamRef.current
          ? (streamRef.current as WebSocketStream | Stream | DualStreamManager).getInput()
          : null);
    };

    // Track last Escape press for double-Escape reset
    let lastEscapeTime = 0;

    const handleKeyDown = (event: KeyboardEvent) => {
      // Only process if container is focused
      if (document.activeElement !== container) {
        console.log('[MoonlightStreamViewer] KeyDown ignored - container not focused. Active element:', document.activeElement?.tagName);
        return;
      }

      // CRITICAL: Filter out browser auto-repeat events!
      // When you hold a key, the browser fires repeated keydown events with event.repeat=true.
      // We must NOT forward these to the remote - the remote handles repeat via its own mechanisms.
      // Forwarding browser repeats causes key flooding and stuck key issues.
      if (event.repeat) {
        event.preventDefault();
        event.stopPropagation();
        return;
      }

      // Double-Escape to reset stuck modifiers (common workaround for Moonlight caps lock bug)
      if (event.code === 'Escape') {
        const now = Date.now();
        if (now - lastEscapeTime < 500) { // 500ms window for double-press
          console.log('[MoonlightStreamViewer] Double-Escape detected - resetting input state');
          resetInputState();
          event.preventDefault();
          event.stopPropagation();
          return;
        }
        lastEscapeTime = now;
      }

      // Intercept copy keystrokes for clipboard sync (cross-platform)
      const isCtrlC = event.ctrlKey && !event.shiftKey && event.code === 'KeyC';
      const isCmdC = event.metaKey && !event.shiftKey && event.code === 'KeyC';
      const isCtrlShiftC = event.ctrlKey && event.shiftKey && event.code === 'KeyC';
      const isCmdShiftC = event.metaKey && event.shiftKey && event.code === 'KeyC';
      const isCopyKeystroke = isCtrlC || isCmdC || isCtrlShiftC || isCmdShiftC;

      if (isCopyKeystroke && sessionId) {
        // Send the copy keystroke to remote first
        console.log('[Clipboard] Copy keystroke detected, forwarding to remote');
        const input = getInput();
        if (input) {
          // Forward Ctrl+C to remote
          const ctrlCDown = new KeyboardEvent('keydown', {
            code: 'KeyC',
            key: 'c',
            ctrlKey: true,
            shiftKey: event.shiftKey,
            metaKey: false,
            bubbles: true,
            cancelable: true,
          });
          input.onKeyDown(ctrlCDown);

          const ctrlCUp = new KeyboardEvent('keyup', {
            code: 'KeyC',
            key: 'c',
            ctrlKey: true,
            shiftKey: event.shiftKey,
            metaKey: false,
            bubbles: true,
            cancelable: true,
          });
          input.onKeyUp(ctrlCUp);
        }

        // Wait briefly for remote clipboard to update, then sync back to local
        setTimeout(async () => {
          try {
            const apiClient = helixApi.getApiClient();
            const response = await apiClient.v1ExternalAgentsClipboardDetail(sessionId);
            const clipboardData: TypesClipboardData = response.data;

            if (!clipboardData || !clipboardData.type || !clipboardData.data) {
              console.log('[Clipboard] Remote clipboard empty after copy');
              showClipboardToast('Copied', 'success');
              return;
            }

            if (clipboardData.type === 'image') {
              const base64Data = clipboardData.data;
              const byteCharacters = atob(base64Data);
              const byteNumbers = new Array(byteCharacters.length);
              for (let i = 0; i < byteCharacters.length; i++) {
                byteNumbers[i] = byteCharacters.charCodeAt(i);
              }
              const byteArray = new Uint8Array(byteNumbers);
              const blob = new Blob([byteArray], { type: 'image/png' });

              await navigator.clipboard.write([
                new ClipboardItem({ 'image/png': blob })
              ]);
              console.log(`[Clipboard] Synced image from remote (${byteArray.length} bytes)`);
            } else if (clipboardData.type === 'text') {
              await navigator.clipboard.writeText(clipboardData.data);
              console.log(`[Clipboard] Synced text from remote (${clipboardData.data.length} chars)`);
            }

            lastRemoteClipboardHash.current = `${clipboardData.type}:${clipboardData.data.substring(0, 100)}`;
            showClipboardToast('Copied', 'success');
          } catch (err) {
            console.error('[Clipboard] Failed to sync clipboard after copy:', err);
            // Still show success - the remote copy likely worked even if sync failed
            showClipboardToast('Copied', 'success');
          }
        }, 300); // Wait 300ms for remote clipboard to update

        event.preventDefault();
        event.stopPropagation();
        return;
      }

      // Intercept paste keystrokes for clipboard sync (cross-platform)
      const isCtrlV = event.ctrlKey && !event.shiftKey && event.code === 'KeyV';
      const isCmdV = event.metaKey && !event.shiftKey && event.code === 'KeyV';
      const isCtrlShiftV = event.ctrlKey && event.shiftKey && event.code === 'KeyV';
      const isCmdShiftV = event.metaKey && event.shiftKey && event.code === 'KeyV';
      const isPasteKeystroke = isCtrlV || isCmdV || isCtrlShiftV || isCmdShiftV;

      if (isPasteKeystroke && sessionId) {
        event.preventDefault();
        event.stopPropagation();

        // Remember which keystroke the user pressed so we can forward the same one
        const userPressedShift = event.shiftKey;
        console.log(`[Clipboard] Paste keystroke detected (shift=${userPressedShift}), syncing local → remote`);

        // Handle clipboard sync asynchronously (don't block keystroke processing)
        navigator.clipboard.read().then(clipboardItems => {
          if (clipboardItems.length === 0) {
            console.warn('[Clipboard] Empty clipboard, ignoring paste');
            showClipboardToast('Clipboard is empty', 'error');
            return;
          }

          const item = clipboardItems[0];
          let clipboardPayload: TypesClipboardData;

          // Read clipboard data
          if (item.types.includes('image/png')) {
            item.getType('image/png').then(blob => {
              blob.arrayBuffer().then(arrayBuffer => {
                const base64 = btoa(String.fromCharCode(...new Uint8Array(arrayBuffer)));
                clipboardPayload = { type: 'image', data: base64 };
                syncAndPaste(clipboardPayload);
              });
            });
          } else if (item.types.includes('text/plain')) {
            item.getType('text/plain').then(blob => {
              blob.text().then(text => {
                clipboardPayload = { type: 'text', data: text };
                syncAndPaste(clipboardPayload);
              });
            });
          } else {
            console.warn('[Clipboard] Unsupported clipboard type:', item.types);
            showClipboardToast(`Unsupported clipboard type: ${item.types.join(', ')}`, 'error');
          }
        }).catch(err => {
          console.error('[Clipboard] Failed to read clipboard:', err);
          const errMsg = err instanceof Error ? err.message : String(err);
          showClipboardToast(`Paste failed: ${errMsg}`, 'error');
        });

        // Helper function to sync clipboard and forward keystroke
        const syncAndPaste = (payload: TypesClipboardData) => {
          const apiClient = helixApi.getApiClient();
          apiClient.v1ExternalAgentsClipboardCreate(sessionId, payload).then(() => {
            console.log(`[Clipboard] Synced ${payload.type} to remote`);
            showClipboardToast('Pasted', 'success');

            // Forward the SAME keystroke the user pressed:
            // - User pressed Ctrl+V → send Ctrl+V (for Zed, most GUI apps)
            // - User pressed Ctrl+Shift+V → send Ctrl+Shift+V (for terminals)
            const input = getInput();
            if (input) {
              const pasteKeyDown = new KeyboardEvent('keydown', {
                code: 'KeyV',
                key: userPressedShift ? 'V' : 'v',
                ctrlKey: true,
                shiftKey: userPressedShift,
                metaKey: false,
                bubbles: true,
                cancelable: true,
              });
              input.onKeyDown(pasteKeyDown);

              const pasteKeyUp = new KeyboardEvent('keyup', {
                code: 'KeyV',
                key: userPressedShift ? 'V' : 'v',
                ctrlKey: true,
                shiftKey: userPressedShift,
                metaKey: false,
                bubbles: true,
                cancelable: true,
              });
              input.onKeyUp(pasteKeyUp);

              console.log(`[Clipboard] Forwarded Ctrl+${userPressedShift ? 'Shift+' : ''}V to remote`);
            }
          }).catch(err => {
            console.error('[Clipboard] Failed to sync clipboard:', err);
            const errMsg = err instanceof Error ? err.message : String(err);
            showClipboardToast(`Paste failed: ${errMsg}`, 'error');
          });
        };

        return; // Don't fall through to default handler
      }

      console.log('[MoonlightStreamViewer] KeyDown captured:', event.key, event.code);
      getInput()?.onKeyDown(event);
      // Prevent browser default behavior (e.g., Tab moving focus, Ctrl+W closing tab)
      // This ensures all keys are passed through to the remote desktop
      event.preventDefault();
      event.stopPropagation();
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      // Only process if container is focused
      if (document.activeElement !== container) {
        console.log('[MoonlightStreamViewer] KeyUp ignored - container not focused. Active element:', document.activeElement?.tagName);
        return;
      }

      // Suppress keyup for copy keystrokes (we synthesize complete keydown+keyup in handleKeyDown)
      const isCtrlC = event.ctrlKey && !event.shiftKey && event.code === 'KeyC';
      const isCmdC = event.metaKey && !event.shiftKey && event.code === 'KeyC';
      const isCtrlShiftC = event.ctrlKey && event.shiftKey && event.code === 'KeyC';
      const isCmdShiftC = event.metaKey && event.shiftKey && event.code === 'KeyC';
      const isCopyKeystroke = isCtrlC || isCmdC || isCtrlShiftC || isCmdShiftC;

      if (isCopyKeystroke) {
        // Suppress keyup for copy - we already sent complete keydown+keyup in clipboard handler
        event.preventDefault();
        event.stopPropagation();
        return;
      }

      // Suppress keyup for paste keystrokes (we synthesize complete keydown+keyup in handleKeyDown)
      const isCtrlV = event.ctrlKey && !event.shiftKey && event.code === 'KeyV';
      const isCmdV = event.metaKey && !event.shiftKey && event.code === 'KeyV';
      const isCtrlShiftV = event.ctrlKey && event.shiftKey && event.code === 'KeyV';
      const isCmdShiftV = event.metaKey && event.shiftKey && event.code === 'KeyV';
      const isPasteKeystroke = isCtrlV || isCmdV || isCtrlShiftV || isCmdShiftV;

      if (isPasteKeystroke) {
        // Suppress keyup for paste - we already sent complete keydown+keyup in clipboard handler
        event.preventDefault();
        event.stopPropagation();
        return;
      }

      console.log('[MoonlightStreamViewer] KeyUp captured:', event.key, event.code);
      getInput()?.onKeyUp(event);
      // Prevent browser default behavior to ensure all keys are passed through
      event.preventDefault();
      event.stopPropagation();
    };

    // Reset input state when window regains focus (prevents stuck modifiers after Alt+Tab)
    const handleWindowFocus = () => {
      console.log('[MoonlightStreamViewer] Window regained focus - resetting input state to prevent desync');
      resetInputState();
    };

    // Attach to container, not document (so we only capture when focused)
    container.addEventListener('keydown', handleKeyDown);
    container.addEventListener('keyup', handleKeyUp);
    window.addEventListener('focus', handleWindowFocus);

    return () => {
      container.removeEventListener('keydown', handleKeyDown);
      container.removeEventListener('keyup', handleKeyUp);
      window.removeEventListener('focus', handleWindowFocus);
    };
  }, [isConnected, resetInputState]);

  // Focus container when clicking anywhere in the viewer
  const handleContainerClick = useCallback(() => {
    if (containerRef.current) {
      containerRef.current.focus();
    }
  }, []);

  return (
    <Box
      ref={containerRef}
      className={className}
      tabIndex={0}
      onClick={handleContainerClick}
      sx={{
        position: 'relative',
        width: '100%',
        height: '100%',
        minHeight: 400,
        backgroundColor: '#000',
        display: 'flex',
        flexDirection: 'column',
        outline: 'none',
        cursor: 'default',
      }}
    >
      {/* Toolbar - always visible so user can reconnect/access controls */}
      <Box
        sx={{
          position: 'absolute',
          top: 8,
          left: '50%',
          transform: 'translateX(-50%)',
          zIndex: 1000,
          backgroundColor: 'rgba(0,0,0,0.7)',
          borderRadius: 1,
          display: 'flex',
          gap: 1,
        }}
      >
        <Tooltip title={audioEnabled ? 'Mute audio' : 'Unmute audio'} arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size="small"
            onClick={() => setAudioEnabled(!audioEnabled)}
            sx={{ color: audioEnabled ? 'white' : 'grey' }}
          >
            {audioEnabled ? <VolumeUp fontSize="small" /> : <VolumeOff fontSize="small" />}
          </IconButton>
        </Tooltip>
        <Tooltip title="Reconnect to streaming server" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <span>
            <IconButton
              size="small"
              onClick={reconnect}
              sx={{ color: 'white' }}
              disabled={isConnecting}
            >
              <Refresh fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Stats for nerds - show streaming statistics" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size="small"
            onClick={() => setShowStats(!showStats)}
            sx={{ color: showStats ? 'primary.main' : 'white' }}
          >
            <BarChart fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip title="Charts - visualize throughput, RTT, and bitrate over time" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size="small"
            onClick={() => setShowCharts(!showCharts)}
            sx={{ color: showCharts ? 'primary.main' : 'white' }}
          >
            <Timeline fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip title="Keyboard state monitor - debug key input issues" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size="small"
            onClick={() => setShowKeyboardPanel(!showKeyboardPanel)}
            sx={{ color: showKeyboardPanel ? 'primary.main' : 'white' }}
          >
            <Keyboard fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip
          title={
            modeSwitchCooldown
              ? 'Please wait...'
              : streamingMode === 'websocket'
              ? 'Currently: WebSocket — Click for SSE (experimental)'
              : streamingMode === 'sse'
              ? 'Currently: SSE (experimental) — Click for WebRTC'
              : 'Currently: WebRTC — Click for WebSocket'
          }
          arrow
          slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}
        >
          <span>
            <IconButton
              size="small"
              disabled={modeSwitchCooldown}
              onClick={() => {
                // Cycle through modes: websocket → sse → webrtc → websocket
                setModeSwitchCooldown(true);
                setStreamingMode(prev => {
                  if (prev === 'websocket') return 'sse';
                  if (prev === 'sse') return 'webrtc';
                  return 'websocket';
                });
                setTimeout(() => setModeSwitchCooldown(false), 3000); // 3 second cooldown
              }}
              sx={{
                color: streamingMode === 'websocket'
                  ? 'primary.main'
                  : streamingMode === 'sse'
                  ? '#ff9800'  // Orange for SSE (experimental)
                  : 'white'
              }}
            >
              {streamingMode === 'websocket' ? (
                <Wifi fontSize="small" />
              ) : streamingMode === 'sse' ? (
                <StreamIcon fontSize="small" />
              ) : (
                <SignalCellularAlt fontSize="small" />
              )}
            </IconButton>
          </span>
        </Tooltip>
        {/* Quality mode toggle: video (high) <-> screenshots (low) */}
        {streamingMode === 'websocket' && (
          <Tooltip
            title={
              modeSwitchCooldown
                ? 'Please wait...'
                : qualityMode === 'high'
                ? 'Video mode (60fps) — Click for Screenshot mode'
                : 'Screenshot mode — Click for Video mode'
            }
            arrow
            slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}
          >
            <span>
              <IconButton
                size="small"
                disabled={modeSwitchCooldown}
                onClick={() => {
                  // Toggle between high (video) and low (screenshots)
                  // With cooldown to prevent Wolf deadlock from rapid switching
                  setModeSwitchCooldown(true);
                  setQualityMode(prev => prev === 'high' ? 'low' : 'high');
                  setTimeout(() => setModeSwitchCooldown(false), 3000); // 3 second cooldown
                }}
                sx={{
                  // Orange for screenshot mode, white for video mode
                  color: qualityMode === 'low' ? '#ff9800' : 'white',
                }}
              >
                <Speed fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
        )}
        {/* Bitrate selector */}
        <Tooltip title="Select streaming bitrate" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <Button
            size="small"
            onClick={(e) => setBitrateMenuAnchor(e.currentTarget)}
            sx={{
              color: 'white',
              minWidth: 'auto',
              px: 1,
              fontSize: '0.7rem',
              textTransform: 'none',
            }}
          >
            {userBitrate ?? requestedBitrate}M
          </Button>
        </Tooltip>
        <Menu
          anchorEl={bitrateMenuAnchor}
          open={Boolean(bitrateMenuAnchor)}
          onClose={() => setBitrateMenuAnchor(null)}
          container={containerRef.current} // Render in main container (not transformed toolbar) for correct positioning + fullscreen support
          slotProps={{ paper: { sx: { bgcolor: 'rgba(0,0,0,0.9)', color: 'white' } } }}
          sx={{ zIndex: 100001 }} // Above floating modals (z-index 9999+)
        >
          {[5, 10, 20, 40, 80].map((bitrate) => (
            <MenuItem
              key={bitrate}
              selected={(userBitrate ?? requestedBitrate) === bitrate}
              onClick={() => {
                setUserBitrate(bitrate);
                setBitrateMenuAnchor(null);
                // Record manual selection time - adaptive algorithm will wait 20s before auto-reducing
                manualBitrateSelectionTimeRef.current = Date.now();
                // Reconnect with new bitrate after cooldown
                setModeSwitchCooldown(true);
                setTimeout(() => setModeSwitchCooldown(false), 3000);
                reconnect(5000);
              }}
              sx={{ fontSize: '0.8rem' }}
            >
              {bitrate} Mbps {bitrate === 10 && '(default)'}
            </MenuItem>
          ))}
        </Menu>
        <Tooltip title={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'} arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size="small"
            onClick={toggleFullscreen}
            sx={{ color: 'white' }}
          >
            {isFullscreen ? <FullscreenExit fontSize="small" /> : <Fullscreen fontSize="small" />}
          </IconButton>
        </Tooltip>
      </Box>

      {/* Screenshot Mode / High Latency Warning Banner */}
      {shouldPollScreenshots && isConnected && streamingMode === 'websocket' && (
        <Box
          sx={{
            position: 'absolute',
            top: 50,
            left: '50%',
            transform: 'translateX(-50%)',
            zIndex: 999,
            backgroundColor: 'rgba(255, 152, 0, 0.95)',
            color: 'black',
            padding: '4px 16px',
            borderRadius: 1,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 0.5,
            fontFamily: 'monospace',
            fontSize: '0.75rem',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Wifi sx={{ fontSize: 16 }} />
            <Typography variant="caption" sx={{ fontWeight: 'bold' }}>
              {qualityMode === 'adaptive'
                ? `High latency detected — using screenshots (${screenshotFps} FPS)`
                : `Screenshot mode (${screenshotFps} FPS @ ${screenshotQuality}% quality)`
              }
            </Typography>
          </Box>
          {qualityMode === 'adaptive' && adaptiveLockedToScreenshots && (
            <Typography variant="caption" sx={{ fontSize: '0.65rem', opacity: 0.8 }}>
              Video paused to save bandwidth. Click speed icon to retry video.
            </Typography>
          )}
        </Box>
      )}

      {/* Loading Overlay - shown during restart/reconnect (hides error messages) */}
      {showLoadingOverlay && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.9)',
            zIndex: 2000,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 3,
          }}
        >
          <CircularProgress size={60} sx={{ color: 'primary.main' }} />
          <Typography variant="h6" sx={{ color: 'white' }}>
            {isRestart ? 'Restarting session...' : 'Starting session...'}
          </Typography>
          <Typography variant="body2" sx={{ color: 'grey.400' }}>
            {isRestart
              ? 'Stopping old session and starting with fresh startup script'
              : 'Creating new session and running startup script'}
          </Typography>
        </Box>
      )}

      {/* Disconnected Overlay - prominent reconnection indicator */}
      {!isConnecting && !isConnected && !error && retryCountdown === null && !showLoadingOverlay && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.85)',
            zIndex: 1500,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
          }}
        >
          <CircularProgress size={48} sx={{ color: 'warning.main' }} />
          <Typography variant="h6" sx={{ color: 'white' }}>
            Connecting...
          </Typography>
          <Typography variant="body2" sx={{ color: 'grey.400', textAlign: 'center', maxWidth: 300 }}>
            {status || 'Attempting to reconnect...'}
          </Typography>
          <Button
            variant="contained"
            color="primary"
            onClick={reconnect}
            startIcon={<Refresh />}
            sx={{ mt: 2 }}
          >
            Reconnect Now
          </Button>
        </Box>
      )}

      {/* Status Overlay */}
      {(isConnecting || error || retryCountdown !== null) && (
        <Box
          sx={{
            position: 'absolute',
            top: '50%',
            left: '50%',
            transform: 'translate(-50%, -50%)',
            zIndex: 999,
            textAlign: 'center',
          }}
        >
          {isConnecting && (
            <Box sx={{ color: 'white' }}>
              <CircularProgress size={40} sx={{ mb: 2 }} />
              <Typography variant="body1">{status}</Typography>
            </Box>
          )}

          {retryCountdown !== null && (
            <Alert severity="warning" sx={{ maxWidth: 400 }}>
              Stream busy (attempt {retryAttemptDisplay}) - retrying in {retryCountdown} second{retryCountdown !== 1 ? 's' : ''}...
            </Alert>
          )}

          {error && retryCountdown === null && (
            <Alert
              severity="error"
              sx={{ maxWidth: 400 }}
              action={
                <Button
                  color="inherit"
                  size="small"
                  onClick={() => {
                    setError(null);
                    connect();
                  }}
                >
                  Reconnect
                </Button>
              }
            >
              {error}
            </Alert>
          )}
        </Box>
      )}

      {/* Video Element (WebRTC mode) */}
      <video
        ref={videoRef}
        autoPlay
        playsInline
        controls={false}
        onMouseDown={handleMouseDown}
        onMouseUp={handleMouseUp}
        onMouseMove={handleMouseMove}
        onContextMenu={handleContextMenu}
        style={{
          width: '100%',
          height: '100%',
          objectFit: 'contain',
          backgroundColor: '#000',
          cursor: 'none', // Hide default cursor to prevent double cursor effect
          display: streamingMode === 'webrtc' ? 'block' : 'none',
        }}
        onClick={() => {
          // Unmute on first interaction (browser autoplay policy)
          if (videoRef.current) {
            videoRef.current.muted = false;
          }
          // Focus container for keyboard input
          if (containerRef.current) {
            containerRef.current.focus();
          }
        }}
      />

      {/* Canvas Element (WebSocket mode) - centered with proper aspect ratio */}
      <canvas
        ref={canvasRef}
        onMouseDown={handleMouseDown}
        onMouseUp={handleMouseUp}
        onMouseMove={handleMouseMove}
        onContextMenu={handleContextMenu}
        style={{
          // Use calculated dimensions to maintain aspect ratio
          // Canvas doesn't support objectFit like video, so we calculate size manually
          width: canvasDisplaySize ? `${canvasDisplaySize.width}px` : '100%',
          height: canvasDisplaySize ? `${canvasDisplaySize.height}px` : '100%',
          // Center the canvas within the container
          position: 'absolute',
          left: '50%',
          top: '50%',
          transform: 'translate(-50%, -50%)',
          backgroundColor: '#000',
          cursor: 'none', // Hide default cursor to prevent double cursor effect
          display: (streamingMode === 'websocket' || streamingMode === 'sse') ? 'block' : 'none',
        }}
        onClick={() => {
          // Focus container for keyboard input
          if (containerRef.current) {
            containerRef.current.focus();
          }
        }}
      />

      {/* Screenshot overlay for low-quality mode */}
      {/* Shows rapidly-updated screenshots instead of video stream while keeping input working */}
      {shouldPollScreenshots && screenshotUrl && streamingMode === 'websocket' && (
        <img
          src={screenshotUrl}
          alt="Remote Desktop Screenshot"
          style={{
            width: '100%',
            height: '100%',
            position: 'absolute',
            left: 0,
            top: 0,
            objectFit: 'contain',
            pointerEvents: 'none', // Allow clicks to pass through to canvas for input
            zIndex: 10, // Above canvas but below UI elements
          }}
        />
      )}

      {/* Custom cursor dot to show local mouse position */}
      <Box
        sx={{
          position: 'absolute',
          left: cursorPosition.x,
          top: cursorPosition.y,
          width: 8,
          height: 8,
          borderRadius: '50%',
          backgroundColor: 'rgba(255, 255, 255, 0.8)',
          border: '1px solid rgba(0, 0, 0, 0.5)',
          pointerEvents: 'none',
          zIndex: 1000,
          transform: 'translate(-50%, -50%)',
          display: isConnected && hasMouseMoved ? 'block' : 'none',
          transition: 'opacity 0.2s',
        }}
        id="custom-cursor"
      />

      {/* Input Hint - removed since auto-focus handles keyboard input */}

      {/* Stats for Nerds Overlay */}
      {showStats && (stats || qualityMode === 'low') && (
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
            📊 Stats for Nerds
          </Typography>

          <Box sx={{ '& > div': { mb: 0.3, lineHeight: 1.5 } }}>
            <div><strong>Transport:</strong> {streamingMode === 'websocket' ? 'WebSocket (L7)' : 'WebRTC'}</div>
            {stats?.video?.codec && (
              <>
                <div><strong>Codec:</strong> {stats.video.codec}</div>
                <div><strong>Resolution:</strong> {stats.video.width}x{stats.video.height}</div>
                <div><strong>FPS:</strong> {stats.video.fps}</div>
                {streamingMode === 'websocket' ? (
                  <div><strong>Bitrate:</strong> {stats.video.totalBitrate} Mbps <span style={{ color: '#888' }}>req: {requestedBitrate}</span></div>
                ) : (
                  <div><strong>Bitrate:</strong> {stats.video.bitrate} Mbps <span style={{ color: '#888' }}>req: {requestedBitrate}</span></div>
                )}
                <div><strong>Decoded:</strong> {stats.video.framesDecoded} frames</div>
                <div>
                  <strong>Dropped:</strong> {stats.video.framesDropped} frames
                  {stats.video.framesDropped > 0 && <span style={{ color: '#ff6b6b' }}> ⚠️</span>}
                </div>
                {/* RTT (WebSocket mode) */}
                {streamingMode === 'websocket' && stats.video.rttMs !== undefined && (
                  <div>
                    <strong>RTT:</strong> {stats.video.rttMs.toFixed(0)} ms
                    {stats.video.isHighLatency && <span style={{ color: '#ff9800' }}> ⚠️ High latency</span>}
                  </div>
                )}
                {/* Batching stats (WebSocket mode) - shows congestion handling */}
                {streamingMode === 'websocket' && stats.video.batchingRatio !== undefined && (
                  <div>
                    <strong>Batching:</strong> {stats.video.batchingRatio > 0
                      ? `${stats.video.batchingRatio}% (avg ${stats.video.avgBatchSize?.toFixed(1) || 0} frames/batch)`
                      : 'OFF'}
                    {stats.video.batchingRatio > 0 && <span style={{ color: '#ff9800' }}> 📦</span>}
                    {stats.video.batchingRequested && <span style={{ color: '#ff9800' }}> (requested)</span>}
                  </div>
                )}
                {/* Frame latency (WebSocket mode) - actual delivery delay based on PTS */}
                {/* Positive = frames arriving late (bad), Negative = frames arriving early (good/buffered) */}
                {streamingMode === 'websocket' && stats.video.frameLatencyMs !== undefined && (
                  <div>
                    <strong>Frame Drift:</strong> {stats.video.frameLatencyMs > 0 ? '+' : ''}{stats.video.frameLatencyMs.toFixed(0)} ms
                    {stats.video.frameLatencyMs > 200 && <span style={{ color: '#ff6b6b' }}> ⚠️ Behind</span>}
                    {stats.video.frameLatencyMs < -500 && <span style={{ color: '#4caf50' }}> (buffered)</span>}
                  </div>
                )}
                {/* Decoder queue (WebSocket mode) - detects if decoder can't keep up */}
                {streamingMode === 'websocket' && stats.video.decodeQueueSize !== undefined && (
                  <div>
                    <strong>Decode Queue:</strong> {stats.video.decodeQueueSize}
                    {stats.video.maxDecodeQueueSize > 3 && (
                      <span style={{ color: '#888' }}> (peak: {stats.video.maxDecodeQueueSize})</span>
                    )}
                    {stats.video.decodeQueueSize > 3 && <span style={{ color: '#ff6b6b' }}> ⚠️ Backed up</span>}
                  </div>
                )}
                {/* Frames skipped to keyframe (WebSocket mode) - shows when decoder fell behind and skipped ahead */}
                {streamingMode === 'websocket' && stats.video.framesSkippedToKeyframe !== undefined && (
                  <div>
                    <strong>Skipped to KF:</strong> {stats.video.framesSkippedToKeyframe} frames
                    {stats.video.framesSkippedToKeyframe > 0 && <span style={{ color: '#ff9800' }}> ⏭️</span>}
                  </div>
                )}
                {/* WebRTC-only stats - not available in WebSocket mode */}
                {streamingMode === 'webrtc' && (
                  <>
                    <div>
                      <strong>Packets Lost:</strong> {stats.video.packetsLost} / {stats.video.packetsReceived}
                      {stats.video.packetsLost > 0 && <span style={{ color: '#ff6b6b' }}> ⚠️</span>}
                    </div>
                    <div><strong>Jitter:</strong> {stats.video.jitter} ms</div>
                    {stats.connection.rtt && <div><strong>RTT:</strong> {stats.connection.rtt} ms</div>}
                  </>
                )}
              </>
            )}
            {!stats?.video?.codec && !shouldPollScreenshots && <div>Waiting for video data...</div>}
            {/* Screenshot mode stats */}
            {shouldPollScreenshots && (
              <>
                <div style={{ marginTop: 8, borderTop: '1px solid rgba(0, 255, 0, 0.3)', paddingTop: 8 }}>
                  <strong style={{ color: '#ff9800' }}>📸 Screenshot Mode</strong>
                </div>
                <div><strong>FPS:</strong> {screenshotFps} <span style={{ color: '#888' }}>target: ≥2</span></div>
                <div>
                  <strong>JPEG Quality:</strong> {screenshotQuality}%
                  <span style={{ color: '#888' }}> (adaptive 10-90)</span>
                </div>
              </>
            )}
          </Box>
        </Box>
      )}

      {/* Adaptive Bitrate Charts Panel */}
      {showCharts && (() => {
        // Extract ref values for rendering (refs persist across reconnects)
        const throughputHistory = throughputHistoryRef.current;
        const rttHistory = rttHistoryRef.current;
        const bitrateHistory = bitrateHistoryRef.current;

        return (
        <Box
          sx={{
            position: 'absolute',
            bottom: 60,
            left: 10,
            right: 10,
            backgroundColor: 'rgba(0, 0, 0, 0.95)',
            borderRadius: 2,
            border: '1px solid rgba(0, 255, 0, 0.3)',
            zIndex: 1500,
            p: 2,
            maxHeight: '40%',
            overflow: 'auto',
          }}
        >
          <Typography variant="caption" sx={{ fontWeight: 'bold', display: 'block', mb: 2, color: '#00ff00' }}>
            📈 Adaptive Bitrate Charts (60s history)
          </Typography>

          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
            {/* Throughput vs Requested Bitrate Chart */}
            <Box sx={{ flex: '1 1 400px', minWidth: 300 }}>
              <Typography variant="caption" sx={{ color: '#888', display: 'block', mb: 1 }}>
                Throughput vs Requested Bitrate (Mbps)
              </Typography>
              <Box sx={{ height: 150, ...chartContainerStyles }}>
                {throughputHistory.length > 1 ? (
                  <LineChart
                    xAxis={[{
                      data: throughputHistory.map((_, i) => i - throughputHistory.length + 1),
                      label: 'Seconds ago',
                      labelStyle: axisLabelStyle,
                    }]}
                    yAxis={[{
                      min: 0,
                      max: Math.max(Math.max(...throughputHistory), Math.max(...bitrateHistory), 10) * 1.2,
                      labelStyle: axisLabelStyle,
                    }]}
                    series={[
                      {
                        data: bitrateHistory,
                        label: 'Requested',
                        color: '#888',
                        showMark: false,
                        curve: 'stepAfter',
                      },
                      {
                        data: throughputHistory,
                        label: 'Actual',
                        color: '#00ff00',
                        showMark: false,
                        curve: 'linear',
                        area: true,
                      },
                    ]}
                    height={120}
                    margin={{ left: 50, right: 10, top: 30, bottom: 25 }}
                    grid={{ horizontal: true, vertical: false }}
                    sx={darkChartStyles}
                    slotProps={{ legend: chartLegendProps }}
                  />
                ) : (
                  <Box sx={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666' }}>
                    Collecting data...
                  </Box>
                )}
              </Box>
            </Box>

            {/* RTT Chart */}
            <Box sx={{ flex: '1 1 400px', minWidth: 300 }}>
              <Typography variant="caption" sx={{ color: '#888', display: 'block', mb: 1 }}>
                Round-Trip Time (ms) - Spikes indicate congestion
              </Typography>
              <Box sx={{ height: 150, ...chartContainerStyles }}>
                {rttHistory.length > 1 ? (
                  <LineChart
                    xAxis={[{
                      data: rttHistory.map((_, i) => i - rttHistory.length + 1),
                      label: 'Seconds ago',
                      labelStyle: axisLabelStyle,
                    }]}
                    yAxis={[{
                      min: 0,
                      max: Math.max(Math.max(...rttHistory), 100) * 1.2,
                      labelStyle: axisLabelStyle,
                    }]}
                    series={[
                      {
                        data: rttHistory.map(() => 150), // Threshold line at 150ms
                        label: 'High Latency Threshold',
                        color: '#ff9800',
                        showMark: false,
                        curve: 'linear',
                      },
                      {
                        data: rttHistory,
                        label: 'RTT',
                        color: rttHistory[rttHistory.length - 1] > 150 ? '#ff6b6b' : '#00c8ff',
                        showMark: false,
                        curve: 'linear',
                        area: true,
                      },
                    ]}
                    height={120}
                    margin={{ left: 50, right: 10, top: 30, bottom: 25 }}
                    grid={{ horizontal: true, vertical: false }}
                    sx={darkChartStyles}
                    slotProps={{ legend: chartLegendProps }}
                  />
                ) : (
                  <Box sx={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666' }}>
                    Collecting data...
                  </Box>
                )}
              </Box>
            </Box>
          </Box>

          <Typography variant="caption" sx={{ display: 'block', mt: 2, color: '#666', fontStyle: 'italic' }}>
            Test with Chrome DevTools → Network → Throttling → "Fast 4G" or "Slow 4G" to see adaptive behavior
          </Typography>
        </Box>
        );
      })()}

      {/* Keyboard State Monitor Panel */}
      {showKeyboardPanel && sessionId && (
        <KeyboardObservabilityPanel
          sandboxInstanceId={sessionId}
          onClose={() => setShowKeyboardPanel(false)}
        />
      )}

      {/* Clipboard Toast Notification */}
      <Box
        sx={{
          position: 'absolute',
          bottom: 40,
          left: '50%',
          transform: `translateX(-50%) translateY(${clipboardToast.visible ? '0' : '20px'})`,
          zIndex: 2500,
          backgroundColor: clipboardToast.type === 'success'
            ? 'rgba(46, 125, 50, 0.95)'
            : 'rgba(211, 47, 47, 0.95)',
          color: 'white',
          padding: '8px 20px',
          borderRadius: 2,
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.4)',
          opacity: clipboardToast.visible ? 1 : 0,
          transition: 'opacity 0.2s ease, transform 0.2s ease',
          pointerEvents: 'none',
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          fontFamily: 'system-ui, -apple-system, sans-serif',
          fontSize: '0.875rem',
          fontWeight: 500,
          whiteSpace: 'nowrap',
          maxWidth: '80%',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
        }}
      >
        {clipboardToast.type === 'success' ? '✓' : '✕'} {clipboardToast.message}
      </Box>
    </Box>
  );
};

export default MoonlightStreamViewer;
 
