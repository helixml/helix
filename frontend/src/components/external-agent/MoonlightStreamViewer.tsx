import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton, Button, Tooltip } from '@mui/material';
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
} from '@mui/icons-material';
import KeyboardObservabilityPanel from './KeyboardObservabilityPanel';
import { getApi, apiGetApps } from '../../lib/moonlight-web-ts/api';
import { Stream } from '../../lib/moonlight-web-ts/stream/index';
import { WebSocketStream } from '../../lib/moonlight-web-ts/stream/websocket-stream';
import { DualStreamManager } from '../../lib/moonlight-web-ts/stream/dual-stream-manager';
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
  width = 3840,
  height = 2160,
  className = '',
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null); // Canvas for WebSocket-only mode
  const containerRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<Stream | WebSocketStream | DualStreamManager | null>(null); // Stream instance from moonlight-web
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
  const [requestedBitrate, setRequestedBitrate] = useState<number>(40); // Mbps
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

      // Get streaming bitrate from backend config (falls back to 40 Mbps)
      let streamingBitrateMbps = 40; // Default: 40 Mbps
      try {
        const configResponse = await apiClient.v1ConfigList();
        if (configResponse.data.streaming_bitrate_mbps) {
          streamingBitrateMbps = configResponse.data.streaming_bitrate_mbps;
          console.log(`[MoonlightStreamViewer] Using configured bitrate: ${streamingBitrateMbps} Mbps`);
        }
      } catch (err) {
        console.warn('[MoonlightStreamViewer] Failed to fetch streaming bitrate config, using default:', err);
      }

      // Store for stats display
      setRequestedBitrate(streamingBitrateMbps);

      // Get default stream settings and customize
      const settings = defaultStreamSettings();
      settings.videoSize = 'custom';
      settings.videoSizeCustom = { width: 1920, height: 1080 };  // 1080p resolution (AMD GPU hardware encoder limit)
      settings.bitrate = streamingBitrateMbps * 1000;  // Convert to kbps - Configured bitrate (P-frames more efficient than all I-frames)
      settings.packetSize = 1024;
      settings.fps = 60;
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

      let stream: Stream | WebSocketStream | DualStreamManager;

      // Check if using WebSocket-only mode
      if (streamingMode === 'websocket') {
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

      // Listen for stream events
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
  }, [sessionId, hostId, appId, width, height, audioEnabled, onConnectionChange, onError, helixApi, account, isPersonalDevEnvironment, streamingMode, wolfLobbyId, onClientIdCalculated, qualityMode]);

  // Disconnect
  const disconnect = useCallback(() => {
    console.log('[MoonlightStreamViewer] disconnect() called, cleaning up stream resources');

    if (streamRef.current) {
      // Properly close the stream to prevent "AlreadyStreaming" errors
      try {
        // Check if it's a DualStreamManager
        if (streamRef.current instanceof DualStreamManager) {
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

  // Track previous streaming mode for reconnection
  const previousStreamingModeRef = useRef<StreamingMode>(streamingMode);

  // Reconnect when streaming mode changes (user toggled the transport)
  // Uses 5-second delay to wait for moonlight-web session cleanup (prevents Wolf conflicts)
  // moonlight-web cleanup can take up to 15s (host.cancel() HTTP call), but 5s covers typical cases
  useEffect(() => {
    if (previousStreamingModeRef.current !== streamingMode) {
      console.log('[MoonlightStreamViewer] Streaming mode changed from', previousStreamingModeRef.current, 'to', streamingMode);
      console.log('[MoonlightStreamViewer] Using 5s delay to wait for moonlight-web cleanup');
      previousStreamingModeRef.current = streamingMode;
      reconnect(5000); // 5 seconds for mode switches to allow cleanup
    }
  }, [streamingMode, reconnect]);

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

  // Auto-connect when wolfLobbyId becomes available
  // wolfLobbyId is fetched asynchronously from session data, so it's undefined on initial render
  // If we connect before it's available, we use the wrong app_id (apps mode instead of lobbies mode)
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

    console.log('[MoonlightStreamViewer] Auto-connecting with wolfLobbyId:', wolfLobbyId);
    hasConnectedRef.current = true;
    connect();
  }, [wolfLobbyId, sessionId, isPersonalDevEnvironment, connect]);

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
      // Send to stream
      streamRef.current?.getInput().onMouseWheel(event);
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

    // WebSocket mode - poll stats from WebSocketStream or DualStreamManager
    if (streamingMode === 'websocket') {
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
          },
          connection: {
            transport: `WebSocket (L7)${isForcedLow ? ' - Force ~1fps' : qualityMode === 'high' ? ' - Force 60fps' : ''}`,
          },
          timestamp: new Date().toISOString(),
        });
        // Update high latency state for warning banner
        setIsHighLatency(wsStats.isHighLatency);
        // Show orange border for forced low quality mode
        setIsOnFallback(isForcedLow);
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
  }, [width, height, streamingMode]);

  // Input event handlers
  const handleMouseDown = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
    streamRef.current?.getInput().onMouseDown(event.nativeEvent, getStreamRect());
  }, [getStreamRect]);

  const handleMouseUp = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
    streamRef.current?.getInput().onMouseUp(event.nativeEvent);
  }, []);

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

    streamRef.current?.getInput().onMouseMove(event.nativeEvent, getStreamRect());
  }, [getStreamRect, hasMouseMoved]);

  const handleContextMenu = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
  }, []);

  // Reset all input state - clears stuck modifiers and mouse buttons
  const resetInputState = useCallback(() => {
    const input = streamRef.current?.getInput();
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

      // Intercept paste keystrokes for clipboard sync (cross-platform)
      const isCtrlV = event.ctrlKey && !event.shiftKey && event.code === 'KeyV';
      const isCmdV = event.metaKey && !event.shiftKey && event.code === 'KeyV';
      const isCtrlShiftV = event.ctrlKey && event.shiftKey && event.code === 'KeyV';
      const isCmdShiftV = event.metaKey && event.shiftKey && event.code === 'KeyV';
      const isPasteKeystroke = isCtrlV || isCmdV || isCtrlShiftV || isCmdShiftV;

      if (isPasteKeystroke && sessionId) {
        event.preventDefault();
        event.stopPropagation();

        console.log('[Clipboard] Paste keystroke detected, syncing local → remote');

        // Handle clipboard sync asynchronously (don't block keystroke processing)
        navigator.clipboard.read().then(clipboardItems => {
          if (clipboardItems.length === 0) {
            console.warn('[Clipboard] Empty clipboard, ignoring paste');
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
          }
        }).catch(err => {
          console.error('[Clipboard] Failed to read clipboard:', err);
        });

        // Helper function to sync clipboard and forward keystroke
        const syncAndPaste = (payload: TypesClipboardData) => {
          const apiClient = helixApi.getApiClient();
          apiClient.v1ExternalAgentsClipboardCreate(sessionId, payload).then(() => {
            console.log(`[Clipboard] Synced ${payload.type} to remote`);

            // Send Ctrl+Shift+V to remote (works in both terminals and regular apps)
            const input = streamRef.current?.getInput();
            if (input) {
              const ctrlShiftVDown = new KeyboardEvent('keydown', {
                code: 'KeyV',
                key: 'V',
                ctrlKey: true,
                shiftKey: true,
                metaKey: false,
                bubbles: true,
                cancelable: true,
              });
              input.onKeyDown(ctrlShiftVDown);

              const ctrlShiftVUp = new KeyboardEvent('keyup', {
                code: 'KeyV',
                key: 'V',
                ctrlKey: true,
                shiftKey: true,
                metaKey: false,
                bubbles: true,
                cancelable: true,
              });
              input.onKeyUp(ctrlShiftVUp);

              console.log('[Clipboard] Forwarded Ctrl+Shift+V to remote');
            }
          }).catch(err => {
            console.error('[Clipboard] Failed to sync clipboard:', err);
          });
        };

        return; // Don't fall through to default handler
      }

      console.log('[MoonlightStreamViewer] KeyDown captured:', event.key, event.code);
      streamRef.current?.getInput().onKeyDown(event);
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
      streamRef.current?.getInput().onKeyUp(event);
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
        <Tooltip title="Keyboard state monitor - debug key input issues" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size="small"
            onClick={() => setShowKeyboardPanel(!showKeyboardPanel)}
            sx={{ color: showKeyboardPanel ? 'primary.main' : 'white' }}
          >
            <Keyboard fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip title={modeSwitchCooldown ? 'Please wait...' : streamingMode === 'websocket' ? 'Currently: WebSocket — Click to switch to WebRTC' : 'Currently: WebRTC — Click to switch to WebSocket'} arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <span>
            <IconButton
              size="small"
              disabled={modeSwitchCooldown}
              onClick={() => {
                // Toggle mode with cooldown to prevent Wolf deadlock from rapid switching
                setModeSwitchCooldown(true);
                setStreamingMode(prev => prev === 'websocket' ? 'webrtc' : 'websocket');
                setTimeout(() => setModeSwitchCooldown(false), 3000); // 3 second cooldown
              }}
              sx={{ color: streamingMode === 'websocket' ? 'primary.main' : 'white' }}
            >
              {streamingMode === 'websocket' ? <Wifi fontSize="small" /> : <SignalCellularAlt fontSize="small" />}
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
          display: streamingMode === 'websocket' ? 'block' : 'none',
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

      {/* Keyboard State Monitor Panel */}
      {showKeyboardPanel && sessionId && (
        <KeyboardObservabilityPanel
          sandboxInstanceId={sessionId}
          onClose={() => setShowKeyboardPanel(false)}
        />
      )}
    </Box>
  );
};

export default MoonlightStreamViewer;
 
