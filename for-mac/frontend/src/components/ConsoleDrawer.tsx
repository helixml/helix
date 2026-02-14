import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { GetConsoleOutput, SendConsoleInput } from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

interface ConsoleDrawerProps {
  vmState: string;
  onClose: () => void;
}

const MIN_HEIGHT = 120;
const DEFAULT_HEIGHT = 280;
const TITLEBAR_HEIGHT = 52;

export function ConsoleDrawer({ vmState, onClose }: ConsoleDrawerProps) {
  const termRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const [height, setHeight] = useState(() => {
    const saved = localStorage.getItem('helix-console-height');
    return saved ? Math.max(MIN_HEIGHT, parseInt(saved, 10) || DEFAULT_HEIGHT) : DEFAULT_HEIGHT;
  });

  // Drag state stored in ref so the overlay's native event handlers
  // always see current values without needing React re-renders.
  const dragRef = useRef<{
    active: boolean;
    startY: number;
    startHeight: number;
  } | null>(null);
  const [showOverlay, setShowOverlay] = useState(false);

  // Persist height to localStorage on change (only meaningful values)
  useEffect(() => {
    localStorage.setItem('helix-console-height', String(Math.round(height)));
  }, [height]);

  // The overlay covers the ENTIRE viewport as a portal on document.body.
  // This guarantees we receive mousemove/mouseup even when the cursor
  // is over an iframe, xterm, or any other element that would normally
  // swallow events.
  function handlePointerDownOnHandle(e: React.PointerEvent) {
    e.preventDefault();
    e.stopPropagation();
    dragRef.current = {
      active: true,
      startY: e.clientY,
      startHeight: height,
    };
    setShowOverlay(true);
  }

  function handleOverlayPointerMove(e: React.PointerEvent) {
    if (!dragRef.current) return;
    const maxHeight = window.innerHeight - TITLEBAR_HEIGHT;
    const deltaY = dragRef.current.startY - e.clientY;
    const newHeight = Math.min(maxHeight, Math.max(MIN_HEIGHT, dragRef.current.startHeight + deltaY));
    setHeight(newHeight);
  }

  function handleOverlayPointerUp() {
    dragRef.current = null;
    setShowOverlay(false);
    fitRef.current?.fit();
  }

  // Terminal setup
  useEffect(() => {
    if (!termRef.current) return;
    if (vmState !== 'running' && vmState !== 'starting') return;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1a1a2e',
        foreground: '#e0e0e0',
        cursor: '#00d4aa',
        selectionBackground: '#3a3a5c',
      },
      scrollback: 10000,
      convertEol: true,
    });

    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(termRef.current);
    fit.fit();

    terminalRef.current = term;
    fitRef.current = fit;

    GetConsoleOutput().then((output) => {
      if (output) term.write(output);
    });

    EventsOn('console:output', (data: string) => {
      term.write(data);
    });

    term.onData((data) => {
      SendConsoleInput(data).catch((err) => {
        console.error('Failed to send console input:', err);
      });
    });

    const resizeObserver = new ResizeObserver(() => {
      fit.fit();
    });
    resizeObserver.observe(termRef.current);

    return () => {
      EventsOff('console:output');
      resizeObserver.disconnect();
      term.dispose();
      terminalRef.current = null;
      fitRef.current = null;
    };
  }, [vmState]);

  const isActive = vmState === 'running' || vmState === 'starting';

  return (
    <>
      <div className="console-drawer" style={{ height }}>
        <div
          className={`console-resize-handle ${showOverlay ? 'dragging' : ''}`}
          onPointerDown={handlePointerDownOnHandle}
        />
        <div className="console-drawer-header">
          <h3>Console</h3>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            {isActive && (
              <span style={{ color: 'var(--text-faded)', fontSize: 11 }}>Login: see Settings</span>
            )}
            <button className="panel-close" onClick={onClose} style={{ width: 24, height: 24, fontSize: 16 }}>
              &times;
            </button>
          </div>
        </div>
        {isActive ? (
          <>
            <div className="console-terminal" ref={termRef} />
            <div className="console-bottom-pad" />
          </>
        ) : (
          <div style={{ padding: 20, color: 'var(--text-faded)', fontSize: 13 }}>
            Start the environment to access the serial console.
          </div>
        )}
      </div>

      {showOverlay && createPortal(
        <div
          onPointerMove={handleOverlayPointerMove}
          onPointerUp={handleOverlayPointerUp}
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 9999,
            cursor: 'ns-resize',
          }}
        />,
        document.body,
      )}
    </>
  );
}
