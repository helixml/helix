import React, { FC, useState, useEffect, useCallback } from 'react';
import { Box, Button, Typography, CircularProgress } from '@mui/material';
import PlayArrow from '@mui/icons-material/PlayArrow';

import MoonlightStreamViewer from './MoonlightStreamViewer';
import ScreenshotViewer from './ScreenshotViewer';
import useApi from '../../hooks/useApi';
import useSnackbar from '../../hooks/useSnackbar';

// Hook to track Wolf app state for external agent sessions
const useWolfAppState = (sessionId: string) => {
  const api = useApi();
  const [wolfState, setWolfState] = React.useState<string>('loading');

  React.useEffect(() => {
    const apiClient = api.getApiClient();
    const fetchState = async () => {
      try {
        const response = await apiClient.v1SessionsWolfAppStateDetail(sessionId);
        if (response.data) {
          const newState = response.data.state || 'absent';
          setWolfState(newState);
        }
      } catch (err) {
        console.error('Failed to fetch Wolf state:', err);
      }
    };

    fetchState();
    const interval = setInterval(fetchState, 3000); // Poll every 3 seconds

    return () => {
      clearInterval(interval);
    };
  }, [sessionId]); // Removed 'api' - getApiClient() is stable

  // Backend now returns 'starting' state for recently-created lobbies
  const isRunning = wolfState === 'running' || wolfState === 'resumable';
  const isStarting = wolfState === 'starting';
  // Show "paused" only if container was previously running but is now absent
  const isPaused = wolfState === 'absent';

  return { wolfState, isRunning, isPaused, isStarting };
};

interface ExternalAgentDesktopViewerProps {
  sessionId: string;
  wolfLobbyId?: string;
  height?: number; // Optional - required for screenshot mode, ignored for stream mode (uses flex)
  mode?: 'screenshot' | 'stream'; // Screenshot mode for Kanban cards, stream mode for floating window
  onClientIdCalculated?: (clientId: string) => void;
  // Display settings from app's ExternalAgentConfig
  displayWidth?: number;
  displayHeight?: number;
  displayFps?: number;
}

