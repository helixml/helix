import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton } from '@mui/material';
import { Fullscreen, FullscreenExit, ContentCopy, Refresh } from '@mui/icons-material';
import { useRDPConnection } from '../../hooks/useRDPConnection';



interface GuacamoleIframeClientProps {
  sessionId: string;
  isRunner?: boolean; // true for fleet page runners, false for sessions
  onConnectionChange?: (isConnected: boolean) => void;
  onError?: (error: string) => void;
  width?: number;
  height?: number;
  className?: string;
}

interface GuacamoleMessage {
  type: string;
  data: any;
}

const GuacamoleIframeClient: React.FC<GuacamoleIframeClientProps> = ({
  sessionId,
  isRunner = false,
  onConnectionChange,
  onError,
  width = 1024,
  height = 768,
  className = '',
}) => {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  
  const [isReady, setIsReady] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState('Initializing...');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [clipboardText, setClipboardText] = useState('');

  // Use the RDP connection hook
  const { 
    connectionInfo, 
    isLoading: isLoadingConnection,
    error: connectionError,
    fetchConnectionInfo,
    clearError: clearConnectionError
  } = useRDPConnection();

  // Send message to iframe
  const sendMessage = useCallback((type: string, data?: any) => {
    if (iframeRef.current?.contentWindow) {
      iframeRef.current.contentWindow.postMessage({ type, data }, '*');
    }
  }, []);

  // Handle messages from iframe
  const handleMessage = useCallback((event: MessageEvent<GuacamoleMessage>) => {
    if (event.source !== iframeRef.current?.contentWindow) return;

    // Filter out noise messages from extensions/other sources
    if (!event.data || typeof event.data !== 'object') return;
    if (!event.data.type) return;
    if (event.data.msgId || event.data.source) return; // Extension messages

    const { type, data } = event.data;

    switch (type) {
      case 'ready':
        setIsReady(true);
        setStatus('Ready to connect');
        break;

      case 'status':
        setStatus(data.message);
        if (data.type === 'error') {
          setError(data.message);
          setIsConnecting(false);
          setIsConnected(false);
          onError?.(data.message);
        }
        break;

      case 'connected':
        setIsConnected(data);
        setIsConnecting(false);
        setError(null);
        onConnectionChange?.(data);
        if (data) {
          setStatus('Connected successfully');
        }
        break;

      case 'error':
        setError(data);
        setIsConnecting(false);
        setIsConnected(false);
        onError?.(data);
        break;

      case 'clipboard':
        setClipboardText(data);
        // Try to update browser clipboard
        if (navigator.clipboard) {
          navigator.clipboard.writeText(data).catch(err => 
            console.warn('Failed to write to clipboard:', err)
          );
        }
        break;

      default:
        // Silently ignore unknown message types to reduce console noise
        break;
    }
  }, [onConnectionChange, onError]);

  // Connect to RDP
  const connect = useCallback(async () => {
    if (!isReady) return;

    setIsConnecting(true);
    setError(null);
    clearConnectionError();

    try {
      // Fetch connection info from API
      const connInfo = await fetchConnectionInfo(sessionId, isRunner);
      
      if (!connInfo) {
        throw new Error('Failed to fetch RDP connection information');
      }

      const config = {
        sessionId,
        hostname: connInfo.host,
        port: connInfo.rdp_port,
        username: connInfo.username,
        password: connInfo.rdp_password,
        width,
        height,
        audioEnabled: true,
        wsUrl: connInfo.proxy_url || (isRunner 
          ? `/api/v1/external-agents/runners/${sessionId}/rdp/proxy`
          : `/api/v1/external-agents/${sessionId}/rdp/proxy`)
      };

      sendMessage('connect', config);
    } catch (err: any) {
      const errorMsg = err.message || 'Failed to connect to RDP';
      setError(errorMsg);
      onError?.(errorMsg);
      setIsConnecting(false);
    }
  }, [isReady, sessionId, isRunner, fetchConnectionInfo, clearConnectionError, width, height, sendMessage, onError]);

  // Disconnect
  const disconnect = useCallback(() => {
    sendMessage('disconnect');
    setIsConnected(false);
    setIsConnecting(false);
  }, [sendMessage]);

  // Reconnect
  const reconnect = useCallback(() => {
    disconnect();
    setTimeout(connect, 1000);
  }, [disconnect, connect]);

  // Send clipboard content to remote
  const sendClipboard = useCallback(async () => {
    if (!isConnected || !navigator.clipboard) return;

    try {
      const text = await navigator.clipboard.readText();
      sendMessage('sendClipboard', text);
    } catch (err) {
      console.warn('Failed to read clipboard:', err);
    }
  }, [isConnected, sendMessage]);

  // Toggle fullscreen
  const toggleFullscreen = useCallback(() => {
    if (!containerRef.current) return;

    if (!isFullscreen) {
      containerRef.current.requestFullscreen?.();
    } else {
      document.exitFullscreen?.();
    }
  }, [isFullscreen]);

  // Setup message listener
  useEffect(() => {
    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, [handleMessage]);

  // Handle fullscreen events
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange);
  }, []);

  // Auto-connect when ready
  useEffect(() => {
    if (isReady && !isConnecting && !isConnected && !isLoadingConnection) {
      connect();
    }
  }, [isReady, isConnecting, isConnected, isLoadingConnection, connect]);

  // Handle connection errors
  useEffect(() => {
    if (connectionError) {
      setError(connectionError);
      onError?.(connectionError);
    }
  }, [connectionError, onError]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      disconnect();
    };
  }, [disconnect]);

  // Handle keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isConnected) return;

      // Ctrl+V - paste clipboard
      if (e.ctrlKey && e.key === 'v') {
        e.preventDefault();
        sendClipboard();
      }

      // Ctrl+Alt+Del equivalent (Ctrl+Alt+End)
      if (e.ctrlKey && e.altKey && e.key === 'End') {
        e.preventDefault();
        sendMessage('sendKeys', {
          keys: [
            { keysym: 0xFFE3, pressed: true },  // Ctrl down
            { keysym: 0xFFE9, pressed: true },  // Alt down
            { keysym: 0xFFFF, pressed: true },  // Del down
            { keysym: 0xFFFF, pressed: false }, // Del up
            { keysym: 0xFFE9, pressed: false }, // Alt up
            { keysym: 0xFFE3, pressed: false }  // Ctrl up
          ]
        });
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isConnected, sendClipboard, sendMessage]);

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
        flexDirection: 'column'
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
          opacity: isConnected ? 1 : 0,
          transition: 'opacity 0.3s'
        }}
      >
        <IconButton 
          size="small" 
          onClick={sendClipboard}
          sx={{ color: 'white' }}
          title="Paste Clipboard (Ctrl+V)"
          disabled={!isConnected}
        >
          <ContentCopy fontSize="small" />
        </IconButton>
        <IconButton 
          size="small" 
          onClick={reconnect}
          sx={{ color: 'white' }}
          title="Reconnect"
          disabled={isConnecting}
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

      {/* Status Overlay */}
      {(isConnecting || isLoadingConnection || error) && (
        <Box 
          sx={{ 
            position: 'absolute', 
            top: '50%', 
            left: '50%', 
            transform: 'translate(-50%, -50%)',
            zIndex: 999,
            textAlign: 'center'
          }}
        >
          {(isConnecting || isLoadingConnection) && (
            <Box sx={{ color: 'white' }}>
              <CircularProgress size={40} sx={{ mb: 2 }} />
              <Typography variant="body1">
                {isLoadingConnection ? 'Fetching connection details...' : status}
              </Typography>
            </Box>
          )}
          
          {error && (
            <Alert severity="error" sx={{ maxWidth: 400 }}>
              {error}
            </Alert>
          )}
        </Box>
      )}

      {/* Guacamole RDP Client Iframe */}
      <iframe
        ref={iframeRef}
        src="/rdp-client.html"
        style={{
          width: '100%',
          height: '100%',
          border: 'none',
          backgroundColor: '#000'
        }}
        title="Guacamole RDP Client"
        allow="clipboard-read; clipboard-write; fullscreen"
        sandbox="allow-same-origin allow-scripts allow-forms"
      />

      {/* Connection Info */}
      {isConnected && connectionInfo && (
        <Box 
          sx={{ 
            position: 'absolute', 
            bottom: 8, 
            left: 8, 
            backgroundColor: 'rgba(0,0,0,0.7)',
            color: 'white',
            padding: 1,
            borderRadius: 1,
            fontSize: '0.75rem',
            opacity: 0.8
          }}
        >
          <Typography variant="caption" component="div">
            <strong>Connected:</strong> {connectionInfo.host}:{connectionInfo.rdp_port}
          </Typography>
          <Typography variant="caption" component="div">
            <strong>User:</strong> {connectionInfo.username}
          </Typography>
          <Typography variant="caption" component="div">
            <strong>Type:</strong> {isRunner ? 'Agent Runner' : 'Session'} RDP
          </Typography>
          <Typography variant="caption" component="div">
            <strong>Protocol:</strong> RDP via Apache Guacamole
          </Typography>
        </Box>
      )}

      {/* Keyboard Shortcuts Help */}
      <Box 
        sx={{ 
          position: 'absolute', 
          bottom: 8, 
          right: 8, 
          backgroundColor: 'rgba(0,0,0,0.7)',
          color: 'white',
          padding: 1,
          borderRadius: 1,
          fontSize: '0.7rem',
          opacity: isConnected ? 0.6 : 0,
          transition: 'opacity 0.3s',
          maxWidth: 200
        }}
      >
        <Typography variant="caption" component="div">
          <strong>Shortcuts:</strong>
        </Typography>
        <Typography variant="caption" component="div">
          Ctrl+V: Paste
        </Typography>
        <Typography variant="caption" component="div">
          Ctrl+Alt+End: Ctrl+Alt+Del
        </Typography>
      </Box>
    </Box>
  );
};

export default GuacamoleIframeClient;