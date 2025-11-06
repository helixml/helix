import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton, Button } from '@mui/material';
import {
  Fullscreen,
  FullscreenExit,
  Refresh,
  Videocam,
  VideocamOff,
  VolumeUp,
  VolumeOff,
  BarChart,
} from '@mui/icons-material';
import { getApi, apiGetApps } from '../../lib/moonlight-web-ts/api';
import { Stream } from '../../lib/moonlight-web-ts/stream/index';
import { defaultStreamSettings } from '../../lib/moonlight-web-ts/component/settings_menu';
import { getSupportedVideoFormats } from '../../lib/moonlight-web-ts/stream/video';
import useApi from '../../hooks/useApi';
import { useAccount } from '../../contexts/account';
import { FRONTEND_INSTANCE_ID } from '../../utils/instanceId';
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
  width = 3840,
  height = 2160,
  className = '',
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<any>(null); // Stream instance from moonlight-web
  const retryAttemptRef = useRef(0); // Use ref to avoid closure issues
  const previousLobbyIdRef = useRef<string | undefined>(undefined); // Track lobby changes

  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState('Initializing...');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [videoEnabled, setVideoEnabled] = useState(true);
  const [audioEnabled, setAudioEnabled] = useState(true);
  const [cursorPosition, setCursorPosition] = useState({ x: 0, y: 0 });
  const [hasMouseMoved, setHasMouseMoved] = useState(false);
  const [retryCountdown, setRetryCountdown] = useState<number | null>(null);
  const [retryAttemptDisplay, setRetryAttemptDisplay] = useState(0);
  const [showStats, setShowStats] = useState(false);

  // Clipboard sync state
  const lastRemoteClipboardHash = useRef<string>(''); // Track changes to avoid unnecessary writes
  const [stats, setStats] = useState<any>(null);
  const lastBytesRef = useRef<{ bytes: number; timestamp: number } | null>(null);

  const helixApi = useApi();
  const account = useAccount();

  // Connect to stream
  const connect = useCallback(async () => {
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
        try {
          const response = await fetch('/api/v1/wolf/ui-app-id', {
            headers: {
              'Authorization': `Bearer ${account.user?.token || ''}`,
            },
          });
          if (response.ok) {
            const data = await response.json();
            actualAppId = parseInt(data.wolf_ui_app_id, 10);
            console.log(`MoonlightStreamViewer: Using Wolf UI app ID ${actualAppId} for lobbies mode, lobby ${wolfLobbyId}`);
          } else {
            console.warn('MoonlightStreamViewer: Failed to fetch Wolf UI app ID, using default 0');
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

      // Get default stream settings and customize
      const settings = defaultStreamSettings();
      settings.videoSize = 'custom';
      settings.videoSizeCustom = { width: 3840, height: 2160 };  // 4K resolution
      settings.bitrate = 40000;  // 40 Mbps (reduced from 80 - P-frames more efficient than all I-frames)
      settings.packetSize = 1024;
      settings.fps = 60;
      settings.videoSampleQueueSize = 100;  // Increased for 4K60@80Mbps (was 5, then 50)
      settings.audioSampleQueueSize = 20;
      settings.playAudioLocal = !audioEnabled;

      // Use H264 Main profile (matches Wolf encoder)
      // Wolf doesn't support 4:4:4, only 4:2:0 (NV12)
      const supportedFormats = {
        H264: true,
        H264_HIGH8_444: false,  // Wolf doesn't support 4:4:4
        H265: false,
        H265_MAIN10: false,
        H265_REXT8_444: false,
        H265_REXT10_444: false,
        AV1_MAIN8: false,
        AV1_MAIN10: false,
        AV1_HIGH8_444: false,
        AV1_HIGH10_444: false
      };

      // Create Stream instance with mode-aware parameters
      console.log('[MoonlightStreamViewer] Creating Stream instance', {
        mode: moonlightWebMode,
        hostId,
        actualAppId,
        sessionId,
      });

      let stream;
      if (moonlightWebMode === 'multi') {
        // Multi-WebRTC architecture: backend created streamer via POST /api/streamers
        // Connect to persistent streamer via peer endpoint
        // Include instance ID for multi-tab support
        const streamerID = `agent-${sessionId}-${FRONTEND_INSTANCE_ID}`;
        stream = new Stream(
          api,
          hostId, // Wolf host ID (always 0 for local)
          actualAppId, // App ID (backend already knows it)
          settings,
          supportedFormats,
          [width, height],
          "peer", // Peer mode - connects to existing streamer
          undefined, // No session ID needed
          streamerID // Streamer ID - unique per browser tab
        );
      } else {
        // Single mode (kickoff approach): Fresh "create" connection with explicit client_unique_id
        // - Kickoff used: session="agent-{sessionId}-kickoff", client_unique_id="helix-agent-{sessionId}"
        // - Browser uses: session="agent-{sessionId}-{instanceId}", client_unique_id="helix-agent-{sessionId}-{instanceId}"
        // Different session_id → Fresh streamer process (no peer reuse)
        // Unique client_unique_id per browser tab → Multiple tabs can stream simultaneously!
        stream = new Stream(
          api,
          hostId, // Wolf host ID (always 0 for local)
          actualAppId, // Moonlight app ID from Wolf
          settings,
          supportedFormats,
          [width, height],
          "create", // Create mode - fresh session/streamer (kickoff already terminated)
          `agent-${sessionId}-${FRONTEND_INSTANCE_ID}`, // Unique per browser tab
          undefined, // No streamer ID
          `helix-agent-${sessionId}-${FRONTEND_INSTANCE_ID}` // Unique per browser tab → enables multi-tab streaming
        );
      }

      streamRef.current = stream;

      // Listen for stream events
      stream.addInfoListener((event: any) => {
        const data = event.detail;

        if (data.type === 'connectionComplete') {
          setIsConnected(true);
          setIsConnecting(false);
          setStatus('Streaming active');
          setError(null); // Clear any previous errors on successful connection
          retryAttemptRef.current = 0; // Reset retry counter on successful connection
          setRetryAttemptDisplay(0);
          onConnectionChange?.(true);

          // Auto-join lobby if in lobbies mode (after connection established)
          if (wolfLobbyId && sessionId) {
            console.log('[AUTO-JOIN] Connection established, triggering auto-join for lobby:', wolfLobbyId);

            // Use setTimeout to ensure connection is fully established before triggering auto-join
            setTimeout(async () => {
              try {
                // SECURITY: Backend derives Wolf client_id from Wolf API (no frontend parameter)
                // This prevents manipulation - backend matches by client_unique_id pattern
                console.log('[AUTO-JOIN] Triggering secure auto-join (backend derives Wolf client_id)');

                const apiClient = helixApi.getApiClient();
                const response = await apiClient.v1ExternalAgentsAutoJoinLobbyCreate(sessionId);

                if (response.status === 200) {
                  console.log('[AUTO-JOIN] ✅ Successfully auto-joined lobby:', response.data);
                } else {
                  console.warn('[AUTO-JOIN] Failed to auto-join lobby. Status:', response.status);
                }
              } catch (err: any) {
                // Log error but don't fail - user can still manually join
                console.error('[AUTO-JOIN] Error calling auto-join endpoint:', err);
                console.error('[AUTO-JOIN] User can still manually join lobby via Wolf UI');
              }
            }, 1000); // Wait 1 second after connection complete
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
        }
      });

      // Attach media stream to video element
      if (videoRef.current) {
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
      setIsConnecting(false);
      retryAttemptRef.current = 0; // Reset retry counter on different error
      setRetryAttemptDisplay(0);
      onError?.(errorMsg);
    }
  }, [sessionId, hostId, appId, width, height, audioEnabled, onConnectionChange, onError, helixApi, account, isPersonalDevEnvironment]);

  // Disconnect
  const disconnect = useCallback(() => {
    console.log('[MoonlightStreamViewer] disconnect() called, cleaning up stream resources');

    if (streamRef.current) {
      // Properly close the stream to prevent "AlreadyStreaming" errors
      try {
        console.log('[MoonlightStreamViewer] Closing WebSocket and RTCPeerConnection...');

        // Close WebSocket connection if it exists
        if (streamRef.current.ws) {
          console.log('[MoonlightStreamViewer] Closing WebSocket, readyState:', streamRef.current.ws.readyState);
          streamRef.current.ws.close();
        }

        // Close RTCPeerConnection if it exists
        if (streamRef.current.peer) {
          console.log('[MoonlightStreamViewer] Closing RTCPeerConnection');
          streamRef.current.peer.close();
        }

        // Stop all media stream tracks
        const mediaStream = streamRef.current.getMediaStream();
        if (mediaStream) {
          const tracks = mediaStream.getTracks();
          console.log('[MoonlightStreamViewer] Stopping', tracks.length, 'media tracks');
          tracks.forEach((track: MediaStreamTrack) => track.stop());
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
    console.log('[MoonlightStreamViewer] disconnect() completed');
  }, []);

  // Reconnect
  const reconnect = useCallback(() => {
    disconnect();
    setTimeout(connect, 1000);
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

  // Detect lobby changes and reconnect (for test script restart scenarios)
  useEffect(() => {
    if (wolfLobbyId && previousLobbyIdRef.current && previousLobbyIdRef.current !== wolfLobbyId) {
      console.log('[MoonlightStreamViewer] Lobby changed from', previousLobbyIdRef.current, 'to', wolfLobbyId);
      console.log('[MoonlightStreamViewer] Disconnecting old stream and reconnecting to new lobby');
      reconnect();
    }
    previousLobbyIdRef.current = wolfLobbyId;
  }, [wolfLobbyId, reconnect]);

  // Auto-connect on mount
  useEffect(() => {
    connect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Empty deps intentional - only connect once on mount

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

  // Handle video/audio toggle
  useEffect(() => {
    if (videoRef.current) {
      // Mute/unmute video element
      videoRef.current.muted = !audioEnabled;

      // TODO: Signal to Stream instance to stop sending video/audio tracks
    }
  }, [videoEnabled, audioEnabled]);

  // Poll WebRTC stats when stats overlay is visible
  useEffect(() => {
    if (!showStats || !streamRef.current) {
      return;
    }

    const pollStats = async () => {
      const peer = streamRef.current?.getPeer?.();
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
  }, [showStats]);

  // Calculate stream rectangle for mouse coordinate mapping
  const getStreamRect = useCallback((): DOMRect => {
    if (!videoRef.current || !streamRef.current) {
      return new DOMRect(0, 0, width, height);
    }

    const videoSize = streamRef.current.getStreamerSize() || [width, height];
    const videoAspect = videoSize[0] / videoSize[1];

    const boundingRect = videoRef.current.getBoundingClientRect();
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
  }, [width, height]);

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
      {/* Toolbar */}
      <Box
        sx={{
          position: 'absolute',
          top: 8,
          right: 8,
          zIndex: 1000,
          backgroundColor: 'rgba(0,0,0,0.7)',
          borderRadius: 1,
          display: 'flex',
          gap: 1,
          opacity: isConnected ? 1 : 0,
          transition: 'opacity 0.3s',
        }}
      >
        <IconButton
          size="small"
          onClick={() => setVideoEnabled(!videoEnabled)}
          sx={{ color: videoEnabled ? 'white' : 'grey' }}
          title={videoEnabled ? 'Disable Video' : 'Enable Video'}
        >
          {videoEnabled ? <Videocam fontSize="small" /> : <VideocamOff fontSize="small" />}
        </IconButton>
        <IconButton
          size="small"
          onClick={() => setAudioEnabled(!audioEnabled)}
          sx={{ color: audioEnabled ? 'white' : 'grey' }}
          title={audioEnabled ? 'Mute Audio' : 'Unmute Audio'}
        >
          {audioEnabled ? <VolumeUp fontSize="small" /> : <VolumeOff fontSize="small" />}
        </IconButton>
        <IconButton
          size="small"
          onClick={reconnect}
          sx={{ color: 'white' }}
          title="Reconnect"
          disabled={isConnecting}
        >
          <Refresh fontSize="small" />
        </IconButton>
        <IconButton
          size="small"
          onClick={() => setShowStats(!showStats)}
          sx={{ color: showStats ? 'primary.main' : 'white' }}
          title="Stats for Nerds"
        >
          <BarChart fontSize="small" />
        </IconButton>
        <IconButton
          size="small"
          onClick={toggleFullscreen}
          sx={{ color: 'white' }}
          title={isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'}
        >
          {isFullscreen ? <FullscreenExit fontSize="small" /> : <Fullscreen fontSize="small" />}
        </IconButton>
      </Box>

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

      {/* Video Element */}
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
      {showStats && stats && (
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
            {stats.video.codec && (
              <>
                <div><strong>Codec:</strong> {stats.video.codec}</div>
                <div><strong>Resolution:</strong> {stats.video.width}x{stats.video.height}</div>
                <div><strong>FPS:</strong> {stats.video.fps}</div>
                <div><strong>Bitrate:</strong> {stats.video.bitrate} Mbps <span style={{ color: '#888' }}>req: 80</span></div>
                <div><strong>Decoded:</strong> {stats.video.framesDecoded} frames</div>
                <div>
                  <strong>Dropped:</strong> {stats.video.framesDropped} frames
                  {stats.video.framesDropped > 0 && <span style={{ color: '#ff6b6b' }}> ⚠️</span>}
                </div>
                <div>
                  <strong>Packets Lost:</strong> {stats.video.packetsLost} / {stats.video.packetsReceived}
                  {stats.video.packetsLost > 0 && <span style={{ color: '#ff6b6b' }}> ⚠️</span>}
                </div>
                <div><strong>Jitter:</strong> {stats.video.jitter} ms</div>
                {stats.connection.rtt && <div><strong>RTT:</strong> {stats.connection.rtt} ms</div>}
              </>
            )}
            {!stats.video.codec && <div>Waiting for video data...</div>}
          </Box>
        </Box>
      )}
    </Box>
  );
};

export default MoonlightStreamViewer;
 
