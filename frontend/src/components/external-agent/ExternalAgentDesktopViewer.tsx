import React, { FC, useState, useEffect, useCallback } from 'react';
import { Box, Button, Typography, CircularProgress, IconButton, Tooltip, Collapse } from '@mui/material';
import PlayArrow from '@mui/icons-material/PlayArrow';
import ChatIcon from '@mui/icons-material/Chat';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';

import MoonlightStreamViewer from './MoonlightStreamViewer';
import ScreenshotViewer from './ScreenshotViewer';
import SandboxDropZone from './SandboxDropZone';
import EmbeddedSessionView from '../session/EmbeddedSessionView';
import RobustPromptInput from '../common/RobustPromptInput';
import useApi from '../../hooks/useApi';
import useSnackbar from '../../hooks/useSnackbar';
import { useStreaming } from '../../contexts/streaming';
import { SESSION_TYPE_TEXT } from '../../types';
import { Api } from '../../api/api';

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
  // Session panel settings (for stream mode only)
  showSessionPanel?: boolean; // Enable the collapsible session panel feature
  specTaskId?: string; // For prompt history sync
  projectId?: string; // For prompt history sync
  apiClient?: Api<unknown>['api']; // For prompt history sync
  defaultPanelOpen?: boolean; // Default state of the session panel (default: false)
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
  showSessionPanel = false,
  specTaskId,
  projectId,
  apiClient,
  defaultPanelOpen = false,
}) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const streaming = useStreaming();
  const { isRunning, isPaused, isStarting } = useWolfAppState(sessionId);
  const [isResuming, setIsResuming] = useState(false);
  // Track if we've ever been running - once running, keep stream mounted to avoid fullscreen exit
  const [hasEverBeenRunning, setHasEverBeenRunning] = useState(false);
  // Session panel state
  const [sessionPanelOpen, setSessionPanelOpen] = useState(defaultPanelOpen);

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

  // Handler for sending messages from the session panel
  // IMPORTANT: This hook must be before any early returns to satisfy React's rules of hooks
  const handleSendMessage = useCallback(async (message: string, interrupt?: boolean) => {
    await streaming.NewInference({
      type: SESSION_TYPE_TEXT,
      message,
      sessionId,
      interrupt: interrupt ?? true,
    });
  }, [streaming, sessionId]);

  // Session panel width
  const SESSION_PANEL_WIDTH = 400;

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
          refreshInterval={1500}
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
    <Box sx={{ display: 'flex', flex: 1, minHeight: 0, width: '100%', position: 'relative' }}>
      {/* Main desktop viewer */}
      <SandboxDropZone sessionId={sessionId} disabled={!isRunning}>
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
            // Suppress MoonlightStreamViewer's overlay when we're showing our own reconnecting overlay
            // This prevents double spinners when Wolf container state changes
            suppressOverlay={showReconnectingOverlay}
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

          {/* Session panel toggle button - positioned on the right edge */}
          {showSessionPanel && (
            <Tooltip title={sessionPanelOpen ? 'Hide session panel' : 'Show session panel'}>
              <IconButton
                onClick={() => setSessionPanelOpen(!sessionPanelOpen)}
                sx={{
                  position: 'absolute',
                  right: sessionPanelOpen ? SESSION_PANEL_WIDTH + 8 : 8,
                  top: 8,
                  zIndex: 200,
                  bgcolor: 'background.paper',
                  border: '1px solid',
                  borderColor: 'divider',
                  boxShadow: 2,
                  transition: 'right 0.3s ease',
                  '&:hover': {
                    bgcolor: 'action.hover',
                  },
                }}
              >
                {sessionPanelOpen ? (
                  <ChevronRightIcon />
                ) : (
                  <ChatIcon />
                )}
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </SandboxDropZone>

      {/* Collapsible session panel */}
      {showSessionPanel && (
        <Collapse
          in={sessionPanelOpen}
          orientation="horizontal"
          sx={{
            flexShrink: 0,
            borderLeft: sessionPanelOpen ? '1px solid' : 'none',
            borderColor: 'divider',
          }}
        >
          <Box
            sx={{
              width: SESSION_PANEL_WIDTH,
              height: '100%',
              display: 'flex',
              flexDirection: 'column',
              bgcolor: 'background.paper',
            }}
          >
            {/* Session messages */}
            <Box sx={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
              <EmbeddedSessionView sessionId={sessionId} />
            </Box>

            {/* Prompt input */}
            <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider', flexShrink: 0 }}>
              <RobustPromptInput
                sessionId={sessionId}
                specTaskId={specTaskId}
                projectId={projectId}
                apiClient={apiClient}
                onSend={handleSendMessage}
                placeholder="Send message to agent..."
              />
            </Box>
          </Box>
        </Collapse>
      )}
    </Box>
  );
};

export default ExternalAgentDesktopViewer;