const ExternalAgentDesktopViewer: FC<ExternalAgentDesktopViewerProps> = ({
  sessionId,
  wolfLobbyId,
  height,
  mode = 'stream', // Default to stream for floating window
  onClientIdCalculated,
  displayWidth,
  displayHeight,
  displayFps,
}) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const { isRunning, isPaused, isStarting } = useWolfAppState(sessionId);
  const [isResuming, setIsResuming] = useState(false);
  // Track if we've ever been running - once running, keep stream mounted to avoid fullscreen exit
  const [hasEverBeenRunning, setHasEverBeenRunning] = useState(false);

  // Once running, remember it to prevent unmounting on transient state changes
  useEffect(() => {
    if (isRunning && !hasEverBeenRunning) {
      setHasEverBeenRunning(true);
    }
  }, [isRunning, hasEverBeenRunning]);

  const handleResume = async (e?: React.MouseEvent) => {
    e?.stopPropagation(); // Prevent click from bubbling to parent (e.g., Kanban card navigation)
    setIsResuming(true);
    try {
      await api.post(`/api/v1/sessions/${sessionId}/resume`);
      snackbar.success('External agent started successfully');
    } catch (error: any) {
      console.error('Failed to resume agent:', error);
      snackbar.error(error?.message || 'Failed to start agent');
    } finally {
      setIsResuming(false);
    }
  };

  // For screenshot mode (Kanban cards), use traditional early-return rendering
  // For stream mode (floating window), keep stream mounted to prevent fullscreen exit on hiccups

  // Screenshot mode: use traditional early-return rendering
  if (mode === 'screenshot') {
    // Starting state - show spinner
    if (isStarting) {
      return (
        <Box
          sx={{
            width: '100%',
            height: height,
            position: 'relative',
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 1,
            overflow: 'hidden',
            backgroundColor: '#1a1a1a',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
          }}
        >
          <CircularProgress size={32} sx={{ color: 'primary.main' }} />
          <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.7)', fontWeight: 500 }}>
            Starting Desktop...
          </Typography>
        </Box>
      );
    }

    if (isPaused) {
      const screenshotUrl = `/api/v1/external-agents/${sessionId}/screenshot?t=${Date.now()}`;
      return (
        <Box
          sx={{
            width: '100%',
            height: height,
            position: 'relative',
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 1,
            overflow: 'hidden',
            backgroundColor: '#1a1a1a',
          }}
        >
          <Box
            component="img"
            src={screenshotUrl}
            alt="Paused Desktop"
            sx={{
              width: '100%',
              height: '100%',
              objectFit: 'contain',
              filter: 'grayscale(0.5) brightness(0.7) blur(1px)',
              opacity: 0.6,
            }}
            onError={(e) => {
              e.currentTarget.style.display = 'none';
            }}
          />
          <Box
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 2,
              backgroundColor: 'rgba(0,0,0,0.3)',
            }}
          >
            <Typography variant="body1" sx={{ color: 'rgba(255,255,255,0.9)', fontWeight: 500 }}>
              Desktop Paused
            </Typography>
            <Button
              variant="contained"
              color="primary"
              size="large"
              startIcon={isResuming ? <CircularProgress size={20} /> : <PlayArrow />}
              onClick={handleResume}
              disabled={isResuming}
            >
              {isResuming ? 'Starting...' : 'Start Desktop'}
            </Button>
          </Box>
        </Box>
      );
    }

    return (
      <Box sx={{
        height: height,
        width: '100%',
        overflow: 'hidden'
      }}>
        <ScreenshotViewer
          sessionId={sessionId}
          autoRefresh={true}
          refreshInterval={3000}
          enableStreaming={false}
          showToolbar={false}
          showTimestamp={false}
          height={height}
        />
      </Box>
    );
  }

  // Stream mode (floating window) - KEEP STREAM MOUNTED to prevent fullscreen exit on hiccups
  // Once we've been running, show overlays instead of unmounting the stream viewer
  // Use flex: 1 to fill available space (no fixed height)

  // Starting state before we've ever been running - show spinner
  if (isStarting && !hasEverBeenRunning) {
    return (
      <Box
        sx={{
          width: '100%',
          flex: 1,
          minHeight: 0,
          position: 'relative',
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 1,
          overflow: 'hidden',
          backgroundColor: '#1a1a1a',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 2,
        }}
      >
        <CircularProgress size={32} sx={{ color: 'primary.main' }} />
        <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.7)', fontWeight: 500 }}>
          Starting Desktop...
        </Typography>
      </Box>
    );
  }

  // Paused state before we've ever been running - show paused UI
  if (isPaused && !hasEverBeenRunning) {
    const screenshotUrl = `/api/v1/external-agents/${sessionId}/screenshot?t=${Date.now()}`;
    return (
      <Box
        sx={{
          width: '100%',
          flex: 1,
          minHeight: 0,
          position: 'relative',
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 1,
          overflow: 'hidden',
          backgroundColor: '#1a1a1a',
        }}
      >
        <Box
          component="img"
          src={screenshotUrl}
          alt="Paused Desktop"
          sx={{
            width: '100%',
            height: '100%',
            objectFit: 'contain',
            filter: 'grayscale(0.5) brightness(0.7) blur(1px)',
            opacity: 0.6,
          }}
          onError={(e) => {
            e.currentTarget.style.display = 'none';
          }}
        />
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
            backgroundColor: 'rgba(0,0,0,0.3)',
          }}
        >
          <Typography variant="body1" sx={{ color: 'rgba(255,255,255,0.9)', fontWeight: 500 }}>
            Desktop Paused
          </Typography>
          <Button
            variant="contained"
            color="primary"
            size="large"
            startIcon={isResuming ? <CircularProgress size={20} /> : <PlayArrow />}
            onClick={handleResume}
            disabled={isResuming}
          >
            {isResuming ? 'Starting...' : 'Start Desktop'}
          </Button>
        </Box>
      </Box>
    );
  }

  // Once running (or has ever been running) - ALWAYS keep MoonlightStreamViewer mounted
  // Show overlays for state changes instead of unmounting (prevents fullscreen exit)
  const showReconnectingOverlay = !isRunning && hasEverBeenRunning;

  return (
    <Box sx={{
      flex: 1,
      minHeight: 0,
      width: '100%',
      overflow: 'hidden',
      position: 'relative',
    }}>
      <MoonlightStreamViewer
        sessionId={sessionId}
        wolfLobbyId={wolfLobbyId}
        width={displayWidth}
        height={displayHeight}
        fps={displayFps}
        onError={(error) => {
          console.error('Stream viewer error:', error);
        }}
        onClientIdCalculated={onClientIdCalculated}
      />

      {/* Reconnecting overlay - shown when state changes but stream stays mounted */}
      {showReconnectingOverlay && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
            backgroundColor: 'rgba(0,0,0,0.7)',
            zIndex: 100,
          }}
        >
          <CircularProgress size={40} sx={{ color: 'warning.main' }} />
          <Typography variant="body1" sx={{ color: 'rgba(255,255,255,0.9)', fontWeight: 500 }}>
            {isPaused ? 'Desktop Paused' : 'Reconnecting...'}
          </Typography>
          {isPaused && (
            <Button
              variant="contained"
              color="primary"
              size="large"
              startIcon={isResuming ? <CircularProgress size={20} /> : <PlayArrow />}
              onClick={handleResume}
              disabled={isResuming}
              sx={{ mt: 1 }}
            >
              {isResuming ? 'Starting...' : 'Restart Desktop'}
            </Button>
          )}
        </Box>
      )}
    </Box>
  );
};

export default ExternalAgentDesktopViewer;
