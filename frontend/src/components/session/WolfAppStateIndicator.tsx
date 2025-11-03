import React, { useEffect, useState, useCallback } from 'react';
import { Box, Chip, CircularProgress, Tooltip } from '@mui/material';
import { Computer, PlayArrow, Pause, Info } from '@mui/icons-material';
import useApi from '../../hooks/useApi';

interface WolfAppStateIndicatorProps {
  sessionId: string;
  refreshInterval?: number; // milliseconds
}

const WolfAppStateIndicator: React.FC<WolfAppStateIndicatorProps> = ({
  sessionId,
  refreshInterval = 2000, // Default: poll every 2 seconds
}) => {
  const api = useApi();
  const [state, setState] = useState<string>('loading');
  const [hasWebsocket, setHasWebsocket] = useState<boolean>(false);
  const [wolfAppId, setWolfAppId] = useState<string>('');
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  // Fetch Wolf app state - memoized to prevent recreating on every render
  const fetchState = useCallback(async () => {
    try {
      const response = await api.getApiClient().v1SessionsWolfAppStateDetail(sessionId);
      if (response.data) {
        setState(response.data.state || 'absent');
        setHasWebsocket(response.data.has_websocket || false);
        setWolfAppId(response.data.wolf_app_id || '');
        setError(null);
      }
      setLoading(false);
    } catch (err: any) {
      console.error('Failed to fetch Wolf app state:', err);
      setError(err.message || 'Failed to fetch state');
      setLoading(false);
    }
  }, [sessionId]); // Only sessionId needed - getApiClient() is stable

  // Initial fetch and polling interval combined
  useEffect(() => {
    fetchState(); // Initial fetch
    const interval = setInterval(fetchState, refreshInterval);
    return () => clearInterval(interval);
  }, [sessionId, refreshInterval, fetchState]);

  if (loading) {
    return (
      <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 1 }}>
        <CircularProgress size={16} />
        <span>Loading state...</span>
      </Box>
    );
  }

  if (error) {
    return (
      <Tooltip title={error}>
        <Chip
          icon={<Info />}
          label="Error"
          color="error"
          size="small"
          variant="outlined"
        />
      </Tooltip>
    );
  }

  // State-based rendering
  const getStateConfig = () => {
    switch (state) {
      case 'absent':
        return {
          label: 'Not Started',
          color: 'default' as const,
          icon: <Pause />,
          tooltip: 'Wolf app has not been created yet',
        };
      case 'resumable':
        return {
          label: hasWebsocket ? 'Streaming' : 'Ready',
          color: 'success' as const,
          icon: hasWebsocket ? <PlayArrow /> : <Computer />,
          tooltip: hasWebsocket
            ? 'Container running and browser connected'
            : 'Container running, ready for browser connection',
        };
      case 'running':
        return {
          label: 'Streaming',
          color: 'success' as const,
          icon: <PlayArrow />,
          tooltip: 'Container running with active browser connection',
        };
      default:
        return {
          label: state,
          color: 'default' as const,
          icon: <Info />,
          tooltip: `Unknown state: ${state}`,
        };
    }
  };

  const config = getStateConfig();

  return (
    <Tooltip title={`${config.tooltip}${wolfAppId ? ` (App ID: ${wolfAppId})` : ''}`}>
      <Chip
        icon={config.icon}
        label={config.label}
        color={config.color}
        size="small"
        variant="outlined"
      />
    </Tooltip>
  );
};

export default WolfAppStateIndicator;
