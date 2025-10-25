import React, { useRef, useEffect, useState } from 'react';
import { Box, Typography, Alert, CircularProgress } from '@mui/material';

interface MoonlightWebPlayerProps {
  sessionId: string;
  wolfLobbyId?: string;
  isPersonalDevEnvironment?: boolean;
  onConnectionChange?: (isConnected: boolean) => void;
  onError?: (error: string) => void;
  width?: number;
  height?: number;
  className?: string;
}

/**
 * MoonlightWebPlayer - Iframe wrapper for moonlight-web-stream
 *
 * This component renders an iframe pointing to the moonlight-web-stream service
 * which provides complete browser-based Moonlight streaming with WebRTC.
 *
 * moonlight-web-stream is a battle-tested Rust implementation that:
 * - Acts as Moonlight client connecting to Wolf
 * - Bridges to browser via WebRTC (universal browser support)
 * - Handles video (H.264/H.265/AV1), audio (Opus), and all input devices
 * - Manages NAT traversal via STUN/TURN servers
 *
 * Architecture:
 * Browser (iframe) → moonlight-web service (Rust) → Wolf (Moonlight protocol)
 */
const MoonlightWebPlayer: React.FC<MoonlightWebPlayerProps> = ({
  sessionId,
  wolfLobbyId,
  isPersonalDevEnvironment = false,
  onConnectionChange,
  onError,
  className = '',
}) => {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Construct moonlight-web stream URL
  // In lobbies mode (wolfLobbyId present): Connect to Wolf UI (appId=0) to browse lobbies
  // In apps mode (no wolfLobbyId): Connect directly to specific app
  // hostId=0 refers to the Wolf server configured in moonlight-web
  const streamUrl = wolfLobbyId
    ? `/moonlight/stream.html?hostId=0&appId=0` // Lobbies mode: connect to Wolf UI browser
    : `/moonlight/stream.html?hostId=0&appId=1`; // Apps mode: connect to specific app

  useEffect(() => {
    // Handle iframe load
    const handleLoad = () => {
      setIsLoading(false);
      onConnectionChange?.(true);
    };

    // Handle iframe errors
    const handleError = () => {
      const errorMsg = 'Failed to load moonlight-web stream';
      setError(errorMsg);
      setIsLoading(false);
      onError?.(errorMsg);
    };

    const iframe = iframeRef.current;
    if (iframe) {
      iframe.addEventListener('load', handleLoad);
      iframe.addEventListener('error', handleError);

      return () => {
        iframe.removeEventListener('load', handleLoad);
        iframe.removeEventListener('error', handleError);
      };
    }
  }, [onConnectionChange, onError]);

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
      {/* Loading State */}
      {isLoading && !error && (
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
          <CircularProgress size={40} sx={{ mb: 2, color: 'white' }} />
          <Typography variant="body1" sx={{ color: 'white' }}>
            Loading Moonlight stream...
          </Typography>
        </Box>
      )}

      {/* Error Display */}
      {error && (
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
          <Alert severity="error" sx={{ maxWidth: 400 }}>
            {error}
          </Alert>
        </Box>
      )}

      {/* Moonlight Web Stream Iframe */}
      <iframe
        ref={iframeRef}
        src={streamUrl}
        style={{
          width: '100%',
          height: '100%',
          border: 'none',
          backgroundColor: '#000',
        }}
        title="Moonlight Web Stream"
        allow="autoplay; fullscreen; gamepad; clipboard-read; clipboard-write"
        sandbox="allow-same-origin allow-scripts allow-forms allow-modals allow-pointer-lock"
      />

      {/* Usage Hint */}
      {!isLoading && !error && (
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
          💡 Powered by moonlight-web-stream | Fullscreen with F11 | Gamepad support enabled
        </Typography>
      )}
    </Box>
  );
};

export default MoonlightWebPlayer;
