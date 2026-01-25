import React, { useRef, useEffect, useCallback, useState } from 'react';
import { Box, Typography, Alert, CircularProgress, IconButton, Tooltip } from '@mui/material';
import { Refresh, OpenInFull, CloseFullscreen } from '@mui/icons-material';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';
import { useAccount } from '../../contexts/account';

export interface ClaudeTerminalProps {
  sessionId: string;
  onConnectionChange?: (connected: boolean) => void;
  onError?: (error: string) => void;
  className?: string;
  autoConnect?: boolean;
}

type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'error';

/**
 * ClaudeTerminal - Web terminal for Claude Code sessions
 *
 * Connects to the session's terminal WebSocket endpoint and provides
 * an xterm.js interface for interacting with Claude Code running in tmux.
 */
const ClaudeTerminal: React.FC<ClaudeTerminalProps> = ({
  sessionId,
  onConnectionChange,
  onError,
  className = '',
  autoConnect = true,
}) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [error, setError] = useState<string | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const account = useAccount();

  // Send resize message to server
  const sendResize = useCallback((rows: number, cols: number) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: 'resize',
        rows,
        cols,
      }));
    }
  }, []);

  // Initialize xterm.js terminal
  const initTerminal = useCallback(() => {
    if (!terminalRef.current || xtermRef.current) return;

    const terminal = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        selectionBackground: '#264f78',
        black: '#000000',
        red: '#cd3131',
        green: '#0dbc79',
        yellow: '#e5e510',
        blue: '#2472c8',
        magenta: '#bc3fbc',
        cyan: '#11a8cd',
        white: '#e5e5e5',
        brightBlack: '#666666',
        brightRed: '#f14c4c',
        brightGreen: '#23d18b',
        brightYellow: '#f5f543',
        brightBlue: '#3b8eea',
        brightMagenta: '#d670d6',
        brightCyan: '#29b8db',
        brightWhite: '#ffffff',
      },
      scrollback: 10000,
      convertEol: true,
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();

    terminal.loadAddon(fitAddon);
    terminal.loadAddon(webLinksAddon);

    terminal.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;

    // Handle user input
    terminal.onData((data) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({
          type: 'input',
          data,
        }));
      }
    });

    // Handle resize
    terminal.onResize(({ rows, cols }) => {
      sendResize(rows, cols);
    });

    return terminal;
  }, [sendResize]);

  // Connect to WebSocket
  const connect = useCallback(() => {
    if (!sessionId) return;

    // Close existing connection
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    setConnectionState('connecting');
    setError(null);

    const token = account.user?.token || '';
    if (!token) {
      setError('Not authenticated');
      setConnectionState('error');
      onError?.('Not authenticated');
      return;
    }

    // Build WebSocket URL
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const terminalSize = fitAddonRef.current?.proposeDimensions();
    const cols = terminalSize?.cols || 120;
    const rows = terminalSize?.rows || 40;

    const wsUrl = `${protocol}//${host}/api/v1/external-agents/${sessionId}/ws/terminal?cols=${cols}&rows=${rows}`;

    const ws = new WebSocket(wsUrl, ['bearer', token]);

    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      setConnectionState('connected');
      onConnectionChange?.(true);
      xtermRef.current?.writeln('\x1b[32mConnected to Claude Code terminal\x1b[0m\r\n');
      // Send initial size
      if (terminalSize) {
        sendResize(terminalSize.rows, terminalSize.cols);
      }
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        // Binary terminal output
        const text = new TextDecoder().decode(event.data);
        xtermRef.current?.write(text);
      } else if (typeof event.data === 'string') {
        // Text message (shouldn't happen normally)
        xtermRef.current?.write(event.data);
      }
    };

    ws.onerror = () => {
      setError('WebSocket connection error');
      setConnectionState('error');
      onError?.('WebSocket connection error');
    };

    ws.onclose = (event) => {
      setConnectionState('disconnected');
      onConnectionChange?.(false);

      if (event.code !== 1000) {
        // Abnormal close, schedule reconnect
        xtermRef.current?.writeln('\r\n\x1b[33mConnection closed. Reconnecting in 3 seconds...\x1b[0m');
        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, 3000);
      } else {
        xtermRef.current?.writeln('\r\n\x1b[90mDisconnected\x1b[0m');
      }
    };

    wsRef.current = ws;
  }, [sessionId, account.user?.token, onConnectionChange, onError, sendResize]);

  // Handle window resize
  useEffect(() => {
    const handleResize = () => {
      if (fitAddonRef.current && xtermRef.current) {
        fitAddonRef.current.fit();
      }
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  // Initialize terminal and connect
  useEffect(() => {
    initTerminal();

    if (autoConnect) {
      connect();
    }

    return () => {
      // Cleanup
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (xtermRef.current) {
        xtermRef.current.dispose();
        xtermRef.current = null;
      }
    };
  }, [initTerminal, connect, autoConnect]);

  // Re-fit terminal when container size changes
  useEffect(() => {
    const observer = new ResizeObserver(() => {
      if (fitAddonRef.current) {
        fitAddonRef.current.fit();
      }
    });

    if (terminalRef.current) {
      observer.observe(terminalRef.current);
    }

    return () => observer.disconnect();
  }, []);

  const handleReconnect = useCallback(() => {
    xtermRef.current?.clear();
    connect();
  }, [connect]);

  const toggleFullscreen = useCallback(() => {
    const container = terminalRef.current?.parentElement;
    if (!container) return;

    if (!isFullscreen) {
      container.requestFullscreen?.();
      setIsFullscreen(true);
    } else {
      document.exitFullscreen?.();
      setIsFullscreen(false);
    }
  }, [isFullscreen]);

  // Listen for fullscreen changes
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
      // Refit terminal after fullscreen change
      setTimeout(() => fitAddonRef.current?.fit(), 100);
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange);
  }, []);

  return (
    <Box
      className={className}
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        backgroundColor: '#1e1e1e',
        borderRadius: 1,
        overflow: 'hidden',
      }}
    >
      {/* Toolbar */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 1,
          py: 0.5,
          backgroundColor: '#2d2d2d',
          borderBottom: '1px solid #404040',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography
            variant="caption"
            sx={{
              color: connectionState === 'connected' ? '#0dbc79' :
                     connectionState === 'connecting' ? '#e5e510' :
                     connectionState === 'error' ? '#cd3131' : '#666',
            }}
          >
            {connectionState === 'connected' ? 'Connected' :
             connectionState === 'connecting' ? 'Connecting...' :
             connectionState === 'error' ? 'Error' : 'Disconnected'}
          </Typography>
          {connectionState === 'connecting' && (
            <CircularProgress size={12} sx={{ color: '#e5e510' }} />
          )}
        </Box>

        <Box>
          <Tooltip title="Reconnect">
            <IconButton
              size="small"
              onClick={handleReconnect}
              sx={{ color: '#d4d4d4' }}
            >
              <Refresh fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
            <IconButton
              size="small"
              onClick={toggleFullscreen}
              sx={{ color: '#d4d4d4' }}
            >
              {isFullscreen ? <CloseFullscreen fontSize="small" /> : <OpenInFull fontSize="small" />}
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Error message */}
      {error && (
        <Alert severity="error" sx={{ m: 1 }}>
          {error}
        </Alert>
      )}

      {/* Terminal container */}
      <Box
        ref={terminalRef}
        sx={{
          flex: 1,
          overflow: 'hidden',
          p: 0.5,
          '& .xterm': {
            height: '100%',
          },
          '& .xterm-viewport': {
            overflow: 'auto !important',
          },
        }}
      />
    </Box>
  );
};

export default ClaudeTerminal;
