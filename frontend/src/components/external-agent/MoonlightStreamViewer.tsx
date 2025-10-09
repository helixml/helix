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
import { getApi } from '../../lib/moonlight-web-ts/api';
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
      // Create API instance pointing to our moonlight-web backend
      const api = await getApi('/moonlight/api');

      // Set credentials (must match moonlight-web-config/config.json)
      api.credentials = 'helix';

      // Get default stream settings and customize
      const settings = defaultStreamSettings();
      settings.bitrate = 20000;
      settings.packetSize = 1024;
      settings.fps = 60;
      settings.playAudioLocal = !audioEnabled;

      // Detect supported video formats
      const supportedFormats = await getSupportedVideoFormats();

      // Create Stream instance
      const stream = new Stream(
        api,
        hostId, // Wolf host ID
        appId, // App ID (Wolf lobby)
        settings,
        supportedFormats,
        [width, height]
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

  return (
    <Box
      ref={containerRef}
      className={className}
      sx={{
        position: 'relative',
        width: '100%',
        height: '100%',
        minHeight: 400,
        backgroundColor: '#000',
        display: 'flex',
        flexDirection: 'column',
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
