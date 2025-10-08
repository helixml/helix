import React, { useState, useEffect, useCallback } from 'react';
import { Box, Typography, Alert, IconButton, Button, Paper, Chip } from '@mui/material';
import { Refresh, OpenInNew, Fullscreen, FullscreenExit } from '@mui/icons-material';

interface ScreenshotViewerProps {
  sessionId: string;
  isRunner?: boolean;
  isPersonalDevEnvironment?: boolean;
  onError?: (error: string) => void;
  width?: number;
  height?: number;
  className?: string;
  autoRefresh?: boolean;
  refreshInterval?: number; // in milliseconds
}

const ScreenshotViewer: React.FC<ScreenshotViewerProps> = ({
  sessionId,
  isRunner = false,
  isPersonalDevEnvironment = false,
  onError,
  width = 1024,
  height = 768,
  className = '',
  autoRefresh = true,
  refreshInterval = 2000, // Default 2 seconds
}) => {
  const [screenshotUrl, setScreenshotUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const containerRef = React.useRef<HTMLDivElement>(null);

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

  // Fetch screenshot
  const fetchScreenshot = useCallback(async () => {
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
      setError(null);
    } catch (err: any) {
      const errorMsg = err.message || 'Failed to fetch screenshot';
      setError(errorMsg);
      onError?.(errorMsg);
      setIsLoading(false);
    }
  }, [getScreenshotEndpoint, screenshotUrl, onError]);

  // Auto-refresh screenshot
  useEffect(() => {
    if (!autoRefresh) return;

    const interval = setInterval(() => {
      fetchScreenshot();
    }, refreshInterval);

    return () => clearInterval(interval);
  }, [autoRefresh, refreshInterval, fetchScreenshot]);

  // Initial fetch
  useEffect(() => {
    fetchScreenshot();
  }, [sessionId]);

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
          <Typography variant="body1">Loading screenshot...</Typography>
        </Box>
      )}

    </Box>
  );
};

export default ScreenshotViewer;
