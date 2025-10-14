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
  width = 1920,
  height = 1080,
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


  // Connect to stream
  const connect = useCallback(async () => {
    setIsConnecting(true);
    setError(null);
    setStatus('Connecting to streaming server...');

    try {
      // CRITICAL: Set credentials in sessionStorage BEFORE calling getApi
      // This prevents the blocking credential prompt modal
      sessionStorage.setItem('mlCredentials', 'helix');

      // Create API instance pointing to our moonlight-web backend
      const api = await getApi('/moonlight/api');

      // Credentials already set above
      api.credentials = 'helix';

      // Get default stream settings and customize
      const settings = defaultStreamSettings();
      settings.bitrate = 20000;
      settings.packetSize = 1024;
      settings.fps = 60;
      settings.playAudioLocal = !audioEnabled;

      // Force H264 only for compatibility with Wolf-UI
      // getSupportedVideoFormats() might return AV1 which Wolf-UI doesn't handle well
      const supportedFormats = {
        H264: true,
        H264_HIGH8_444: false,
        H265: false,
        H265_MAIN10: false,
        H265_REXT8_444: false,
        H265_REXT10_444: false,
        AV1_MAIN8: false,
        AV1_MAIN10: false,
        AV1_HIGH8_444: false,
        AV1_HIGH10_444: false
      };

      // If we have a wolfLobbyId, fetch apps to find the correct app ID
      let actualAppId = appId;
      if (wolfLobbyId) {
        try {
          // Use the authenticated API client to fetch apps
          const apps = await apiGetApps(api, { host_id: hostId });

          // Find app matching our session or use the first available app
          if (apps && apps.length > 0) {
            actualAppId = apps[0].app_id; // Note: field is app_id not id
            console.log(`Found Moonlight app ID: ${actualAppId}`, apps[0]);
            setStatus(`Connecting to app: ${apps[0].title || actualAppId}`);
          } else {
            console.warn('No Moonlight apps available');
            setError('No streaming apps available');
            setIsConnecting(false);
            return;
          }
        } catch (err: any) {
          console.warn('Failed to fetch Moonlight apps:', err.message);
          setError('Failed to fetch streaming apps');
          setIsConnecting(false);
          return;
        }
      }

      // Create Stream instance
      // For external agents, join the existing keepalive session instead of creating new
      // Keepalive session ID format: "agent-{sessionId}"
      const stream = new Stream(
        api,
        hostId, // Wolf host ID (always 0 for local)
        actualAppId, // Moonlight app ID
        settings,
        supportedFormats,
        [width, height],
        "join", // Join existing keepalive session
        `agent-${sessionId}` // Keepalive session ID format
      );

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
  }, [hostId, appId, width, height, audioEnabled, onConnectionChange, onError]);

  // Disconnect
  const disconnect = useCallback(() => {
    if (streamRef.current) {
      // Stream class handles cleanup internally
      streamRef.current = null;
    }

    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }

    setIsConnected(false);
    setIsConnecting(false);
    setStatus('Disconnected');
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
    return () => {
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
    streamRef.current?.getInput().onMouseMove(event.nativeEvent, getStreamRect());
  }, [getStreamRect]);

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
        }}
        onClick={() => {
          // Unmute on first interaction (browser autoplay policy)
          if (videoRef.current) {
            videoRef.current.muted = false;
          }
        }}
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
          💡 Click video to unmute | F11 for fullscreen | Gamepad supported
        </Typography>
      )}
    </Box>
  );
};

export default MoonlightStreamViewer;
