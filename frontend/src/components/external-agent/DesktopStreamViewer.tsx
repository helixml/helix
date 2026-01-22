import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton, Button, Tooltip, Menu, MenuItem } from '@mui/material';
import {
  Fullscreen,
  FullscreenExit,
  Refresh,
  BarChart,
  Wifi,
  SignalCellularAlt,
  Speed,
  Stream as StreamIcon,
  Timeline,
  CameraAlt,
  TouchApp,
  PanTool,
} from '@mui/icons-material';
import {
  WebSocketStream,
  CursorImageData,
  RemoteUserInfo,
  RemoteCursorPosition,
  AgentCursorInfo,
  RemoteTouchInfo,
} from '../../lib/helix-stream/stream/websocket-stream';
import { defaultStreamSettings, getDefaultBitrateForResolution } from '../../lib/helix-stream/component/settings_menu';
import { getWebCodecsSupportedVideoFormats, getStandardVideoFormats } from '../../lib/helix-stream/stream/video';
import useApi from '../../hooks/useApi';
import { useAccount } from '../../contexts/account';
import { useVideoStream } from '../../contexts/VideoStreamContext';
import { TypesClipboardData } from '../../api/api';
import { DesktopStreamViewerProps, StreamStats, ActiveConnection, QualityMode } from './DesktopStreamViewer.types';
import StatsOverlay from './StatsOverlay';
import ChartsPanel from './ChartsPanel';
import ConnectionOverlay from './ConnectionOverlay';
import RemoteCursorsOverlay from './RemoteCursorsOverlay';
import AgentCursorOverlay from './AgentCursorOverlay';
import CursorRenderer from './CursorRenderer';

/**
 * DesktopStreamViewer - Native React component for desktop streaming
 *
 * This component provides video streaming from remote desktop sandboxes.
 *
 * Architecture:
 * - Uses WebSocket for video streaming and input
 * - WebSocketStream class manages the connection
 * - StreamInput handles mouse/keyboard/gamepad/touch
 * - Screenshot mode available as low-bandwidth fallback
 */
