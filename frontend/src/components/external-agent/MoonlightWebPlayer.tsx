import React, { useRef, useEffect, useState } from 'react';
import { Box, Typography, Alert, CircularProgress } from '@mui/material';
import useApi from '../../hooks/useApi';

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
  const [streamUrl, setStreamUrl] = useState<string>('');
  const api = useApi();
  const apiClient = api.getApiClient();

  // Fetch Wolf UI app ID for lobbies mode
  useEffect(() => {
    const fetchAppId = async () => {
      console.log('[AUTO-JOIN DEBUG] MoonlightWebPlayer URL construction:', {
        sessionId,
        wolfLobbyId,
        isPersonalDevEnvironment,
        mode: wolfLobbyId ? 'LOBBIES' : 'APPS'
      });

      if (wolfLobbyId) {
        // Lobbies mode: Fetch Wolf UI app ID dynamically from Wolf
        // Pass session_id to identify which Wolf instance to query
        try {
          const response = await fetch(`/api/v1/wolf/ui-app-id?session_id=${encodeURIComponent(sessionId)}`);
          if (response.ok) {
            const data = await response.json();
            const wolfUIAppID = data.wolf_ui_app_id;

            // TODO: Auto-join implementation - pass lobby context to moonlight-web
            // Current: Only passes hostId and appId (Wolf UI browser)
            // Needed: lobbyId and lobbyPin for auto-joining
            // See: design/2025-10-30-lobby-auto-join-investigation.md
            const url = `/moonlight/stream.html?hostId=0&appId=${wolfUIAppID}`;
            setStreamUrl(url);

            console.log('[AUTO-JOIN DEBUG] Constructed lobbies mode URL:', {
              wolfUIAppID,
              wolfLobbyId,
              url,
              note: 'AUTO-JOIN NOT IMPLEMENTED - lobby context not passed to moonlight-web'
            });
          } else {
            console.warn('MoonlightWebPlayer: Failed to fetch Wolf UI app ID, using default 0');
            setStreamUrl(`/moonlight/stream.html?hostId=0&appId=0`);
          }
        } catch (err) {
          console.warn('MoonlightWebPlayer: Failed to fetch Wolf UI app ID, using default 0:', err);
          setStreamUrl(`/moonlight/stream.html?hostId=0&appId=0`);
        }
      } else {
        // Apps mode: connect directly to specific app
        console.log('[AUTO-JOIN DEBUG] Apps mode - direct connection to app 1');
        setStreamUrl(`/moonlight/stream.html?hostId=0&appId=1`);
      }
    };

    fetchAppId();
  }, [sessionId, wolfLobbyId]);

  useEffect(() => {
    // Handle iframe load
    const handleLoad = async () => {
      setIsLoading(false);
      onConnectionChange?.(true);

      // Auto-join lobby if in lobbies mode (after connection established)
      if (wolfLobbyId && sessionId) {
        console.log('[AUTO-JOIN] Connection established, triggering auto-join for lobby:', wolfLobbyId);

        try {
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
      }
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
      {streamUrl && (
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
      )}

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
