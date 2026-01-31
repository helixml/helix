/**
 * ConnectionOverlay - Unified connection status overlay for streaming
 */

import React from 'react';
import { Box, Typography, Button, Alert, CircularProgress } from '@mui/material';
import { Refresh } from '@mui/icons-material';

export interface ConnectionOverlayProps {
  isConnected: boolean;
  isConnecting: boolean;
  error: string | null;
  status: string;
  retryCountdown: number | null;
  retryAttemptDisplay: number;
  reconnectClicked: boolean;
  onReconnect: () => void;
  onClearError: () => void;
}

const ConnectionOverlay: React.FC<ConnectionOverlayProps> = ({
  isConnected,
  isConnecting,
  error,
  status,
  retryCountdown,
  retryAttemptDisplay,
  reconnectClicked,
  onReconnect,
  onClearError,
}) => {
  // Don't render if we're connected and there's no pending state
  if (isConnected && !isConnecting && !error && retryCountdown === null) {
    return null;
  }

  return (
    <Box
      sx={{
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.85)',
        zIndex: 1000,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        textAlign: 'center',
        gap: 2,
      }}
    >
      {/* Connecting state - spinner with status message */}
      {isConnecting && (
        <Box sx={{ color: 'white' }}>
          <CircularProgress size={40} sx={{ mb: 2 }} />
          <Typography variant="body1">{status}</Typography>
        </Box>
      )}

      {/* Retry countdown - waiting before retry */}
      {retryCountdown !== null && !isConnecting && (
        <Alert severity="warning" sx={{ maxWidth: 400 }}>
          Stream busy (attempt {retryAttemptDisplay}) - retrying in {retryCountdown} second{retryCountdown !== 1 ? 's' : ''}...
        </Alert>
      )}

      {/* Disconnected state - no active connection, no error, not connecting */}
      {!isConnecting && !isConnected && !error && retryCountdown === null && (
        <>
          <Typography variant="h6" sx={{ color: 'white' }}>
            Disconnected
          </Typography>
          <Typography variant="body2" sx={{ color: 'grey.400', textAlign: 'center', maxWidth: 300 }}>
            {status || 'Connection lost'}
          </Typography>
          <Button
            variant="contained"
            color="primary"
            onClick={onReconnect}
            disabled={reconnectClicked}
            startIcon={reconnectClicked ? <CircularProgress size={20} /> : <Refresh />}
            sx={{ mt: 2 }}
          >
            {reconnectClicked ? 'Reconnecting...' : 'Reconnect Now'}
          </Button>
        </>
      )}

      {/* Error state - show error with reconnect option */}
      {error && retryCountdown === null && !isConnecting && (
        <Alert
          severity="error"
          sx={{ maxWidth: 400 }}
          action={
            <Button
              color="inherit"
              size="small"
              disabled={reconnectClicked}
              onClick={() => {
                onClearError();
                onReconnect();
              }}
              startIcon={reconnectClicked ? <CircularProgress size={14} color="inherit" /> : undefined}
            >
              {reconnectClicked ? 'Reconnecting...' : 'Reconnect'}
            </Button>
          }
        >
          {error}
        </Alert>
      )}
    </Box>
  );
};

export default ConnectionOverlay;