const DesktopStreamViewer: React.FC<DesktopStreamViewerProps> = ({
  sessionId,
  sandboxId,
  hostId = 0,
  appId = 1,
  onConnectionChange,
  onError,
  onClientIdCalculated,
  width = 1920,
  height = 1080,
  fps = 60,
  className = '',
  suppressOverlay = false,
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null); // Canvas for WebSocket video mode
  const containerRef = useRef<HTMLDivElement>(null);
  const hiddenInputRef = useRef<HTMLInputElement>(null); // Hidden input for iOS/iPad virtual keyboard
  const streamRef = useRef<WebSocketStream | null>(null); // WebSocket stream instance
  const retryAttemptRef = useRef(0); // Use ref to avoid closure issues
  const previousLobbyIdRef = useRef<string | undefined>(undefined); // Track lobby changes
  const isExplicitlyClosingRef = useRef(false); // Track explicit close to prevent spurious "Reconnecting..." state
  const pendingReconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null); // Cancel pending reconnects to prevent duplicate streams
  const manualReconnectAttemptsRef = useRef(0); // Track manual reconnect attempts to prevent infinite loops

  // Generate unique UUID for this component instance (persists across re-renders)
  // This ensures multiple floating windows get different streaming client IDs
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
  const [reconnectClicked, setReconnectClicked] = useState(false); // Immediate feedback when button clicked
  const [isVisible, setIsVisible] = useState(false); // Track if component is visible (for deferred connection)
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [cursorPosition, setCursorPosition] = useState({ x: 0, y: 0 });
  // Ref for direct DOM cursor updates (bypasses React for smooth 60fps cursor movement)
  const cursorPositionRef = useRef({ x: 0, y: 0 });
  const trackpadCursorRef = useRef<HTMLDivElement>(null);
  const [hasMouseMoved, setHasMouseMoved] = useState(false);
  const [isMouseOverCanvas, setIsMouseOverCanvas] = useState(false); // Track if mouse is over canvas for cursor visibility
  // Client-side cursor rendering state
  // Initialize with null/default to show native system pointer until server sends cursor
  const [cursorImage, setCursorImage] = useState<CursorImageData | null>(null);
  const [cursorCssName, setCursorCssName] = useState<string | null>('default'); // CSS cursor name fallback (GNOME headless)
  // Refs for cursor type (avoids stale closures in setTimeout callbacks)
  const cursorCssNameRef = useRef<string | null>('default');
  const cursorImageRef = useRef<CursorImageData | null>(null);
  const [cursorVisible, setCursorVisible] = useState(true);
  // Multi-player cursor state
  const [selfUser, setSelfUser] = useState<RemoteUserInfo | null>(null);
  const [selfClientId, setSelfClientId] = useState<number | null>(null);
  // Ref to track selfClientId synchronously (avoids stale closure in event handlers)
  const selfClientIdRef = useRef<number | null>(null);
  const [remoteUsers, setRemoteUsers] = useState<Map<number, RemoteUserInfo>>(new Map());
  const [remoteCursors, setRemoteCursors] = useState<Map<number, RemoteCursorPosition>>(new Map());
  const [agentCursor, setAgentCursor] = useState<AgentCursorInfo | null>(null);
  const [remoteTouches, setRemoteTouches] = useState<Map<string, RemoteTouchInfo>>(new Map());
  const [retryCountdown, setRetryCountdown] = useState<number | null>(null);
  const [retryAttemptDisplay, setRetryAttemptDisplay] = useState(0);
  const [showStats, setShowStats] = useState(false);
  const [requestedBitrate, setRequestedBitrate] = useState<number>(10); // Mbps (from backend config)
  const [userBitrate, setUserBitrate] = useState<number | null>(null); // User-selected bitrate (overrides backend)
  const [bitrateMenuAnchor, setBitrateMenuAnchor] = useState<null | HTMLElement>(null);
  const manualBitrateSelectionTimeRef = useRef<number>(0); // Track when user manually selected bitrate (20s cooldown before auto-reduce)
  // Bandwidth recommendation state - instead of auto-switching, we show a recommendation popup
  const [bitrateRecommendation, setBitrateRecommendation] = useState<{
    type: 'decrease' | 'increase' | 'screenshot';
    targetBitrate: number;
    reason: string;
    frameDrift?: number; // Current frame drift for decrease recommendations
    measuredThroughput?: number; // Measured throughput for increase recommendations
  } | null>(null);
  const [canvasDisplaySize, setCanvasDisplaySize] = useState<{ width: number; height: number } | null>(null);
  const [containerSize, setContainerSize] = useState<{ width: number; height: number } | null>(null);
  const [isHighLatency, setIsHighLatency] = useState(false); // Show warning when RTT > 150ms
  const [isThrottled, setIsThrottled] = useState(false); // Show warning when input throttling is active
  const [debugKeyEvent, setDebugKeyEvent] = useState<string | null>(null); // Debug: show last key event for iPad troubleshooting
  const [debugThrottleRatio, setDebugThrottleRatio] = useState<number | null>(null); // Debug override for throttle ratio
  // Quality mode: video or screenshot-based fallback
  // - 'video': 60fps video over WebSocket (default)
  // - 'screenshot': Screenshot-based polling (for low bandwidth)
  const [qualityMode, setQualityMode] = useState<QualityMode>('video'); // Default to WebSocket video
  const [isOnFallback, setIsOnFallback] = useState(false); // True when in screenshot mode
  const [modeSwitchCooldown, setModeSwitchCooldown] = useState(false); // Prevent rapid mode switching

  // Touch input mode: 'direct' (touch-to-click) or 'trackpad' (relative movement like a laptop trackpad)
  // - 'direct': Touch position = cursor position (default for desktop UIs)
  // - 'trackpad': Drag finger = move cursor relatively, tap = click, double-tap-drag = drag (better for mobile)
  const [touchMode, setTouchMode] = useState<'direct' | 'trackpad'>('direct');
  // Virtual keyboard height - used to shrink content when iOS/Android keyboard is open
  const [keyboardHeight, setKeyboardHeight] = useState(0);
  // Track if device has touch capability (only show touch mode toggle on touch devices)
  const [hasTouchCapability, setHasTouchCapability] = useState(false);
  // Touch tracking refs for trackpad mode gestures
  const lastTouchPosRef = useRef<{ x: number; y: number } | null>(null);
  const twoFingerStartYRef = useRef<number | null>(null);
  // Double-tap-and-drag gesture detection
  const lastTapTimeRef = useRef<number>(0);
  const touchStartTimeRef = useRef<number>(0); // Track when touch started (for tap vs drag detection)
  const touchMovedRef = useRef<boolean>(false); // Track if finger moved significantly during touch
  const [isDragging, setIsDragging] = useState(false); // True when in double-tap-drag mode (mouse button held)
  const pendingClickTimeoutRef = useRef<NodeJS.Timeout | null>(null); // Pending single-tap click (for double-tap detection)
  // Trackpad mode constants
  // Sensitivity: lower = more precise, less sensitive. 1.0 = 1:1 movement.
  // Mac trackpads typically use ~0.5-0.8 base sensitivity with acceleration.
  const TRACKPAD_CURSOR_SENSITIVITY = 0.8; // Base multiplier for cursor movement (reduced from 1.5)
  const DOUBLE_TAP_THRESHOLD_MS = 300; // Max time between taps for double-tap
  const TAP_MAX_DURATION_MS = 400; // Max touch duration to be considered a tap (increased from 200 for mobile)
  const TAP_MAX_MOVEMENT_PX = 15; // Max finger movement to be considered a tap (increased for touch screens)

  // Mouse button constants (X11/evdev standard: 1=left, 2=middle, 3=right)
  const MOUSE_BUTTON_LEFT = 1;
  const MOUSE_BUTTON_MIDDLE = 2;
  const MOUSE_BUTTON_RIGHT = 3;

  // Pinch-to-zoom state for mobile/tablet
  const [zoomLevel, setZoomLevel] = useState(1); // 1 = no zoom, 2 = 2x zoom, etc.
  const [panOffset, setPanOffset] = useState({ x: 0, y: 0 }); // Pan offset when zoomed
  const pinchStartDistanceRef = useRef<number | null>(null); // Distance between fingers at pinch start
  const pinchStartZoomRef = useRef<number>(1); // Zoom level at pinch start
  const pinchCenterRef = useRef<{ x: number; y: number } | null>(null); // Center point of pinch
  const lastPinchCenterRef = useRef<{ x: number; y: number } | null>(null); // For panning while zoomed
  const twoFingerGestureTypeRef = useRef<'undecided' | 'pinch' | 'scroll'>('undecided'); // Track gesture type
  const PINCH_VS_SCROLL_THRESHOLD = 30; // Pixels of distance change to classify as pinch vs scroll
  const SCROLL_SENSITIVITY = 2.0; // Multiplier for scroll speed
  const MIN_ZOOM = 1; // Minimum zoom (no zoom out beyond 1:1)
  const MAX_ZOOM = 5; // Maximum zoom level

  // iOS detection for video element fullscreen (iOS Safari doesn't support requestFullscreen on divs)
  const [isIOS, setIsIOS] = useState(false);
  // iOS custom fullscreen mode (not native video fullscreen - our custom overlay with full interaction)
  const [isIOSFullscreen, setIsIOSFullscreen] = useState(false);

  // Toolbar icon sizes - larger on touch devices for easier tapping
  const toolbarIconSize = hasTouchCapability ? 'medium' : 'small';
  const toolbarFontSize = hasTouchCapability ? 'medium' : 'small';

  // Screenshot-based low-quality mode state
  const [screenshotUrl, setScreenshotUrl] = useState<string | null>(null);
  const screenshotIntervalRef = useRef<NodeJS.Timeout | null>(null);
  // Track whether we're waiting for first screenshot after entering screenshot mode
  // This is used to hide the loading overlay - using a ref instead of checking screenshotUrl
  // to avoid race conditions when switching modes rapidly
  const waitingForFirstScreenshotRef = useRef(false);
  // Adaptive JPEG quality control - targets 2 FPS (500ms max per frame)
  const [screenshotQuality, setScreenshotQuality] = useState(70); // JPEG quality 10-90
  const [screenshotFps, setScreenshotFps] = useState(0); // Current FPS for display
  const screenshotQualityRef = useRef(70); // Ref for use in async callback

  // Clipboard sync state
  const lastRemoteClipboardHash = useRef<string>(''); // Track changes to avoid unnecessary writes
  const [stats, setStats] = useState<StreamStats | null>(null);

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

  // Register video stream with global context when connected in video mode
  // This allows other components (like screenshot thumbnails) to slow their polling
  const { registerStream } = useVideoStream();
  useEffect(() => {
    if (isConnected && qualityMode === 'video') {
      const unregister = registerStream();
      return unregister;
    }
  }, [isConnected, qualityMode, registerStream]);

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

  // Video start timeout - detect GStreamer pipeline failures that cause hangs
  const videoStartTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const VIDEO_START_TIMEOUT_MS = 15000; // 15 seconds to start video after connection
  const clipboardToastTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  // Auto-dismiss timeout for bitrate recommendation banner
  const bitrateRecommendationTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const BITRATE_RECOMMENDATION_DISMISS_MS = 15000; // 15 seconds before auto-dismiss

  // STREAM REGISTRY: Track all active streaming connections for debugging
  // This helps catch bugs where we accidentally have multiple streams active
  //
  // The streaming architecture has these connection types:
  // - 'websocket-stream': WebSocketStream instance (provides input)
  // - 'websocket-video-enabled': WS video is enabled on the WebSocket stream
  // - 'screenshot-polling': Screenshot HTTP polling for video (used with websocket-stream for input)
  //
  // Valid combinations:
  // - [websocket-stream, websocket-video-enabled] - WebSocket video mode
  // - [websocket-stream, screenshot-polling] - WebSocket + screenshots mode
  //
  const activeConnectionsRef = useRef<ActiveConnection[]>([]);
  const [activeConnectionsDisplay, setActiveConnectionsDisplay] = useState<ActiveConnection[]>([]);

  // Helper to generate unique stream ID
  const generateStreamId = useCallback(() => {
    return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  }, []);

  // Validate that current connections are in a valid state
  const validateConnectionState = useCallback(() => {
    const connections = activeConnectionsRef.current;
    const types = connections.map(c => c.type);

    // Check for invalid combinations
    const hasWebSocket = types.includes('websocket-stream');
    const hasWsVideo = types.includes('websocket-video-enabled');
    const hasScreenshot = types.includes('screenshot-polling');

    const videoSourceCount = [hasWsVideo, hasScreenshot].filter(Boolean).length;

    // Invalid: multiple video sources
    if (videoSourceCount > 1) {
      console.error('[StreamRegistry] INVALID: Multiple video sources active!', types);
      return false;
    }

    // Invalid: video source without transport
    if ((hasWsVideo || hasScreenshot) && !hasWebSocket) {
      console.error('[StreamRegistry] INVALID: Video source without WebSocket transport!', types);
      return false;
    }

    return true;
  }, []);

  // Register a new connection
  const registerConnection = useCallback((type: ActiveConnection['type']): string => {
    const id = generateStreamId();
    const connection: ActiveConnection = { id, type, createdAt: Date.now() };
    activeConnectionsRef.current.push(connection);
    setActiveConnectionsDisplay([...activeConnectionsRef.current]);

    console.log(`[StreamRegistry] Registered: ${type}:${id}`);
    validateConnectionState();
    return id;
  }, [generateStreamId, validateConnectionState]);

  // Unregister a connection
  const unregisterConnection = useCallback((id: string) => {
    const before = activeConnectionsRef.current.length;
    const removed = activeConnectionsRef.current.find(c => c.id === id);
    activeConnectionsRef.current = activeConnectionsRef.current.filter(c => c.id !== id);
    setActiveConnectionsDisplay([...activeConnectionsRef.current]);
    const after = activeConnectionsRef.current.length;
    if (before !== after && removed) {
      console.log(`[StreamRegistry] Unregistered: ${removed.type}:${id} (${before} → ${after} active)`);
    }
  }, []);

  // Clear all connections (used on disconnect)
  const clearAllConnections = useCallback(() => {
    if (activeConnectionsRef.current.length > 0) {
      console.log(`[StreamRegistry] Clearing all: ${activeConnectionsRef.current.map(c => c.type).join(', ')}`);
      activeConnectionsRef.current = [];
      setActiveConnectionsDisplay([]);
    }
  }, []);

  // Track IDs of current connections for cleanup
  const currentWebSocketStreamIdRef = useRef<string | null>(null);
  const currentWebSocketVideoIdRef = useRef<string | null>(null);
  const currentScreenshotVideoIdRef = useRef<string | null>(null);

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
    // CRITICAL: Close any existing stream FIRST to prevent duplicate streams
    // This is a belt-and-suspenders check - reconnect() should have called disconnect(),
    // but this ensures we never have two streams active at once even if connect() is
    // called directly or there's a race condition
    if (streamRef.current) {
      console.log('[DesktopStreamViewer] Closing existing stream before creating new one');
      try {
        if (streamRef.current instanceof WebSocketStream) {
          streamRef.current.close();
        } else {
          // Fallback: close WebSocket directly
          if ((streamRef.current as any).ws) {
            (streamRef.current as any).ws.close();
          }
          if ((streamRef.current as any).peer) {
            (streamRef.current as any).peer.close();
          }
        }
      } catch (err) {
        console.warn('[DesktopStreamViewer] Error closing existing stream:', err);
      }
      streamRef.current = null;
    }

    // Clear all connection registrations from previous connection
    clearAllConnections();
    currentWebSocketStreamIdRef.current = null;
    currentWebSocketVideoIdRef.current = null;
    currentScreenshotVideoIdRef.current = null;

    // Reset explicit close flag - we're starting a new connection
    isExplicitlyClosingRef.current = false;

    // Generate fresh UUID for EVERY connection attempt to avoid stale state on reconnect
    componentInstanceIdRef.current = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = Math.random() * 16 | 0;
      const v = c === 'x' ? r : (r & 0x3 | 0x8);
      return v.toString(16);
    });

    setIsConnecting(true);
    setError(null);
    setStatus('Connecting to streaming server...');

    try {
      const apiClient = helixApi.getApiClient();

      // App ID is not used for WebSocket mode - we connect directly to the container
      let actualAppId = appId;

      // Get Helix JWT from account context
      const helixToken = account.user?.token || '';

      if (!helixToken) {
        console.error('[DesktopStreamViewer] No token available');
        throw new Error('Not authenticated - please log in');
      }

      // API object for WebSocketStream (credentials used for auth)
      const api = {
        host_url: `/api/v1`,
        credentials: helixToken,
      };

      // Get streaming bitrate: user-selected > backend config > resolution-based default
      // Default: 10 Mbps for 4K, 5 Mbps for 1080p and below
      const defaultBitrateKbps = getDefaultBitrateForResolution(width, height);
      let streamingBitrateMbps = defaultBitrateKbps / 1000;

      if (userBitrate !== null) {
        // User explicitly selected a bitrate - use it
        streamingBitrateMbps = userBitrate;
        console.log(`[DesktopStreamViewer] Using user-selected bitrate: ${streamingBitrateMbps} Mbps`);
      } else {
        // Try to get from backend config
        try {
          const configResponse = await apiClient.v1ConfigList();
          if (configResponse.data.streaming_bitrate_mbps) {
            streamingBitrateMbps = configResponse.data.streaming_bitrate_mbps;
            console.log(`[DesktopStreamViewer] Using configured bitrate: ${streamingBitrateMbps} Mbps`);
          }
        } catch (err) {
          console.warn('[DesktopStreamViewer] Failed to fetch streaming bitrate config, using default:', err);
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
      settings.playAudioLocal = false; // Audio is controlled via setAudioEnabled control message

      // Check for videoMode URL param to switch between capture pipelines
      // Usage: ?videoMode=native or ?videoMode=zerocopy
      const urlParams = new URLSearchParams(window.location.search);
      const videoModeParam = urlParams.get('videoMode');
      if (videoModeParam === 'native' || videoModeParam === 'zerocopy' || videoModeParam === 'shm') {
        settings.videoMode = videoModeParam;
        console.log('[DesktopStreamViewer] Using videoMode from URL param:', videoModeParam);
      }

      // Detect browser codec support using WebCodecs
      console.log('[DesktopStreamViewer] Detecting WebCodecs supported codecs...');
      const supportedFormats = await getWebCodecsSupportedVideoFormats();
      console.log('[DesktopStreamViewer] WebCodecs supported formats:', supportedFormats);

      // Create WebSocketStream instance
      console.log('[DesktopStreamViewer] Creating WebSocketStream', {
        hostId,
        actualAppId,
        sessionId,
        qualityMode,
      });

      const streamSettings = { ...settings };

      if (qualityMode === 'screenshot') {
        console.log('[DesktopStreamViewer] Screenshot mode: WebSocket for input + screenshot overlay');
      } else {
        console.log('[DesktopStreamViewer] Video mode: WebSocket for video and input');
      }

      const stream = new WebSocketStream(
        api,
        hostId,
        actualAppId,
        streamSettings,
        supportedFormats,
        [width, height],
        sessionId,
        undefined, // clientUniqueId
        account.user?.name, // userName for multi-player presence
        undefined // avatarUrl
      );

      // Set canvas for WebSocket stream rendering
      if (canvasRef.current) {
        if (qualityMode !== 'screenshot') {
          // Video mode: stream renders frames to canvas
          stream.setCanvas(canvasRef.current);
        } else {
          // Screenshot mode: stream is only used for input, not video rendering
          // But we still need to set canvas dimensions for proper mouse coordinate mapping
          canvasRef.current.width = 1920;
          canvasRef.current.height = 1080;
        }
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
          hasEverConnectedRef.current = true; // Mark first successful connection
          setError(null); // Clear any previous errors on successful connection
          retryAttemptRef.current = 0; // Reset retry counter on successful connection
          manualReconnectAttemptsRef.current = 0; // Reset manual reconnect counter on successful connection
          setRetryAttemptDisplay(0);
          onConnectionChange?.(true);

          // Register WebSocket stream connection
          if (currentWebSocketStreamIdRef.current) {
            unregisterConnection(currentWebSocketStreamIdRef.current);
          }
          currentWebSocketStreamIdRef.current = registerConnection('websocket-stream');

          // Start video timeout - if video doesn't start within 15 seconds, GStreamer pipeline likely failed
          // This catches GStreamer errors like resolution mismatches that cause silent hangs
          if (videoStartTimeoutRef.current) {
            clearTimeout(videoStartTimeoutRef.current);
          }
          videoStartTimeoutRef.current = setTimeout(() => {
            console.error('[DesktopStreamViewer] Video start timeout - GStreamer pipeline may have failed');
            setError('Video stream failed to start. The desktop may have encountered a pipeline error. Click the Restart button (top right) to restart the session.');
            setIsConnecting(false);
            setIsConnected(false);
            onConnectionChange?.(false);
          }, VIDEO_START_TIMEOUT_MS);

          // Keep overlay visible until video/screenshot actually arrives
          // - 'video' mode: wait for videoStarted event (first WS keyframe)
          // - 'screenshot' mode: wait for first screenshot (handled in screenshot polling)
          if (qualityMode === 'screenshot') {
            setStatus('Waiting for screenshot...');
            // Mark that we're waiting for first screenshot - this is checked by the
            // screenshot polling effect to know when to hide the loading overlay
            waitingForFirstScreenshotRef.current = true;
            // CRITICAL: Disable video on server when starting in screenshot mode
            // This prevents the server from sending video frames we can't render
            console.log('[DesktopStreamViewer] Starting in screenshot mode - disabling WS video');
            stream.setVideoEnabled(false);
          } else {
            setStatus('Waiting for video...');
          }
          // isConnecting stays true until video/screenshot arrives

        } else if (data.type === 'videoStarted') {
          // First keyframe received and being decoded - video is now visible
          console.log('[DesktopStreamViewer] Video started - hiding connecting overlay');
          // Clear video start timeout - video arrived successfully
          if (videoStartTimeoutRef.current) {
            clearTimeout(videoStartTimeoutRef.current);
            videoStartTimeoutRef.current = null;
          }
          // Register WebSocket video enabled (unregister any previous)
          if (currentWebSocketVideoIdRef.current) {
            unregisterConnection(currentWebSocketVideoIdRef.current);
          }
          currentWebSocketVideoIdRef.current = registerConnection('websocket-video-enabled');
          setIsConnecting(false);
          setStatus('Streaming active');
        } else if (data.type === 'error') {
          // Ignore errors during explicit close (e.g., bitrate change, mode switch)
          // These are expected and should not show error UI
          if (isExplicitlyClosingRef.current) {
            console.log('[DesktopStreamViewer] Ignoring error during explicit close:', data.message);
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

            console.warn(`[DesktopStreamViewer] AlreadyStreaming error from stream (attempt ${nextAttempt}), will retry in ${retryDelaySeconds} seconds...`);

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
              console.log(`[DesktopStreamViewer] Retrying connection after AlreadyStreaming stream error (attempt ${nextAttempt})`);
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
          // IMPORTANT: Only handle if this event is from the CURRENT stream.
          // The OLD stream's close event fires asynchronously and may arrive after
          // a NEW stream has been created. Ignore events from old streams.
          if (stream !== streamRef.current) {
            console.log('[DesktopStreamViewer] Ignoring disconnected from old stream');
            return;
          }

          console.log('[DesktopStreamViewer] Stream disconnected');
          setIsConnected(false);
          onConnectionChange?.(false);

          // If explicitly closed (unmount, HMR, user-initiated disconnect), show Disconnected overlay
          // Otherwise, WebSocketStream will auto-reconnect, so show "Reconnecting..." state
          if (isExplicitlyClosingRef.current) {
            console.log('[DesktopStreamViewer] Explicit close - showing Disconnected overlay');
            setIsConnecting(false);
            setStatus('Disconnected');
          } else {
            console.log('[DesktopStreamViewer] Unexpected disconnect - will auto-reconnect');
            setIsConnecting(true);
            setStatus('Reconnecting...');
          }
        } else if (data.type === 'reconnecting') {
          // Show reconnection attempt in status
          console.log(`[DesktopStreamViewer] Reconnecting attempt ${data.attempt}`);
          setIsConnecting(true);
          setStatus(`Reconnecting (attempt ${data.attempt})...`);
        } else if (data.type === 'reconnectAborted') {
          // WebSocketStream refused to reconnect (this.closed was true unexpectedly)
          // IMPORTANT: Only handle if this event is from the CURRENT stream.
          // The OLD stream's close event fires asynchronously and may arrive after
          // a NEW stream has been created. Ignore events from old streams.
          if (stream !== streamRef.current) {
            console.log('[DesktopStreamViewer] Ignoring reconnectAborted from old stream');
            return;
          }

          console.warn('[DesktopStreamViewer] Reconnect aborted by stream:', data.reason);

          // If we weren't explicitly closing, this is unexpected - try to reconnect ourselves
          // But limit to 3 attempts to prevent infinite loops
          const MAX_MANUAL_RECONNECT_ATTEMPTS = 3;
          if (!isExplicitlyClosingRef.current && manualReconnectAttemptsRef.current < MAX_MANUAL_RECONNECT_ATTEMPTS) {
            manualReconnectAttemptsRef.current++;
            console.log(`[DesktopStreamViewer] Unexpected reconnect abort - will manually reconnect (attempt ${manualReconnectAttemptsRef.current}/${MAX_MANUAL_RECONNECT_ATTEMPTS})`);
            reconnectRef.current(1000, 'Reconnecting...');
          } else if (manualReconnectAttemptsRef.current >= MAX_MANUAL_RECONNECT_ATTEMPTS) {
            // Too many manual reconnect attempts - give up and show error
            console.error('[DesktopStreamViewer] Max manual reconnect attempts reached, giving up');
            setError('Connection failed repeatedly. Please refresh the page to try again.');
            setIsConnecting(false);
            setStatus('Connection failed');
            manualReconnectAttemptsRef.current = 0; // Reset for next user-initiated reconnect
          } else {
            // We were explicitly closing, show disconnected state
            console.log('[DesktopStreamViewer] Reconnect aborted during explicit close');
            setIsConnecting(false);
            setStatus('Disconnected');
          }
        }
        // Cursor events
        else if (data.type === 'cursorImage') {
          // Attribute cursor shape to the user who caused it
          // Use ref to avoid stale closure issues
          const currentSelfId = selfClientIdRef.current;
          const lastMover = data.lastMoverID;

          // If lastMover is set and it's NOT us, update the remote user's cursor
          if (lastMover && lastMover !== currentSelfId) {
            // Another user caused the cursor shape change - update their remote cursor
            setRemoteCursors(prev => {
              const existing = prev.get(lastMover);
              if (existing) {
                return new Map(prev).set(lastMover, { ...existing, cursorImage: data.cursor });
              }
              return prev;
            });
          } else {
            // It's our movement, or no tracking info (single-user mode) - update local cursor
            setCursorImage(data.cursor);
            cursorImageRef.current = data.cursor;
          }
        } else if (data.type === 'cursorPosition') {
          // DON'T update cursor position from server - use locally tracked position only
          // Server position creates feedback loop + lag. Local mouse tracking is authoritative.
          // DON'T update hotspot from cursorPosition events - this can cause mismatches
          // when the server sends a hotspot for a cached cursor we don't have.
          // The hotspot should only be updated via cursorImage events which include
          // both the cursor image AND the correct hotspot together.
        } else if (data.type === 'cursorName') {
          // CSS cursor name fallback when pixel capture fails (GNOME headless mode)
          // Attribute cursor shape to the user who caused it (like cursorImage)
          const currentSelfId = selfClientIdRef.current;
          const lastMover = data.lastMoverID;

          if (lastMover && lastMover !== currentSelfId) {
            // Another user caused the cursor shape change - update their remote cursor
            setRemoteCursors(prev => {
              const existing = prev.get(lastMover);
              if (existing) {
                // Create a cursorImage-like object with just the cursorName for rendering
                return new Map(prev).set(lastMover, {
                  ...existing,
                  cursorImage: { cursorId: 0, hotspotX: data.hotspotX, hotspotY: data.hotspotY, width: 24, height: 24, imageUrl: '', cursorName: data.cursorName },
                });
              }
              return prev;
            });
          } else {
            // It's our movement - update local cursor
            setCursorImage(null);
            setCursorCssName(data.cursorName);
            cursorImageRef.current = null;
            cursorCssNameRef.current = data.cursorName;
          }
        }
        // Multi-player cursor events
        else if (data.type === 'remoteCursor') {
          setRemoteCursors(prev => {
            const existing = prev.get(data.cursor.userId);
            // Merge with existing to preserve cursorImage (set by CursorImage events)
            const updated = {
              ...existing,       // Preserve cursorImage if it exists
              ...data.cursor,    // Update position, color, lastSeen
            };
            return new Map(prev).set(data.cursor.userId, updated);
          });
        } else if (data.type === 'remoteUserJoined') {
          // Use ref to check selfClientId (avoids stale closure - ref was set synchronously by selfId event)
          const currentSelfClientId = selfClientIdRef.current;
          const isThisMe = data.user.userId === currentSelfClientId;
          console.log('[MULTIPLAYER_DEBUG] DesktopStreamViewer remoteUserJoined:', {
            userId: data.user.userId,
            userName: data.user.userName,
            selfClientIdRef: currentSelfClientId,
            isThisMe
          });
          // Check if this is ourselves using the clientId from SelfId message (reliable)
          if (isThisMe) {
            console.log('[MULTIPLAYER_DEBUG] This is our own user info, setting selfUser');
            setSelfUser(data.user);
          }
          setRemoteUsers(prev => new Map(prev).set(data.user.userId, data.user));
        } else if (data.type === 'remoteUserLeft') {
          setRemoteUsers(prev => {
            const next = new Map(prev);
            next.delete(data.userId);
            return next;
          });
          setRemoteCursors(prev => {
            const next = new Map(prev);
            next.delete(data.userId);
            return next;
          });
        } else if (data.type === 'agentCursor') {
          // Always update agent cursor state (visibility handled in render based on lastSeen)
          setAgentCursor(data.agent);
        } else if (data.type === 'remoteTouch') {
          const touchKey = `${data.touch.userId}-${data.touch.touchId}`;
          if (data.touch.eventType === 'end' || data.touch.eventType === 'cancel') {
            setRemoteTouches(prev => {
              const next = new Map(prev);
              next.delete(touchKey);
              return next;
            });
          } else {
            setRemoteTouches(prev => new Map(prev).set(touchKey, data.touch));
          }
        } else if (data.type === 'selfId') {
          // Server tells us our assigned clientId
          // Update ref synchronously to avoid stale closure issues in subsequent event handlers
          selfClientIdRef.current = data.clientId;
          console.log('[MULTIPLAYER_DEBUG] DesktopStreamViewer received selfId:', data.clientId, 'ref updated synchronously');
          setSelfClientId(data.clientId);
        }
        });

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

        console.warn(`[DesktopStreamViewer] AlreadyStreaming error detected (attempt ${nextAttempt}), will retry in ${retryDelaySeconds} seconds...`);

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
          console.log(`[DesktopStreamViewer] Retrying connection after AlreadyStreaming error (attempt ${nextAttempt})`);
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
  }, [sessionId, hostId, appId, width, height, onConnectionChange, onError, helixApi, account, sandboxId, onClientIdCalculated, qualityMode, userBitrate]);

  // Disconnect
  // preserveState: if true, don't reset status/isConnecting (used during planned reconnects)
  const disconnect = useCallback((preserveState = false) => {
    console.log('[DesktopStreamViewer] disconnect() called, cleaning up stream resources, preserveState:', preserveState);

    // Mark as explicitly closing to prevent 'disconnected' event from showing "Reconnecting..." UI
    isExplicitlyClosingRef.current = true;

    // Clear any pending bandwidth recommendation (stale recommendations shouldn't persist across sessions)
    setBitrateRecommendation(null);

    // Cancel any pending reconnect timeout
    if (pendingReconnectTimeoutRef.current) {
      console.log('[DesktopStreamViewer] Cancelling pending reconnect timeout in disconnect');
      clearTimeout(pendingReconnectTimeoutRef.current);
      pendingReconnectTimeoutRef.current = null;
    }

    // Cancel video start timeout to prevent false errors during intentional disconnect
    if (videoStartTimeoutRef.current) {
      clearTimeout(videoStartTimeoutRef.current);
      videoStartTimeoutRef.current = null;
    }

    if (streamRef.current) {
      // Properly close the stream to prevent "AlreadyStreaming" errors
      try {
        console.log('[DesktopStreamViewer] Closing WebSocketStream...');
        streamRef.current.close();
      } catch (err) {
        console.warn('[DesktopStreamViewer] Error during stream cleanup:', err);
      }

      streamRef.current = null;
      console.log('[DesktopStreamViewer] Stream reference cleared');
    } else {
      console.log('[DesktopStreamViewer] No active stream to disconnect');
    }

    setIsConnected(false);
    // Only reset status/isConnecting if not preserving state (i.e., not a planned reconnect)
    if (!preserveState) {
      setIsConnecting(false);
      setStatus('Disconnected');
    }
    setIsHighLatency(false); // Reset latency warning on disconnect
    setIsOnFallback(false); // Reset fallback state on disconnect

    // Clear presence state - other users are no longer visible when disconnected
    setRemoteUsers(new Map());
    setRemoteCursors(new Map());
    setRemoteTouches(new Map());
    setAgentCursor(null);
    setSelfUser(null);
    setSelfClientId(null);
    selfClientIdRef.current = null;

    // Clear all connection registrations
    clearAllConnections();
    currentWebSocketStreamIdRef.current = null;
    currentWebSocketVideoIdRef.current = null;
    currentScreenshotVideoIdRef.current = null;

    console.log('[DesktopStreamViewer] disconnect() completed');
  }, [clearAllConnections]);

  // Ref to connect function for use in setTimeout (avoids stale closure issues)
  const connectRef = useRef(connect);
  useEffect(() => { connectRef.current = connect; }, [connect]);

  // Reconnect with configurable delay and optional reason message
  // Default 1 second delay for fast reconnects - infrastructure is reliable now
  const reconnect = useCallback((delayMs = 1000, reason?: string) => {
    // CRITICAL: Cancel any pending reconnect to prevent duplicate streams
    // This happens when user rapidly changes bitrate or mode
    if (pendingReconnectTimeoutRef.current) {
      console.log('[DesktopStreamViewer] Cancelling pending reconnect');
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

  // Toggle fullscreen - with cross-browser support (Chrome, Safari, Firefox)
  // On iOS, uses custom CSS fullscreen since native video fullscreen doesn't support interaction
  const toggleFullscreen = useCallback(() => {
    if (!containerRef.current) return;

    const elem = containerRef.current as any;
    const doc = document as any;

    // Check current fullscreen state (including iOS custom fullscreen)
    const currentlyFullscreen = isFullscreen || isIOSFullscreen;

    if (!currentlyFullscreen) {
      // On iOS, use custom CSS-based fullscreen that maintains full interactivity
      // Native video fullscreen (webkitEnterFullscreen) doesn't allow touch/keyboard input
      if (isIOS) {
        console.log('[Fullscreen] Using iOS custom CSS fullscreen for full interactivity');
        setIsIOSFullscreen(true);
        setIsFullscreen(true);
        // Focus container for keyboard input
        containerRef.current?.focus();
        return;
      }

      // Try all fullscreen APIs in order of preference
      // Standard API (Chrome, Firefox, Edge, Safari 16.4+)
      if (elem.requestFullscreen) {
        elem.requestFullscreen().catch(() => {
          // Fallback to iOS-style CSS fullscreen if native fails
          console.log('[Fullscreen] Native fullscreen failed, using CSS fallback');
          setIsIOSFullscreen(true);
          setIsFullscreen(true);
        });
      }
      // Webkit (Safari, iOS Safari, iOS Chrome - all use WebKit)
      else if (elem.webkitRequestFullscreen) {
        elem.webkitRequestFullscreen();
      }
      // Webkit with capital S (older Android Chrome)
      else if (elem.webkitRequestFullScreen) {
        elem.webkitRequestFullScreen();
      }
      // Mozilla (older Firefox)
      else if (elem.mozRequestFullScreen) {
        elem.mozRequestFullScreen();
      }
      // MS (old IE/Edge)
      else if (elem.msRequestFullscreen) {
        elem.msRequestFullscreen();
      }
      // Last resort: CSS fullscreen
      else {
        console.log('[Fullscreen] No native API available, using CSS fallback');
        setIsIOSFullscreen(true);
        setIsFullscreen(true);
      }
    } else {
      // Exit fullscreen
      // If using iOS custom fullscreen, just toggle the state
      if (isIOSFullscreen) {
        setIsIOSFullscreen(false);
        setIsFullscreen(false);
        return;
      }

      // Exit native fullscreen
      if (doc.exitFullscreen) {
        doc.exitFullscreen().catch(() => {});
      } else if (doc.webkitExitFullscreen) {
        doc.webkitExitFullscreen();
      } else if (doc.webkitCancelFullScreen) {
        doc.webkitCancelFullScreen();
      } else if (doc.mozCancelFullScreen) {
        doc.mozCancelFullScreen();
      } else if (doc.msExitFullscreen) {
        doc.msExitFullscreen();
      }
    }
  }, [isFullscreen, isIOSFullscreen, isIOS]);

  // Handle fullscreen events (cross-browser support)
  useEffect(() => {
    const doc = document as any;
    const handleFullscreenChange = () => {
      const fullscreenElement = doc.fullscreenElement ||
        doc.webkitFullscreenElement ||
        doc.webkitCurrentFullScreenElement ||
        doc.mozFullScreenElement ||
        doc.msFullscreenElement;
      setIsFullscreen(!!fullscreenElement);
    };

    // Listen for all vendor-prefixed fullscreen change events
    document.addEventListener('fullscreenchange', handleFullscreenChange);
    document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
    document.addEventListener('mozfullscreenchange', handleFullscreenChange);
    document.addEventListener('MSFullscreenChange', handleFullscreenChange);
    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
      document.removeEventListener('webkitfullscreenchange', handleFullscreenChange);
      document.removeEventListener('mozfullscreenchange', handleFullscreenChange);
      document.removeEventListener('MSFullscreenChange', handleFullscreenChange);
    };
  }, []);

  // Detect touch capability, iOS, and phone vs tablet on mount
  const [isPhone, setIsPhone] = useState(false);
  useEffect(() => {
    // Check for touch support: touchscreen or coarse pointer (touch/stylus)
    const hasTouch = 'ontouchstart' in window ||
      navigator.maxTouchPoints > 0 ||
      window.matchMedia('(pointer: coarse)').matches;
    setHasTouchCapability(hasTouch);

    // Detect iOS (iPhone, iPad, iPod) - needed for video element fullscreen
    const iOS = /iPad|iPhone|iPod/.test(navigator.userAgent) ||
      (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
    setIsIOS(iOS);

    // Detect phones (iPhone or Android phone) - for virtual keyboard handling
    // Phones have narrow screens, tablets are wider
    const isIPhone = /iPhone/.test(navigator.userAgent);
    const isAndroidPhone = /Android/.test(navigator.userAgent) && /Mobile/.test(navigator.userAgent);
    setIsPhone(isIPhone || isAndroidPhone);
  }, []);

  // Track virtual keyboard height via visualViewport API (phones only)
  // When keyboard opens on iOS/Android phones, visualViewport.height shrinks
  // Also track viewport offset to handle zoom scenarios
  const [viewportOffset, setViewportOffset] = useState(0);
  useEffect(() => {
    if (!window.visualViewport || !isPhone) return;

    const handleResize = () => {
      const viewport = window.visualViewport;
      if (!viewport) return;

      // Calculate keyboard height accounting for zoom
      // visualViewport.height is the visible height after keyboard opens
      // We need to use scale to handle pinch-zoom scenarios
      const scale = viewport.scale || 1;
      const visibleHeight = viewport.height;

      // Keyboard height = difference between layout height and visible height
      // Account for zoom by using the scaled visible height
      const layoutHeight = window.innerHeight;
      const kbHeight = Math.max(0, Math.round((layoutHeight - visibleHeight) * scale));

      // Track viewport offset (how much the viewport has scrolled due to zoom/keyboard)
      const offset = viewport.offsetTop || 0;

      setKeyboardHeight(kbHeight);
      setViewportOffset(offset);
    };

    window.visualViewport.addEventListener('resize', handleResize);
    window.visualViewport.addEventListener('scroll', handleResize);
    // Initial check
    handleResize();

    return () => {
      window.visualViewport?.removeEventListener('resize', handleResize);
      window.visualViewport?.removeEventListener('scroll', handleResize);
    };
  }, [isPhone]);

  // Auto-fullscreen on landscape rotation for touch devices (mobile/tablet)
  useEffect(() => {
    if (!hasTouchCapability) return;

    const handleOrientationChange = () => {
      const doc = document as any;
      const elem = containerRef.current as any;

      // Check if device is in landscape orientation
      const isLandscape = window.innerWidth > window.innerHeight;
      const fullscreenElement = doc.fullscreenElement ||
        doc.webkitFullscreenElement ||
        doc.webkitCurrentFullScreenElement ||
        doc.mozFullScreenElement ||
        doc.msFullscreenElement;

      if (isLandscape && !fullscreenElement && elem) {
        // Auto-enter fullscreen when rotated to landscape
        console.log('[DesktopStreamViewer] Landscape detected, entering fullscreen');
        // Try all fullscreen APIs
        if (elem.requestFullscreen) {
          elem.requestFullscreen().catch(() => {
            console.log('[DesktopStreamViewer] Fullscreen request failed (requires user gesture)');
          });
        } else if (elem.webkitRequestFullscreen) {
          elem.webkitRequestFullscreen();
        } else if (elem.webkitRequestFullScreen) {
          elem.webkitRequestFullScreen();
        } else if (elem.mozRequestFullScreen) {
          elem.mozRequestFullScreen();
        } else if (elem.msRequestFullscreen) {
          elem.msRequestFullscreen();
        }
      } else if (!isLandscape && fullscreenElement) {
        // Exit fullscreen when rotated back to portrait
        console.log('[DesktopStreamViewer] Portrait detected, exiting fullscreen');
        if (doc.exitFullscreen) {
          doc.exitFullscreen().catch(() => {});
        } else if (doc.webkitExitFullscreen) {
          doc.webkitExitFullscreen();
        } else if (doc.webkitCancelFullScreen) {
          doc.webkitCancelFullScreen();
        } else if (doc.mozCancelFullScreen) {
          doc.mozCancelFullScreen();
        } else if (doc.msExitFullscreen) {
          doc.msExitFullscreen();
        }
      }
    };

    // Listen for orientation changes (screen.orientation API) and resize (fallback)
    if (screen.orientation) {
      screen.orientation.addEventListener('change', handleOrientationChange);
    }
    window.addEventListener('resize', handleOrientationChange);

    return () => {
      if (screen.orientation) {
        screen.orientation.removeEventListener('change', handleOrientationChange);
      }
      window.removeEventListener('resize', handleOrientationChange);
    };
  }, [hasTouchCapability]);

  // Load touch mode preference from localStorage on mount
  useEffect(() => {
    const savedTouchMode = localStorage.getItem('desktopStreamTouchMode');
    if (savedTouchMode === 'trackpad' || savedTouchMode === 'direct') {
      setTouchMode(savedTouchMode);
    }
  }, []);

  // Save touch mode preference to localStorage when it changes
  useEffect(() => {
    localStorage.setItem('desktopStreamTouchMode', touchMode);
  }, [touchMode]);

  // Update input config when touch mode changes
  useEffect(() => {
    const input = streamRef.current?.getInput();
    if (input) {
      // Map UI touch mode to StreamInput touchMode
      // 'direct' → 'touch' (sends direct touch events to server)
      // 'trackpad' → 'mouseRelative' (relative mouse movement, tap=click, two-finger=scroll)
      // Note: We handle double-tap-drag ourselves in the touch handlers
      const streamTouchMode = touchMode === 'trackpad' ? 'mouseRelative' : 'touch';
      input.setConfig({ touchMode: streamTouchMode } as any);
      console.log(`[DesktopStreamViewer] Touch mode changed to ${touchMode} (stream: ${streamTouchMode})`);
    }
  }, [touchMode]);

  // Initialize cursor position at center of stream when entering trackpad mode on touch devices
  // Also re-initialize when stream connects (canvas may not exist until connected)
  useEffect(() => {
    if (touchMode === 'trackpad' && hasTouchCapability && isConnected && containerRef.current) {
      const containerRect = containerRef.current.getBoundingClientRect();
      // Get stream rect from the canvas or video element
      const streamRect = containerRef.current.querySelector('canvas, video')?.getBoundingClientRect();
      if (streamRect && streamRect.width > 0 && streamRect.height > 0) {
        // Position cursor at center of stream
        const centerX = (streamRect.x - containerRect.x) + streamRect.width / 2;
        const centerY = (streamRect.y - containerRect.y) + streamRect.height / 2;
        setCursorPosition({ x: centerX, y: centerY });
        cursorPositionRef.current = { x: centerX, y: centerY };
        // Update DOM directly for immediate visual feedback
        if (trackpadCursorRef.current) {
          trackpadCursorRef.current.style.transform = `translate(${centerX}px, ${centerY}px)`;
        }
        // Mark as moved so cursor is visible
        setHasMouseMoved(true);
        console.log(`[DesktopStreamViewer] Initialized trackpad cursor at center: (${centerX.toFixed(0)}, ${centerY.toFixed(0)})`);
      }
    }
  }, [touchMode, hasTouchCapability, isConnected]);

  // Track previous quality mode for hot-switching
  const previousQualityModeRef = useRef<'video' | 'screenshot'>(qualityMode);

  // Hot-switch between quality modes without reconnecting
  // - 'video': Video over WebSocket
  // - 'screenshot': Screenshot polling (separate HTTP requests)
  useEffect(() => {
    if (previousQualityModeRef.current === qualityMode) return;

    const prevMode = previousQualityModeRef.current;
    const newMode = qualityMode;
    console.log('[DesktopStreamViewer] Quality mode changed from', prevMode, 'to', newMode);
    previousQualityModeRef.current = newMode;

    // Update fallback state immediately for UI feedback
    setIsOnFallback(newMode === 'screenshot');

    // Only hot-switch if connected with WebSocket stream
    if (!isConnected || !streamRef.current) {
      console.log('[DesktopStreamViewer] Not connected, skipping hot-switch');
      return;
    }

    const wsStream = streamRef.current;

    // Teardown previous mode
    if (prevMode === 'video') {
      // Disable WS video when switching to screenshot mode
      console.log('[DesktopStreamViewer] Disabling WS video for quality mode switch');
      wsStream.setVideoEnabled(false);
      // Unregister WebSocket video connection
      if (currentWebSocketVideoIdRef.current) {
        unregisterConnection(currentWebSocketVideoIdRef.current);
        currentWebSocketVideoIdRef.current = null;
      }
    }
    // 'screenshot' mode: screenshot polling will auto-stop via shouldPollScreenshots becoming false

    // Setup new mode
    if (newMode === 'video') {
      // Enable WS video
      console.log('[DesktopStreamViewer] Enabling WS video for video mode');
      // Show loading overlay while waiting for first video frame
      // The videoStarted event will hide it (handler already exists for initial connection)
      setIsConnecting(true);
      setStatus('Switching to video stream...');
      wsStream.setVideoEnabled(true);
      if (canvasRef.current) {
        wsStream.setCanvas(canvasRef.current);
      }
    } else if (newMode === 'screenshot') {
      // Disable WS video for screenshot mode
      console.log('[DesktopStreamViewer] Disabling WS video for screenshot mode');
      setIsConnecting(true);
      setStatus('Switching to screenshots...');
      waitingForFirstScreenshotRef.current = true;
      wsStream.setVideoEnabled(false);
    }
  }, [qualityMode, isConnected, sessionId]);

  // NOTE: We only support WebSocket video + screenshots (no alternative transport modes)


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
        console.log('[DesktopStreamViewer] Skipping bitrate-change reconnect (initial connection in progress)');
      }
      previousUserBitrateRef.current = userBitrate;
      return;
    }
    // Reconnect if bitrate actually changed (including from null to a value)
    if (previousUserBitrateRef.current !== userBitrate) {
      const prevBitrate = previousUserBitrateRef.current;
      console.log('[DesktopStreamViewer] Bitrate changed from', prevBitrate, 'to', userBitrate);

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
    if (sandboxId && previousLobbyIdRef.current && previousLobbyIdRef.current !== sandboxId) {
      console.log('[DesktopStreamViewer] Lobby changed from', previousLobbyIdRef.current, 'to', sandboxId);
      console.log('[DesktopStreamViewer] Disconnecting old stream and reconnecting to new lobby');
      // Use reconnectRef to get the latest reconnect function (avoids stale closure)
      reconnectRef.current(1000, 'Reconnecting to new lobby...');
    }
    previousLobbyIdRef.current = sandboxId;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sandboxId]); // Only trigger on lobby changes

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

  // Auto-connect when sandboxId becomes available AND component is visible
  // sandboxId is fetched asynchronously from session data, so it's undefined on initial render
  // If we connect before it's available, we use the wrong app_id (apps mode instead of lobbies mode)
  // NEW: Wait for visibility before connecting (saves bandwidth when component not in view)
  // NEW: Probe bandwidth FIRST, then connect at optimal bitrate (avoids reconnect on startup)
  const hasConnectedRef = useRef(false);
  const hasEverConnectedRef = useRef(false); // True after first successful connection (distinguishes initial vs reconnect)
  useEffect(() => {
    // Only auto-connect once
    if (hasConnectedRef.current) return;

    // Wait for auth context to initialize before attempting connection
    // This prevents "Not authenticated" errors during hot reload when auth state isn't loaded yet
    if (!account.initialized) {
      console.log('[DesktopStreamViewer] Waiting for auth context to initialize...');
      return;
    }

    // Wait for component to become visible before connecting
    // This prevents wasting bandwidth on hidden tabs/components
    if (!isVisible) {
      console.log('[DesktopStreamViewer] Waiting for component to become visible before connecting...');
      return;
    }

    // WebSocketStream connects directly via /api/v1/external-agents/{sessionId}/ws/stream
    // Lower encoder latency and more consistent frame pacing outweigh quality benefits
    hasConnectedRef.current = true;
    setIsConnecting(true);
    // Use resolution-based default: 10 Mbps for 4K, 5 Mbps for 1080p and below
    const defaultBitrate = getDefaultBitrateForResolution(width, height) / 1000;
    console.log(`[DesktopStreamViewer] Auto-connecting at ${defaultBitrate} Mbps for ${width}x${height}`);
    setUserBitrate(defaultBitrate);
    setRequestedBitrate(defaultBitrate);
    connect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sandboxId, sessionId, isVisible, width, height, account.initialized]); // Only trigger on props and visibility, not on function identity changes

  // Cleanup on unmount
  useEffect(() => {
    console.log('[DesktopStreamViewer] Component mounted, setting up cleanup handler');
    return () => {
      console.log('[DesktopStreamViewer] Component unmounting, calling disconnect()');
      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Only run on mount/unmount

  // iOS Safari fix: Force reconnect when page becomes visible
  // iOS Safari can suspend WebSockets without properly closing them, leaving the stream black
  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible' && isConnected) {
        console.log('[DesktopStreamViewer] Page became visible, checking stream health...');
        // Check if the stream is still healthy by looking at the WebSocket state
        const stream = streamRef.current;
        if (stream) {
          const ws = (stream as any).ws as WebSocket | undefined;
          if (ws && (ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING)) {
            console.log('[DesktopStreamViewer] WebSocket was closed while page was hidden, forcing reconnect');
            reconnect(500, 'Reconnecting after page visibility change...');
          }
        }
      }
    };

    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
  }, [isConnected, reconnect]);

  // iOS Safari frame stall detection
  // iOS Safari can silently break the VideoDecoder without triggering error callbacks,
  // leaving the canvas black while React still thinks we're connected.
  // This health check monitors lastFrameRenderTime and forces reconnection if frames stop.
  const FRAME_STALL_THRESHOLD_MS = 5000; // 5 seconds without frames = stall
  const FRAME_STALL_CHECK_INTERVAL_MS = 3000; // Check every 3 seconds
  useEffect(() => {
    // Only run health check in video mode when connected
    if (!isConnected || qualityMode === 'screenshot' || isConnecting) {
      return;
    }

    const checkFrameHealth = () => {
      const stream = streamRef.current;
      if (!stream || !(stream instanceof WebSocketStream)) return;

      // Check WebSocket state first (belt and suspenders with visibility handler)
      const ws = (stream as any).ws as WebSocket | undefined;
      if (ws && (ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING)) {
        console.log('[DesktopStreamViewer] Frame health check: WebSocket closed, forcing reconnect');
        reconnect(500, 'Reconnecting (connection lost)...');
        return;
      }

      // Check if frames have been rendered recently
      const stats = stream.getStats();
      const now = performance.now();
      const timeSinceLastFrame = stats.lastFrameRenderTime > 0 ? now - stats.lastFrameRenderTime : 0;

      // Only trigger stall detection after we've received at least one frame (lastFrameRenderTime > 0)
      // and if we've been connected long enough for frames to be expected
      if (stats.lastFrameRenderTime > 0 && timeSinceLastFrame > FRAME_STALL_THRESHOLD_MS) {
        console.log(`[DesktopStreamViewer] Frame stall detected: ${Math.round(timeSinceLastFrame)}ms since last frame, forcing reconnect`);
        console.log('[DesktopStreamViewer] Stats at stall:', {
          fps: stats.fps,
          framesDecoded: stats.framesDecoded,
          decodeQueueSize: stats.decodeQueueSize,
          wsReadyState: ws?.readyState,
        });
        reconnect(500, 'Reconnecting (video stalled)...');
      }
    };

    const intervalId = setInterval(checkFrameHealth, FRAME_STALL_CHECK_INTERVAL_MS);
    return () => clearInterval(intervalId);
  }, [isConnected, qualityMode, isConnecting, reconnect]);

  // Auto-focus container when stream connects for keyboard input
  useEffect(() => {
    if (isConnected && containerRef.current) {
      containerRef.current.focus();
    }
  }, [isConnected]);

  // Reset reconnectClicked when isConnecting becomes true (connection attempt has started)
  // This provides immediate button feedback: click → disable → wait for isConnecting
  useEffect(() => {
    if (isConnecting) {
      setReconnectClicked(false);
    }
  }, [isConnecting]);

  // Screenshot polling for screenshot mode (low-bandwidth fallback)
  // Targets 2 FPS minimum (500ms max per frame)
  // Dynamically adjusts JPEG quality based on fetch time
  const shouldPollScreenshots = qualityMode === 'screenshot';

  // Notify server to pause/resume video based on quality mode
  // - 'video': WS video enabled (main video source)
  // - 'screenshot': WS video disabled (screenshots are the video source)
  useEffect(() => {
    const stream = streamRef.current;
    if (!stream || !(stream instanceof WebSocketStream) || !isConnected) {
      return;
    }

    // Control video streaming based on quality mode
    if (qualityMode === 'screenshot') {
      console.log('[DesktopStreamViewer] Screenshot mode - disabling WS video');
      stream.setVideoEnabled(false);
    } else if (qualityMode === 'video') {
      console.log('[DesktopStreamViewer] Video mode - enabling WS video');
      stream.setVideoEnabled(true);
    }
  }, [qualityMode, isConnected]);

  // Auto-dismiss bitrate recommendation after a fixed duration
  useEffect(() => {
    if (!bitrateRecommendation) {
      // Clear any pending timeout when recommendation is dismissed
      if (bitrateRecommendationTimeoutRef.current) {
        clearTimeout(bitrateRecommendationTimeoutRef.current);
        bitrateRecommendationTimeoutRef.current = null;
      }
      return;
    }

    // Set up auto-dismiss timeout
    bitrateRecommendationTimeoutRef.current = setTimeout(() => {
      console.log('[DesktopStreamViewer] Auto-dismissing bitrate recommendation after timeout');
      setBitrateRecommendation(null);
      bitrateRecommendationTimeoutRef.current = null;
    }, BITRATE_RECOMMENDATION_DISMISS_MS);

    return () => {
      if (bitrateRecommendationTimeoutRef.current) {
        clearTimeout(bitrateRecommendationTimeoutRef.current);
        bitrateRecommendationTimeoutRef.current = null;
      }
    };
  }, [bitrateRecommendation]);

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

    console.log('[DesktopStreamViewer] Starting screenshot polling (low mode)');

    // Register screenshot polling connection
    if (currentScreenshotVideoIdRef.current) {
      unregisterConnection(currentScreenshotVideoIdRef.current);
    }
    const screenshotId = registerConnection('screenshot-polling');
    currentScreenshotVideoIdRef.current = screenshotId;

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
        // include_cursor=false because the frontend renders its own cursor overlay
        // based on cursor metadata from the video stream
        const endpoint = `/api/v1/external-agents/${sessionId}/screenshot?format=jpeg&quality=${currentQuality}&include_cursor=false`;
        const response = await fetch(endpoint);

        if (!response.ok) {
          console.warn('[DesktopStreamViewer] Screenshot fetch failed:', response.status);
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
          // Hide connecting overlay on first screenshot after entering screenshot mode
          // IMPORTANT: Do this OUTSIDE setScreenshotUrl callback to avoid nested state update issues
          if (waitingForFirstScreenshotRef.current) {
            console.log('[Screenshot] First screenshot received - hiding connecting overlay');
            // Clear video start timeout - screenshot arrived successfully
            if (videoStartTimeoutRef.current) {
              clearTimeout(videoStartTimeoutRef.current);
              videoStartTimeoutRef.current = null;
            }
            waitingForFirstScreenshotRef.current = false;
            setIsConnecting(false);
            setStatus('Streaming active');
          }

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
        console.warn('[DesktopStreamViewer] Screenshot fetch error:', err);
        // Schedule next fetch after a short delay on error
        if (isPolling) setTimeout(fetchScreenshot, 200);
      }
    };

    // Start continuous polling
    fetchScreenshot();

    return () => {
      isPolling = false;
      // Unregister screenshot polling connection
      if (screenshotId) {
        unregisterConnection(screenshotId);
        if (currentScreenshotVideoIdRef.current === screenshotId) {
          currentScreenshotVideoIdRef.current = null;
        }
      }
    };
  }, [shouldPollScreenshots, isConnected, sessionId, registerConnection, unregisterConnection]);

  // Cleanup screenshot URL on unmount
  useEffect(() => {
    return () => {
      if (screenshotUrl) {
        URL.revokeObjectURL(screenshotUrl);
      }
    };
  }, [screenshotUrl]);

  // Note: Users manually switch between video and screenshot modes via toolbar toggle

  // Adaptive bitrate: reduce based on RTT, increase based on bandwidth probe
  // RTT = ping/pong round-trip time - simple and reliable indicator of network latency
  // Bandwidth probe = active test before increasing to verify headroom exists
  const stableCheckCountRef = useRef(0); // Count of checks with low RTT
  const congestionCheckCountRef = useRef(0); // Count of consecutive checks with high RTT (dampening)
  const lastBitrateChangeRef = useRef(0);
  const bandwidthProbeInProgressRef = useRef(false); // Prevent concurrent probes

  // Bandwidth probe: actively test available bandwidth before increasing bitrate
  // Fetches random data and measures throughput to verify headroom exists
  // Uses PARALLEL requests to fill high-BDP pipes (critical for high-latency links like satellite/VPN)
  // NOTE: Uses dedicated bandwidth-probe endpoint that returns random bytes immediately,
  // unlike screenshot which has capture latency before bytes start flowing
  // Returns measured throughput in Mbps (0 on failure)
  const runBandwidthProbe = useCallback(async (): Promise<number> => {
    if (bandwidthProbeInProgressRef.current) {
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

      // Fire all requests simultaneously (uses generic endpoint, no session required)
      const probePromises = Array.from({ length: probeCount }, (_, i) =>
        fetch(`/api/v1/bandwidth-probe?size=${probeSize}`)
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
  }, []);

  useEffect(() => {
    if (!isConnected || !streamRef.current) {
      stableCheckCountRef.current = 0;
      return;
    }

    // Screenshot mode doesn't have frame latency metrics
    if (qualityMode === 'screenshot') {
      return;
    }

    const CHECK_INTERVAL_MS = 1000;       // Check every second (for congestion detection)
    const CONNECTION_GRACE_PERIOD_MS = 60000; // Wait 60s after connection before showing recommendations
    // Long grace period because stream startup often has transient issues (buffering, pipeline init)
    // that look like congestion but resolve quickly. Avoid false positives that annoy users.
    const REDUCE_COOLDOWN_MS = 300000;    // Don't show another recommendation within 5 minutes
    const INCREASE_COOLDOWN_MS = 300000;  // Don't show another recommendation within 5 minutes
    const MANUAL_SELECTION_COOLDOWN_MS = 60000;  // Don't auto-reduce within 60s of user manually selecting bitrate
    const BITRATE_OPTIONS = [5, 10, 20, 40, 80]; // Available bitrates in ascending order
    const MIN_BITRATE = 5;
    const STABLE_CHECKS_FOR_INCREASE = 300; // Need 5 minutes of low RTT before running bandwidth probe
    const CONGESTION_CHECKS_FOR_REDUCE = 30; // Need 30 consecutive high RTT samples (30s) before reducing
    const RTT_THRESHOLD = 500;    // Reduce if RTT exceeds 500ms (severe latency)
    // Using RTT (ping/pong) instead of frame drift - simpler and more reliable

    // Track when connection started for grace period
    const connectionStartTime = Date.now();

    const checkBandwidth = () => {
      const stream = streamRef.current;
      if (!stream) return;

      // Skip recommendations during initial connection grace period
      // This prevents false positives during WebSocket handshake and initial buffering
      if (Date.now() - connectionStartTime < CONNECTION_GRACE_PERIOD_MS) {
        return;
      }

      // Get RTT from stream stats (simple ping/pong measurement)
      const stats = stream.getStats();
      const rtt = stats.rttMs;

      const currentBitrate = userBitrate || requestedBitrate;
      const now = Date.now();

      // Skip auto-changes if user manually selected bitrate within cooldown period
      // This lets the stream settle after user explicitly chooses a bitrate
      const timeSinceManualSelection = now - manualBitrateSelectionTimeRef.current;
      if (timeSinceManualSelection < MANUAL_SELECTION_COOLDOWN_MS) {
        return; // Don't make any bitrate changes during cooldown
      }

      // RTT-based congestion detection:
      // - High RTT indicates network latency/congestion
      // - Simple and reliable - just measures ping/pong round-trip time
      // - No complicated PTS calculations that can have edge cases
      const congestionDetected = rtt > RTT_THRESHOLD;

      // Reduce bitrate on sustained high RTT (dampening prevents single-spike reductions)
      if (congestionDetected && currentBitrate > MIN_BITRATE) {
        // Increment congestion counter - require multiple consecutive high RTT samples
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
              console.log(`[AdaptiveBitrate] Sustained high RTT (${congestionCheckCountRef.current} samples, ${rtt.toFixed(0)}ms), recommending: ${currentBitrate} -> ${newBitrate} Mbps`);

              // Show recommendation popup instead of auto-switching
              setBitrateRecommendation({
                type: 'decrease',
                targetBitrate: newBitrate,
                reason: `Your connection is experiencing delays (${rtt.toFixed(0)}ms RTT)`,
                frameDrift: rtt, // Keep field name for backwards compat, but using RTT value
              });

              // Reset counters so we don't keep re-recommending
              lastBitrateChangeRef.current = now;
              stableCheckCountRef.current = 0;
              congestionCheckCountRef.current = 0;
              return;
            }
          }
        }
      } else if (congestionDetected && currentBitrate === MIN_BITRATE) {
        // Already at minimum bitrate but still experiencing congestion
        // Recommend switching to screenshot mode for better reliability
        congestionCheckCountRef.current++;
        stableCheckCountRef.current = 0;

        if (congestionCheckCountRef.current >= CONGESTION_CHECKS_FOR_REDUCE) {
          const timeSinceLastChange = now - lastBitrateChangeRef.current;

          if (timeSinceLastChange > REDUCE_COOLDOWN_MS) {
            console.log(`[AdaptiveBitrate] At minimum bitrate (${MIN_BITRATE}Mbps) but still experiencing congestion (${rtt.toFixed(0)}ms RTT), recommending screenshot mode`);

            setBitrateRecommendation({
              type: 'screenshot',
              targetBitrate: MIN_BITRATE, // Keep same bitrate, just switch mode
              reason: `Video streaming is struggling even at ${MIN_BITRATE}Mbps`,
              frameDrift: rtt, // Keep field name for backwards compat, but using RTT value
            });

            lastBitrateChangeRef.current = now;
            stableCheckCountRef.current = 0;
            congestionCheckCountRef.current = 0;
            return;
          }
        }
      } else {
        // Low RTT - connection is stable at current bitrate
        congestionCheckCountRef.current = 0; // Reset congestion counter on good sample
        // DISABLED: We no longer recommend increasing bitrate even when stable.
        // Lower bitrates (5 Mbps) provide smoother streaming than higher bitrates -
        // lower encoder latency and more consistent frame pacing outweigh quality benefits.
        // Users can manually increase bitrate via the menu if they want higher quality.
        // stableCheckCountRef.current++;
        //
        // // Try to increase if stable for a while
        // if (stableCheckCountRef.current >= STABLE_CHECKS_FOR_INCREASE) {
        //   const timeSinceLastChange = now - lastBitrateChangeRef.current;
        //
        //   if (timeSinceLastChange > INCREASE_COOLDOWN_MS) {
        //     const currentIndex = BITRATE_OPTIONS.indexOf(currentBitrate);
        //
        //     if (currentIndex !== -1 && currentIndex < BITRATE_OPTIONS.length - 1) {
        //       // Run bandwidth probe to measure actual throughput
        //       // Then jump directly to the highest sustainable bitrate (not just +1 tier)
        //       console.log(`[AdaptiveBitrate] Stable for ${stableCheckCountRef.current}s, probing to find optimal bitrate...`);
        //
        //       // Mark that we're attempting an increase (prevent re-triggering during probe)
        //       stableCheckCountRef.current = 0;
        //
        //       runBandwidthProbe().then((measuredThroughputMbps) => {
        //         if (measuredThroughputMbps <= 0) {
        //           console.log(`[AdaptiveBitrate] Probe failed, staying at ${currentBitrate} Mbps`);
        //           lastBitrateChangeRef.current = Date.now();
        //           return;
        //         }
        //
        //         // Calculate max sustainable bitrate with 25% headroom
        //         // If we measure 100 Mbps, we can sustain 100/1.25 = 80 Mbps
        //         const maxSustainableBitrate = measuredThroughputMbps / 1.25;
        //
        //         // Find the highest BITRATE_OPTIONS that fits
        //         let targetBitrate = currentBitrate;
        //         for (let i = BITRATE_OPTIONS.length - 1; i >= 0; i--) {
        //           if (BITRATE_OPTIONS[i] <= maxSustainableBitrate && BITRATE_OPTIONS[i] > currentBitrate) {
        //             targetBitrate = BITRATE_OPTIONS[i];
        //             break;
        //           }
        //         }
        //
        //         if (targetBitrate > currentBitrate) {
        //           console.log(`[AdaptiveBitrate] Probe measured ${measuredThroughputMbps.toFixed(1)} Mbps → max sustainable ${maxSustainableBitrate.toFixed(1)} Mbps`);
        //           console.log(`[AdaptiveBitrate] Recommending upgrade: ${currentBitrate} → ${targetBitrate} Mbps`);
        //
        //           // Show recommendation popup instead of auto-switching
        //           setBitrateRecommendation({
        //             type: 'increase',
        //             targetBitrate: targetBitrate,
        //             reason: `Your connection has improved (measured ${measuredThroughputMbps.toFixed(0)} Mbps)`,
        //             measuredThroughput: measuredThroughputMbps,
        //           });
        //
        //           lastBitrateChangeRef.current = Date.now();
        //         } else {
        //           console.log(`[AdaptiveBitrate] Probe measured ${measuredThroughputMbps.toFixed(1)} Mbps → max sustainable ${maxSustainableBitrate.toFixed(1)} Mbps (not enough for next tier)`);
        //           lastBitrateChangeRef.current = Date.now();
        //         }
        //       });
        //     }
        //   }
        // }
      }
    };

    const intervalId = setInterval(checkBandwidth, CHECK_INTERVAL_MS);

    return () => clearInterval(intervalId);
  }, [isConnected, qualityMode, userBitrate, requestedBitrate, runBandwidthProbe, addChartEvent]);


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

  // Track visibility for deferred connection - only connect when component is visible
  // This saves bandwidth and avoids connection issues on high-latency networks
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (entry.isIntersecting && !isVisible) {
          console.log('[DesktopStreamViewer] Component became visible - will trigger connection');
          setIsVisible(true);
        }
        // Note: we don't set isVisible=false when hidden - once connected, stay connected
      },
      { threshold: 0.1 } // Trigger when 10% visible
    );

    observer.observe(container);
    return () => observer.disconnect();
  }, [isVisible]);

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
    if (!containerSize || !canvasRef.current || false /* WebSocket only */) return;

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
  }, [containerSize, isConnected]);

  // Auto-sync clipboard from remote → local every 2 seconds
  useEffect(() => {
    if (!isConnected || !sessionId) return;

    const syncClipboard = async () => {
      // Skip if tab is hidden (save bandwidth and CPU)
      if (document.visibilityState === 'hidden') {
        return;
      }

      // Skip if clipboard API is not available (e.g., Safari without HTTPS)
      if (!navigator.clipboard) {
        return;
      }

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

    // Poll every 2.7 seconds (prime to avoid sync with other polling)
    const syncInterval = setInterval(syncClipboard, 2700);

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

      // Send scroll via WebSocketStream
      const input = streamRef.current && 'getInput' in streamRef.current
        ? (streamRef.current as WebSocketStream).getInput()
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

  // Apply debug throttle ratio override to WebSocketStream
  useEffect(() => {
    if (streamRef.current instanceof WebSocketStream) {
      (streamRef.current as WebSocketStream).setThrottleRatio(debugThrottleRatio);
    }
  }, [debugThrottleRatio]);

  // Poll stream stats when stats overlay or charts are visible
  useEffect(() => {
    if ((!showStats && !showCharts) || !streamRef.current) {
      return;
    }

    // Poll stats from WebSocketStream
    const pollWsStats = () => {
        const currentStream = streamRef.current;
        if (!currentStream) return;

        const wsStream = currentStream as WebSocketStream;
        const wsStats = wsStream.getStats();
        const isScreenshotMode = qualityMode === 'screenshot';

        // Determine codec string based on quality mode
        const codecDisplay = isScreenshotMode
          ? 'JPEG (Screenshot)'
          : `${wsStats.codecString} (WebSocket)`;

        setStats({
          video: {
            codec: codecDisplay,
            width: isScreenshotMode ? (width || 1920) : wsStats.width,
            height: isScreenshotMode ? (height || 1080) : wsStats.height,
            fps: isScreenshotMode ? screenshotFps : wsStats.fps,
            receiveFps: isScreenshotMode ? 0 : wsStats.receiveFps,
            videoPayloadBitrate: isScreenshotMode ? 'N/A' : wsStats.videoPayloadBitrateMbps.toFixed(2),
            totalBitrate: isScreenshotMode ? 'N/A' : wsStats.totalBitrateMbps.toFixed(2),
            framesDecoded: isScreenshotMode ? 0 : wsStats.framesDecoded,
            framesReceived: isScreenshotMode ? 0 : wsStats.framesReceived,
            framesDropped: isScreenshotMode ? 0 : wsStats.framesDropped,
            rttMs: wsStats.rttMs,
            encoderLatencyMs: wsStats.encoderLatencyMs,
            isHighLatency: wsStats.isHighLatency,
            batchingRatio: wsStats.batchingRatio,
            avgBatchSize: wsStats.avgBatchSize,
            batchesReceived: wsStats.batchesReceived,
            frameLatencyMs: wsStats.frameLatencyMs,
            adaptiveThrottleRatio: wsStats.adaptiveThrottleRatio,
            effectiveInputFps: wsStats.effectiveInputFps,
            isThrottled: wsStats.isThrottled,
            decodeQueueSize: wsStats.decodeQueueSize,
            maxDecodeQueueSize: wsStats.maxDecodeQueueSize,
            framesSkippedToKeyframe: wsStats.framesSkippedToKeyframe,
            // Jitter stats
            receiveJitterMs: wsStats.receiveJitterMs,
            renderJitterMs: wsStats.renderJitterMs,
            avgReceiveIntervalMs: wsStats.avgReceiveIntervalMs,
            avgRenderIntervalMs: wsStats.avgRenderIntervalMs,
            // Debug flags
            usingSoftwareDecoder: wsStats.usingSoftwareDecoder,
          },
          // Input buffer stats (detects TCP send buffer congestion)
          input: {
            bufferBytes: wsStats.inputBufferBytes,
            maxBufferBytes: wsStats.maxInputBufferBytes,
            avgBufferBytes: wsStats.avgInputBufferBytes,
            inputsSent: wsStats.inputsSent,
            inputsDropped: wsStats.inputsDroppedDueToCongestion,
            congested: wsStats.inputCongested,
            // Send latency (should be ~0 if ws.send is truly non-blocking)
            lastSendMs: wsStats.lastSendDurationMs,
            maxSendMs: wsStats.maxSendDurationMs,
            avgSendMs: wsStats.avgSendDurationMs,
            bufferBeforeSend: wsStats.bufferedAmountBeforeSend,
            bufferAfterSend: wsStats.bufferedAmountAfterSend,
            bufferStaleMs: wsStats.bufferStaleMs,
            // Event loop latency (detects main thread blocking)
            eventLoopLatencyMs: wsStats.eventLoopLatencyMs,
            maxEventLoopLatencyMs: wsStats.maxEventLoopLatencyMs,
            avgEventLoopLatencyMs: wsStats.avgEventLoopLatencyMs,
          },
          connection: {
            transport: isScreenshotMode
              ? 'Screenshot + WebSocket Input'
              : 'WebSocket Video + Input',
          },
          timestamp: new Date().toISOString(),
        });
        // Update high latency state for warning banner
        setIsHighLatency(wsStats.isHighLatency);
        // Update throttle state for warning banner
        setIsThrottled(wsStats.isThrottled);
        // Show orange border for screenshot mode
        setIsOnFallback(isScreenshotMode);

        // Update chart history (60 seconds of data) - use refs to persist across reconnects
        throughputHistoryRef.current = [...throughputHistoryRef.current, wsStats.totalBitrateMbps].slice(-CHART_HISTORY_LENGTH);
        rttHistoryRef.current = [...rttHistoryRef.current, wsStats.rttMs].slice(-CHART_HISTORY_LENGTH);
        bitrateHistoryRef.current = [...bitrateHistoryRef.current, requestedBitrate].slice(-CHART_HISTORY_LENGTH);
        // Frame drift for charts
        frameDriftHistoryRef.current = [...frameDriftHistoryRef.current, wsStats.frameLatencyMs].slice(-CHART_HISTORY_LENGTH);
        // Trigger re-render for charts
        if (showCharts) {
          setChartUpdateTrigger(prev => prev + 1);
        }
      };

    // Poll every second
    const interval = setInterval(pollWsStats, 1000);
    pollWsStats(); // Initial call

    return () => clearInterval(interval);
  }, [showStats, showCharts, width, height, qualityMode, requestedBitrate]);

  // Calculate stream rectangle for mouse coordinate mapping
  const getStreamRect = useCallback((): DOMRect => {
    // Check if we're in screenshot mode (screenshot overlay is visible)
    const inScreenshotMode = shouldPollScreenshots && screenshotUrl;

    // In screenshot mode, the img uses containerRef with objectFit: contain
    // In video mode, use canvas for rendering
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

    // Use canvas for input and rendering
    const element = canvasRef.current;
    if (!element) {
      return new DOMRect(0, 0, width, height);
    }

    // Canvas is already sized to maintain aspect ratio,
    // so bounding rect IS the video content area
    const boundingRect = element.getBoundingClientRect();
    return new DOMRect(
      boundingRect.x,
      boundingRect.y,
      boundingRect.width,
      boundingRect.height
    );
  }, [width, height, shouldPollScreenshots, screenshotUrl]);

  // Get input handler from the WebSocket stream
  const getInputHandler = useCallback(() => {
    // For all modes, get input from the main stream
    if (streamRef.current && 'getInput' in streamRef.current) {
      return (streamRef.current as WebSocketStream).getInput();
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

    // Update custom cursor position - must match input coordinate space
    // Input uses getStreamRect() which accounts for letterboxing, so custom cursor
    // must also be positioned relative to stream rect, not container
    if (containerRef.current) {
      const containerRect = containerRef.current.getBoundingClientRect();
      const streamRect = getStreamRect();

      // Calculate position relative to stream rect (video content area)
      const relX = event.clientX - streamRect.x;
      const relY = event.clientY - streamRect.y;

      // Clamp to stream bounds so cursor stays within video content
      const clampedX = Math.max(0, Math.min(relX, streamRect.width));
      const clampedY = Math.max(0, Math.min(relY, streamRect.height));

      // Convert back to container-relative coords for CSS positioning
      setCursorPosition({
        x: (streamRect.x - containerRect.x) + clampedX,
        y: (streamRect.y - containerRect.y) + clampedY,
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

  // Touch event handlers - delegates to StreamInput which handles different touch modes
  // In trackpad mode, we handle gestures:
  // - One finger drag: move cursor
  // - One finger tap: left click
  // - Double-tap-drag: click and drag
  // - Two finger tap: right click
  // - Three finger tap: middle click
  // - Two finger scroll: scroll
  const handleTouchStart = useCallback((event: React.TouchEvent) => {
    event.preventDefault();
    const handler = getInputHandler();
    const rect = getStreamRect();
    if (!handler) return;

    // Track touch start time and reset movement tracking
    touchStartTimeRef.current = Date.now();
    touchMovedRef.current = false;

    // In trackpad mode, handle gestures
    if (touchMode === 'trackpad') {
      if (event.touches.length === 1) {
        const touch = event.touches[0];
        const now = Date.now();
        lastTouchPosRef.current = { x: touch.clientX, y: touch.clientY };

        // Initialize cursor at center of stream if this is first touch
        if (!hasMouseMoved && containerRef.current) {
          const containerRect = containerRef.current.getBoundingClientRect();
          setCursorPosition({
            x: (rect.x - containerRect.x) + rect.width / 2,
            y: (rect.y - containerRect.y) + rect.height / 2,
          });
          setHasMouseMoved(true);
        }

        // Check for double-tap: if second tap within threshold, start drag mode
        // Only triggers if the previous touch was a real tap (short, no movement)
        if (now - lastTapTimeRef.current < DOUBLE_TAP_THRESHOLD_MS && !isDragging) {
          console.log('[DesktopStreamViewer] Double-tap detected, starting drag mode');

          // Cancel the pending single-tap click (if any) - we're starting a drag, not a second click
          if (pendingClickTimeoutRef.current) {
            clearTimeout(pendingClickTimeoutRef.current);
            pendingClickTimeoutRef.current = null;
            console.log('[DesktopStreamViewer] Cancelled pending click for double-tap-drag');
          }

          setIsDragging(true);
          // Send cursor position to remote before starting drag
          if (containerRef.current) {
            const containerRect = containerRef.current.getBoundingClientRect();
            const streamOffsetX = rect.x - containerRect.x;
            const streamOffsetY = rect.y - containerRect.y;
            const streamRelativeX = cursorPosition.x - streamOffsetX;
            const streamRelativeY = cursorPosition.y - streamOffsetY;
            const streamX = (streamRelativeX / rect.width) * width;
            const streamY = (streamRelativeY / rect.height) * height;
            handler.sendMousePosition?.(streamX, streamY, width, height);
          }
          // Send mouse button down to start drag
          handler.sendMouseButton?.(true, MOUSE_BUTTON_LEFT);
        }
      }

      // Two-finger gesture: could be pinch-to-zoom or scroll
      if (event.touches.length === 2) {
        const touch1 = event.touches[0];
        const touch2 = event.touches[1];
        // Calculate initial distance between fingers
        const dx = touch2.clientX - touch1.clientX;
        const dy = touch2.clientY - touch1.clientY;
        const distance = Math.sqrt(dx * dx + dy * dy);
        pinchStartDistanceRef.current = distance;
        pinchStartZoomRef.current = zoomLevel;
        // Calculate center point of pinch
        const centerX = (touch1.clientX + touch2.clientX) / 2;
        const centerY = (touch1.clientY + touch2.clientY) / 2;
        pinchCenterRef.current = { x: centerX, y: centerY };
        lastPinchCenterRef.current = { x: centerX, y: centerY };
        // Reset gesture type - will be determined on first move
        twoFingerGestureTypeRef.current = 'undecided';
        // Also keep scroll tracking for fallback
        twoFingerStartYRef.current = (touch1.clientY + touch2.clientY) / 2;
      }
    }

    // Delegate to StreamInput for actual input handling
    handler.onTouchStart(event.nativeEvent, rect);
  }, [getStreamRect, getInputHandler, touchMode, hasMouseMoved, isDragging, DOUBLE_TAP_THRESHOLD_MS, cursorPosition, width, height, zoomLevel]);

  const handleTouchMove = useCallback((event: React.TouchEvent) => {
    event.preventDefault();
    const handler = getInputHandler();
    const rect = getStreamRect();
    if (!handler) return;

    // In trackpad mode, update cursor position based on touch movement delta
    if (touchMode === 'trackpad' && event.touches.length === 1 && lastTouchPosRef.current && containerRef.current) {
      const touch = event.touches[0];
      const dx = touch.clientX - lastTouchPosRef.current.x;
      const dy = touch.clientY - lastTouchPosRef.current.y;

      // Mark as moved if finger moved significantly (not a tap)
      if (Math.abs(dx) > TAP_MAX_MOVEMENT_PX || Math.abs(dy) > TAP_MAX_MOVEMENT_PX) {
        touchMovedRef.current = true;
      }

      // Apply pointer acceleration curve (like macOS trackpad)
      // Small/slow movements: reduced for precision
      // Fast/large movements: amplified for quick navigation
      const applyAcceleration = (delta: number): number => {
        const speed = Math.abs(delta);
        // Below threshold: precision mode (dampen small movements)
        if (speed < 2) {
          return delta * 0.5 * TRACKPAD_CURSOR_SENSITIVITY;
        }
        // Acceleration curve: starts at base sensitivity, ramps up for fast movement
        // Factor of 0.03 means at 30px movement, we get 1.9x base sensitivity
        const accelerationFactor = 1 + (speed * 0.03);
        return delta * accelerationFactor * TRACKPAD_CURSOR_SENSITIVITY;
      };

      const scaledDx = applyAcceleration(dx);
      const scaledDy = applyAcceleration(dy);

      const containerRect = containerRef.current.getBoundingClientRect();
      const streamOffsetX = rect.x - containerRect.x;
      const streamOffsetY = rect.y - containerRect.y;

      // Calculate new cursor position, clamped to stream bounds
      // Use ref for current position (not stale React state)
      const currentPos = cursorPositionRef.current;
      const newX = Math.max(streamOffsetX, Math.min(streamOffsetX + rect.width, currentPos.x + scaledDx));
      const newY = Math.max(streamOffsetY, Math.min(streamOffsetY + rect.height, currentPos.y + scaledDy));

      // Update ref immediately (synchronous, for next frame calculation)
      cursorPositionRef.current = { x: newX, y: newY };

      // Update DOM directly for smooth 60fps cursor movement (bypasses React render cycle)
      if (trackpadCursorRef.current) {
        trackpadCursorRef.current.style.transform = `translate(${newX}px, ${newY}px)`;
      }

      // Also update React state (debounced - React will batch these)
      // This is needed for click coordinate calculations
      setCursorPosition({ x: newX, y: newY });

      // Convert to stream coordinates and send to remote
      // cursorPosition is container-relative, need to convert to stream-relative
      const streamRelativeX = newX - streamOffsetX;
      const streamRelativeY = newY - streamOffsetY;
      const streamX = (streamRelativeX / rect.width) * width;
      const streamY = (streamRelativeY / rect.height) * height;
      handler.sendMousePosition?.(streamX, streamY, width, height);

      lastTouchPosRef.current = { x: touch.clientX, y: touch.clientY };
      // Don't delegate single-finger trackpad movement to StreamInput - we handle it ourselves
      return;
    }

    // Two-finger gesture: could be pinch-to-zoom OR scroll
    if (event.touches.length === 2 && pinchStartDistanceRef.current !== null) {
      const touch1 = event.touches[0];
      const touch2 = event.touches[1];

      // Calculate current distance between fingers
      const fingerDx = touch2.clientX - touch1.clientX;
      const fingerDy = touch2.clientY - touch1.clientY;
      const currentDistance = Math.sqrt(fingerDx * fingerDx + fingerDy * fingerDy);
      const distanceChange = Math.abs(currentDistance - pinchStartDistanceRef.current);

      // Calculate current center point
      const centerX = (touch1.clientX + touch2.clientX) / 2;
      const centerY = (touch1.clientY + touch2.clientY) / 2;

      // Calculate center movement (for scroll detection)
      const centerDx = lastPinchCenterRef.current ? centerX - lastPinchCenterRef.current.x : 0;
      const centerDy = lastPinchCenterRef.current ? centerY - lastPinchCenterRef.current.y : 0;
      const centerMovement = Math.sqrt(centerDx * centerDx + centerDy * centerDy);

      // Determine gesture type on first significant move
      if (twoFingerGestureTypeRef.current === 'undecided') {
        // If distance between fingers changes significantly, it's a pinch
        // If center moves but distance stays same, it's a scroll
        if (distanceChange > PINCH_VS_SCROLL_THRESHOLD) {
          twoFingerGestureTypeRef.current = 'pinch';
        } else if (centerMovement > 10) {
          // Center moved but fingers didn't spread/pinch - it's a scroll
          twoFingerGestureTypeRef.current = 'scroll';
        }
      }

      // Handle scroll gesture - send scroll events to remote
      if (twoFingerGestureTypeRef.current === 'scroll') {
        // Send scroll wheel events to remote desktop
        handler.sendMouseWheel?.(
          -centerDx * SCROLL_SENSITIVITY,  // Invert X for natural scrolling
          centerDy * SCROLL_SENSITIVITY    // Y is not inverted (swipe up = scroll up)
        );
        lastPinchCenterRef.current = { x: centerX, y: centerY };
        return;
      }

      // Handle pinch gesture - local zoom
      if (twoFingerGestureTypeRef.current === 'pinch' || twoFingerGestureTypeRef.current === 'undecided') {
        // Calculate zoom change
        const scale = currentDistance / pinchStartDistanceRef.current;
        const newZoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, pinchStartZoomRef.current * scale));
        setZoomLevel(newZoom);

        // Pan while pinching (move the view with the gesture)
        if (lastPinchCenterRef.current && containerRef.current) {
          const panDx = centerX - lastPinchCenterRef.current.x;
          const panDy = centerY - lastPinchCenterRef.current.y;

          // Update pan offset, clamping to bounds
          const containerRect = containerRef.current.getBoundingClientRect();
          const maxPanX = (containerRect.width * (newZoom - 1)) / 2;
          const maxPanY = (containerRect.height * (newZoom - 1)) / 2;

          setPanOffset(prev => ({
            x: Math.max(-maxPanX, Math.min(maxPanX, prev.x + panDx)),
            y: Math.max(-maxPanY, Math.min(maxPanY, prev.y + panDy)),
          }));
        }

        lastPinchCenterRef.current = { x: centerX, y: centerY };
        return;
      }
    }

    // Delegate to StreamInput for other touch handling
    handler.onTouchMove(event.nativeEvent, rect);
  }, [getStreamRect, getInputHandler, touchMode, TRACKPAD_CURSOR_SENSITIVITY, TAP_MAX_MOVEMENT_PX, cursorPosition, width, height, zoomLevel, MIN_ZOOM, MAX_ZOOM]);

  const handleTouchEnd = useCallback((event: React.TouchEvent) => {
    event.preventDefault();
    const handler = getInputHandler();
    const rect = getStreamRect();
    if (!handler) return;

    const now = Date.now();
    const touchDuration = now - touchStartTimeRef.current;
    const wasTap = touchDuration < TAP_MAX_DURATION_MS && !touchMovedRef.current;

    // In trackpad mode, handle gestures
    if (touchMode === 'trackpad') {
      // Helper to send current cursor position to remote before clicks
      const sendCursorPositionToRemote = () => {
        if (!containerRef.current) return;
        const containerRect = containerRef.current.getBoundingClientRect();
        const streamOffsetX = rect.x - containerRect.x;
        const streamOffsetY = rect.y - containerRect.y;
        const streamRelativeX = cursorPosition.x - streamOffsetX;
        const streamRelativeY = cursorPosition.y - streamOffsetY;
        const streamX = (streamRelativeX / rect.width) * width;
        const streamY = (streamRelativeY / rect.height) * height;
        handler.sendMousePosition?.(streamX, streamY, width, height);
      };

      // End drag mode if active
      if (isDragging) {
        console.log('[DesktopStreamViewer] Ending drag mode');
        handler.sendMouseButton?.(false, MOUSE_BUTTON_LEFT);
        setIsDragging(false);
      }

      // Handle multi-finger taps (check changedTouches for the fingers that just lifted)
      // Note: event.touches shows remaining fingers, changedTouches shows lifted fingers
      const liftedFingers = event.changedTouches.length;
      const remainingFingers = event.touches.length;
      const totalFingers = liftedFingers + remainingFingers;

      if (wasTap && remainingFingers === 0) {
        // Send cursor position before any click so remote knows where to click
        sendCursorPositionToRemote();

        // All fingers lifted - check how many were in the tap
        if (totalFingers === 2) {
          // Two-finger tap = right click
          console.log('[DesktopStreamViewer] Two-finger tap = right click');
          handler.sendMouseButton?.(true, MOUSE_BUTTON_RIGHT);
          handler.sendMouseButton?.(false, MOUSE_BUTTON_RIGHT);
        } else if (totalFingers >= 3) {
          // Three-finger tap = middle click
          console.log('[DesktopStreamViewer] Three-finger tap = middle click');
          handler.sendMouseButton?.(true, MOUSE_BUTTON_MIDDLE);
          handler.sendMouseButton?.(false, MOUSE_BUTTON_MIDDLE);
        } else if (totalFingers === 1 && !isDragging) {
          // Single tap = left click (but not if we just ended a drag)
          // Check if cursor ALREADY indicates a text field (from hover position)
          // Only focus hidden input if we're tapping on a text field to avoid keyboard flash
          const cssName = cursorCssNameRef.current;
          const imgCursor = cursorImageRef.current;
          const isAlreadyTextCursor = cssName === 'text' ||
            cssName === 'vertical-text' ||
            imgCursor?.cursorName === 'text' ||
            imgCursor?.cursorName === 'vertical-text' ||
            imgCursor?.cursorName?.includes('xterm') ||
            imgCursor?.cursorName?.includes('ibeam');

          // IMPORTANT: Focus hidden input IMMEDIATELY (within user gesture) for iOS keyboard
          // iOS only shows keyboard if focus happens directly from user gesture, not in setTimeout
          // But ONLY focus if cursor indicates text field to avoid keyboard flash on every tap
          // Only do this on phones (iPhone/Android), not tablets or desktops
          if (hiddenInputRef.current && isPhone && isAlreadyTextCursor) {
            hiddenInputRef.current.focus();
            console.log('[DesktopStreamViewer] Focused hidden input on tap (text cursor detected, phone)');
          }

          // Delay the click to allow for double-tap-drag detection
          // If a second tap comes before the timeout, this click is cancelled in handleTouchStart
          pendingClickTimeoutRef.current = setTimeout(() => {
            pendingClickTimeoutRef.current = null;
            console.log('[DesktopStreamViewer] Single tap = left click (delayed)');
            handler.sendMouseButton?.(true, MOUSE_BUTTON_LEFT);
            handler.sendMouseButton?.(false, MOUSE_BUTTON_LEFT);

            // After click is sent, check cursor type after delay (wait for remote to update cursor)
            // If cursor changed to text, focus to show keyboard. If not text, ensure blurred.
            setTimeout(() => {
              if (hiddenInputRef.current) {
                // Use refs to avoid stale closure values
                const cssNameNow = cursorCssNameRef.current;
                const imgCursorNow = cursorImageRef.current;
                const isTextCursor = cssNameNow === 'text' ||
                  cssNameNow === 'vertical-text' ||
                  imgCursorNow?.cursorName === 'text' ||
                  imgCursorNow?.cursorName === 'vertical-text' ||
                  imgCursorNow?.cursorName?.includes('xterm') ||
                  imgCursorNow?.cursorName?.includes('ibeam');

                if (isTextCursor) {
                  console.log('[DesktopStreamViewer] Text cursor confirmed - keeping virtual keyboard');
                } else {
                  // Not a text cursor - dismiss keyboard
                  console.log('[DesktopStreamViewer] Non-text cursor - dismissing virtual keyboard');
                  hiddenInputRef.current.blur();
                  containerRef.current?.focus();
                }
              }
            }, 300); // Wait for remote cursor update
          }, DOUBLE_TAP_THRESHOLD_MS);
        }
      }

      // Only record tap time for double-tap detection if it was a real tap
      if (wasTap && totalFingers === 1) {
        lastTapTimeRef.current = now;
      }

      // Don't delegate single-finger taps to StreamInput in trackpad mode
      // Clean up and return
      lastTouchPosRef.current = null;
      twoFingerStartYRef.current = null;
      pinchStartDistanceRef.current = null;
      pinchCenterRef.current = null;
      lastPinchCenterRef.current = null;
      return;
    }

    // Delegate to StreamInput for actual input handling (non-trackpad mode)
    handler.onTouchEnd(event.nativeEvent, rect);

    // Clean up touch tracking
    lastTouchPosRef.current = null;
    twoFingerStartYRef.current = null;
    pinchStartDistanceRef.current = null;
    pinchCenterRef.current = null;
    lastPinchCenterRef.current = null;
  }, [getStreamRect, getInputHandler, touchMode, isDragging, cursorPosition, width, height]);

  const handleTouchCancel = useCallback((event: React.TouchEvent) => {
    event.preventDefault();
    const handler = getInputHandler();
    const rect = getStreamRect();
    if (!handler) return;

    // Cancel any pending click
    if (pendingClickTimeoutRef.current) {
      clearTimeout(pendingClickTimeoutRef.current);
      pendingClickTimeoutRef.current = null;
    }

    // Cancel any ongoing drag
    if (touchMode === 'trackpad' && isDragging) {
      handler.sendMouseButton?.(false, MOUSE_BUTTON_LEFT);
      setIsDragging(false);
    }

    handler.onTouchCancel?.(event.nativeEvent, rect);

    // Clean up touch tracking
    lastTouchPosRef.current = null;
    twoFingerStartYRef.current = null;
    pinchStartDistanceRef.current = null;
    pinchCenterRef.current = null;
    lastPinchCenterRef.current = null;
  }, [getStreamRect, getInputHandler, touchMode, isDragging]);

  // Reset all input state - clears stuck modifiers and mouse buttons
  const resetInputState = useCallback(() => {
    const input = getInputHandler();
    if (!input) return;

    console.log('[DesktopStreamViewer] Resetting stuck input state (modifiers + mouse buttons)');

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

    console.log('[DesktopStreamViewer] Input state reset complete');
  }, []);

  // Attach native keyboard event listeners when connected
  useEffect(() => {
    if (!isConnected || !containerRef.current) return;

    const container = containerRef.current;

    // Helper to get input handler (WebSocketStream handles input for all quality modes)
    const getInput = () => {
      return streamRef.current && 'getInput' in streamRef.current
        ? (streamRef.current as WebSocketStream).getInput()
        : null;
    };

    // Track last Escape press for double-Escape reset
    let lastEscapeTime = 0;

    const handleKeyDown = (event: KeyboardEvent) => {
      // Debug: Update visual debug indicator for iPad troubleshooting
      setDebugKeyEvent(`↓ key="${event.key}" code="${event.code}" keyCode=${event.keyCode}`);

      // Only process if container is focused
      if (document.activeElement !== container) {
        console.log('[DesktopStreamViewer] KeyDown ignored - container not focused. Active element:', document.activeElement?.tagName);
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

      // Double-Escape to reset stuck modifiers
      if (event.code === 'Escape') {
        const now = Date.now();
        if (now - lastEscapeTime < 500) { // 500ms window for double-press
          console.log('[DesktopStreamViewer] Double-Escape detected - resetting input state');
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

            // Skip local clipboard sync if API not available (e.g., Safari without HTTPS)
            if (!navigator.clipboard) {
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

        // Skip if clipboard API is not available (e.g., Safari without HTTPS)
        if (!navigator.clipboard) {
          console.warn('[Clipboard] Clipboard API not available');
          showClipboardToast('Clipboard not available', 'error');
          return;
        }

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
          // Note: Browser Clipboard API only supports PNG for images (per W3C spec)
          console.log(`[Clipboard] Available types: ${item.types.join(', ')}`);
          if (item.types.includes('image/png')) {
            console.log(`[Clipboard] Reading image/png from clipboard`);
            item.getType('image/png').then(blob => {
              console.log(`[Clipboard] Got PNG blob, size: ${blob.size} bytes`);
              blob.arrayBuffer().then(arrayBuffer => {
                const base64 = btoa(String.fromCharCode(...new Uint8Array(arrayBuffer)));
                console.log(`[Clipboard] Encoded to base64, length: ${base64.length}`);
                clipboardPayload = { type: 'image', data: base64 };
                syncAndPaste(clipboardPayload);
              });
            }).catch(err => {
              console.error('[Clipboard] Failed to get image/png:', err);
              showClipboardToast('Failed to read image from clipboard', 'error');
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

      console.log('[DesktopStreamViewer] KeyDown captured:', event.key, event.code);
      getInput()?.onKeyDown(event);
      // Prevent browser default behavior (e.g., Tab moving focus, Ctrl+W closing tab)
      // This ensures all keys are passed through to the remote desktop
      event.preventDefault();
      event.stopPropagation();
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      // Only process if container is focused
      if (document.activeElement !== container) {
        console.log('[DesktopStreamViewer] KeyUp ignored - container not focused. Active element:', document.activeElement?.tagName);
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

      console.log('[DesktopStreamViewer] KeyUp captured:', event.key, event.code);
      getInput()?.onKeyUp(event);
      // Prevent browser default behavior to ensure all keys are passed through
      event.preventDefault();
      event.stopPropagation();
    };

    // Handle beforeinput for Android virtual keyboards and swipe typing
    // Android sends key="Unidentified" for virtual keyboard presses, but beforeinput
    // gives us the actual text being inserted. This also handles swipe/gesture typing.
    const handleBeforeInput = (event: Event) => {
      // Only process if container is focused
      if (document.activeElement !== container) {
        return;
      }

      const inputEvent = event as InputEvent;
      const input = getInput();
      if (input && input.onBeforeInput(inputEvent)) {
        // Handler consumed the event - prevent default to avoid duplicate input
        event.preventDefault();
      }
    };

    // Reset input state when window regains focus (prevents stuck modifiers after Alt+Tab)
    const handleWindowFocus = () => {
      console.log('[DesktopStreamViewer] Window regained focus - resetting input state to prevent desync');
      resetInputState();
    };

    // Attach to container, not document (so we only capture when focused)
    container.addEventListener('keydown', handleKeyDown);
    container.addEventListener('keyup', handleKeyUp);
    container.addEventListener('beforeinput', handleBeforeInput);
    window.addEventListener('focus', handleWindowFocus);

    return () => {
      container.removeEventListener('keydown', handleKeyDown);
      container.removeEventListener('keyup', handleKeyUp);
      container.removeEventListener('beforeinput', handleBeforeInput);
      window.removeEventListener('focus', handleWindowFocus);
    };
  }, [isConnected, resetInputState]);

  // Focus container when clicking anywhere in the viewer
  // On phones (iPhone/Android), show/hide virtual keyboard based on cursor type
  // This is disabled on tablets since they often have hardware keyboards
  const handleContainerClick = useCallback(() => {
    if (isPhone && hiddenInputRef.current) {
      // Check if cursor is a text cursor (indicates text field is focused in remote desktop)
      // Text cursors: 'text', 'vertical-text', or cursorName containing 'text' or 'xterm'
      const isTextCursor = cursorCssName === 'text' ||
        cursorCssName === 'vertical-text' ||
        cursorImage?.cursorName === 'text' ||
        cursorImage?.cursorName === 'vertical-text' ||
        cursorImage?.cursorName?.includes('xterm') ||
        cursorImage?.cursorName?.includes('ibeam');

      if (isTextCursor) {
        // Focus hidden input to trigger virtual keyboard
        hiddenInputRef.current.focus();
        console.log('[DesktopStreamViewer] Text cursor detected - showing virtual keyboard (phone)');
      } else {
        // Blur hidden input to dismiss virtual keyboard
        hiddenInputRef.current.blur();
        // Focus container for keyboard events (hardware keyboard still works)
        containerRef.current?.focus();
        console.log('[DesktopStreamViewer] Non-text cursor - dismissing virtual keyboard (phone)');
      }
    } else if (containerRef.current) {
      containerRef.current.focus();
    }
  }, [cursorCssName, cursorImage, isPhone]);

  // Compute native CSS cursor style from cursor image or CSS name
  // Local user sees native cursor (no glowing overlay), remote users see glowing cursors
  const nativeCursorStyle = (() => {
    if (!isConnected) return 'default';
    if (!cursorVisible) return 'none';

    // If we have a cursor image with URL, use it as a custom cursor
    if (cursorImage?.imageUrl) {
      // CSS cursor: url(image) hotspotX hotspotY, fallback
      return `url(${cursorImage.imageUrl}) ${cursorImage.hotspotX} ${cursorImage.hotspotY}, auto`;
    }

    // If we have a CSS cursor name (from GNOME headless), use it directly
    if (cursorCssName) {
      return cursorCssName;
    }

    // Default fallback
    return 'default';
  })();

  return (
    <Box
      ref={containerRef}
      className={`${className} desktop-stream-viewer`}
      data-video-container="true"
      tabIndex={0}
      onClick={handleContainerClick}
      sx={{
        // Normal mode: relative positioning within parent
        // iOS fullscreen mode: fixed positioning covering entire viewport
        // When keyboard is open on phones, use fixed positioning to stay above keyboard
        position: isIOSFullscreen ? 'fixed' : keyboardHeight > 0 ? 'fixed' : 'relative',
        top: isIOSFullscreen ? 0 : keyboardHeight > 0 ? Math.max(0, viewportOffset) : undefined,
        left: isIOSFullscreen ? 0 : keyboardHeight > 0 ? 0 : undefined,
        right: isIOSFullscreen ? 0 : keyboardHeight > 0 ? 0 : undefined,
        bottom: isIOSFullscreen ? 0 : undefined,
        width: isIOSFullscreen ? '100vw' : keyboardHeight > 0 ? '100vw' : '100%',
        // Use dvh (dynamic viewport height) for iOS Safari toolbar handling
        // Falls back to vh for older browsers
        // When virtual keyboard is open, use the visible viewport height
        height: isIOSFullscreen
          ? '100dvh'
          : keyboardHeight > 0
            ? `calc(100vh - ${keyboardHeight}px - ${viewportOffset}px)`
            : '100%',
        // Ensure content doesn't overflow above the visible area
        maxHeight: keyboardHeight > 0 ? `calc(100vh - ${keyboardHeight}px)` : undefined,
        minHeight: isIOSFullscreen ? undefined : keyboardHeight > 0 ? 150 : 400,
        zIndex: keyboardHeight > 0 ? 1000 : undefined,
        backgroundColor: '#000',
        display: 'flex',
        flexDirection: 'column',
        outline: 'none',
        // High z-index for iOS fullscreen to cover everything
        zIndex: isIOSFullscreen ? 9999 : undefined,
        // Prevent iOS tap highlight (blue rectangle on touch)
        WebkitTapHighlightColor: 'transparent',
        WebkitTouchCallout: 'none',
        WebkitUserSelect: 'none',
        userSelect: 'none',
        // Cursor is hidden only on the canvas element, not the container
        // This ensures the cursor is visible in the black letterbox/pillarbox bars
        // Fallback height for iOS when dvh isn't supported
        '@supports not (height: 100dvh)': isIOSFullscreen ? {
          height: '-webkit-fill-available',
        } : {},
      }}
    >
      {/* Hidden input for iOS/iPad virtual keyboard support */}
      {/* iOS only shows keyboard when focus() is called directly within a user gesture */}
      {/* (not in setTimeout or Promise.then - those lose the gesture context) */}
      <input
        ref={hiddenInputRef}
        type="text"
        inputMode="text"
        autoComplete="off"
        autoCorrect="off"
        autoCapitalize="off"
        spellCheck={false}
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          width: 1,
          height: 1,
          opacity: 0,
          pointerEvents: 'none',
          fontSize: 16, // Prevents iOS auto-zoom on focus
        }}
        onKeyDown={(e) => {
          // Forward keyboard events to the stream input handler
          console.log('[DesktopStreamViewer] Hidden input keydown:', e.key, e.code);
          const input = streamRef.current?.getInput();
          if (input) {
            input.onKeyDown(e.nativeEvent);
          }
          e.preventDefault();
        }}
        onKeyUp={(e) => {
          console.log('[DesktopStreamViewer] Hidden input keyup:', e.key, e.code);
          const input = streamRef.current?.getInput();
          if (input) {
            input.onKeyUp(e.nativeEvent);
          }
          e.preventDefault();
        }}
        onBeforeInput={(e) => {
          // Handle beforeinput for swipe/gesture typing (Gboard, SwiftKey, etc.)
          // This fires before input, giving us access to the full text being inserted
          const inputEvent = e.nativeEvent as InputEvent;
          const inputType = inputEvent.inputType;
          const data = inputEvent.data;

          // insertText is used for swipe typing and autocomplete
          if (data && (inputType === 'insertText' || inputType === 'insertCompositionText')) {
            console.log('[DesktopStreamViewer] Hidden input beforeinput:', inputType, data);
            const input = streamRef.current?.getInput();
            if (input) {
              // Send the complete text (handles multi-character swipe results)
              for (const char of data) {
                input.sendText(char);
              }
            }
            e.preventDefault();
          }
        }}
        onInput={(e) => {
          // Fallback: Handle text input from virtual keyboard (iOS sends input events, not keydown)
          // This may fire after beforeinput, but we prevent default in beforeinput to avoid duplicates
          const inputEvent = e.nativeEvent as InputEvent;
          const data = inputEvent.data;
          if (data && !e.defaultPrevented) {
            console.log('[DesktopStreamViewer] Hidden input text (fallback):', data);
            const input = streamRef.current?.getInput();
            if (input) {
              // Send each character as a key event
              for (const char of data) {
                input.sendText(char);
              }
            }
          }
          // Clear the input to prevent accumulation
          (e.target as HTMLInputElement).value = '';
        }}
        onCompositionEnd={(e) => {
          // Handle composition end for IME and some swipe keyboards
          // This fires when the user completes a composition (e.g., selects from IME suggestions)
          const data = e.data;
          if (data) {
            console.log('[DesktopStreamViewer] Hidden input compositionEnd:', data);
            const input = streamRef.current?.getInput();
            if (input) {
              // Send the complete composed text
              for (const char of data) {
                input.sendText(char);
              }
            }
          }
          // Clear the input
          (e.target as HTMLInputElement).value = '';
        }}
      />

      {/* Toolbar - always visible so user can reconnect/access controls */}
      {/* z-index 1100 ensures toolbar is above connection overlay (z-index 1000) */}
      <Box
        sx={{
          position: 'absolute',
          top: 8,
          left: '50%',
          transform: 'translateX(-50%)',
          zIndex: 1100,
          backgroundColor: 'rgba(0,0,0,0.7)',
          borderRadius: 1,
          display: 'flex',
          flexWrap: 'wrap',
          justifyContent: 'center',
          gap: 0.5,
          px: 0.5,
          py: 0.5,
          maxWidth: 'calc(100vw - 16px)',
          cursor: 'default', // Show normal cursor in toolbar for usability
          pointerEvents: 'auto', // Ensure toolbar captures pointer events on iPad
        }}
        onClick={(e) => e.stopPropagation()} // Prevent clicks from reaching canvas
        onPointerDown={(e) => e.stopPropagation()} // Prevent pointer events from reaching canvas
      >
        {/* Control icons group - stays together, doesn't wrap internally */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <Tooltip title="Reconnect to streaming server" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <span>
            <IconButton
              size={toolbarIconSize as 'small' | 'medium'}
              onClick={() => {
                setReconnectClicked(true);
                reconnect(1000, 'Reconnecting...');
              }}
              sx={{ color: 'white' }}
              disabled={reconnectClicked || isConnecting}
            >
              {reconnectClicked || isConnecting ? <CircularProgress size={16} sx={{ color: 'white' }} /> : <Refresh fontSize={toolbarFontSize as 'small' | 'medium'} />}
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Stats for nerds - show streaming statistics" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size={toolbarIconSize as 'small' | 'medium'}
            onClick={() => setShowStats(!showStats)}
            sx={{ color: showStats ? 'primary.main' : 'white' }}
          >
            <BarChart fontSize={toolbarFontSize as 'small' | 'medium'} />
          </IconButton>
        </Tooltip>
        <Tooltip title="Charts - visualize throughput, RTT, and bitrate over time" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size={toolbarIconSize as 'small' | 'medium'}
            onClick={() => setShowCharts(!showCharts)}
            sx={{ color: showCharts ? 'primary.main' : 'white' }}
          >
            <Timeline fontSize={toolbarFontSize as 'small' | 'medium'} />
          </IconButton>
        </Tooltip>
        {/* Quality mode toggle: Video → Screenshots */}
        <Tooltip
          title={
            modeSwitchCooldown
              ? 'Please wait...'
              : qualityMode === 'video'
              ? 'Video streaming (60fps) — Click for Screenshot mode'
              : 'Screenshot mode — Click for Video streaming'
          }
          arrow
          slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}
        >
          <span>
            <IconButton
              size={toolbarIconSize as 'small' | 'medium'}
              disabled={modeSwitchCooldown}
              onClick={() => {
                // Toggle: video ↔ screenshot
                setModeSwitchCooldown(true);
                setQualityMode(prev => prev === 'video' ? 'screenshot' : 'video');
                setTimeout(() => setModeSwitchCooldown(false), 3000); // 3 second cooldown
              }}
              sx={{
                color: qualityMode === 'video'
                  ? '#4caf50'  // Green for video streaming
                  : '#ff9800',  // Orange for screenshot mode
              }}
            >
              {qualityMode === 'video' ? (
                <Speed fontSize={toolbarFontSize as 'small' | 'medium'} />
              ) : (
                <CameraAlt fontSize={toolbarFontSize as 'small' | 'medium'} />
              )}
            </IconButton>
          </span>
        </Tooltip>
        {/* Touch mode toggle: Direct touch ↔ Trackpad mode (only on touch devices) */}
        {hasTouchCapability && (
          <Tooltip
            title={
              touchMode === 'direct'
                ? 'Direct touch — Tap on screen position. Click for Trackpad mode'
                : 'Trackpad mode — Drag to move cursor, tap to click. Click for Direct touch'
            }
            arrow
            slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}
          >
            <IconButton
              size={toolbarIconSize as 'small' | 'medium'}
              onClick={() => setTouchMode(prev => prev === 'direct' ? 'trackpad' : 'direct')}
              sx={{
                color: touchMode === 'trackpad' ? '#2196f3' : 'white',  // Blue when trackpad mode active
              }}
            >
              {touchMode === 'direct' ? (
                <TouchApp fontSize={toolbarFontSize as 'small' | 'medium'} />
              ) : (
                <PanTool fontSize={toolbarFontSize as 'small' | 'medium'} />
              )}
            </IconButton>
          </Tooltip>
        )}
        {/* Zoom indicator (on touch devices) - shows when zoomed, tap to reset */}
        {hasTouchCapability && zoomLevel > 1 && (
          <Tooltip
            title="Tap to reset zoom"
            arrow
            slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}
          >
            <Button
              size={toolbarIconSize as 'small' | 'medium'}
              onClick={() => {
                setZoomLevel(1);
                setPanOffset({ x: 0, y: 0 });
              }}
              sx={{
                color: 'white',
                backgroundColor: 'rgba(33, 150, 243, 0.8)',
                minWidth: 'auto',
                px: 1,
                py: 0.25,
                fontSize: '0.75rem',
                fontWeight: 'bold',
                '&:hover': {
                  backgroundColor: 'rgba(33, 150, 243, 1)',
                },
              }}
            >
              {zoomLevel.toFixed(1)}x
            </Button>
          </Tooltip>
        )}
        {/* Bitrate selector - hidden in screenshot mode (has its own adaptive quality) */}
        {qualityMode !== 'screenshot' && (
          <Tooltip title="Select streaming bitrate" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
            <Button
              size={toolbarIconSize as 'small' | 'medium'}
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
              {bitrate} Mbps {bitrate === getDefaultBitrateForResolution(width, height) / 1000 && '(default)'}
            </MenuItem>
          ))}
        </Menu>
        <Tooltip title={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'} arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
          <IconButton
            size={toolbarIconSize as 'small' | 'medium'}
            onClick={toggleFullscreen}
            sx={{ color: 'white' }}
          >
            {isFullscreen ? <FullscreenExit fontSize={toolbarFontSize as 'small' | 'medium'} /> : <Fullscreen fontSize={toolbarFontSize as 'small' | 'medium'} />}
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
              size={toolbarIconSize as 'small' | 'medium'}
              onClick={() => {
                if (bitrateRecommendation.type === 'screenshot') {
                  // Switch to screenshot mode
                  setQualityMode('screenshot');
                  addChartEvent('reduce', 'User switched to screenshot mode');
                } else {
                  // Change bitrate
                  setUserBitrate(bitrateRecommendation.targetBitrate);
                  addChartEvent(
                    bitrateRecommendation.type === 'decrease' ? 'reduce' : 'increase',
                    `User accepted: ${userBitrate ?? requestedBitrate}→${bitrateRecommendation.targetBitrate} Mbps`
                  );
                }
                manualBitrateSelectionTimeRef.current = Date.now();
                setBitrateRecommendation(null);
              }}
              sx={{
                backgroundColor: bitrateRecommendation.type === 'screenshot'
                  ? 'rgba(244, 67, 54, 0.9)' // Red for screenshot recommendation
                  : bitrateRecommendation.type === 'decrease'
                    ? 'rgba(255, 152, 0, 0.9)'
                    : 'rgba(76, 175, 80, 0.9)',
                color: 'white',
                fontSize: '0.65rem',
                px: 1,
                py: 0.25,
                minWidth: 'auto',
                textTransform: 'none',
                borderRadius: 1,
                '&:hover': {
                  backgroundColor: bitrateRecommendation.type === 'screenshot'
                    ? 'rgba(244, 67, 54, 1)'
                    : bitrateRecommendation.type === 'decrease'
                      ? 'rgba(255, 152, 0, 1)'
                      : 'rgba(76, 175, 80, 1)',
                },
              }}
            >
              {bitrateRecommendation.type === 'screenshot'
                ? 'Struggling · Try screenshots'
                : bitrateRecommendation.type === 'decrease'
                  ? `Slow connection · Try ${bitrateRecommendation.targetBitrate}M`
                  : `Improved · Try ${bitrateRecommendation.targetBitrate}M`}
            </Button>
          </Tooltip>
        )}

        </Box>
        {/* End control icons group */}

        {/* Presence indicators - connected users + agent */}
        {/* This group can wrap to a new line on narrow screens */}
        {isConnected && remoteUsers.size > 0 && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            {Array.from(remoteUsers.values()).map((user) => (
              <Tooltip key={user.userId} title={user.userName} arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
                <Box
                  sx={{
                    width: 22,
                    height: 22,
                    minWidth: 22,
                    minHeight: 22,
                    borderRadius: '50%',
                    backgroundColor: user.color,
                    border: user.userId === selfClientId ? '2px solid white' : 'none',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 10,
                    fontWeight: 'bold',
                    color: 'white',
                    cursor: 'default',
                    flexShrink: 0,
                  }}
                >
                    {user.avatarUrl ? (
                      <Box
                        component="img"
                        src={user.avatarUrl}
                        sx={{ width: '100%', height: '100%', borderRadius: '50%' }}
                      />
                    ) : (
                      user.userName.charAt(0).toUpperCase()
                    )}
                  </Box>
                </Tooltip>
              ))}
            {/* Agent indicator */}
            <Tooltip title="AI Agent" arrow slotProps={{ popper: { disablePortal: true, sx: { zIndex: 10000 } } }}>
              <Box
                sx={{
                  width: 22,
                  height: 22,
                  minWidth: 22,
                  minHeight: 22,
                  borderRadius: '50%',
                  backgroundColor: '#00D4FF',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 11,
                  cursor: 'default',
                  flexShrink: 0,
                  opacity: agentCursor ? 1 : 0.5,
                }}
              >
                🤖
              </Box>
            </Tooltip>
          </Box>
        )}
      </Box>

      {/* Screenshot Mode / High Latency Warning Banner */}
      {shouldPollScreenshots && isConnected && (
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

      {/* Adaptive Throttle Warning Banner - shown when input rate is reduced due to high latency */}
      {isThrottled && isConnected && !shouldPollScreenshots && (
        <Box
          sx={{
            position: 'absolute',
            top: 50,
            left: '50%',
            transform: 'translateX(-50%)',
            zIndex: 999,
            backgroundColor: 'rgba(255, 152, 0, 0.9)',
            color: 'black',
            padding: '4px 16px',
            borderRadius: 1,
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            fontFamily: 'monospace',
            fontSize: '0.75rem',
          }}
        >
          <Wifi sx={{ fontSize: 16 }} />
          <Typography variant="caption" sx={{ fontWeight: 'bold' }}>
            High latency detected - input rate reduced to {stats?.video?.effectiveInputFps?.toFixed(0) || '?'} Hz
          </Typography>
        </Box>
      )}

      {/* Connection Status Overlay */}
      {!suppressOverlay && (
        <ConnectionOverlay
          isConnected={isConnected}
          isConnecting={isConnecting}
          error={error}
          status={status}
          retryCountdown={retryCountdown}
          retryAttemptDisplay={retryAttemptDisplay}
          reconnectClicked={reconnectClicked}
          onReconnect={() => {
            setReconnectClicked(true);
            reconnect(500, 'Reconnecting...');
          }}
          onClearError={() => setError(null)}
        />
      )}

      {/* Canvas Element - centered with proper aspect ratio */}
      <canvas
        ref={canvasRef}
        onMouseDown={handleMouseDown}
        onMouseUp={handleMouseUp}
        onMouseMove={handleMouseMove}
        onMouseEnter={() => {
          resetInputState();
          setIsMouseOverCanvas(true);
        }}
        onMouseLeave={() => {
          setIsMouseOverCanvas(false);
        }}
        onContextMenu={handleContextMenu}
        onTouchStart={handleTouchStart}
        onTouchMove={handleTouchMove}
        onTouchEnd={handleTouchEnd}
        onTouchCancel={handleTouchCancel}
        style={{
          // Use calculated dimensions to maintain aspect ratio
          // Canvas doesn't support objectFit like video, so we calculate size manually
          width: canvasDisplaySize ? `${canvasDisplaySize.width}px` : '100%',
          height: canvasDisplaySize ? `${canvasDisplaySize.height}px` : '100%',
          // Center the canvas within the container, with pinch-zoom and pan transform
          position: 'absolute',
          left: '50%',
          top: '50%',
          // Order: translate to center, then apply zoom, then apply pan offset
          transform: `translate(-50%, -50%) scale(${zoomLevel}) translate(${panOffset.x / zoomLevel}px, ${panOffset.y / zoomLevel}px)`,
          transformOrigin: 'center center',
          backgroundColor: '#000',
          cursor: nativeCursorStyle, // Use native cursor (custom image or CSS name from server)
          // Always visible for input capture
          // In video mode: renders video AND handles input
          // In screenshot mode: transparent but handles input (screenshot overlays on top)
          display: 'block',
          // Transparent in screenshot mode so overlays are visible, but still captures input
          opacity: qualityMode === 'video' ? 1 : 0,
          zIndex: 20,
          // Prevent browser from handling touch gestures (no scroll, pan, zoom)
          // This ensures all touch events go to our handlers
          touchAction: 'none',
        }}
        onClick={() => {
          // Focus container for keyboard input
          if (containerRef.current) {
            containerRef.current.focus();
          }
        }}
      />

      {/* Screenshot overlay for screenshot mode */}
      {/* Shows rapidly-updated screenshots instead of video stream while keeping input working */}
      {shouldPollScreenshots && screenshotUrl && (
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
            // Apply same pinch-zoom and pan as canvas
            transform: zoomLevel > 1 ? `scale(${zoomLevel}) translate(${panOffset.x / zoomLevel}px, ${panOffset.y / zoomLevel}px)` : undefined,
            transformOrigin: 'center center',
          }}
        />
      )}

      {/* Remote user cursors (Figma-style multi-player) - uses glowing CursorRenderer */}
      <RemoteCursorsOverlay
        cursors={remoteCursors}
        users={remoteUsers}
        selfClientId={selfClientId}
        selfClientIdRef={selfClientIdRef}
        canvasDisplaySize={canvasDisplaySize}
        containerSize={containerSize}
        streamWidth={width}
        streamHeight={height}
      />

      {/* Local cursor for trackpad mode on touch devices (iPad) */}
      {/* Native CSS cursor doesn't work well on iPad, so we render an overlay cursor */}
      {/* Only show after cursor position is initialized (hasMouseMoved) to avoid flash at (0,0) */}
      {/* Uses transform + ref for smooth 60fps updates (bypasses React render cycle) */}
      {isConnected && touchMode === 'trackpad' && hasTouchCapability && cursorVisible && hasMouseMoved && (
        <div
          ref={trackpadCursorRef}
          style={{
            position: 'absolute',
            left: 0,
            top: 0,
            transform: `translate(${cursorPosition.x}px, ${cursorPosition.y}px)`,
            willChange: 'transform',
            pointerEvents: 'none',
            zIndex: 1000,
          }}
        >
          <CursorRenderer
            x={0}
            y={0}
            cursorImage={cursorImage}
            cursorCssName={cursorCssName}
            showDebugDot={false}
          />
        </div>
      )}

      {/* AI Agent cursor */}
      <AgentCursorOverlay
        agentCursor={agentCursor}
        canvasDisplaySize={canvasDisplaySize}
        containerSize={containerSize}
        streamWidth={width}
        streamHeight={height}
      />

      {/* Remote touch events */}
      {Array.from(remoteTouches.values()).map((touch) => {
        const user = remoteUsers.get(touch.userId);
        // Prefer color from touch event, fall back to user color, then default
        const color = touch.color || user?.color || '#888888';
        const size = 32 + touch.pressure * 16;

        // Scale remote touch from screen coordinates to container-relative coordinates
        // Use configured stream resolution (width/height props), not canvas dimensions
        if (!canvasDisplaySize || !containerSize) return null;

        const streamWidth = width;
        const streamHeight = height;

        const scaleX = canvasDisplaySize.width / streamWidth;
        const scaleY = canvasDisplaySize.height / streamHeight;
        const offsetX = (containerSize.width - canvasDisplaySize.width) / 2;
        const offsetY = (containerSize.height - canvasDisplaySize.height) / 2;
        const displayX = offsetX + touch.x * scaleX;
        const displayY = offsetY + touch.y * scaleY;

        return (
          <Box
            key={`touch-${touch.userId}-${touch.touchId}`}
            sx={{
              position: 'absolute',
              left: displayX - size / 2,
              top: displayY - size / 2,
              width: size,
              height: size,
              borderRadius: '50%',
              border: `3px solid ${color}`,
              backgroundColor: `${color}40`,
              pointerEvents: 'none',
              zIndex: 1001,
              animation: touch.eventType === 'start' ? 'touchStart 0.3s' : 'none',
            }}
          />
        );
      })}

      {/* Input Hint - removed since auto-focus handles keyboard input */}
      {/* Presence Indicator - moved to toolbar above */}

      {/* Stats for Nerds Overlay */}
      {showStats && (stats || qualityMode === 'screenshot') && (
        <StatsOverlay
          stats={stats}
          qualityMode={qualityMode}
          activeConnections={activeConnectionsDisplay}
          requestedBitrate={requestedBitrate}
          debugThrottleRatio={debugThrottleRatio}
          onDebugThrottleRatioChange={setDebugThrottleRatio}
          shouldPollScreenshots={shouldPollScreenshots}
          screenshotFps={screenshotFps}
          screenshotQuality={screenshotQuality}
          debugKeyEvent={debugKeyEvent}
        />
      )}

      {/* Adaptive Bitrate Charts Panel */}
      {showCharts && (
        <ChartsPanel
          throughputHistory={throughputHistoryRef.current}
          rttHistory={rttHistoryRef.current}
          bitrateHistory={bitrateHistoryRef.current}
          frameDriftHistory={frameDriftHistoryRef.current}
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

export default DesktopStreamViewer;
 
