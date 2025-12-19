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
  CameraAlt,
} from '@mui/icons-material';
import KeyboardObservabilityPanel from './KeyboardObservabilityPanel';
import { LineChart } from '@mui/x-charts';
import {
  darkChartStyles,
  chartContainerStyles,
  chartLegendProps,
  axisLabelStyle,
} from '../wolf/chartStyles';
// getApi import removed - we create API object directly instead of using cached singleton
import { Stream } from '../../lib/moonlight-web-ts/stream/index';
import { WebSocketStream, codecToWebCodecsString, codecToDisplayName } from '../../lib/moonlight-web-ts/stream/websocket-stream';
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
  const sseCanvasRef = useRef<HTMLCanvasElement>(null); // Separate canvas for SSE mode (avoids conflicts)
  const containerRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<Stream | WebSocketStream | null>(null); // Stream instance from moonlight-web
  const sseStatsRef = useRef({
    framesDecoded: 0,
    framesDropped: 0,
    lastFrameTime: 0,
    frameCount: 0,
    currentFps: 0,
    width: 0,
    height: 0,
    codecString: '',           // Actual codec from init event
    // Frame latency tracking (arrival time vs expected based on PTS)
    firstFramePtsUs: null as number | null,
    firstFrameArrivalTime: null as number | null,
    currentFrameLatencyMs: 0,
    frameLatencySamples: [] as number[],
  }); // SSE-specific stats for the inline decoder
  const retryAttemptRef = useRef(0); // Use ref to avoid closure issues
  const previousLobbyIdRef = useRef<string | undefined>(undefined); // Track lobby changes
  const isExplicitlyClosingRef = useRef(false); // Track explicit close to prevent spurious "Reconnecting..." state
  const pendingReconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null); // Cancel pending reconnects to prevent duplicate streams

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
  // Bandwidth recommendation state - instead of auto-switching, we show a recommendation popup
  const [bitrateRecommendation, setBitrateRecommendation] = useState<{
    type: 'decrease' | 'increase';
    targetBitrate: number;
    reason: string;
    frameDrift?: number; // Current frame drift for decrease recommendations
    measuredThroughput?: number; // Measured throughput for increase recommendations
  } | null>(null);
  const [streamingMode, setStreamingMode] = useState<StreamingMode>('websocket'); // Default to WebSocket video transport
  const [canvasDisplaySize, setCanvasDisplaySize] = useState<{ width: number; height: number } | null>(null);
  const [containerSize, setContainerSize] = useState<{ width: number; height: number } | null>(null);
  const [isHighLatency, setIsHighLatency] = useState(false); // Show warning when RTT > 150ms
  // Quality mode: video delivery method (hot-switchable without disrupting WebSocket connection)
  // - 'high': 60fps video over WebSocket
  // - 'sse': 60fps video over SSE (lower latency for long connections)
  // - 'low': Screenshot-based (for low bandwidth)
  // Note: 'adaptive' mode removed for simplicity - users can manually switch
  const [qualityMode, setQualityMode] = useState<'high' | 'sse' | 'low'>('high'); // Default to high mode (switch to screenshot manually if needed)
  const [isOnFallback, setIsOnFallback] = useState(false); // True when on low-quality fallback stream
  const [modeSwitchCooldown, setModeSwitchCooldown] = useState(false); // Prevent rapid mode switching (causes Wolf deadlock)

  // Screenshot-based low-quality mode state
  const [screenshotUrl, setScreenshotUrl] = useState<string | null>(null);
  const screenshotIntervalRef = useRef<NodeJS.Timeout | null>(null);
  // Adaptive JPEG quality control - targets 2 FPS (500ms max per frame)
  const [screenshotQuality, setScreenshotQuality] = useState(70); // JPEG quality 10-90
  const [screenshotFps, setScreenshotFps] = useState(0); // Current FPS for display
  const screenshotQualityRef = useRef(70); // Ref for use in async callback

  // Clipboard sync state
  const lastRemoteClipboardHash = useRef<string>(''); // Track changes to avoid unnecessary writes
  const [stats, setStats] = useState<any>(null);

  // Chart history for visualizing adaptive bitrate behavior (60 seconds of data)
  // Uses refs to persist across reconnects - only reset when component unmounts
  const CHART_HISTORY_LENGTH = 60;
  const throughputHistoryRef = useRef<number[]>([]);
  const rttHistoryRef = useRef<number[]>([]);
  const bitrateHistoryRef = useRef<number[]>([]);
  const frameDriftHistoryRef = useRef<number[]>([]);
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

  // Video start timeout - detect Wolf pipeline failures that cause hangs
  const videoStartTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const VIDEO_START_TIMEOUT_MS = 15000; // 15 seconds to start video after connection
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
    // Reset explicit close flag - we're starting a new connection
    isExplicitlyClosingRef.current = false;

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
      } else if (sessionId) {
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

      let stream: Stream | WebSocketStream;

      // WebSocket mode: always connect via WebSocket for input
      // qualityMode determines video source (hot-switched after connect):
      // - 'high': Video over WebSocket (default)
      // - 'sse': Video over SSE (hot-switched via useEffect)
      // - 'low': Screenshot polling (hot-switched via useEffect)
      if (streamingMode === 'websocket') {
        // WebSocket mode: use WebSocket for input, qualityMode determines video source
        console.log('[MoonlightStreamViewer] Using WebSocket streaming mode, qualityMode:', qualityMode);

        const streamSettings = { ...settings };
        const qualitySessionId = sessionId ? `${sessionId}-hq` : undefined;

        if (qualityMode === 'low') {
          console.log('[MoonlightStreamViewer] Low mode: WebSocket for input + screenshot overlay');
        } else if (qualityMode === 'sse') {
          console.log('[MoonlightStreamViewer] SSE mode: WebSocket for input, SSE for video (hot-switched after connect)');
        } else {
          console.log('[MoonlightStreamViewer] High mode: WebSocket for video and input');
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
          hasEverConnectedRef.current = true; // Mark first successful connection
          setError(null); // Clear any previous errors on successful connection
          retryAttemptRef.current = 0; // Reset retry counter on successful connection
          setRetryAttemptDisplay(0);
          onConnectionChange?.(true);

          // Start video timeout - if video doesn't start within 15 seconds, Wolf pipeline likely failed
          // This catches GStreamer errors like resolution mismatches that cause silent hangs
          if (videoStartTimeoutRef.current) {
            clearTimeout(videoStartTimeoutRef.current);
          }
          videoStartTimeoutRef.current = setTimeout(() => {
            console.error('[MoonlightStreamViewer] Video start timeout - Wolf pipeline may have failed');
            setError('Video stream failed to start. The streaming server may have encountered a pipeline error. Click the Restart button (top right) to restart the session.');
            setIsConnecting(false);
            setIsConnected(false);
            onConnectionChange?.(false);
          }, VIDEO_START_TIMEOUT_MS);

          // Keep overlay visible until video/screenshot actually arrives
          // - 'high' mode: wait for videoStarted event (first WS keyframe)
          // - 'sse' mode: wait for first SSE keyframe (handled in SSE decoder)
          // - 'low' mode: wait for first screenshot (handled in screenshot polling)
          if (qualityMode === 'low') {
            setStatus('Waiting for screenshot...');
          } else {
            setStatus('Waiting for video...');
          }
          // isConnecting stays true until video/screenshot arrives

          // Auto-join lobby if in lobbies mode (after video starts playing)
          // Set pending flag - actual join triggered by onCanPlay handler
          if (wolfLobbyId && sessionId) {
            console.log('[AUTO-JOIN] Connection established, waiting for video to start before auto-join');
            setPendingAutoJoin(true);
          }
        } else if (data.type === 'videoStarted') {
          // First keyframe received and being decoded - video is now visible
          // Only relevant for WebSocket video mode ('high')
          console.log('[MoonlightStreamViewer] Video started - hiding connecting overlay');
          // Clear video start timeout - video arrived successfully
          if (videoStartTimeoutRef.current) {
            clearTimeout(videoStartTimeoutRef.current);
            videoStartTimeoutRef.current = null;
          }
          setIsConnecting(false);
          setStatus('Streaming active');
        } else if (data.type === 'error') {
          // Ignore errors during explicit close (e.g., bitrate change, mode switch)
          // These are expected and should not show error UI
          if (isExplicitlyClosingRef.current) {
            console.log('[MoonlightStreamViewer] Ignoring error during explicit close:', data.message);
            return;
          }

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
              reconnectRef.current(1000, 'Reconnecting...');
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
          // WebSocket disconnected
          console.log('[MoonlightStreamViewer] Stream disconnected');
          setIsConnected(false);
          onConnectionChange?.(false);

          // If explicitly closed (unmount, HMR, user-initiated disconnect), show Disconnected overlay
          // Otherwise, WebSocketStream will auto-reconnect, so show "Reconnecting..." state
          if (isExplicitlyClosingRef.current) {
            console.log('[MoonlightStreamViewer] Explicit close - showing Disconnected overlay');
            setIsConnecting(false);
            setStatus('Disconnected');
          } else {
            console.log('[MoonlightStreamViewer] Unexpected disconnect - will auto-reconnect');
            setIsConnecting(true);
            setStatus('Reconnecting...');
          }
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
          setStatus('Reconnecting...');
          setIsConnecting(true);
          connectRef.current();
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
  }, [sessionId, hostId, appId, width, height, audioEnabled, onConnectionChange, onError, helixApi, account, streamingMode, wolfLobbyId, onClientIdCalculated, qualityMode, userBitrate]);

  // Disconnect
  // preserveState: if true, don't reset status/isConnecting (used during planned reconnects)
  const disconnect = useCallback((preserveState = false) => {
    console.log('[MoonlightStreamViewer] disconnect() called, cleaning up stream resources, preserveState:', preserveState);

    // Mark as explicitly closing to prevent 'disconnected' event from showing "Reconnecting..." UI
    isExplicitlyClosingRef.current = true;

    // Clear any pending bandwidth recommendation (stale recommendations shouldn't persist across sessions)
    setBitrateRecommendation(null);

    // Cancel any pending reconnect timeout
    if (pendingReconnectTimeoutRef.current) {
      console.log('[MoonlightStreamViewer] Cancelling pending reconnect timeout in disconnect');
      clearTimeout(pendingReconnectTimeoutRef.current);
      pendingReconnectTimeoutRef.current = null;
    }

    // Cancel video start timeout to prevent false errors during intentional disconnect
    if (videoStartTimeoutRef.current) {
      clearTimeout(videoStartTimeoutRef.current);
      videoStartTimeoutRef.current = null;
    }

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
      if (sseVideoDecoderRef.current.state !== 'closed') {
        try {
          sseVideoDecoderRef.current.close();
        } catch (err) {
          console.warn('[MoonlightStreamViewer] Error closing SSE VideoDecoder:', err);
        }
      }
      sseVideoDecoderRef.current = null;
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

    if (streamRef.current) {
      // Properly close the stream to prevent "AlreadyStreaming" errors
      try {
        if (streamRef.current instanceof WebSocketStream) {
          // WebSocketStream (has close() method)
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
    // Only reset status/isConnecting if not preserving state (i.e., not a planned reconnect)
    if (!preserveState) {
      setIsConnecting(false);
      setStatus('Disconnected');
    }
    setPendingAutoJoin(false); // Reset auto-join state on disconnect
    setIsHighLatency(false); // Reset latency warning on disconnect
    setIsOnFallback(false); // Reset fallback state on disconnect
    console.log('[MoonlightStreamViewer] disconnect() completed');
  }, []);

  // Ref to connect function for use in setTimeout (avoids stale closure issues)
  const connectRef = useRef(connect);
  useEffect(() => { connectRef.current = connect; }, [connect]);

  // Reconnect with configurable delay and optional reason message
  // Default 1 second delay for fast reconnects - infrastructure is reliable now
  const reconnect = useCallback((delayMs = 1000, reason?: string) => {
    // CRITICAL: Cancel any pending reconnect to prevent duplicate streams
    // This happens when user rapidly changes bitrate or mode
    if (pendingReconnectTimeoutRef.current) {
      console.log('[MoonlightStreamViewer] Cancelling pending reconnect');
      clearTimeout(pendingReconnectTimeoutRef.current);
      pendingReconnectTimeoutRef.current = null;
    }

    // Show reason IMMEDIATELY (before disconnect) to avoid flashing 'Disconnected'
    if (reason) {
      setStatus(reason);
      setIsConnecting(true);
    }

    // Disconnect but preserve our status/isConnecting state
    disconnect(true);

    // Use ref to get latest connect function when timeout fires
    // This avoids stale closure issues when state changes during the delay
    pendingReconnectTimeoutRef.current = setTimeout(() => {
      pendingReconnectTimeoutRef.current = null;
      connectRef.current();
    }, delayMs);
  }, [disconnect]);

  // Ref to reconnect function for use in closures (avoids stale closure issues)
  const reconnectRef = useRef(reconnect);
  useEffect(() => { reconnectRef.current = reconnect; }, [reconnect]);

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

  // Handle streaming mode changes - reconnect when switching between websocket and webrtc
  // Note: SSE video is now controlled by qualityMode, not streamingMode
  useEffect(() => {
    if (previousStreamingModeRef.current === streamingMode) return;

    const prevMode = previousStreamingModeRef.current;
    const newMode = streamingMode;
    console.log('[MoonlightStreamViewer] Streaming mode changed from', prevMode, 'to', newMode);
    previousStreamingModeRef.current = newMode;

    // Switching between websocket and webrtc requires full reconnect (different protocols)
    console.log('[MoonlightStreamViewer] Full reconnect needed for mode switch');
    const modeLabel = newMode === 'webrtc' ? 'WebRTC' : 'WebSocket';
    // Use reconnectRef to get the latest reconnect function (avoids stale closure)
    reconnectRef.current(1000, `Switching to ${modeLabel}...`);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streamingMode, sessionId]); // Only trigger on mode/session changes, not on reconnect/isConnected changes

  // Track previous quality mode for hot-switching
  const previousQualityModeRef = useRef<'high' | 'sse' | 'low'>(qualityMode);

  // Hot-switch between quality modes without reconnecting
  // All three modes use the same WebSocket connection for input, just different video delivery:
  // - 'high': Video over WebSocket
  // - 'sse': Video over SSE (separate EventSource)
  // - 'low': Screenshot polling (separate HTTP requests)
  useEffect(() => {
    if (previousQualityModeRef.current === qualityMode) return;

    const prevMode = previousQualityModeRef.current;
    const newMode = qualityMode;
    console.log('[MoonlightStreamViewer] Quality mode changed from', prevMode, 'to', newMode);
    previousQualityModeRef.current = newMode;

    // Update fallback state immediately for UI feedback
    setIsOnFallback(newMode === 'low');

    // Only hot-switch if connected with WebSocket stream
    if (!isConnected || !streamRef.current || !(streamRef.current instanceof WebSocketStream)) {
      console.log('[MoonlightStreamViewer] Not connected or not WebSocket stream, skipping hot-switch');
      return;
    }

    const wsStream = streamRef.current as WebSocketStream;

    // Step 1: Teardown previous mode's video source
    if (prevMode === 'sse') {
      // Close SSE connection
      console.log('[MoonlightStreamViewer] Closing SSE for quality mode switch');
      if (sseEventSourceRef.current) {
        sseEventSourceRef.current.close();
        sseEventSourceRef.current = null;
      }
      if (sseVideoDecoderRef.current) {
        // Only close if not already closed
        if (sseVideoDecoderRef.current.state !== 'closed') {
          try {
            sseVideoDecoderRef.current.close();
          } catch (err) {
            console.warn('[SSE Video] Error closing decoder:', err);
          }
        }
        sseVideoDecoderRef.current = null;
      }
    } else if (prevMode === 'high') {
      // Disable WS video (will be re-enabled if switching back to 'high')
      console.log('[MoonlightStreamViewer] Disabling WS video for quality mode switch');
      wsStream.setVideoEnabled(false);
    }
    // 'low' mode: screenshot polling will auto-stop via shouldPollScreenshots becoming false

    // Step 2: Setup new mode's video source
    if (newMode === 'high') {
      // Enable WS video
      console.log('[MoonlightStreamViewer] Enabling WS video for high mode');
      wsStream.setVideoEnabled(true);
      if (canvasRef.current) {
        wsStream.setCanvas(canvasRef.current);
      }
    } else if (newMode === 'sse') {
      // CRITICAL: Disable WS video before opening SSE to prevent duplicate video streams
      // This is redundant if coming from 'high' (already disabled above) but ensures
      // WS video is definitely off regardless of previous mode
      console.log('[MoonlightStreamViewer] Disabling WS video before SSE setup');
      wsStream.setVideoEnabled(false);

      // Defensive cleanup: close any stale SSE resources before opening new ones
      // This handles edge cases like rapid mode cycling or race conditions
      if (sseEventSourceRef.current) {
        console.log('[MoonlightStreamViewer] Closing stale SSE EventSource before reopening');
        sseEventSourceRef.current.close();
        sseEventSourceRef.current = null;
      }
      if (sseVideoDecoderRef.current) {
        console.log('[MoonlightStreamViewer] Closing stale SSE decoder before reopening');
        if (sseVideoDecoderRef.current.state !== 'closed') {
          try {
            sseVideoDecoderRef.current.close();
          } catch (err) {
            console.warn('[SSE Video] Error closing stale decoder:', err);
          }
        }
        sseVideoDecoderRef.current = null;
      }

      // Open SSE connection for video
      const qualitySessionId = sessionId ? `${sessionId}-hq` : sessionId;
      const sseUrl = `/moonlight/api/sse/video?session_id=${encodeURIComponent(qualitySessionId || '')}`;
      console.log('[MoonlightStreamViewer] Opening SSE for video:', sseUrl);

      const eventSource = new EventSource(sseUrl, { withCredentials: true });
      sseEventSourceRef.current = eventSource;
      sseReceivedFirstKeyframeRef.current = false;

      // Reset SSE stats
      sseStatsRef.current = {
        framesDecoded: 0,
        framesDropped: 0,
        lastFrameTime: performance.now(),
        frameCount: 0,
        currentFps: 0,
        width: 0,
        height: 0,
        codecString: '',
        firstFramePtsUs: null,
        firstFrameArrivalTime: null,
        currentFrameLatencyMs: 0,
        frameLatencySamples: [],
      };

      // Setup SSE decoder using the hot-switch canvas
      if (sseCanvasRef.current) {
        const canvas = sseCanvasRef.current;
        const ctx = canvas.getContext('2d', { alpha: false, desynchronized: true });

        eventSource.addEventListener('init', async (e: MessageEvent) => {
          try {
            const init = JSON.parse(e.data);
            console.log('[SSE Video] Init from quality switch:', init);

            if (!init.width || !init.height || init.width <= 0 || init.height <= 0) {
              console.error('[SSE Video] Invalid init data:', init);
              return;
            }

            canvas.width = init.width;
            canvas.height = init.height;

            // Use shared codec utilities from websocket-stream.ts
            const codecString = codecToWebCodecsString(init.video_codec);
            console.log(`[SSE Video] Codec: ${codecString} (video_codec=0x${init.video_codec?.toString(16)})`);

            const decoder = new VideoDecoder({
              output: (frame: VideoFrame) => {
                if (ctx && canvas.width > 0 && canvas.height > 0) {
                  ctx.drawImage(frame, 0, 0);
                }
                frame.close();
                sseStatsRef.current.framesDecoded++;
                sseStatsRef.current.lastFrameTime = performance.now();
              },
              error: (err: Error) => {
                console.error('[SSE Video] Decoder error, will reconnect:', err);
                // Close SSE resources
                if (sseEventSourceRef.current) {
                  sseEventSourceRef.current.close();
                  sseEventSourceRef.current = null;
                }
                if (sseVideoDecoderRef.current && sseVideoDecoderRef.current.state !== 'closed') {
                  try {
                    sseVideoDecoderRef.current.close();
                  } catch (closeErr) {
                    console.warn('[SSE Video] Error closing decoder after error:', closeErr);
                  }
                }
                sseVideoDecoderRef.current = null;
                sseReceivedFirstKeyframeRef.current = false;
                // Reconnect with the same mode (reconnect preserves qualityMode)
                setTimeout(() => reconnectRef.current(1000), 500);
              },
            });

            // Configure decoder with Annex B format for in-band parameter sets
            const decoderConfig: VideoDecoderConfig = {
              codec: codecString,
              codedWidth: init.width,
              codedHeight: init.height,
              hardwareAcceleration: 'prefer-hardware',
            };
            // Add format hints for H264/HEVC - required for Annex B streams
            if (codecString.startsWith('avc1')) {
              // @ts-ignore - avc property is part of the spec but not in TypeScript types yet
              decoderConfig.avc = { format: 'annexb' };
            }
            if (codecString.startsWith('hvc1') || codecString.startsWith('hev1')) {
              // @ts-ignore - hevc property for Annex B format
              decoderConfig.hevc = { format: 'annexb' };
            }
            decoder.configure(decoderConfig);

            sseVideoDecoderRef.current = decoder;
            sseStatsRef.current.width = init.width;
            sseStatsRef.current.height = init.height;
            sseStatsRef.current.codecString = codecString;
          } catch (err) {
            console.error('[SSE Video] Failed to parse init:', err);
          }
        });

        eventSource.addEventListener('video', (e: MessageEvent) => {
          const decoder = sseVideoDecoderRef.current;
          if (!decoder || decoder.state !== 'configured') return;

          const arrivalTime = performance.now();

          try {
            const frame = JSON.parse(e.data);

            // Skip delta frames until first keyframe
            if (!sseReceivedFirstKeyframeRef.current) {
              if (!frame.keyframe) return;
              console.log('[SSE Video] First keyframe received - hiding connecting overlay');
              sseReceivedFirstKeyframeRef.current = true;
              // Clear video start timeout - video arrived successfully
              if (videoStartTimeoutRef.current) {
                clearTimeout(videoStartTimeoutRef.current);
                videoStartTimeoutRef.current = null;
              }
              // Hide the connecting overlay now that video is visible
              setIsConnecting(false);
              setStatus('Streaming active');
            }

            // Frame latency tracking (same algorithm as WebSocketStream)
            const ptsUs = frame.pts; // microseconds
            const stats = sseStatsRef.current;
            if (stats.firstFramePtsUs === null) {
              stats.firstFramePtsUs = ptsUs;
              stats.firstFrameArrivalTime = arrivalTime;
              stats.currentFrameLatencyMs = 0;
            } else {
              const ptsDeltaMs = (ptsUs - stats.firstFramePtsUs) / 1000;
              const expectedArrivalTime = stats.firstFrameArrivalTime! + ptsDeltaMs;
              const latencyMs = arrivalTime - expectedArrivalTime;
              stats.frameLatencySamples.push(latencyMs);
              if (stats.frameLatencySamples.length > 30) {
                stats.frameLatencySamples.shift();
              }
              stats.currentFrameLatencyMs = stats.frameLatencySamples.reduce((a, b) => a + b, 0) / stats.frameLatencySamples.length;
            }

            const binaryString = atob(frame.data);
            const bytes = new Uint8Array(binaryString.length);
            for (let i = 0; i < binaryString.length; i++) {
              bytes[i] = binaryString.charCodeAt(i);
            }

            // Debug: Log frame details for first few keyframes to compare with WebSocket
            if (frame.keyframe && sseStatsRef.current.framesDecoded < 3) {
              const hexBytes = Array.from(bytes.slice(0, 32))
                .map(b => b.toString(16).padStart(2, '0'))
                .join(' ');
              console.log(`[SSE Video] Keyframe #${sseStatsRef.current.framesDecoded + 1}: ${bytes.length} bytes, first 32: ${hexBytes}`);
              console.log(`[SSE Video] Decoder state: ${decoder.state}, decodeQueueSize: ${decoder.decodeQueueSize}`);
            }

            const chunk = new EncodedVideoChunk({
              type: frame.keyframe ? 'key' : 'delta',
              timestamp: frame.pts,
              data: bytes,
            });
            decoder.decode(chunk);
          } catch (err) {
            console.error('[SSE Video] Failed to decode frame:', err);
          }
        });

        eventSource.addEventListener('stop', () => {
          console.log('[SSE Video] Stopped');
          if (sseVideoDecoderRef.current) {
            if (sseVideoDecoderRef.current.state !== 'closed') {
              try {
                sseVideoDecoderRef.current.close();
              } catch (err) {
                console.warn('[SSE Video] Error closing decoder on stop:', err);
              }
            }
            sseVideoDecoderRef.current = null;
          }
        });

        eventSource.onerror = (err) => {
          console.error('[SSE Video] Error:', err);
        };
      }
    } else if (newMode === 'low') {
      // CRITICAL: Disable WS video for screenshot mode to prevent video streaming
      // This is redundant with the video control effect but ensures WS video is definitely off
      console.log('[MoonlightStreamViewer] Disabling WS video for screenshot mode');
      wsStream.setVideoEnabled(false);
      // Screenshot polling will auto-start via shouldPollScreenshots becoming true
    }
  }, [qualityMode, isConnected, sessionId]);

  // Handle initial connection with SSE quality mode
  // The hot-switch handler above only triggers on qualityMode CHANGES, not initial state
  // This effect runs once when first connected and sets up SSE if that's the initial mode
  const hasInitializedSseRef = useRef(false);
  useEffect(() => {
    // Only run once when first connected with SSE mode
    if (hasInitializedSseRef.current || !isConnected || qualityMode !== 'sse') {
      return;
    }

    // Check if we have a WebSocket stream
    if (!streamRef.current || !(streamRef.current instanceof WebSocketStream)) {
      return;
    }

    // Check if SSE was already set up by hot-switch (prevents duplicate EventSource)
    // This can happen when user switches to SSE mode after connecting in another mode
    if (sseEventSourceRef.current) {
      console.log('[MoonlightStreamViewer] SSE already initialized by hot-switch, skipping duplicate setup');
      hasInitializedSseRef.current = true;
      return;
    }

    console.log('[MoonlightStreamViewer] Initial connection with SSE mode - setting up SSE video');
    hasInitializedSseRef.current = true;

    const wsStream = streamRef.current as WebSocketStream;

    // Disable WS video
    wsStream.setVideoEnabled(false);

    // Open SSE connection for video
    const qualitySessionId = sessionId ? `${sessionId}-hq` : sessionId;
    const sseUrl = `/moonlight/api/sse/video?session_id=${encodeURIComponent(qualitySessionId || '')}`;
    console.log('[MoonlightStreamViewer] Opening SSE for initial video:', sseUrl);

    const eventSource = new EventSource(sseUrl, { withCredentials: true });
    sseEventSourceRef.current = eventSource;
    sseReceivedFirstKeyframeRef.current = false;

    // Reset SSE stats
    sseStatsRef.current = {
      framesDecoded: 0,
      framesDropped: 0,
      lastFrameTime: performance.now(),
      frameCount: 0,
      currentFps: 0,
      width: 0,
      height: 0,
      codecString: '',
      firstFramePtsUs: null,
      firstFrameArrivalTime: null,
      currentFrameLatencyMs: 0,
      frameLatencySamples: [],
    };

    // Setup SSE decoder using the hot-switch canvas
    if (sseCanvasRef.current) {
      const canvas = sseCanvasRef.current;
      const ctx = canvas.getContext('2d', { alpha: false, desynchronized: true });

      eventSource.addEventListener('init', async (e: MessageEvent) => {
        try {
          const init = JSON.parse(e.data);
          console.log('[SSE Video] Init from initial setup:', init);

          if (!init.width || !init.height || init.width <= 0 || init.height <= 0) {
            console.error('[SSE Video] Invalid init data:', init);
            return;
          }

          canvas.width = init.width;
          canvas.height = init.height;

          // Use shared codec utilities from websocket-stream.ts
          const codecString = codecToWebCodecsString(init.video_codec);
          console.log(`[SSE Video] Codec (initial): ${codecString} (video_codec=0x${init.video_codec?.toString(16)})`);

          const decoder = new VideoDecoder({
            output: (frame: VideoFrame) => {
              if (ctx && canvas.width > 0 && canvas.height > 0) {
                ctx.drawImage(frame, 0, 0);
              }
              frame.close();
              sseStatsRef.current.framesDecoded++;
              sseStatsRef.current.lastFrameTime = performance.now();
            },
            error: (err: Error) => {
              console.error('[SSE Video] Decoder error (initial), will reconnect:', err);
              // Close SSE resources
              if (sseEventSourceRef.current) {
                sseEventSourceRef.current.close();
                sseEventSourceRef.current = null;
              }
              if (sseVideoDecoderRef.current && sseVideoDecoderRef.current.state !== 'closed') {
                try {
                  sseVideoDecoderRef.current.close();
                } catch (closeErr) {
                  console.warn('[SSE Video] Error closing decoder after error:', closeErr);
                }
              }
              sseVideoDecoderRef.current = null;
              sseReceivedFirstKeyframeRef.current = false;
              hasInitializedSseRef.current = false; // Allow re-initialization
              // Reconnect with the same mode (reconnect preserves qualityMode)
              setTimeout(() => reconnectRef.current(1000), 500);
            },
          });

          // Configure decoder with Annex B format for in-band parameter sets
          const decoderConfig: VideoDecoderConfig = {
            codec: codecString,
            codedWidth: init.width,
            codedHeight: init.height,
            hardwareAcceleration: 'prefer-hardware',
          };
          // Add format hints for H264/HEVC - required for Annex B streams
          if (codecString.startsWith('avc1')) {
            // @ts-ignore - avc property is part of the spec but not in TypeScript types yet
            decoderConfig.avc = { format: 'annexb' };
          }
          if (codecString.startsWith('hvc1') || codecString.startsWith('hev1')) {
            // @ts-ignore - hevc property for Annex B format
            decoderConfig.hevc = { format: 'annexb' };
          }
          decoder.configure(decoderConfig);

          sseVideoDecoderRef.current = decoder;
          sseStatsRef.current.width = init.width;
          sseStatsRef.current.height = init.height;
          sseStatsRef.current.codecString = codecString;
        } catch (err) {
          console.error('[SSE Video] Failed to parse init:', err);
        }
      });

      eventSource.addEventListener('video', (e: MessageEvent) => {
        const decoder = sseVideoDecoderRef.current;
        if (!decoder || decoder.state !== 'configured') return;

        const arrivalTime = performance.now();

        try {
          const frame = JSON.parse(e.data);

          // Skip delta frames until first keyframe
          if (!sseReceivedFirstKeyframeRef.current) {
            if (!frame.keyframe) return;
            console.log('[SSE Video] First keyframe received (initial) - hiding connecting overlay');
            sseReceivedFirstKeyframeRef.current = true;
            // Clear video start timeout - video arrived successfully
            if (videoStartTimeoutRef.current) {
              clearTimeout(videoStartTimeoutRef.current);
              videoStartTimeoutRef.current = null;
            }
            // Hide the connecting overlay now that video is visible
            setIsConnecting(false);
            setStatus('Streaming active');
          }

          // Frame latency tracking (same algorithm as WebSocketStream)
          const ptsUs = frame.pts; // microseconds
          const stats = sseStatsRef.current;
          if (stats.firstFramePtsUs === null) {
            stats.firstFramePtsUs = ptsUs;
            stats.firstFrameArrivalTime = arrivalTime;
            stats.currentFrameLatencyMs = 0;
          } else {
            const ptsDeltaMs = (ptsUs - stats.firstFramePtsUs) / 1000;
            const expectedArrivalTime = stats.firstFrameArrivalTime! + ptsDeltaMs;
            const latencyMs = arrivalTime - expectedArrivalTime;
            stats.frameLatencySamples.push(latencyMs);
            if (stats.frameLatencySamples.length > 30) {
              stats.frameLatencySamples.shift();
            }
            stats.currentFrameLatencyMs = stats.frameLatencySamples.reduce((a, b) => a + b, 0) / stats.frameLatencySamples.length;
          }

          const binaryString = atob(frame.data);
          const bytes = new Uint8Array(binaryString.length);
          for (let i = 0; i < binaryString.length; i++) {
            bytes[i] = binaryString.charCodeAt(i);
          }

          // Debug: Log frame details for first few keyframes to compare with WebSocket
          if (frame.keyframe && sseStatsRef.current.framesDecoded < 3) {
            const hexBytes = Array.from(bytes.slice(0, 32))
              .map(b => b.toString(16).padStart(2, '0'))
              .join(' ');
            console.log(`[SSE Video] Keyframe #${sseStatsRef.current.framesDecoded + 1} (initial): ${bytes.length} bytes, first 32: ${hexBytes}`);
            console.log(`[SSE Video] Decoder state: ${decoder.state}, decodeQueueSize: ${decoder.decodeQueueSize}`);
          }

          const chunk = new EncodedVideoChunk({
            type: frame.keyframe ? 'key' : 'delta',
            timestamp: frame.pts,
            data: bytes,
          });
          decoder.decode(chunk);
        } catch (err) {
          console.error('[SSE Video] Failed to decode frame:', err);
        }
      });

      eventSource.addEventListener('stop', () => {
        console.log('[SSE Video] Stopped (initial)');
        if (sseVideoDecoderRef.current) {
          if (sseVideoDecoderRef.current.state !== 'closed') {
            try {
              sseVideoDecoderRef.current.close();
            } catch (err) {
              console.warn('[SSE Video] Error closing decoder on stop (initial):', err);
            }
          }
          sseVideoDecoderRef.current = null;
        }
      });

      eventSource.onerror = (err) => {
        console.error('[SSE Video] Error (initial):', err);
      };
    }
  }, [isConnected, qualityMode, sessionId]);

  // Reset SSE initialization flag when connection is lost
  // This allows SSE to be re-initialized on reconnect
  useEffect(() => {
    if (!isConnected) {
      hasInitializedSseRef.current = false;
    }
  }, [isConnected]);

  // Track previous user bitrate for reconnection
  // Initialize to a sentinel value (-1) to distinguish "not yet set" from "set to null"
  const previousUserBitrateRef = useRef<number | null | undefined>(undefined);

  // Reconnect when user bitrate changes (user selected new bitrate or adaptive reduction)
  // IMPORTANT: Skip reconnect during INITIAL connection only (before first successful connection)
  // The initial bandwidth probe sets userBitrate BEFORE calling connect(), so we must not
  // trigger a reconnect on that first bitrate change or we'll get double-connections
  // NOTE: No function dependencies to avoid re-running when connect/reconnect identities change
  useEffect(() => {
    // Skip on first render (previousUserBitrateRef is undefined)
    if (previousUserBitrateRef.current === undefined) {
      previousUserBitrateRef.current = userBitrate;
      return;
    }
    // Skip if we're in the INITIAL connection (started connecting but never connected yet)
    // hasConnectedRef = true means we've started connecting
    // hasEverConnectedRef = false means we've never successfully connected
    // This distinguishes initial connection from reconnection after a drop
    if (hasConnectedRef.current && !hasEverConnectedRef.current) {
      // Only log once per bitrate change, not on every re-render
      if (previousUserBitrateRef.current !== userBitrate) {
        console.log('[MoonlightStreamViewer] Skipping bitrate-change reconnect (initial connection in progress)');
      }
      previousUserBitrateRef.current = userBitrate;
      return;
    }
    // Reconnect if bitrate actually changed (including from null to a value)
    if (previousUserBitrateRef.current !== userBitrate) {
      const prevBitrate = previousUserBitrateRef.current;
      console.log('[MoonlightStreamViewer] Bitrate changed from', prevBitrate, 'to', userBitrate);

      // Build informative status so user knows WHY we're reconnecting
      const reason = userBitrate !== null
        ? `Connecting at ${userBitrate} Mbps...`
        : 'Reconnecting...';

      // Use reconnectRef to get the latest reconnect function (avoids stale closure)
      reconnectRef.current(1000, reason);
    }
    previousUserBitrateRef.current = userBitrate;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [userBitrate]); // Only trigger on bitrate changes, not on reconnect/isConnected changes

  // Detect lobby changes and reconnect (for test script restart scenarios)
  useEffect(() => {
    if (wolfLobbyId && previousLobbyIdRef.current && previousLobbyIdRef.current !== wolfLobbyId) {
      console.log('[MoonlightStreamViewer] Lobby changed from', previousLobbyIdRef.current, 'to', wolfLobbyId);
      console.log('[MoonlightStreamViewer] Disconnecting old stream and reconnecting to new lobby');
      // Use reconnectRef to get the latest reconnect function (avoids stale closure)
      reconnectRef.current(1000, 'Reconnecting to new lobby...');
    }
    previousLobbyIdRef.current = wolfLobbyId;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wolfLobbyId]); // Only trigger on lobby changes

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

  // Calculate optimal bitrate from measured throughput (with 25% headroom + extra pessimism)
  // We go down one notch from what we could theoretically support to be conservative
  const calculateOptimalBitrate = useCallback((throughputMbps: number): number => {
    const BITRATE_OPTIONS = [5, 10, 20, 40, 80];
    const maxSustainableBitrate = throughputMbps / 1.25;

    // Find highest bitrate option that fits
    let optimalIndex = 0;
    for (let i = BITRATE_OPTIONS.length - 1; i >= 0; i--) {
      if (BITRATE_OPTIONS[i] <= maxSustainableBitrate) {
        optimalIndex = i;
        break;
      }
    }

    // Be more pessimistic: go down one notch since quality difference is minimal
    // and we'd rather start low and recommend increasing than start high and have stuttering
    const pessimisticIndex = Math.max(0, optimalIndex - 1);
    return BITRATE_OPTIONS[pessimisticIndex];
  }, []);

  // Auto-connect when wolfLobbyId becomes available
  // wolfLobbyId is fetched asynchronously from session data, so it's undefined on initial render
  // If we connect before it's available, we use the wrong app_id (apps mode instead of lobbies mode)
  // NEW: Probe bandwidth FIRST, then connect at optimal bitrate (avoids reconnect on startup)
  const hasConnectedRef = useRef(false);
  const hasEverConnectedRef = useRef(false); // True after first successful connection (distinguishes initial vs reconnect)
  useEffect(() => {
    // Only auto-connect once
    if (hasConnectedRef.current) return;

    // If wolfLobbyId prop is expected but not yet loaded, wait for it
    // We detect this by checking if sessionId is provided (external agent mode)
    // In this mode, wolfLobbyId should be provided by the parent once session data loads
    if (sessionId && !wolfLobbyId) {
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wolfLobbyId, sessionId]); // Only trigger on props, not on function identity changes

  // Cleanup on unmount
  useEffect(() => {
    console.log('[MoonlightStreamViewer] Component mounted, setting up cleanup handler');
    return () => {
      console.log('[MoonlightStreamViewer] Component unmounting, calling disconnect()');
      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Only run on mount/unmount

  // Auto-focus container when stream connects for keyboard input
  useEffect(() => {
    if (isConnected && containerRef.current) {
      containerRef.current.focus();
    }
  }, [isConnected]);

  // Screenshot polling for low-quality mode (manual screenshot fallback)
  // Targets 2 FPS minimum (500ms max per frame)
  // Dynamically adjusts JPEG quality based on fetch time
  const shouldPollScreenshots = qualityMode === 'low';

  // Notify server to pause/resume video based on quality mode
  // - 'high': WS video enabled (main video source)
  // - 'sse': WS video disabled (SSE is the video source, handled by SSE setup)
  // - 'low': WS video disabled (screenshots are the video source)
  useEffect(() => {
    const stream = streamRef.current;
    if (!stream || !(stream instanceof WebSocketStream) || !isConnected) {
      return;
    }

    // Only control WS video for 'high' and 'low' modes
    // SSE mode handles its own video enable/disable in the SSE setup effects
    if (qualityMode === 'low') {
      console.log('[MoonlightStreamViewer] Screenshot mode - disabling WS video');
      stream.setVideoEnabled(false);
    } else if (qualityMode === 'high') {
      console.log('[MoonlightStreamViewer] High quality mode - enabling WS video');
      stream.setVideoEnabled(true);
    }
    // SSE mode: do nothing here - SSE setup/hot-switch handles video state
  }, [qualityMode, isConnected]);

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

    console.log('[MoonlightStreamViewer] Starting screenshot polling (low mode)');

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
            // Hide connecting overlay on first screenshot
            if (!oldUrl) {
              console.log('[Screenshot] First screenshot received - hiding connecting overlay');
              // Clear video start timeout - screenshot arrived successfully
              if (videoStartTimeoutRef.current) {
                clearTimeout(videoStartTimeoutRef.current);
                videoStartTimeoutRef.current = null;
              }
              setIsConnecting(false);
              setStatus('Streaming active');
            }
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

  // Note: Adaptive screenshot RTT detection removed - users manually switch to 'low' mode
  // for screenshot fallback. This simplifies the quality mode to just three options:
  // high (WS video), sse (SSE video), low (screenshots)

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

    // Only WebSocket streaming mode supports adaptive bitrate (WebRTC has its own congestion control)
    // Adaptive bitrate works for both 'high' and 'sse' quality modes within WebSocket streaming
    if (streamingMode !== 'websocket') {
      return;
    }

    // Screenshot mode doesn't have frame latency metrics
    if (qualityMode === 'low') {
      return;
    }

    const CHECK_INTERVAL_MS = 1000;       // Check every second (for congestion detection)
    const REDUCE_COOLDOWN_MS = 300000;    // Don't show another recommendation within 5 minutes
    const INCREASE_COOLDOWN_MS = 300000;  // Don't show another recommendation within 5 minutes
    const MANUAL_SELECTION_COOLDOWN_MS = 60000;  // Don't auto-reduce within 60s of user manually selecting bitrate
    const BITRATE_OPTIONS = [5, 10, 20, 40, 80]; // Available bitrates in ascending order
    const MIN_BITRATE = 5;
    const STABLE_CHECKS_FOR_INCREASE = 300; // Need 5 minutes of low frame drift before running bandwidth probe
    const CONGESTION_CHECKS_FOR_REDUCE = 3; // Need 3 consecutive high drift samples before reducing (dampening)
    const FRAME_DRIFT_THRESHOLD = 200;    // Reduce if frames arriving > 200ms late (positive drift = behind)

    const checkBandwidth = () => {
      const stream = streamRef.current;
      if (!stream) return;

      // Get frame drift from stream stats (the reliable metric for congestion detection)
      // Frame drift = how late frames are arriving compared to their PTS
      // Positive = frames arriving late (congestion), Negative = frames arriving early (buffered)
      let frameDrift = 0;

      if (qualityMode === 'sse') {
        // SSE mode: get frame latency from SSE stats (video comes via SSE, not WebSocket)
        frameDrift = sseStatsRef.current.currentFrameLatencyMs;
      } else if (stream instanceof WebSocketStream) {
        // WebSocket high mode: get frame latency from WebSocket stats
        const stats = stream.getStats();
        frameDrift = stats.frameLatencyMs;
      } else {
        return; // Unsupported stream type (WebRTC has its own congestion control)
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
            // Step down one tier - but show recommendation instead of auto-switching
            const currentIndex = BITRATE_OPTIONS.indexOf(currentBitrate);
            if (currentIndex > 0) {
              const newBitrate = BITRATE_OPTIONS[currentIndex - 1];
              console.log(`[AdaptiveBitrate] Sustained high frame drift (${congestionCheckCountRef.current} samples, ${frameDrift.toFixed(0)}ms), recommending: ${currentBitrate} -> ${newBitrate} Mbps`);

              // Show recommendation popup instead of auto-switching
              setBitrateRecommendation({
                type: 'decrease',
                targetBitrate: newBitrate,
                reason: `Your connection is experiencing delays (${frameDrift.toFixed(0)}ms frame drift)`,
                frameDrift: frameDrift,
              });

              // Reset counters so we don't keep re-recommending
              lastBitrateChangeRef.current = now;
              stableCheckCountRef.current = 0;
              congestionCheckCountRef.current = 0;
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
                  console.log(`[AdaptiveBitrate] Recommending upgrade: ${currentBitrate} → ${targetBitrate} Mbps`);

                  // Show recommendation popup instead of auto-switching
                  setBitrateRecommendation({
                    type: 'increase',
                    targetBitrate: targetBitrate,
                    reason: `Your connection has improved (measured ${measuredThroughputMbps.toFixed(0)} Mbps)`,
                    measuredThroughput: measuredThroughputMbps,
                  });

                  lastBitrateChangeRef.current = Date.now();
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
  }, [isConnected, streamingMode, qualityMode, userBitrate, requestedBitrate, runBandwidthProbe, addChartEvent]);

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
    // NOTE: HTML canvas elements default to 300x150, NOT 0! We must detect this and use
    // the intended resolution (width/height props) as fallback, otherwise the aspect ratio
    // calculation will be wrong (300/150 = 2.0 instead of 16:9 = 1.777).
    const canvas = canvasRef.current;
    const isDefaultDimensions = canvas.width === 300 && canvas.height === 150;
    const canvasWidth = isDefaultDimensions ? width : canvas.width;
    const canvasHeight = isDefaultDimensions ? height : canvas.height;

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
  }, [containerSize, width, height]);

  // Update canvas display size when canvas dimensions change (after first frame is rendered)
  useEffect(() => {
    if (!containerSize || !canvasRef.current || streamingMode !== 'websocket') return;

    const checkCanvasDimensions = () => {
      const canvas = canvasRef.current;
      if (!canvas || canvas.width === 0 || canvas.height === 0) return;

      // Skip if canvas still has HTML default dimensions (300x150)
      // Wait for actual video dimensions to be set by WebCodecs
      const isDefaultDimensions = canvas.width === 300 && canvas.height === 150;
      if (isDefaultDimensions) return;

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
      // Send to stream (WebSocketStream handles input for all quality modes)
      const input = streamRef.current && 'getInput' in streamRef.current
        ? (streamRef.current as WebSocketStream | Stream).getInput()
        : null;
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

    // WebSocket mode - poll stats from WebSocketStream
    // SSE video mode also uses WebSocketStream for input and session management
    if (streamingMode === 'websocket') {
      const pollWsStats = () => {
        const currentStream = streamRef.current;
        if (!currentStream) return;

        const wsStream = currentStream as WebSocketStream;
        const wsStats = wsStream.getStats();
        const isForcedLow = qualityMode === 'low';

        // In SSE quality mode, video stats come from SSE decoder, not WebSocket
        const sseStats = sseStatsRef.current;
        const isSSE = qualityMode === 'sse';

        // Determine codec string based on quality mode
        let codecDisplay: string;
        if (isForcedLow) {
          codecDisplay = 'JPEG (Screenshot)';
        } else if (isSSE) {
          codecDisplay = `${sseStats.codecString || 'Unknown'} (SSE)`;
        } else {
          codecDisplay = `${wsStats.codecString} (WebSocket)`;
        }

        setStats({
          video: {
            codec: codecDisplay,
            width: isForcedLow ? (width || 1920) : (isSSE ? sseStats.width : wsStats.width),
            height: isForcedLow ? (height || 1080) : (isSSE ? sseStats.height : wsStats.height),
            fps: isForcedLow ? screenshotFps : (isSSE ? sseStats.currentFps : wsStats.fps),
            videoPayloadBitrate: (isSSE || isForcedLow) ? 'N/A' : wsStats.videoPayloadBitrateMbps.toFixed(2),
            totalBitrate: (isSSE || isForcedLow) ? 'N/A' : wsStats.totalBitrateMbps.toFixed(2),
            framesDecoded: isForcedLow ? 0 : (isSSE ? sseStats.framesDecoded : wsStats.framesDecoded),
            framesDropped: isForcedLow ? 0 : (isSSE ? sseStats.framesDropped : wsStats.framesDropped),
            rttMs: wsStats.rttMs,                                              // RTT still from WebSocket
            isHighLatency: wsStats.isHighLatency,                              // High latency flag from WS
            // Batching stats (only for non-SSE mode)
            batchingRatio: isSSE ? 0 : wsStats.batchingRatio,
            avgBatchSize: isSSE ? 0 : wsStats.avgBatchSize,
            batchesReceived: isSSE ? 0 : wsStats.batchesReceived,
            // Frame latency and decoder queue (works in both WebSocket and SSE modes)
            frameLatencyMs: isSSE ? sseStats.currentFrameLatencyMs : wsStats.frameLatencyMs,
            batchingRequested: isSSE ? false : wsStats.batchingRequested,
            decodeQueueSize: isSSE ? 0 : wsStats.decodeQueueSize,
            maxDecodeQueueSize: isSSE ? 0 : wsStats.maxDecodeQueueSize,
            framesSkippedToKeyframe: isSSE ? 0 : wsStats.framesSkippedToKeyframe,
          },
          connection: {
            transport: isForcedLow
              ? 'Screenshot + WebSocket Input'
              : isSSE
              ? 'SSE Video + WebSocket Input'
              : 'WebSocket Video + Input',
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
        // Use SSE frame latency when in SSE mode, WebSocket frame latency otherwise
        const currentFrameDrift = isSSE ? sseStats.currentFrameLatencyMs : wsStats.frameLatencyMs;
        frameDriftHistoryRef.current = [...frameDriftHistoryRef.current, currentFrameDrift].slice(-CHART_HISTORY_LENGTH);
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

    // Use appropriate canvas/video element for each mode
    let element: HTMLCanvasElement | HTMLVideoElement | null = null;
    if (streamingMode === 'websocket') {
      // Always use canvasRef for WebSocket mode - it's our consistent input surface
      // (SSE canvas is just for rendering, input goes through the transparent canvasRef)
      element = canvasRef.current;
    } else {
      element = videoRef.current;
    }

    // If no element, return fallback (but with proper position approximation)
    if (!element) {
      return new DOMRect(0, 0, width, height);
    }

    const boundingRect = element.getBoundingClientRect();

    // For WebSocket mode: canvas is already sized to maintain aspect ratio,
    // so bounding rect IS the video content area (no letterboxing)
    // Note: We don't need streamRef here - the canvas position is correct regardless
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
    // Use stream's size if available, otherwise fall back to props (which are the intended resolution)
    const videoSize = streamRef.current?.getStreamerSize() || [width, height];
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

  // Get input handler - always from the main stream
  // SSE quality mode still uses the same WebSocketStream for input
  const getInputHandler = useCallback(() => {
    // For all modes, get input from the main stream
    if (streamRef.current && 'getInput' in streamRef.current) {
      return (streamRef.current as WebSocketStream | Stream).getInput();
    }
    return null;
  }, []);

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

    // Helper to get input handler (WebSocketStream handles input for all quality modes)
    const getInput = () => {
      return streamRef.current && 'getInput' in streamRef.current
        ? (streamRef.current as WebSocketStream | Stream).getInput()
        : null;
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
              onClick={() => reconnect(1000, 'Reconnecting...')}
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
              ? 'Currently: WebSocket (L7) — Click for WebRTC'
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
                // Toggle between websocket and webrtc (connection protocol)
                // Video source (WS/SSE/screenshot) is controlled by Speed toggle
                setModeSwitchCooldown(true);
                setStreamingMode(prev => prev === 'websocket' ? 'webrtc' : 'websocket');
                setTimeout(() => setModeSwitchCooldown(false), 3000); // 3 second cooldown
              }}
              sx={{
                color: streamingMode === 'websocket' ? 'primary.main' : 'white'
              }}
            >
              {streamingMode === 'websocket' ? (
                <Wifi fontSize="small" />
              ) : (
                <SignalCellularAlt fontSize="small" />
              )}
            </IconButton>
          </span>
        </Tooltip>
        {/* Quality mode toggle: WS Video (high) → SSE Video (sse) → Screenshots (low) */}
        {streamingMode === 'websocket' && (
          <Tooltip
            title={
              modeSwitchCooldown
                ? 'Please wait...'
                : qualityMode === 'high'
                ? 'WebSocket Video (60fps) — Click for SSE Video'
                : qualityMode === 'sse'
                ? 'SSE Video (60fps) — Click for Screenshot mode'
                : 'Screenshot mode — Click for WebSocket Video'
            }
            arrow
            slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}
          >
            <span>
              <IconButton
                size="small"
                disabled={modeSwitchCooldown}
                onClick={() => {
                  // Cycle: high → sse → low → high
                  // All modes use WebSocket for input, just different video delivery
                  setModeSwitchCooldown(true);
                  setQualityMode(prev => {
                    if (prev === 'high') return 'sse';
                    if (prev === 'sse') return 'low';
                    return 'high';
                  });
                  setTimeout(() => setModeSwitchCooldown(false), 3000); // 3 second cooldown
                }}
                sx={{
                  // Different colors for each mode
                  color: qualityMode === 'high'
                    ? '#4caf50'  // Green for WebSocket video
                    : qualityMode === 'sse'
                    ? '#2196f3'  // Blue for SSE
                    : '#ff9800',  // Orange for screenshot mode
                }}
              >
                {qualityMode === 'high' ? (
                  <Speed fontSize="small" />
                ) : qualityMode === 'sse' ? (
                  <StreamIcon fontSize="small" />
                ) : (
                  <CameraAlt fontSize="small" />
                )}
              </IconButton>
            </span>
          </Tooltip>
        )}
        {/* Bitrate selector - hidden in screenshot mode (has its own adaptive quality) */}
        {qualityMode !== 'low' && (
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
        )}
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
                // Clear any pending recommendation since user made an explicit choice
                setBitrateRecommendation(null);
                // Record manual selection time - adaptive algorithm will wait 20s before auto-reducing
                manualBitrateSelectionTimeRef.current = Date.now();
                // The userBitrate change effect will handle reconnecting with proper status message
                // Don't call reconnect here to avoid duplicate reconnects
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
        {/* Discreet bandwidth recommendation indicator */}
        {bitrateRecommendation && isConnected && (
          <Tooltip
            title={`${bitrateRecommendation.reason}. Click to switch.`}
            arrow
            slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}
          >
            <Button
              size="small"
              onClick={() => {
                setUserBitrate(bitrateRecommendation.targetBitrate);
                manualBitrateSelectionTimeRef.current = Date.now();
                addChartEvent(
                  bitrateRecommendation.type === 'decrease' ? 'reduce' : 'increase',
                  `User accepted: ${userBitrate ?? requestedBitrate}→${bitrateRecommendation.targetBitrate} Mbps`
                );
                setBitrateRecommendation(null);
              }}
              sx={{
                backgroundColor: bitrateRecommendation.type === 'decrease'
                  ? 'rgba(255, 152, 0, 0.9)'
                  : 'rgba(76, 175, 80, 0.9)',
                color: bitrateRecommendation.type === 'decrease' ? 'black' : 'white',
                fontSize: '0.65rem',
                px: 1,
                py: 0.25,
                minWidth: 'auto',
                textTransform: 'none',
                borderRadius: 1,
                '&:hover': {
                  backgroundColor: bitrateRecommendation.type === 'decrease'
                    ? 'rgba(255, 152, 0, 1)'
                    : 'rgba(76, 175, 80, 1)',
                },
              }}
            >
              {bitrateRecommendation.type === 'decrease'
                ? `Slow connection · Try ${bitrateRecommendation.targetBitrate}M`
                : `Improved · Try ${bitrateRecommendation.targetBitrate}M`}
            </Button>
          </Tooltip>
        )}
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
              Screenshot mode ({screenshotFps} FPS @ {screenshotQuality}% quality)
            </Typography>
          </Box>
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
          <Typography variant="h6" sx={{ color: 'white' }}>
            Disconnected
          </Typography>
          <Typography variant="body2" sx={{ color: 'grey.400', textAlign: 'center', maxWidth: 300 }}>
            {status || 'Connection lost'}
          </Typography>
          <Button
            variant="contained"
            color="primary"
            onClick={() => reconnect(1000, 'Reconnecting...')}
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
        onMouseEnter={resetInputState}
        onContextMenu={handleContextMenu}
        onCanPlay={() => {
          // WebRTC mode: hide overlay when video is ready to play
          if (streamingMode === 'webrtc') {
            console.log('[MoonlightStreamViewer] WebRTC video can play - hiding overlay');
            // Clear video start timeout - video arrived successfully
            if (videoStartTimeoutRef.current) {
              clearTimeout(videoStartTimeoutRef.current);
              videoStartTimeoutRef.current = null;
            }
            setIsConnecting(false);
            setStatus('Streaming active');
          }
        }}
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

      {/* Canvas Element (WebSocket mode only) - centered with proper aspect ratio */}
      <canvas
        ref={canvasRef}
        onMouseDown={handleMouseDown}
        onMouseUp={handleMouseUp}
        onMouseMove={handleMouseMove}
        onMouseEnter={resetInputState}
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
          // ALWAYS visible in WebSocket streaming mode for input capture
          // In 'high' mode: renders video AND handles input
          // In 'sse' mode: invisible (transparent) but handles input (SSE canvas renders on top)
          // In 'low' mode: invisible (transparent) but handles input (screenshot overlays)
          display: streamingMode === 'websocket' ? 'block' : 'none',
          // Make transparent in SSE/low modes so overlays are visible, but still captures input
          opacity: qualityMode === 'high' ? 1 : 0,
          // Higher z-index than SSE canvas so it captures input even when transparent
          zIndex: 20,
        }}
        onClick={() => {
          // Focus container for keyboard input
          if (containerRef.current) {
            containerRef.current.focus();
          }
        }}
      />

      {/* SSE Canvas Element (SSE mode only) - separate from WebSocket canvas to avoid conflicts */}
      <canvas
        ref={sseCanvasRef}
        onMouseDown={handleMouseDown}
        onMouseUp={handleMouseUp}
        onMouseMove={handleMouseMove}
        onMouseEnter={resetInputState}
        onContextMenu={handleContextMenu}
        style={{
          // Use calculated dimensions to maintain aspect ratio
          width: canvasDisplaySize ? `${canvasDisplaySize.width}px` : '100%',
          height: canvasDisplaySize ? `${canvasDisplaySize.height}px` : '100%',
          // Center the canvas within the container
          position: 'absolute',
          left: '50%',
          top: '50%',
          transform: 'translate(-50%, -50%)',
          backgroundColor: '#000',
          cursor: 'none',
          // Visible only in SSE mode AND WebSocket streaming (not WebRTC)
          // Must check both conditions - qualityMode persists when switching to WebRTC
          display: streamingMode === 'websocket' && qualityMode === 'sse' ? 'block' : 'none',
          // Lower z-index than WebSocket canvas so input passes through
          zIndex: 15,
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
            <div><strong>Transport:</strong> {streamingMode === 'websocket' ? (qualityMode === 'sse' ? 'SSE Video + WebSocket Input' : 'WebSocket (L7)') : 'WebRTC'}</div>
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
                {/* Frame latency (WebSocket and SSE modes) - actual delivery delay based on PTS */}
                {/* Positive = frames arriving late (bad), Negative = frames arriving early (good/buffered) */}
                {/* Hidden in screenshot mode since there's no video stream to measure */}
                {streamingMode === 'websocket' && qualityMode !== 'low' && stats.video.frameLatencyMs !== undefined && (
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
        const frameDriftHistory = frameDriftHistoryRef.current;

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

            {/* Frame Drift Chart - key metric for adaptive bitrate decisions */}
            <Box sx={{ flex: '1 1 400px', minWidth: 300 }}>
              <Typography variant="caption" sx={{ color: '#888', display: 'block', mb: 1 }}>
                Frame Drift (ms) - Positive = behind, triggers bitrate reduction
              </Typography>
              <Box sx={{ height: 150, ...chartContainerStyles }}>
                {frameDriftHistory.length > 1 ? (
                  <LineChart
                    xAxis={[{
                      data: frameDriftHistory.map((_, i) => i - frameDriftHistory.length + 1),
                      label: 'Seconds ago',
                      labelStyle: axisLabelStyle,
                    }]}
                    yAxis={[{
                      min: Math.min(Math.min(...frameDriftHistory), -100) * 1.2,
                      max: Math.max(Math.max(...frameDriftHistory), 300) * 1.2,
                      labelStyle: axisLabelStyle,
                    }]}
                    series={[
                      {
                        data: frameDriftHistory.map(() => 200), // Threshold line at 200ms
                        label: 'Reduction Threshold',
                        color: '#ff6b6b',
                        showMark: false,
                        curve: 'linear',
                      },
                      {
                        data: frameDriftHistory.map(() => 0), // Zero line
                        label: 'On Time',
                        color: '#4caf50',
                        showMark: false,
                        curve: 'linear',
                      },
                      {
                        data: frameDriftHistory,
                        label: 'Frame Drift',
                        color: frameDriftHistory[frameDriftHistory.length - 1] > 200 ? '#ff6b6b' : '#00c8ff',
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
 
