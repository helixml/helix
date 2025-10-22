import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Box, Typography, Alert, IconButton, Button, Paper, Chip, ToggleButtonGroup, ToggleButton } from '@mui/material';
import { Refresh, OpenInNew, Fullscreen, FullscreenExit, Videocam, CameraAlt } from '@mui/icons-material';
import MoonlightStreamViewer from './MoonlightStreamViewer';

interface ScreenshotViewerProps {
  sessionId: string;
  isRunner?: boolean;
  isPersonalDevEnvironment?: boolean;
  wolfLobbyId?: string; // For Moonlight streaming mode
  onError?: (error: string) => void;
  width?: number;
  height?: number;
  className?: string;
  autoRefresh?: boolean;
  refreshInterval?: number; // in milliseconds
  enableStreaming?: boolean; // Enable streaming mode toggle
}

const ScreenshotViewer: React.FC<ScreenshotViewerProps> = ({
  sessionId,
  isRunner = false,
  isPersonalDevEnvironment = false,
  wolfLobbyId,
  onError,
  width = 3840,
  height = 2160,
  className = '',
  autoRefresh = true,
  refreshInterval = 1000, // Default 1 second
  enableStreaming = true, // Enable streaming by default
}) => {
  const [screenshotUrl, setScreenshotUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [streamingMode, setStreamingMode] = useState<'screenshot' | 'stream'>('screenshot');
  const containerRef = React.useRef<HTMLDivElement>(null);
  const mountTimeRef = React.useRef<Date>(new Date());
  const [isInitialLoading, setIsInitialLoading] = useState(true);

  // Construct screenshot endpoint
  const getScreenshotEndpoint = useCallback(() => {
    if (isPersonalDevEnvironment) {
      return `/api/v1/personal-dev-environments/${sessionId}/screenshot`;
    } else if (isRunner) {
      return `/api/v1/external-agents/runners/${sessionId}/screenshot`;
    } else {
      return `/api/v1/external-agents/${sessionId}/screenshot`;
    }
  }, [sessionId, isRunner, isPersonalDevEnvironment]);

  // Fetch screenshot (useRef to prevent recreation on every render)
  const fetchScreenshotRef = useRef<() => Promise<void>>();

  fetchScreenshotRef.current = async () => {
    const endpoint = getScreenshotEndpoint();

    try {
      const response = await fetch(endpoint, {
        method: 'GET',
        headers: {
          'Content-Type': 'image/png',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch screenshot: ${response.status} ${response.statusText}`);
      }

      const blob = await response.blob();
      const url = URL.createObjectURL(blob);

      // Revoke old URL to prevent memory leaks
      if (screenshotUrl) {
        URL.revokeObjectURL(screenshotUrl);
      }

      setScreenshotUrl(url);
      setLastRefresh(new Date());
      setIsLoading(false);
      setIsInitialLoading(false); // First successful fetch ends initial loading
      setError(null);
    } catch (err: any) {
      const errorMsg = err.message || 'Failed to fetch screenshot';

      // During initial loading (first 60 seconds), suppress errors
      // Container takes time to start and screenshot server to initialize
      if (isInitialLoading) {
        // Keep loading state, don't show error yet
        setIsLoading(true);
      } else {
        // After grace period, show actual errors
        setError(errorMsg);
        onError?.(errorMsg);
        setIsLoading(false);
      }
    }
  };

  // Stable wrapper for calling the ref
  const fetchScreenshot = useCallback(() => {
    fetchScreenshotRef.current?.();
  }, []);

  // Auto-refresh screenshot with RAF for higher priority
  useEffect(() => {
    if (!autoRefresh || streamingMode !== 'screenshot') return;

    let timeoutId: NodeJS.Timeout;
    let rafId: number;

    const refresh = () => {
      rafId = requestAnimationFrame(() => {
        fetchScreenshotRef.current?.();
        timeoutId = setTimeout(refresh, refreshInterval);
      });
    };

    // Start the refresh cycle
    timeoutId = setTimeout(refresh, refreshInterval);

    return () => {
      clearTimeout(timeoutId);
      if (rafId) cancelAnimationFrame(rafId);
    };
  }, [autoRefresh, refreshInterval, streamingMode]);

  // Initial fetch
  useEffect(() => {
    fetchScreenshot();
  }, [sessionId]);

  // End initial loading grace period after 60 seconds
  useEffect(() => {
    const timer = setTimeout(() => {
      setIsInitialLoading(false);
    }, 60000); // 60 seconds grace period for container startup

    return () => clearTimeout(timer);
  }, []);

  // Cleanup screenshot URL on unmount
  useEffect(() => {
    return () => {
      if (screenshotUrl) {
        URL.revokeObjectURL(screenshotUrl);
      }
    };
  }, [screenshotUrl]);

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

  // If in streaming mode, render MoonlightWebPlayer instead
  if (streamingMode === 'stream' && enableStreaming && wolfLobbyId) {
    return (
      <Box
        ref={containerRef}
        className={className}
        sx={{
          position: 'relative',
          width: '100%',
          height: '100%',
          backgroundColor: '#000',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        {/* Mode Toggle */}
        <Box
          sx={{
            position: 'absolute',
            top: 8,
            left: '50%',
            transform: 'translateX(-50%)',
            zIndex: 1001,
            backgroundColor: 'rgba(0,0,0,0.8)',
            borderRadius: 1,
          }}
        >
          <ToggleButtonGroup
            value={streamingMode}
            exclusive
            onChange={(_, value) => value && setStreamingMode(value)}
            size="small"
          >
            <ToggleButton value="screenshot" sx={{ color: 'white' }}>
              <CameraAlt fontSize="small" sx={{ mr: 0.5 }} />
              Screenshot
            </ToggleButton>
            <ToggleButton value="stream" sx={{ color: 'white' }}>
              <Videocam fontSize="small" sx={{ mr: 0.5 }} />
              Live Stream
            </ToggleButton>
          </ToggleButtonGroup>
        </Box>

        <MoonlightStreamViewer
          sessionId={sessionId}
          wolfLobbyId={wolfLobbyId}
          isPersonalDevEnvironment={isPersonalDevEnvironment}
          onError={onError}
          width={width}
          height={height}
        />
      </Box>
    );
  }

  return (
    <Box
      ref={containerRef}
      className={className}
      sx={{
        position: 'relative',
        width: '100%',
        height: '100%',
        backgroundColor: '#000',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {/* Mode Toggle */}
      {enableStreaming && (
        <Box
          sx={{
            position: 'absolute',
            top: 8,
            left: '50%',
            transform: 'translateX(-50%)',
            zIndex: 1001,
            backgroundColor: 'rgba(0,0,0,0.8)',
            borderRadius: 1,
          }}
        >
          <ToggleButtonGroup
            value={streamingMode}
            exclusive
            onChange={(_, value) => value && setStreamingMode(value)}
            size="small"
          >
            <ToggleButton value="screenshot" sx={{ color: 'white' }}>
              <CameraAlt fontSize="small" sx={{ mr: 0.5 }} />
              Screenshot
            </ToggleButton>
            <ToggleButton value="stream" sx={{ color: 'white' }}>
              <Videocam fontSize="small" sx={{ mr: 0.5 }} />
              Live Stream
            </ToggleButton>
          </ToggleButtonGroup>
        </Box>
      )}

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
        }}
      >
        <IconButton
          size="small"
          onClick={fetchScreenshot}
          sx={{ color: 'white' }}
          title="Refresh Screenshot"
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

      {/* Status Chip */}
      {lastRefresh && (
        <Typography
          variant="caption"
          sx={{
            position: 'absolute',
            bottom: 8,
            right: 8,
            zIndex: 1000,
            color: 'rgba(255,255,255,0.5)',
            fontSize: '0.65rem',
          }}
        >
          Last updated: {lastRefresh.toLocaleTimeString()}
        </Typography>
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

      {/* Screenshot Display */}
      {screenshotUrl && !error && (
        <img
          src={screenshotUrl}
          alt="Remote Desktop Screenshot"
          style={{
            width: '100%',
            height: '100%',
            objectFit: 'contain',
          }}
        />
      )}

      {/* Loading State */}
      {isLoading && !screenshotUrl && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            height: '100%',
            color: 'white',
          }}
        >
          <Typography variant="body1">Loading desktop...</Typography>
        </Box>
      )}

    </Box>
  );
};

export default ScreenshotViewer;
