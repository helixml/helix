import React, { useEffect, useRef, useState } from 'react';
import {
  Box,
  Typography,
  Button,
  CircularProgress,
  Alert,
  Card,
  CardContent,
  IconButton,
  Tooltip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
} from '@mui/material';
import {
  Fullscreen,
  FullscreenExit,
  Refresh,
  Settings,
  Download,
  Close,
} from '@mui/icons-material';

interface RDPConnectionInfo {
  session_id: string;
  rdp_url: string;
  rdp_port: number;
  display: string;
  status: string;
  username: string;
  host: string;
}

interface RDPViewerProps {
  sessionId: string;
  onClose?: () => void;
  autoConnect?: boolean;
  width?: number;
  height?: number;
}

const RDPViewer: React.FC<RDPViewerProps> = ({
  sessionId,
  onClose,
  autoConnect = true,
  width = 1280,
  height = 720,
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [connectionInfo, setConnectionInfo] = useState<RDPConnectionInfo | null>(null);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [rdpClient, setRdpClient] = useState<any>(null);

  // Fetch RDP connection info
  const fetchConnectionInfo = async () => {
    try {
      const response = await fetch(`/api/v1/external-agents/${sessionId}/rdp`);
      if (!response.ok) {
        throw new Error(`Failed to get RDP info: ${response.statusText}`);
      }
      const data = await response.json();
      setConnectionInfo(data);
      return data;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to get RDP connection info');
      return null;
    }
  };

  // Initialize RDP connection using HTML5 RDP client (like FreeRDP-WebConnect or similar)
  const initializeRDPConnection = async (connInfo: RDPConnectionInfo) => {
    if (!canvasRef.current) return;

    setIsConnecting(true);
    setError(null);

    try {
      // This is a placeholder for actual RDP client implementation
      // In a real implementation, you would use a library like:
      // - FreeRDP-WebConnect
      // - Apache Guacamole HTML5 client
      // - Custom WebRTC-based RDP client
      
      const canvas = canvasRef.current;
      const ctx = canvas.getContext('2d');
      
      if (!ctx) {
        throw new Error('Failed to get canvas context');
      }

      // For now, show a placeholder indicating RDP connection
      ctx.fillStyle = '#1a1a1a';
      ctx.fillRect(0, 0, canvas.width, canvas.height);
      
      ctx.fillStyle = '#ffffff';
      ctx.font = '24px Arial';
      ctx.textAlign = 'center';
      ctx.fillText('Connecting to Zed Editor...', canvas.width / 2, canvas.height / 2 - 50);
      ctx.fillText(`RDP://${connInfo.host}:${connInfo.rdp_port}`, canvas.width / 2, canvas.height / 2);
      ctx.fillText(`Session: ${connInfo.session_id}`, canvas.width / 2, canvas.height / 2 + 50);

      // Simulate connection delay
      setTimeout(() => {
        if (ctx) {
          ctx.fillStyle = '#1a1a1a';
          ctx.fillRect(0, 0, canvas.width, canvas.height);
          
          ctx.fillStyle = '#00ff00';
          ctx.font = '32px Arial';
          ctx.textAlign = 'center';
          ctx.fillText('✓ Connected to Zed Editor', canvas.width / 2, canvas.height / 2 - 20);
          
          ctx.fillStyle = '#ffffff';
          ctx.font = '16px Arial';
          ctx.fillText('Use an RDP client to connect to:', canvas.width / 2, canvas.height / 2 + 20);
          ctx.fillText(`${connInfo.host}:${connInfo.rdp_port}`, canvas.width / 2, canvas.height / 2 + 40);
          ctx.fillText(`Username: ${connInfo.username} | Password: zed123`, canvas.width / 2, canvas.height / 2 + 60);
        }
        
        setIsConnected(true);
        setIsConnecting(false);
      }, 2000);

    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect to RDP');
      setIsConnecting(false);
    }
  };

  // Connect to RDP
  const connect = async () => {
    const connInfo = connectionInfo || await fetchConnectionInfo();
    if (connInfo) {
      await initializeRDPConnection(connInfo);
    }
  };

  // Disconnect from RDP
  const disconnect = () => {
    if (rdpClient) {
      // Disconnect RDP client
      rdpClient.disconnect();
      setRdpClient(null);
    }
    setIsConnected(false);
    setIsConnecting(false);
  };

  // Toggle fullscreen
  const toggleFullscreen = () => {
    if (!isFullscreen) {
      canvasRef.current?.requestFullscreen();
    } else {
      document.exitFullscreen();
    }
  };

  // Handle fullscreen change
  const handleFullscreenChange = () => {
    setIsFullscreen(!!document.fullscreenElement);
  };

  // Download RDP file
  const downloadRDPFile = () => {
    if (!connectionInfo) return;

    const rdpContent = `full address:s:${connectionInfo.host}:${connectionInfo.rdp_port}
username:s:${connectionInfo.username}
screen mode id:i:2
use multimon:i:0
desktopwidth:i:${width}
desktopheight:i:${height}
session bpp:i:32
winposstr:s:0,3,0,0,800,600
compression:i:1
keyboardhook:i:2
audiocapturemode:i:0
videoplaybackmode:i:1
connection type:i:7
networkautodetect:i:1
bandwidthautodetect:i:1
displayconnectionbar:i:1
enableworkspacereconnect:i:0
disable wallpaper:i:0
allow font smoothing:i:0
allow desktop composition:i:0
disable full window drag:i:1
disable menu anims:i:1
disable themes:i:0
disable cursor setting:i:0
bitmapcachepersistenable:i:1
audiomode:i:0
redirectprinters:i:1
redirectcomports:i:0
redirectsmartcards:i:1
redirectclipboard:i:1
redirectposdevices:i:0
autoreconnection enabled:i:1
authentication level:i:2
prompt for credentials:i:0
negotiate security layer:i:1
remoteapplicationmode:i:0
alternate shell:s:
shell working directory:s:
gatewayhostname:s:
gatewayusagemethod:i:4
gatewaycredentialssource:i:4
gatewayprofileusagemethod:i:0
promptcredentialonce:i:0
gatewaybrokeringtype:i:0
use redirection server name:i:0
rdgiskdcproxy:i:0
kdcproxyname:s:`;

    const blob = new Blob([rdpContent], { type: 'application/rdp' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `zed-editor-${connectionInfo.session_id}.rdp`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  // Initialize on mount
  useEffect(() => {
    if (autoConnect) {
      fetchConnectionInfo();
    }

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    
    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
      disconnect();
    };
  }, [sessionId, autoConnect]);

  // Auto-connect when connection info is available
  useEffect(() => {
    if (connectionInfo && autoConnect && !isConnected && !isConnecting) {
      connect();
    }
  }, [connectionInfo, autoConnect, isConnected, isConnecting]);

  return (
    <Box sx={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <Box sx={{ 
        display: 'flex', 
        alignItems: 'center', 
        justifyContent: 'space-between', 
        p: 1, 
        bgcolor: 'background.paper',
        borderBottom: 1,
        borderColor: 'divider'
      }}>
        <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
          Zed Editor - {sessionId}
        </Typography>
        
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Tooltip title="Refresh Connection">
            <IconButton onClick={() => window.location.reload()} size="small">
              <Refresh />
            </IconButton>
          </Tooltip>
          
          <Tooltip title="Download RDP File">
            <IconButton onClick={downloadRDPFile} size="small" disabled={!connectionInfo}>
              <Download />
            </IconButton>
          </Tooltip>
          
          <Tooltip title="Settings">
            <IconButton onClick={() => setShowSettings(true)} size="small">
              <Settings />
            </IconButton>
          </Tooltip>
          
          <Tooltip title={isFullscreen ? "Exit Fullscreen" : "Fullscreen"}>
            <IconButton onClick={toggleFullscreen} size="small">
              {isFullscreen ? <FullscreenExit /> : <Fullscreen />}
            </IconButton>
          </Tooltip>
          
          {onClose && (
            <Tooltip title="Close">
              <IconButton onClick={onClose} size="small">
                <Close />
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </Box>

      {/* Main content area */}
      <Box sx={{ flexGrow: 1, position: 'relative', overflow: 'hidden' }}>
        {error && (
          <Alert 
            severity="error" 
            sx={{ m: 2 }}
            action={
              <Button color="inherit" size="small" onClick={() => setError(null)}>
                Retry
              </Button>
            }
          >
            {error}
          </Alert>
        )}

        {isConnecting && (
          <Box sx={{ 
            position: 'absolute', 
            top: '50%', 
            left: '50%', 
            transform: 'translate(-50%, -50%)',
            textAlign: 'center',
            zIndex: 1000
          }}>
            <CircularProgress size={60} />
            <Typography variant="h6" sx={{ mt: 2 }}>
              Connecting to Zed Editor...
            </Typography>
          </Box>
        )}

        {/* RDP Canvas */}
        <canvas
          ref={canvasRef}
          width={width}
          height={height}
          style={{
            width: '100%',
            height: '100%',
            objectFit: 'contain',
            backgroundColor: '#000',
            cursor: isConnected ? 'crosshair' : 'default',
          }}
        />

        {/* Connection info overlay */}
        {connectionInfo && !isConnecting && (
          <Card sx={{ 
            position: 'absolute', 
            top: 16, 
            right: 16, 
            minWidth: 300,
            opacity: 0.9,
          }}>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                RDP Connection
              </Typography>
              <Typography variant="body2" color="text.secondary">
                <strong>Host:</strong> {connectionInfo.host}:{connectionInfo.rdp_port}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                <strong>Username:</strong> {connectionInfo.username}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                <strong>Status:</strong> {isConnected ? 'Connected' : connectionInfo.status}
              </Typography>
              <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
                {!isConnected ? (
                  <Button variant="contained" onClick={connect} size="small">
                    Connect
                  </Button>
                ) : (
                  <Button variant="outlined" onClick={disconnect} size="small">
                    Disconnect
                  </Button>
                )}
              </Box>
            </CardContent>
          </Card>
        )}
      </Box>

      {/* Settings Dialog */}
      <Dialog open={showSettings} onClose={() => setShowSettings(false)} maxWidth="sm" fullWidth>
        <DialogTitle>RDP Settings</DialogTitle>
        <DialogContent>
          <Typography variant="body2" paragraph>
            To connect with your own RDP client, use these settings:
          </Typography>
          {connectionInfo && (
            <Box sx={{ fontFamily: 'monospace', bgcolor: 'grey.100', p: 2, borderRadius: 1 }}>
              <div><strong>Server:</strong> {connectionInfo.host}:{connectionInfo.rdp_port}</div>
              <div><strong>Username:</strong> {connectionInfo.username}</div>
              <div><strong>Password:</strong> zed123</div>
              <div><strong>Resolution:</strong> {width}x{height}</div>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={downloadRDPFile} disabled={!connectionInfo}>
            Download RDP File
          </Button>
          <Button onClick={() => setShowSettings(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default RDPViewer;