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
          setWolfState(response.data.state || 'absent');
        }
      } catch (err) {
        console.error('Failed to fetch Wolf state:', err);
      }
    };

    fetchState();
    const interval = setInterval(fetchState, 3000); // Poll every 3 seconds
    return () => clearInterval(interval);
  }, [sessionId]); // Removed 'api' - getApiClient() is stable

  const isRunning = wolfState === 'running' || wolfState === 'resumable';
  const isPaused = wolfState === 'absent' || (!isRunning && wolfState !== 'loading');

  return { wolfState, isRunning, isPaused };
};

interface ExternalAgentDesktopViewerProps {
  sessionId: string;
  wolfLobbyId?: string;
  height: number;
  mode?: 'screenshot' | 'stream'; // Screenshot mode for Kanban cards, stream mode for floating window
  onClientIdCalculated?: (clientId: string) => void;
}

const ExternalAgentDesktopViewer: FC<ExternalAgentDesktopViewerProps> = ({
  sessionId,
  wolfLobbyId,
  height,
  mode = 'stream', // Default to stream for floating window
  onClientIdCalculated,
}) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const { isRunning, isPaused } = useWolfAppState(sessionId);
  const [isResuming, setIsResuming] = useState(false);

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

  if (isPaused) {
    // Paused state - show saved screenshot with gray-out effect + Start button overlay
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
        {/* Paused screenshot with gray-out effect */}
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
            // If screenshot fails to load, hide the image
            e.currentTarget.style.display = 'none';
          }}
        />
        {/* Overlay with Start button */}
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

  // Render based on mode: screenshot for Kanban cards, stream for floating window
  if (mode === 'screenshot') {
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
          height={height}
        />
      </Box>
    );
  }

  // Stream mode (default for floating window)
  return (
    <Box sx={{
      height: height,
      width: '100%',
      overflow: 'hidden'
    }}>
      <MoonlightStreamViewer
        sessionId={sessionId}
        wolfLobbyId={wolfLobbyId || sessionId}
        onError={(error) => {
          console.error('Stream viewer error:', error);
        }}
        onClientIdCalculated={onClientIdCalculated}
      />
    </Box>
  );
};

export default ExternalAgentDesktopViewer;
