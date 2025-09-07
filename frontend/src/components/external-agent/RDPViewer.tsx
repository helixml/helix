import React, { useEffect, useRef, useState, useCallback } from 'react';
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
  AppBar,
  Toolbar,
  Slider,
  FormControlLabel,
  Switch,
  Paper,
} from '@mui/material';
import {
  Fullscreen,
  FullscreenExit,
  Refresh,
  Close,
  Settings,
  Computer,
  VolumeUp,
  VolumeOff,
  ContentCopy,
  KeyboardArrowUp,
  KeyboardArrowDown,
} from '@mui/icons-material';

interface RDPConnectionInfo {
  session_id: string;
  rdp_url: string;
  rdp_port: number;
  rdp_password: string;
  proxy_url: string;
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
  connectionType?: 'session' | 'runner'; // New prop to specify connection type
}

interface GuacamoleInstruction {
  opcode: string;
  args: string[];
}

const RDPViewer: React.FC<RDPViewerProps> = ({
  sessionId,
  onClose,
  autoConnect = true,
  width = 1280,
  height = 720,
  connectionType = 'session',
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [connectionInfo, setConnectionInfo] = useState<RDPConnectionInfo | null>(null);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [scale, setScale] = useState(1.0);
  const [audioEnabled, setAudioEnabled] = useState(false);
  const [clipboardContent, setClipboardContent] = useState('');
  const [displaySize, setDisplaySize] = useState({ width, height });

  // Canvas context for rendering
  const ctxRef = useRef<CanvasRenderingContext2D | null>(null);

  // Fetch RDP connection info
  const fetchConnectionInfo = async () => {
    try {
      const endpoint = connectionType === 'runner' 
        ? `/api/v1/external-agents/runners/${sessionId}/rdp`
        : `/api/v1/external-agents/${sessionId}/rdp`;
      
      const response = await fetch(endpoint);
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
    }
  }, []);

  // Handle Guacamole display instruction
  const handleDisplayInstruction = useCallback((instruction: GuacamoleInstruction) => {
    const canvas = canvasRef.current;
    const ctx = ctxRef.current;
    if (!canvas || !ctx) return;

    switch (instruction.opcode) {
      case 'size':
        // Set display size: size,width,height
        if (instruction.args.length >= 2) {
          const newWidth = parseInt(instruction.args[0]);
          const newHeight = parseInt(instruction.args[1]);
          setDisplaySize({ width: newWidth, height: newHeight });
          canvas.width = newWidth;
          canvas.height = newHeight;
        }
        break;

      case 'png':
        // Draw PNG image: png,layer,x,y,data
        if (instruction.args.length >= 4) {
          const x = parseInt(instruction.args[1]);
          const y = parseInt(instruction.args[2]);
          const base64Data = instruction.args[3];
          
          const img = new Image();
          img.onload = () => {
            ctx.drawImage(img, x, y);
          };
          img.src = `data:image/png;base64,${base64Data}`;
        }
        break;

      case 'copy':
        // Copy rectangle: copy,srcLayer,srcX,srcY,width,height,dstLayer,dstX,dstY
        if (instruction.args.length >= 8) {
          const srcX = parseInt(instruction.args[1]);
          const srcY = parseInt(instruction.args[2]);
          const w = parseInt(instruction.args[3]);
          const h = parseInt(instruction.args[4]);
          const dstX = parseInt(instruction.args[6]);
          const dstY = parseInt(instruction.args[7]);
          
          const imageData = ctx.getImageData(srcX, srcY, w, h);
          ctx.putImageData(imageData, dstX, dstY);
        }
        break;

      case 'rect':
        // Draw rectangle: rect,layer,x,y,width,height
        if (instruction.args.length >= 5) {
          const x = parseInt(instruction.args[1]);
          const y = parseInt(instruction.args[2]);
          const w = parseInt(instruction.args[3]);
          const h = parseInt(instruction.args[4]);
          
          ctx.fillStyle = '#000000'; // Default black
          ctx.fillRect(x, y, w, h);
        }
        break;

      case 'cfill':
        // Color fill: cfill,mask,layer,x,y,width,height,red,green,blue,alpha
        if (instruction.args.length >= 10) {
          const x = parseInt(instruction.args[2]);
          const y = parseInt(instruction.args[3]);
          const w = parseInt(instruction.args[4]);
          const h = parseInt(instruction.args[5]);
          const r = parseInt(instruction.args[6]);
          const g = parseInt(instruction.args[7]);
          const b = parseInt(instruction.args[8]);
          const a = parseInt(instruction.args[9]) / 255;
          
          ctx.fillStyle = `rgba(${r},${g},${b},${a})`;
          ctx.fillRect(x, y, w, h);
        }
        break;

      case 'cursor':
        // Set cursor: cursor,x,y,layer,srcX,srcY,width,height
        if (instruction.args.length >= 2) {
          const x = parseInt(instruction.args[0]);
          const y = parseInt(instruction.args[1]);
          canvas.style.cursor = `url('data:image/png;base64,${instruction.args[7]}') ${x} ${y}, auto`;
        }
        break;

      default:
        console.debug('Unhandled display instruction:', instruction.opcode);
    }
  }, []);

  // Initialize Guacamole RDP connection
  const initializeConnection = useCallback(async (connInfo: RDPConnectionInfo) => {
    if (!canvasRef.current) return;

    setIsConnecting(true);
    setError(null);

    try {
      // Get canvas context
      const canvas = canvasRef.current;
      const ctx = canvas.getContext('2d');
      if (!ctx) {
        throw new Error('Failed to get canvas context');
      }
      ctxRef.current = ctx;

      // Setup canvas
      canvas.width = displaySize.width;
      canvas.height = displaySize.height;
      ctx.fillStyle = '#000000';
      ctx.fillRect(0, 0, canvas.width, canvas.height);

      // Connect to WebSocket proxy
      const wsUrl = connInfo.proxy_url;
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log('WebSocket connected, initializing Guacamole RDP...');
        
        // Send Guacamole handshake
        sendInstruction('select', 'rdp');
        sendInstruction('connect', 
          connInfo.host, 
          connInfo.rdp_port.toString(),
          connInfo.username,
          connInfo.rdp_password,
          displaySize.width.toString(),
          displaySize.height.toString(),
          '96' // DPI
        );
        
        // Enable audio if requested
        if (audioEnabled) {
          sendInstruction('audio');
        }
      };

      ws.onmessage = (event) => {
        const data = event.data as string;
        
        // Parse Guacamole instructions
        const instructions = data.split(';').filter(Boolean);
        
        instructions.forEach(instructionStr => {
          const instruction = parseInstruction(instructionStr);
          if (!instruction) return;

          switch (instruction.opcode) {
            case 'ready':
              console.log('Guacamole RDP connection ready');
              setIsConnected(true);
              setIsConnecting(false);
              
              // Send initial display settings
              sendInstruction('size', displaySize.width.toString(), displaySize.height.toString());
              break;

            case 'error':
              const errorMsg = instruction.args[0] || 'Unknown connection error';
              console.error('Guacamole error:', errorMsg);
              setError(`Connection failed: ${errorMsg}`);
              setIsConnecting(false);
              break;

            case 'clipboard':
              // Handle clipboard data from remote
              if (instruction.args.length > 0) {
                setClipboardContent(atob(instruction.args[0]));
              }
              break;

            default:
              // Handle display instructions
              handleDisplayInstruction(instruction);
          }
        });
      };

      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        setError('Connection failed');
        setIsConnecting(false);
      };

      ws.onclose = (event) => {
        console.log('WebSocket closed:', event.code, event.reason);
        setIsConnected(false);
        wsRef.current = null;
        
        if (event.code !== 1000) {
          setError('Connection lost to remote desktop');
        }
      };

    } catch (err) {
      console.error('Failed to initialize connection:', err);
      setError(err instanceof Error ? err.message : 'Failed to initialize connection');
      setIsConnecting(false);
    }
  }, [displaySize, audioEnabled, sendInstruction, handleDisplayInstruction]);

  // Handle mouse events
  const handleMouseEvent = useCallback((event: React.MouseEvent) => {
    if (!isConnected) return;

    const canvas = canvasRef.current;
    if (!canvas) return;

    const rect = canvas.getBoundingClientRect();
    const x = Math.round((event.clientX - rect.left) / scale);
    const y = Math.round((event.clientY - rect.top) / scale);
    
    let mask = 0;
    if (event.buttons & 1) mask |= 1; // Left button
    if (event.buttons & 2) mask |= 4; // Right button
    if (event.buttons & 4) mask |= 2; // Middle button

    sendInstruction('mouse', x.toString(), y.toString(), mask.toString());
  }, [isConnected, scale, sendInstruction]);

  // Handle keyboard events
  const handleKeyEvent = useCallback((event: React.KeyboardEvent, pressed: boolean) => {
    if (!isConnected) return;

    event.preventDefault();
    const keysym = getKeysym(event.key, event.keyCode);
    if (keysym) {
      sendInstruction('key', keysym.toString(), pressed ? '1' : '0');
    }
  }, [isConnected, sendInstruction]);

  // Convert key to Guacamole keysym
  const getKeysym = (key: string, keyCode: number): number | null => {
    // Common key mappings for Guacamole
    const keyMap: { [key: string]: number } = {
      'Backspace': 0xff08,
      'Tab': 0xff09,
      'Enter': 0xff0d,
      'Escape': 0xff1b,
      'Delete': 0xffff,
      'Home': 0xff50,
      'End': 0xff57,
      'PageUp': 0xff55,
      'PageDown': 0xff56,
      'ArrowLeft': 0xff51,
      'ArrowUp': 0xff52,
      'ArrowRight': 0xff53,
      'ArrowDown': 0xff54,
      'F1': 0xffbe,
      'F2': 0xffbf,
      'F3': 0xffc0,
      'F4': 0xffc1,
      'F5': 0xffc2,
      'F6': 0xffc3,
      'F7': 0xffc4,
      'F8': 0xffc5,
      'F9': 0xffc6,
      'F10': 0xffc7,
      'F11': 0xffc8,
      'F12': 0xffc9,
      'Control': 0xffe3,
      'Alt': 0xffe9,
      'Shift': 0xffe1,
    };

    if (keyMap[key]) {
      return keyMap[key];
    }

    // For printable characters, use Unicode
    if (key.length === 1) {
      return key.charCodeAt(0);
    }

    return keyCode;
  };

  // Connect to RDP
  const connect = async () => {
    const connInfo = connectionInfo || await fetchConnectionInfo();
    if (connInfo) {
      await initializeConnection(connInfo);
    }
  };

  // Disconnect
  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setIsConnected(false);
  }, []);

  // Toggle fullscreen
  const toggleFullscreen = useCallback(() => {
    if (!containerRef.current) return;

    if (!document.fullscreenElement) {
      containerRef.current.requestFullscreen().then(() => {
        setIsFullscreen(true);
      }).catch((err) => {
        console.error('Failed to enter fullscreen:', err);
      });
    } else {
      document.exitFullscreen().then(() => {
        setIsFullscreen(false);
      }).catch((err) => {
        console.error('Failed to exit fullscreen:', err);
      });
    }
  }, []);

  // Handle scale changes
  const handleScaleChange = useCallback((newScale: number) => {
    setScale(newScale);
    if (canvasRef.current) {
      canvasRef.current.style.transform = `scale(${newScale})`;
      canvasRef.current.style.transformOrigin = 'top left';
    }
  }, []);

  // Send clipboard to remote
  const sendClipboard = useCallback(() => {
    if (clipboardContent && isConnected) {
      const base64Data = btoa(clipboardContent);
      sendInstruction('clipboard', base64Data);
    }
  }, [clipboardContent, isConnected, sendInstruction]);

  // Copy from local clipboard
  const copyFromClipboard = useCallback(async () => {
    try {
      const text = await navigator.clipboard.readText();
      setClipboardContent(text);
      sendClipboard();
    } catch (err) {
      console.error('Failed to read clipboard:', err);
    }
  }, [sendClipboard]);

  // Send Ctrl+Alt+Del
  const sendCtrlAltDel = useCallback(() => {
    if (!isConnected) return;
    
    // Send key combination
    sendInstruction('key', '0xffe3', '1'); // Ctrl down
    sendInstruction('key', '0xffe9', '1'); // Alt down
    sendInstruction('key', '0xffff', '1'); // Del down
    
    setTimeout(() => {
      sendInstruction('key', '0xffff', '0'); // Del up
      sendInstruction('key', '0xffe9', '0'); // Alt up
      sendInstruction('key', '0xffe3', '0'); // Ctrl up
    }, 100);
  }, [isConnected, sendInstruction]);

  // Handle component cleanup
  useEffect(() => {
    return () => {
      disconnect();
    };
  }, [disconnect]);

  // Auto-connect on mount
  useEffect(() => {
    if (autoConnect) {
      fetchConnectionInfo().then((connInfo) => {
        if (connInfo) {
          initializeConnection(connInfo);
        }
      });
    }
  }, [autoConnect, initializeConnection]);

  // Handle fullscreen changes
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
    };
  }, []);

  return (
    <Box 
      ref={containerRef}
      sx={{ 
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: '#000',
      }}
    >
      {/* Toolbar */}
      <AppBar position="static" color="default" elevation={1}>
        <Toolbar variant="dense" sx={{ minHeight: 48 }}>
          <Computer sx={{ mr: 1 }} />
          <Typography variant="h6" sx={{ flexGrow: 1, fontSize: '0.9rem' }}>
            Zed Editor - Session {sessionId.slice(-8)}
          </Typography>
          
          {/* Connection Status */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mr: 2 }}>
            {isConnecting && (
              <>
                <CircularProgress size={16} />
                <Typography variant="caption">Connecting...</Typography>
              </>
            )}
            {isConnected && (
              <Typography variant="caption" color="success.main">
                ● Connected
              </Typography>
            )}
            {error && (
              <Typography variant="caption" color="error.main">
                ● Error
              </Typography>
            )}
          </Box>

          {/* Scale Controls */}
          {isConnected && (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mr: 2 }}>
              <Typography variant="caption" sx={{ mr: 1 }}>
                {Math.round(scale * 100)}%
              </Typography>
              <IconButton 
                size="small" 
                onClick={() => handleScaleChange(Math.max(0.5, scale - 0.25))}
                disabled={scale <= 0.5}
              >
                <KeyboardArrowDown />
              </IconButton>
              <IconButton 
                size="small" 
                onClick={() => handleScaleChange(Math.min(2.0, scale + 0.25))}
                disabled={scale >= 2.0}
              >
                <KeyboardArrowUp />
              </IconButton>
            </Box>
          )}

          {/* Control Buttons */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            {!isConnected && !isConnecting && (
              <Tooltip title="Connect">
                <IconButton size="small" onClick={connect}>
                  <Refresh />
                </IconButton>
              </Tooltip>
            )}
            
            {isConnected && (
              <Tooltip title="Copy from Clipboard">
                <IconButton size="small" onClick={copyFromClipboard}>
                  <ContentCopy />
                </IconButton>
              </Tooltip>
            )}
            
            <Tooltip title="Toggle Fullscreen">
              <IconButton size="small" onClick={toggleFullscreen}>
                {isFullscreen ? <FullscreenExit /> : <Fullscreen />}
              </IconButton>
            </Tooltip>

            <Tooltip title="Settings">
              <IconButton size="small" onClick={() => setShowSettings(true)}>
                <Settings />
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
        </Toolbar>
      </AppBar>

      {/* Main Content */}
      <Box sx={{ flexGrow: 1, position: 'relative', overflow: 'auto', backgroundColor: '#000' }}>
        {/* Error Display */}
        {error && (
          <Alert 
            severity="error" 
            sx={{ m: 2 }}
            action={
              <Button size="small" onClick={() => {
                setError(null);
                connect();
              }}>
                Retry
              </Button>
            }
          >
            {error}
          </Alert>
        )}

        {/* Connection Status */}
        {!isConnected && !error && (
          <Box
            sx={{
              position: 'absolute',
              top: '50%',
              left: '50%',
              transform: 'translate(-50%, -50%)',
              textAlign: 'center',
              zIndex: 10,
            }}
          >
            <Card sx={{ p: 3, backgroundColor: 'rgba(0,0,0,0.9)', color: 'white' }}>
              <CardContent>
                {isConnecting ? (
                  <>
                    <CircularProgress sx={{ mb: 2, color: 'white' }} />
                    <Typography variant="h6" gutterBottom>
                      Connecting to Zed Editor
                    </Typography>
                    <Typography variant="body2" color="grey.300">
                      Establishing remote desktop connection...
                    </Typography>
                  </>
                ) : (
                  <>
                    <Computer sx={{ fontSize: 48, mb: 2, color: 'grey.400' }} />
                    <Typography variant="h6" gutterBottom>
                      Remote Desktop Ready
                    </Typography>
                    <Typography variant="body2" color="grey.300" paragraph>
                      Click connect to access your Zed development environment
                    </Typography>
                    <Button 
                      variant="contained" 
                      onClick={connect}
                      startIcon={<Computer />}
                    >
                      Connect to Zed
                    </Button>
                  </>
                )}
              </CardContent>
            </Card>
          </Box>
        )}

        {/* RDP Canvas */}
        <Box 
          sx={{ 
            display: isConnected ? 'block' : 'none',
            overflow: 'auto',
            width: '100%',
            height: '100%',
          }}
        >
          <canvas
            ref={canvasRef}
            style={{
              display: 'block',
              backgroundColor: '#000',
              cursor: isConnected ? 'default' : 'wait',
              transformOrigin: 'top left',
              transform: `scale(${scale})`,
            }}
            width={displaySize.width}
            height={displaySize.height}
            onMouseMove={handleMouseEvent}
            onMouseDown={handleMouseEvent}
            onMouseUp={handleMouseEvent}
            onKeyDown={(e) => handleKeyEvent(e, true)}
            onKeyUp={(e) => handleKeyEvent(e, false)}
            onContextMenu={(e) => e.preventDefault()}
            tabIndex={0} // Make canvas focusable for keyboard events
          />
        </Box>
      </Box>

      {/* Settings Dialog */}
      <Dialog open={showSettings} onClose={() => setShowSettings(false)} maxWidth="sm">
        <DialogTitle>Remote Desktop Settings</DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, pt: 1 }}>
            {/* Connection Info */}
            <Box>
              <Typography variant="subtitle2" gutterBottom>Connection Info</Typography>
              {connectionInfo && (
                <Paper sx={{ p: 2, backgroundColor: 'grey.100' }}>
                  <Typography variant="caption" component="div">
                    <strong>Session:</strong> {connectionInfo.session_id}
                  </Typography>
                  <Typography variant="caption" component="div">
                    <strong>Status:</strong> {connectionInfo.status}
                  </Typography>
                  <Typography variant="caption" component="div">
                    <strong>Display:</strong> {displaySize.width}x{displaySize.height}
                  </Typography>
                  <Typography variant="caption" component="div">
                    <strong>Protocol:</strong> RDP via Guacamole WebSocket
                  </Typography>
                </Paper>
              )}
            </Box>

            {/* Display Settings */}
            <Box>
              <Typography variant="subtitle2" gutterBottom>Display Scale</Typography>
              <Slider
                value={scale}
                min={0.25}
                max={2.0}
                step={0.25}
                onChange={(_, value) => handleScaleChange(value as number)}
                valueLabelDisplay="auto"
                valueLabelFormat={(value) => `${Math.round(value * 100)}%`}
                disabled={!isConnected}
              />
              <Box sx={{ display: 'flex', gap: 1, mt: 1 }}>
                {[0.5, 0.75, 1.0, 1.25, 1.5].map((scaleValue) => (
                  <Button
                    key={scaleValue}
                    size="small"
                    variant={scale === scaleValue ? 'contained' : 'outlined'}
                    onClick={() => handleScaleChange(scaleValue)}
                    disabled={!isConnected}
                  >
                    {Math.round(scaleValue * 100)}%
                  </Button>
                ))}
              </Box>
            </Box>

            {/* Audio Settings */}
            <Box>
              <FormControlLabel
                control={
                  <Switch
                    checked={audioEnabled}
                    onChange={(e) => setAudioEnabled(e.target.checked)}
                    disabled={isConnected} // Can only change before connecting
                  />
                }
                label="Enable Audio"
              />
              <Typography variant="caption" color="text.secondary" display="block">
                Audio can only be enabled before connecting
              </Typography>
            </Box>

            {/* Clipboard */}
            {isConnected && (
              <Box>
                <Typography variant="subtitle2" gutterBottom>Clipboard</Typography>
                <Box sx={{ display: 'flex', gap: 1, mb: 1 }}>
                  <Button 
                    size="small" 
                    variant="outlined" 
                    onClick={copyFromClipboard}
                    startIcon={<ContentCopy />}
                  >
                    Sync from Local
                  </Button>
                  <Button 
                    size="small" 
                    variant="outlined" 
                    onClick={sendClipboard}
                    disabled={!clipboardContent}
                  >
                    Send to Remote
                  </Button>
                </Box>
                {clipboardContent && (
                  <Paper sx={{ p: 1, backgroundColor: 'grey.100', maxHeight: 100, overflow: 'auto' }}>
                    <Typography variant="caption" style={{ wordBreak: 'break-all' }}>
                      {clipboardContent.substring(0, 200)}{clipboardContent.length > 200 ? '...' : ''}
                    </Typography>
                  </Paper>
                )}
              </Box>
            )}

            {/* Remote Control */}
            {isConnected && (
              <Box>
                <Typography variant="subtitle2" gutterBottom>Remote Control</Typography>
                <Box sx={{ display: 'flex', gap: 1 }}>
                  <Button 
                    size="small" 
                    variant="outlined" 
                    onClick={sendCtrlAltDel}
                  >
                    Ctrl+Alt+Del
                  </Button>
                  <Button 
                    size="small" 
                    variant="outlined" 
                    onClick={() => sendInstruction('disconnect')}
                  >
                    Disconnect
                  </Button>
                </Box>
              </Box>
            )}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowSettings(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default RDPViewer;