/**
 * TerminalViewer — xterm.js-based terminal that connects to the sandbox's
 * persistent tmux session via WebSocket PTY.
 *
 * Used by DesktopStreamViewer when interfaceMode === "terminal".
 * Connects to GET /api/v1/sessions/{id}/terminal (WebSocket upgrade).
 */
import React, { useRef, useEffect, useCallback } from "react";
import { Box } from "@mui/material";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

interface TerminalViewerProps {
  sessionId: string;
  onConnectionChange?: (isConnected: boolean) => void;
}

const TerminalViewer: React.FC<TerminalViewerProps> = ({
  sessionId,
  onConnectionChange,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);

  const connect = useCallback(() => {
    if (!containerRef.current || !sessionId) return;

    // Create terminal
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
      theme: {
        background: "#1a1a2e",
        foreground: "#e0e0e0",
        cursor: "#e0e0e0",
        selectionBackground: "#3a3a5e",
      },
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());

    term.open(containerRef.current);
    fitAddon.fit();

    termRef.current = term;
    fitAddonRef.current = fitAddon;

    // Build WebSocket URL — auth is handled by session cookie (same as video stream)
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const cols = term.cols;
    const rows = term.rows;
    const wsUrl = `${protocol}//${window.location.host}/api/v1/sessions/${sessionId}/terminal?cols=${cols}&rows=${rows}`;
    const ws = new WebSocket(wsUrl);
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      term.writeln("\x1b[32mConnected to sandbox shell\x1b[0m\r\n");
      onConnectionChange?.(true);
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      } else {
        term.write(event.data);
      }
    };

    ws.onclose = () => {
      term.writeln("\r\n\x1b[31mDisconnected\x1b[0m");
      onConnectionChange?.(false);
    };

    ws.onerror = () => {
      term.writeln("\r\n\x1b[31mConnection error\x1b[0m");
    };

    wsRef.current = ws;

    // Terminal input → WebSocket
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });

    // Handle resize
    term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", cols, rows }));
      }
    });

    // Fit on window resize
    const handleResize = () => {
      fitAddon.fit();
    };
    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      ws.close();
      term.dispose();
    };
  }, [sessionId, onConnectionChange]);

  useEffect(() => {
    const cleanup = connect();
    return cleanup;
  }, [connect]);

  // Re-fit when container size changes
  useEffect(() => {
    const observer = new ResizeObserver(() => {
      fitAddonRef.current?.fit();
    });
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }
    return () => observer.disconnect();
  }, []);

  return (
    <Box
      ref={containerRef}
      sx={{
        width: "100%",
        height: "100%",
        backgroundColor: "#1a1a2e",
        "& .xterm": {
          padding: "8px",
          height: "100%",
        },
        "& .xterm-viewport": {
          overflowY: "auto !important",
        },
      }}
    />
  );
};

export default TerminalViewer;
