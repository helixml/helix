import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import {
  GetConsoleOutput,
  GetConsolePassword,
  GetLogsOutput,
  SendConsoleInput,
  ResizeConsole,
} from "../../wailsjs/go/main/App";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";

interface ConsoleDrawerProps {
  vmState: string;
  onClose: () => void;
  initialTab?: "console" | "logs";
}

const MIN_HEIGHT = 120;
const DEFAULT_HEIGHT = 280;
const TITLEBAR_HEIGHT = 52;

const TERM_THEME = {
  background: "#1a1a2e",
  foreground: "#e0e0e0",
  cursor: "#00d4aa",
  selectionBackground: "#3a3a5c",
};

export function ConsoleDrawer({
  vmState,
  onClose,
  initialTab,
}: ConsoleDrawerProps) {
  // Console refs
  const consoleContainerRef = useRef<HTMLDivElement>(null);
  const consoleTermRef = useRef<Terminal | null>(null);
  const consoleFitRef = useRef<FitAddon | null>(null);

  // Logs refs
  const logsContainerRef = useRef<HTMLDivElement>(null);
  const logsTermRef = useRef<Terminal | null>(null);
  const logsFitRef = useRef<FitAddon | null>(null);

  const [activeTab, setActiveTab] = useState<"console" | "logs">(
    initialTab ?? "console",
  );
  const [height, setHeight] = useState(() => {
    const saved = localStorage.getItem("helix-console-height");
    return saved
      ? Math.max(MIN_HEIGHT, parseInt(saved, 10) || DEFAULT_HEIGHT)
      : DEFAULT_HEIGHT;
  });

  // Drag state stored in ref so the overlay's native event handlers
  // always see current values without needing React re-renders.
  const dragRef = useRef<{
    active: boolean;
    startY: number;
    startHeight: number;
  } | null>(null);
  const [showOverlay, setShowOverlay] = useState(false);

  // Persist height to localStorage on change
  useEffect(() => {
    localStorage.setItem("helix-console-height", String(Math.round(height)));
  }, [height]);

  // Refit active terminal on tab switch (hidden terminals can't measure)
  useEffect(() => {
    requestAnimationFrame(() => {
      if (activeTab === "console") {
        consoleFitRef.current?.fit();
      } else {
        logsFitRef.current?.fit();
      }
    });
  }, [activeTab]);

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
    const newHeight = Math.min(
      maxHeight,
      Math.max(MIN_HEIGHT, dragRef.current.startHeight + deltaY),
    );
    setHeight(newHeight);
  }

  function handleOverlayPointerUp() {
    dragRef.current = null;
    setShowOverlay(false);
    if (activeTab === "console") {
      consoleFitRef.current?.fit();
    } else {
      logsFitRef.current?.fit();
    }
  }

  // Console terminal setup
  useEffect(() => {
    if (!consoleContainerRef.current) return;
    if (vmState !== "running" && vmState !== "starting") return;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: TERM_THEME,
      scrollback: 10000,
      convertEol: true,
    });

    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(consoleContainerRef.current);
    fit.fit();

    consoleTermRef.current = term;
    consoleFitRef.current = fit;

    GetConsoleOutput().then((output) => {
      if (output) term.write(output);
    });

    EventsOn("console:output", (data: string) => {
      term.write(data);
    });

    term.onData((data) => {
      SendConsoleInput(data).catch((err) => {
        console.error("Failed to send console input:", err);
      });
    });

    // Propagate terminal size to guest serial console
    term.onResize(({ cols, rows }) => {
      ResizeConsole(cols, rows).catch(() => {});
    });
    // Send initial size
    ResizeConsole(term.cols, term.rows).catch(() => {});

    const resizeObserver = new ResizeObserver(() => {
      fit.fit();
    });
    resizeObserver.observe(consoleContainerRef.current);

    return () => {
      EventsOff("console:output");
      resizeObserver.disconnect();
      term.dispose();
      consoleTermRef.current = null;
      consoleFitRef.current = null;
    };
  }, [vmState]);

  // Logs terminal setup
  useEffect(() => {
    if (!logsContainerRef.current) return;
    if (vmState !== "running" && vmState !== "starting") return;

    const term = new Terminal({
      cursorBlink: false,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: { ...TERM_THEME, cursor: "#1a1a2e" },
      scrollback: 10000,
      convertEol: true,
      disableStdin: true,
    });

    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(logsContainerRef.current);
    fit.fit();

    logsTermRef.current = term;
    logsFitRef.current = fit;

    GetLogsOutput().then((output) => {
      if (output) term.write(output);
    });

    EventsOn("logs:output", (data: string) => {
      term.write(data);
    });

    const resizeObserver = new ResizeObserver(() => {
      fit.fit();
    });
    resizeObserver.observe(logsContainerRef.current);

    return () => {
      EventsOff("logs:output");
      resizeObserver.disconnect();
      term.dispose();
      logsTermRef.current = null;
      logsFitRef.current = null;
    };
  }, [vmState]);

  const isActive = vmState === "running" || vmState === "starting";

  return (
    <>
      <div className="console-drawer" style={{ height }}>
        <div
          className={`console-resize-handle ${showOverlay ? "dragging" : ""}`}
          onPointerDown={handlePointerDownOnHandle}
        />
        <div className="console-drawer-header">
          <div className="console-drawer-tabs">
            <button
              className={activeTab === "console" ? "active" : ""}
              onClick={() => setActiveTab("console")}
            >
              Console
            </button>
            <button
              className={activeTab === "logs" ? "active" : ""}
              onClick={() => setActiveTab("logs")}
            >
              Logs
            </button>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            {isActive && activeTab === "console" && (
              <>
                <span style={{ color: "var(--text-faded)", fontSize: 11 }}>
                  Login as ubuntu
                </span>
                <button
                  className="console-copy-pw-btn"
                  onClick={() => {
                    GetConsolePassword().then((pw) => {
                      navigator.clipboard.writeText(pw);
                    });
                  }}
                  title="Copy password to clipboard"
                >
                  Copy password
                </button>
              </>
            )}
            <button
              className="panel-close"
              onClick={onClose}
              style={{ width: 24, height: 24, fontSize: 16 }}
            >
              &times;
            </button>
          </div>
        </div>
        {isActive ? (
          <>
            <div
              className="console-terminal"
              ref={consoleContainerRef}
              style={{ display: activeTab === "console" ? undefined : "none" }}
            />
            <div
              className="console-terminal"
              ref={logsContainerRef}
              style={{ display: activeTab === "logs" ? undefined : "none" }}
            />
            <div className="console-bottom-pad" />
          </>
        ) : (
          <div
            style={{ padding: 20, color: "var(--text-faded)", fontSize: 13 }}
          >
            Start the environment to access the serial console.
          </div>
        )}
      </div>

      {showOverlay &&
        createPortal(
          <div
            onPointerMove={handleOverlayPointerMove}
            onPointerUp={handleOverlayPointerUp}
            style={{
              position: "fixed",
              inset: 0,
              zIndex: 9999,
              cursor: "ns-resize",
            }}
          />,
          document.body,
        )}
    </>
  );
}
