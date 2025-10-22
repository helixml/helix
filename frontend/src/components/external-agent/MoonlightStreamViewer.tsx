import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton } from '@mui/material';
import {
  Fullscreen,
  FullscreenExit,
  Refresh,
  Videocam,
  VideocamOff,
  VolumeUp,
  VolumeOff,
} from '@mui/icons-material';
import { getApi, apiGetApps } from '../../lib/moonlight-web-ts/api';
import { Stream } from '../../lib/moonlight-web-ts/stream/index';
import { defaultStreamSettings } from '../../lib/moonlight-web-ts/component/settings_menu';
import { getSupportedVideoFormats } from '../../lib/moonlight-web-ts/stream/video';
import useApi from '../../hooks/useApi';
import { useAccount } from '../../contexts/account';

interface MoonlightStreamViewerProps {
  sessionId: string;
  wolfLobbyId?: string;
  hostId?: number;
  appId?: number;
  isPersonalDevEnvironment?: boolean;
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
  onConnectionChange,
  onError,
  width = 3840,
  height = 2160,
  className = '',
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<any>(null); // Stream instance from moonlight-web

  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState('Initializing...');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [videoEnabled, setVideoEnabled] = useState(true);
  const [audioEnabled, setAudioEnabled] = useState(true);
  const [cursorPosition, setCursorPosition] = useState({ x: 0, y: 0 });
  const [hasMouseMoved, setHasMouseMoved] = useState(false);

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

      // For external agents, fetch the actual Wolf app ID
      let actualAppId = appId;
      if (sessionId && !isPersonalDevEnvironment) {
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
      settings.bitrate = 10000;  // 10 Mbps for 4K streaming
      settings.packetSize = 1024;
      settings.fps = 60;
      settings.playAudioLocal = !audioEnabled;

      // Use browser's actual codec support (getSupportedVideoFormats checks what browser can decode)
      const supportedFormats = getSupportedVideoFormats();

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
        const streamerID = `agent-${sessionId}`;
        stream = new Stream(
          api,
          hostId, // Wolf host ID (always 0 for local)
          actualAppId, // App ID (backend already knows it)
          settings,
          supportedFormats,
          [width, height],
          "peer", // Peer mode - connects to existing streamer
          undefined, // No session ID needed
          streamerID // Streamer ID - connects to /api/streamers/{id}/peer
        );
      } else {
        // Single mode (kickoff approach): Fresh "create" connection with explicit client_unique_id
        // - Kickoff used: session="agent-{sessionId}-kickoff", client_unique_id="helix-agent-{sessionId}"
        // - Browser uses: session="agent-{sessionId}", client_unique_id="helix-agent-{sessionId}"
        // Different session_id → Fresh streamer process (no peer reuse)
        // Same client_unique_id → Moonlight protocol auto-RESUME!
        stream = new Stream(
          api,
          hostId, // Wolf host ID (always 0 for local)
          actualAppId, // Moonlight app ID from Wolf
          settings,
          supportedFormats,
          [width, height],
          "create", // Create mode - fresh session/streamer (kickoff already terminated)
          `agent-${sessionId}`, // Browser session ID (different from kickoff's "-kickoff" suffix)
          undefined, // No streamer ID
          `helix-agent-${sessionId}` // SAME client_unique_id as kickoff → enables RESUME
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
          onConnectionChange?.(true);
        } else if (data.type === 'error') {
          setError(data.message);
          setIsConnecting(false);
          onError?.(data.message);
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
      setError(errorMsg);
      setIsConnecting(false);
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

  // Auto-connect on mount
  useEffect(() => {
    connect();
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    console.log('[MoonlightStreamViewer] Component mounted, setting up cleanup handler');
    return () => {
      console.log('[MoonlightStreamViewer] Component unmounting, calling disconnect()');
      disconnect();
    };
  }, [disconnect]);

  // Handle video/audio toggle
  useEffect(() => {
    if (videoRef.current) {
      // Mute/unmute video element
      videoRef.current.muted = !audioEnabled;

      // TODO: Signal to Stream instance to stop sending video/audio tracks
    }
  }, [videoEnabled, audioEnabled]);

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

  const handleWheel = useCallback((event: React.WheelEvent) => {
    event.preventDefault();
    streamRef.current?.getInput().onMouseWheel(event.nativeEvent);
  }, []);

  const handleContextMenu = useCallback((event: React.MouseEvent) => {
    event.preventDefault();
  }, []);

  const handleKeyDown = useCallback((event: React.KeyboardEvent) => {
    event.preventDefault();
    streamRef.current?.getInput().onKeyDown(event.nativeEvent);
  }, []);

  const handleKeyUp = useCallback((event: React.KeyboardEvent) => {
    event.preventDefault();
    streamRef.current?.getInput().onKeyUp(event.nativeEvent);
  }, []);

  return (
    <Box
      ref={containerRef}
      className={className}
      tabIndex={0}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      sx={{
        position: 'relative',
        width: '100%',
        height: '100%',
        minHeight: 400,
        backgroundColor: '#000',
        display: 'flex',
        flexDirection: 'column',
        outline: 'none',
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
          onClick={toggleFullscreen}
          sx={{ color: 'white' }}
          title={isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'}
        >
          {isFullscreen ? <FullscreenExit fontSize="small" /> : <Fullscreen fontSize="small" />}
        </IconButton>
      </Box>

      {/* Status Overlay */}
      {(isConnecting || error) && (
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

          {error && (
            <Alert severity="error" sx={{ maxWidth: 400 }}>
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
        onWheel={handleWheel}
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

      {/* Input Hint */}
      {isConnected && (
        <Typography
          variant="caption"
          sx={{
            position: 'absolute',
            bottom: 8,
            left: '50%',
            transform: 'translateX(-50%)',
            color: 'rgba(255,255,255,0.5)',
            fontSize: '0.65rem',
            pointerEvents: 'none',
            backgroundColor: 'rgba(0,0,0,0.7)',
            padding: '4px 8px',
            borderRadius: '4px',
          }}
        >
          💡 Fullscreen for keyboard input
        </Typography>
      )}
    </Box>
  );
};

export default MoonlightStreamViewer;
