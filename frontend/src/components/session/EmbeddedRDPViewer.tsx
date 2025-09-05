import React, { useRef, useState, useEffect, useCallback } from 'react';
import {
  Box,
  Typography,
  IconButton,
  Button,
  Tooltip,
  Alert,
  CircularProgress,
  Card,
  CardContent,
  CardHeader,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  Switch,
  FormControlLabel,
  Slider,
} from '@mui/material';
import {
  Computer,
  Fullscreen,
  FullscreenExit,
  Settings,
  Refresh,
  Close,
  VolumeOff,
  VolumeUp,
  Mouse,
  Keyboard,
} from '@mui/icons-material';

interface RDPConnectionInfo {
  rdp_url: string;
  rdp_port: number;
  rdp_password: string;
  websocket_url: string;
}

interface EmbeddedRDPViewerProps {
  sessionId: string;
  height?: number;
  onClose?: () => void;
  autoConnect?: boolean;
}

interface GuacamoleInstruction {
  opcode: string;
  args: string[];
}

const EmbeddedRDPViewer: React.FC<EmbeddedRDPViewerProps> = ({
  sessionId,
  height = 400,
  onClose,
  autoConnect = true,
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  
  const [connectionInfo, setConnectionInfo] = useState<RDPConnectionInfo | null>(null);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [scale, setScale] = useState(0.8);
  const [audioEnabled, setAudioEnabled] = useState(false);
  const [displaySize, setDisplaySize] = useState({ width: 1280, height: 720 });
  const [lastActivity, setLastActivity] = useState<Date>(new Date());

  const ctxRef = useRef<CanvasRenderingContext2D | null>(null);

  // Fetch RDP connection info
  const fetchConnectionInfo = useCallback(async () => {
    try {
      setError(null);
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
  }, [sessionId]);

  // Parse Guacamole protocol instruction
  const parseInstruction = (data: string): GuacamoleInstruction | null => {
    const parts = data.split(',');
    if (parts.length < 1) return null;
    
    const opcode = parts[0];
    const args = parts.slice(1);
    
    return { opcode, args };
  };

  // Send Guacamole instruction
  const sendInstruction = useCallback((opcode: string, ...args: string[]) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      const instruction = `${opcode}${args.length > 0 ? ',' + args.join(',') : ''};`;
      wsRef.current.send(instruction);
      setLastActivity(new Date());
    }
  }, []);

  // Initialize canvas context
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas) {
      const ctx = canvas.getContext('2d');
      ctxRef.current = ctx;
      
      // Set canvas size based on scale
      canvas.width = displaySize.width * scale;
      canvas.height = displaySize.height * scale;
      
      if (ctx) {
        ctx.imageSmoothingEnabled = true;
        ctx.imageSmoothingQuality = 'high';
      }
    }
  }, [displaySize, scale]);

  // Connect to Guacamole WebSocket
  const connect = useCallback(async () => {
    if (!connectionInfo) {
      const info = await fetchConnectionInfo();
      if (!info) return;
    }

    setIsConnecting(true);
    setError(null);

    try {
      // Use Guacamole client WebSocket endpoint
      const wsUrl = `/guacamole/websocket-tunnel?token=${encodeURIComponent(sessionId)}`;
      const ws = new WebSocket(wsUrl.replace('http', 'ws'));
      
      ws.onopen = () => {
        setIsConnected(true);
        setIsConnecting(false);
        
        // Send connection parameters
        sendInstruction('connect', 
          'rdp',
          'hostname=localhost',
          `port=${connectionInfo?.rdp_port || 3389}`,
          'username=zed',
          `password=${connectionInfo?.rdp_password || 'zed123'}`,
          `width=${Math.floor(displaySize.width * scale)}`,
          `height=${Math.floor(displaySize.height * scale)}`,
          'dpi=96',
          'color-depth=32'
        );
      };

      ws.onmessage = (event) => {
        const instruction = parseInstruction(event.data);
        if (!instruction) return;

        handleGuacamoleInstruction(instruction);
      };

      ws.onclose = () => {
        setIsConnected(false);
        setIsConnecting(false);
        setError('Connection closed');
      };

      ws.onerror = () => {
        setIsConnected(false);
        setIsConnecting(false);
        setError('WebSocket connection failed');
      };

      wsRef.current = ws;
    } catch (err) {
      setIsConnecting(false);
      setError(err instanceof Error ? err.message : 'Failed to connect');
    }
  }, [connectionInfo, sessionId, displaySize, scale, sendInstruction, fetchConnectionInfo]);

  // Handle Guacamole protocol instructions
  const handleGuacamoleInstruction = (instruction: GuacamoleInstruction) => {
    const { opcode, args } = instruction;
    const ctx = ctxRef.current;
    if (!ctx) return;

    switch (opcode) {
      case 'png':
        // Handle PNG image data
        if (args.length >= 6) {
          const layer = args[0];
          const x = parseInt(args[1]);
          const y = parseInt(args[2]);
          const imageData = args[5]; // Base64 PNG data
          
          const img = new Image();
          img.onload = () => {
            ctx.drawImage(img, x, y);
          };
          img.src = `data:image/png;base64,${imageData}`;
        }
        break;
        
      case 'copy':
        // Handle screen region copy
        if (args.length >= 8) {
          const srcLayer = args[0];
          const srcX = parseInt(args[1]);
          const srcY = parseInt(args[2]);
          const srcWidth = parseInt(args[3]);
          const srcHeight = parseInt(args[4]);
          const dstLayer = args[5];
          const dstX = parseInt(args[6]);
          const dstY = parseInt(args[7]);
          
          const imageData = ctx.getImageData(srcX, srcY, srcWidth, srcHeight);
          ctx.putImageData(imageData, dstX, dstY);
        }
        break;
        
      case 'size':
        // Handle display size change
        if (args.length >= 3) {
          const layer = args[0];
          const width = parseInt(args[1]);
          const height = parseInt(args[2]);
          
          setDisplaySize({ width, height });
        }
        break;
        
      case 'ready':
        // Connection is ready
        setError(null);
        break;
        
      case 'error':
        // Handle error
        setError(args[0] || 'Unknown error');
        break;
    }
  };

  // Handle mouse events
  const handleMouseEvent = useCallback((event: React.MouseEvent, type: 'down' | 'up' | 'move') => {
    if (!isConnected) return;
    
    const canvas = canvasRef.current;
    if (!canvas) return;
    
    const rect = canvas.getBoundingClientRect();
    const x = Math.floor((event.clientX - rect.left) / scale);
    const y = Math.floor((event.clientY - rect.top) / scale);
    
    switch (type) {
      case 'down':
        sendInstruction('mouse', x.toString(), y.toString(), '1');
        break;
      case 'up':
        sendInstruction('mouse', x.toString(), y.toString(), '0');
        break;
      case 'move':
        sendInstruction('mouse', x.toString(), y.toString());
        break;
    }
  }, [isConnected, scale, sendInstruction]);

  // Handle keyboard events
  const handleKeyEvent = useCallback((event: React.KeyboardEvent, pressed: boolean) => {
    if (!isConnected) return;
    
    event.preventDefault();
    const keysym = event.keyCode; // Simplified - should map to X11 keysyms
    sendInstruction('key', keysym.toString(), pressed ? '1' : '0');
  }, [isConnected, sendInstruction]);

  // Disconnect
  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setIsConnected(false);
    setIsConnecting(false);
  }, []);

  // Auto-connect on mount
  useEffect(() => {
    if (autoConnect) {
      fetchConnectionInfo();
    }
  }, [autoConnect, fetchConnectionInfo]);

  // Auto-connect when connection info is available
  useEffect(() => {
    if (connectionInfo && autoConnect && !isConnected && !isConnecting) {
      connect();
    }
  }, [connectionInfo, autoConnect, isConnected, isConnecting, connect]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      disconnect();
    };
  }, [disconnect]);

  return (
    <Card sx={{ height: height + 60, display: 'flex', flexDirection: 'column' }}>
      <CardHeader
        avatar={<Computer color="primary" />}
        title={
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Typography variant="h6">Zed Editor</Typography>
            <Chip
              label={isConnected ? 'Connected' : isConnecting ? 'Connecting' : 'Disconnected'}
              color={isConnected ? 'success' : isConnecting ? 'warning' : 'default'}
              size="small"
            />
          </Box>
        }
        subheader={`Session: ${sessionId}`}
        action={
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Tooltip title="Settings">
              <IconButton size="small" onClick={() => setShowSettings(true)}>
                <Settings />
              </IconButton>
            </Tooltip>
            <Tooltip title="Refresh">
              <IconButton size="small" onClick={() => { disconnect(); connect(); }}>
                <Refresh />
              </IconButton>
            </Tooltip>
            {onClose && (
              <Tooltip title="Close">
                <IconButton size="small" onClick={onClose}>
                  <Close />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        }
        sx={{ pb: 1 }}
      />
      
      <CardContent sx={{ flexGrow: 1, p: 0, overflow: 'hidden' }}>
        {error && (
          <Alert severity="error" sx={{ m: 1 }}>
            {error}
            <Button size="small" onClick={connect} sx={{ ml: 1 }}>
              Retry
            </Button>
          </Alert>
        )}
        
        {isConnecting && (
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
            <CircularProgress />
            <Typography sx={{ ml: 2 }}>Connecting to Zed...</Typography>
          </Box>
        )}
        
        {!connectionInfo && !error && (
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
            <Button variant="contained" onClick={fetchConnectionInfo}>
              Connect to Zed Editor
            </Button>
          </Box>
        )}
        
        <Box
          ref={containerRef}
          sx={{
            position: 'relative',
            width: '100%',
            height: '100%',
            overflow: 'auto',
            cursor: isConnected ? 'crosshair' : 'default',
          }}
        >
          <canvas
            ref={canvasRef}
            onMouseDown={(e) => handleMouseEvent(e, 'down')}
            onMouseUp={(e) => handleMouseEvent(e, 'up')}
            onMouseMove={(e) => handleMouseEvent(e, 'move')}
            onKeyDown={(e) => handleKeyEvent(e, true)}
            onKeyUp={(e) => handleKeyEvent(e, false)}
            tabIndex={0}
            style={{
              display: isConnected ? 'block' : 'none',
              width: '100%',
              height: '100%',
              objectFit: 'contain',
            }}
          />
        </Box>
      </CardContent>

      {/* Settings Dialog */}
      <Dialog open={showSettings} onClose={() => setShowSettings(false)} maxWidth="sm" fullWidth>
        <DialogTitle>RDP Viewer Settings</DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, pt: 1 }}>
            <Box>
              <Typography gutterBottom>Scale: {Math.round(scale * 100)}%</Typography>
              <Slider
                value={scale}
                onChange={(_, value) => setScale(value as number)}
                min={0.25}
                max={2}
                step={0.05}
                marks={[
                  { value: 0.5, label: '50%' },
                  { value: 1, label: '100%' },
                  { value: 1.5, label: '150%' },
                ]}
              />
            </Box>
            
            <FormControlLabel
              control={
                <Switch
                  checked={audioEnabled}
                  onChange={(e) => setAudioEnabled(e.target.checked)}
                />
              }
              label="Enable Audio"
            />
            
            <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
              <Mouse />
              <Typography>Last Activity: {lastActivity.toLocaleTimeString()}</Typography>
            </Box>
            
            <Typography variant="body2" color="text.secondary">
              Display Size: {displaySize.width} × {displaySize.height}
            </Typography>
          </Box>
        </DialogContent>
      </Dialog>
    </Card>
  );
};

export default EmbeddedRDPViewer;