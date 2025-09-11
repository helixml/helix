import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton } from '@mui/material';
import { Fullscreen, FullscreenExit, ContentCopy, Refresh, OpenInNew, Keyboard } from '@mui/icons-material';
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
  
  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState('Initializing...');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [clipboardText, setClipboardText] = useState('');
  const [guacamoleURL, setGuacamoleURL] = useState<string | null>(null);

  // Use the RDP connection hook
  const { 
    connectionInfo, 
    isLoading: isLoadingConnection,
    error: connectionError,
    fetchConnectionInfo,
    clearError: clearConnectionError
  } = useRDPConnection();

  // Handle messages from iframe (simplified for status updates only)
  const handleMessage = useCallback((event: MessageEvent<GuacamoleMessage>) => {
    console.log('GuacamoleIframeClient received message:', event.data);
    
    if (event.source !== iframeRef.current?.contentWindow) {
      return;
    }

    // Filter out noise messages from extensions/other sources
    if (!event.data || typeof event.data !== 'object' || !event.data.type) {
      return;
    }
    if (event.data.msgId || event.data.source) {
      return;
    }

    const { type, data } = event.data;
    console.log(`Processing iframe message: ${type}`, data);

    switch (type) {
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

  // Helper function to construct Guacamole direct URL
  const constructGuacamoleURL = (connectionId: string, dataSource: string = 'postgresql') => {
    // Format: "connectionId\0c\0dataSource"
    const identifier = btoa(`${connectionId}\0c\0${dataSource}`);
    return `http://localhost:8080/guacamole/#/client/${identifier}`;
  };

  // Connect to RDP using direct Guacamole URL
  const connect = useCallback(async () => {
    console.log('🔗 connect() called with:', { sessionId, isRunner });
    
    console.log('🔄 Setting connecting state and fetching Guacamole connection ID...');
    setIsConnecting(true);
    setError(null);
    clearConnectionError();

    try {
      // Get Guacamole connection ID from our API
      const endpoint = isRunner 
        ? `/api/v1/external-agents/runners/${sessionId}/guacamole-connection-id`
        : `/api/v1/sessions/${sessionId}/guacamole-connection-id`;
      
      console.log('📡 Fetching Guacamole connection ID from:', endpoint);
      
      const response = await fetch(endpoint, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
          // Add auth headers if needed
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to get Guacamole connection: ${response.status} ${response.statusText}`);
      }

      const data = await response.json();
      console.log('📡 Guacamole connection response:', data);
      
      if (!data.guacamole_connection_id) {
        throw new Error('No Guacamole connection ID returned from API');
      }

      // Construct the direct Guacamole URL
      const guacamoleURL = data.guacamole_url || constructGuacamoleURL(data.guacamole_connection_id);
      console.log('🎯 Using Guacamole URL:', guacamoleURL);

      // Load the direct Guacamole interface
      if (iframeRef.current) {
        iframeRef.current.src = guacamoleURL;
        console.log('✅ Iframe loaded with Guacamole direct URL');
        
        // Set connected state immediately for direct Guacamole
        setIsConnected(true);
        setIsConnecting(false);
        setStatus('Connected via Guacamole');
        onConnectionChange?.(true);
      }
    } catch (err: any) {
      const errorMsg = err.message || 'Failed to connect to Guacamole';
      console.error('❌ Guacamole connection failed:', errorMsg);
      setError(errorMsg);
      onError?.(errorMsg);
      setIsConnecting(false);
    }
  }, [sessionId, isRunner, clearConnectionError, onConnectionChange, onError]);

  // Disconnect
  const disconnect = useCallback(() => {
    // Reset iframe to blank page for direct Guacamole
    if (iframeRef.current) {
      iframeRef.current.src = 'about:blank';
    }
    setIsConnected(false);
    setIsConnecting(false);
    setStatus('Disconnected');
  }, []);

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
      // Send clipboard data directly to iframe if possible
      if (iframeRef.current?.contentWindow) {
        iframeRef.current.contentWindow.postMessage({ type: 'sendClipboard', data: text }, '*');
      }
    } catch (err) {
      console.warn('Failed to read clipboard:', err);
    }
  }, [isConnected]);

  // Toggle fullscreen
  const toggleFullscreen = useCallback(() => {
    if (!containerRef.current) return;

    if (!isFullscreen) {
      containerRef.current.requestFullscreen?.();
    } else {
      document.exitFullscreen?.();
    }
  }, [isFullscreen]);

  // Force focus iframe
  const focusIframe = useCallback(() => {
    if (iframeRef.current) {
      iframeRef.current.focus();
      try {
        iframeRef.current.contentWindow?.focus();
      } catch (e) {
        console.log('Could not focus iframe content window (cross-origin)');
      }
    }
  }, []);

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

  // Auto-connect when component mounts (no longer dependent on isReady from iframe)
  useEffect(() => {
    console.log('🔄 Auto-connect effect triggered:', {
      isConnecting,
      isConnected,
      isLoadingConnection,
      sessionId,
      isRunner,
      timestamp: new Date().toISOString()
    });
    
    if (!isConnecting && !isConnected && !isLoadingConnection) {
      console.log('✅ Conditions met - Auto-connecting...');
      connect();
    } else {
      console.log('❌ Auto-connect blocked:', {
        alreadyConnecting: isConnecting,
        alreadyConnected: isConnected,
        stillLoading: isLoadingConnection,
        canConnect: !isConnecting && !isConnected && !isLoadingConnection
      });
    }
  }, [isConnecting, isConnected, isLoadingConnection, connect]);

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
        // Send key combination directly to iframe if possible
        if (iframeRef.current?.contentWindow) {
          iframeRef.current.contentWindow.postMessage({ 
            type: 'sendKeys', 
            data: {
              keys: [
                { keysym: 0xFFE3, pressed: true },  // Ctrl down
                { keysym: 0xFFE9, pressed: true },  // Alt down
                { keysym: 0xFFFF, pressed: true },  // Del down
                { keysym: 0xFFFF, pressed: false }, // Del up
                { keysym: 0xFFE9, pressed: false }, // Alt up
                { keysym: 0xFFE3, pressed: false }  // Ctrl up
              ]
            }
          }, '*');
        }
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isConnected, sendClipboard]);

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
          onClick={focusIframe}
          sx={{ color: 'white' }}
          title="Enable Keyboard Input"
          disabled={!isConnected}
        >
          <Keyboard fontSize="small" />
        </IconButton>
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

      {/* Click instruction overlay */}
      {isConnected && (
        <Box 
          sx={{ 
            position: 'absolute',
            top: 8,
            left: 8,
            backgroundColor: 'rgba(0,0,0,0.8)',
            color: 'white',
            padding: 1,
            borderRadius: 1,
            fontSize: '0.75rem',
            zIndex: 1001,
            opacity: 0.7,
            pointerEvents: 'none'
          }}
        >
          💡 Click inside to enable keyboard input
        </Box>
      )}

      {/* Guacamole Direct Client Iframe */}
      <iframe
        ref={iframeRef}
        src="about:blank"
        style={{
          width: '100%',
          height: '100%',
          border: 'none',
          backgroundColor: '#000'
        }}
        title="Guacamole Direct Client"
        allow="clipboard-read; clipboard-write; fullscreen"
        // sandbox="allow-same-origin allow-scripts allow-forms allow-top-navigation allow-modals allow-pointer-lock" // Removed for keyboard testing
        onClick={() => {
          // Try to focus on click
          if (iframeRef.current) {
            iframeRef.current.focus();
            try {
              iframeRef.current.contentWindow?.focus();
            } catch (e) {
              console.log('Could not focus iframe content window (cross-origin)');
            }
          }
        }}
        onLoad={() => {
          console.log('🔄 Iframe loaded');
          // Try to focus the iframe content after multiple delays
          [500, 1000, 2000].forEach(delay => {
            setTimeout(() => {
              if (iframeRef.current) {
                iframeRef.current.focus();
                try {
                  iframeRef.current.contentWindow?.focus();
                } catch (e) {
                  console.log('Could not focus iframe content window (cross-origin)');
                }
              }
            }, delay);
          });
        }}
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