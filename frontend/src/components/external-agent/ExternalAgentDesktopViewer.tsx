import React, { FC, useState, useEffect, useCallback } from 'react';
import { Box, Button, Typography, CircularProgress } from '@mui/material';
import PlayArrow from '@mui/icons-material/PlayArrow';

import MoonlightStreamViewer from './MoonlightStreamViewer';
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
}

const ExternalAgentDesktopViewer: FC<ExternalAgentDesktopViewerProps> = ({
  sessionId,
  wolfLobbyId,
  height,
}) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const { isRunning, isPaused } = useWolfAppState(sessionId);
  const [isResuming, setIsResuming] = useState(false);

  const handleResume = async () => {
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
    return (
      <Box
        sx={{
          width: '100%',
          height: height,
          backgroundColor: '#1a1a1a',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 1,
          gap: 2,
        }}
      >
        <Typography variant="body1" sx={{ color: 'rgba(255,255,255,0.5)', fontWeight: 500 }}>
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
    );
  }

  // Floating window always shows live stream (no toggle)
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
      />
    </Box>
  );
};

export default ExternalAgentDesktopViewer;
