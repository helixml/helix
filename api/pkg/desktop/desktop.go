// Package desktop provides desktop integration for Helix sandboxes.
// It manages GNOME RemoteDesktop/ScreenCast D-Bus sessions for video streaming
// and input injection, and provides HTTP APIs for screenshots, clipboard, etc.
package desktop

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

// Config holds server configuration.
type Config struct {
	HTTPPort       string // HTTP server port (default: 9876)
	WolfSocketPath string // Wolf lobby socket path
	XDGRuntimeDir  string // XDG_RUNTIME_DIR for sockets
	SessionID      string // HELIX_SESSION_ID for Wolf
	WolfFreeMode   bool   // Skip Wolf socket reporting (for Hydra/direct WebSocket streaming)
}

// Server is the main desktop integration server.
// It manages D-Bus sessions for video/input and serves HTTP APIs.
type Server struct {
	// D-Bus session state (for Wolf video + input)
	conn          *dbus.Conn
	rdSessionPath dbus.ObjectPath
	scSessionPath dbus.ObjectPath
	scStreamPath  dbus.ObjectPath
	nodeID        uint32

	// Screenshot-dedicated ScreenCast session (separate from Wolf's video)
	// This avoids buffer renegotiation conflicts when capturing screenshots
	ssScSessionPath dbus.ObjectPath
	ssScStreamPath  dbus.ObjectPath
	ssNodeID        uint32

	// Portal session state (for Sway/wlroots via xdg-desktop-portal-wlr)
	portalSessionHandle   string // ScreenCast session handle
	portalRDSessionHandle string // RemoteDesktop session handle (optional)
	compositorType        string // "gnome", "sway", or "unknown"

	// Virtual input for Sway (uinput-based, no D-Bus RemoteDesktop)
	virtualInput *VirtualInput

	// Input socket
	inputListener   net.Listener
	inputSocketPath string

	// Configuration
	config Config

	// Lifecycle
	running atomic.Bool
	wg      sync.WaitGroup
	logger  *slog.Logger

	// Stats
	moveCount      int
	scrollLogCount int

	// Scroll finish detection - send "scroll finished" to Mutter after timeout
	scrollFinishTimer *time.Timer
	scrollFinishMu    sync.Mutex

	// Screenshot serialization - GNOME D-Bus Screenshot API only allows
	// one operation at a time per sender. Concurrent calls return
	// "There is an ongoing operation for this sender" error.
	screenshotMu sync.Mutex

	// Video forwarder - captures PipeWire inside container and forwards to Wolf via SHM
	videoForwarder *VideoForwarder
}

// NewServer creates a new desktop server with the given config.
func NewServer(cfg Config, logger *slog.Logger) *Server {
	if cfg.HTTPPort == "" {
		cfg.HTTPPort = "9876"
	}
	if cfg.WolfSocketPath == "" {
		cfg.WolfSocketPath = "/var/run/wolf/lobby.sock"
	}
	if cfg.XDGRuntimeDir == "" {
		cfg.XDGRuntimeDir = "/run/user/1000"
	}

	return &Server{
		config:          cfg,
		inputSocketPath: cfg.XDGRuntimeDir + "/wolf-input.sock",
		logger:          logger,
	}
}

// Run starts the server and blocks until context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting desktop server",
		"port", s.config.HTTPPort,
		"session_id", s.config.SessionID,
	)

	// Detect compositor type and setup D-Bus sessions accordingly
	s.compositorType = s.detectCompositor()
	s.logger.Info("detected compositor", "type", s.compositorType)

	if isGNOMEEnvironment() {
		// GNOME: Use native Mutter ScreenCast/RemoteDesktop D-Bus APIs
		s.compositorType = "gnome"

		// 1. Connect to D-Bus (with retry)
		if err := s.connectDBus(ctx); err != nil {
			return fmt.Errorf("dbus connect: %w", err)
		}
		defer s.conn.Close()

		// 2. Create RemoteDesktop + ScreenCast sessions
		if err := s.createSession(ctx); err != nil {
			return fmt.Errorf("create session: %w", err)
		}

		// 3. Start session, get PipeWire node ID
		if err := s.startSession(ctx); err != nil {
			return fmt.Errorf("start session: %w", err)
		}

		// 4. Prime keyboard input with a dummy Escape key press+release
		// GNOME's RemoteDesktop keyboard handling requires "priming" - the very first
		// keyboard event is silently dropped. By sending a harmless Escape key at startup,
		// we ensure the user's first real keypress works correctly.
		s.primeKeyboardInput()

		// 5. Create input socket
		if err := s.createInputSocket(); err != nil {
			return fmt.Errorf("create input socket: %w", err)
		}
		defer s.inputListener.Close()
		defer os.Remove(s.inputSocketPath)

		// 6. Start video forwarder - captures from PipeWire inside container
		// and forwards to Wolf via shared memory. This bypasses the cross-container
		// PipeWire authorization issue that prevents Wolf from directly consuming
		// the ScreenCast stream.
		shmSocketPath := filepath.Join(s.config.XDGRuntimeDir, "helix-video.sock")
		s.videoForwarder = NewVideoForwarder(s.nodeID, shmSocketPath, s.logger)
		if err := s.videoForwarder.Start(ctx); err != nil {
			s.logger.Warn("failed to start video forwarder, Wolf will try direct PipeWire",
				"err", err)
		}

		// 7. Report to Wolf - includes both node ID (for direct access attempt)
		// and SHM socket path (for forwarder mode)
		s.reportToWolf()

		// 6b. Create dedicated screenshot ScreenCast session (separate from Wolf's video)
		// This standalone session gets DmaBuf with NVIDIA modifiers, but it's reserved
		// for fast PipeWire-based screenshots - NOT for Wolf video streaming.
		if err := s.createScreenshotSession(ctx); err != nil {
			// Non-fatal - fall back to D-Bus Screenshot API
			s.logger.Warn("failed to create screenshot session, will use D-Bus Screenshot API",
				"err", err)
		} else {
			s.logger.Info("screenshot session ready",
				"screenshot_node_id", s.ssNodeID,
				"video_node_id", s.nodeID)
		}

		// Mark as running BEFORE starting goroutines that check isRunning()
		// CRITICAL: This fixes a race condition where input bridge would exit
		// immediately because s.running was false when the goroutine started.
		s.running.Store(true)

		// 7. Start input bridge
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runInputBridge(ctx)
		}()

		// 8. Start session monitor (detects session closure and recreates)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.monitorSession(ctx)
		}()
	} else if isSwayEnvironment() {
		// Sway: Use XDG Desktop Portal D-Bus APIs (via xdg-desktop-portal-wlr)
		// Sway runs as a headless compositor, xdg-desktop-portal-wlr captures via wlr-screencopy,
		// exposes frames via PipeWire, and our GStreamer pipeline consumes directly.
		s.compositorType = "sway"

		// 1. Connect to D-Bus portal (with retry)
		if err := s.connectDBusPortal(ctx); err != nil {
			return fmt.Errorf("dbus portal connect: %w", err)
		}
		defer s.conn.Close()

		// 2. Create ScreenCast session via portal
		if err := s.createPortalSession(ctx); err != nil {
			return fmt.Errorf("create portal session: %w", err)
		}

		// 3. Start portal session, get PipeWire node ID
		if err := s.startPortalSession(ctx); err != nil {
			return fmt.Errorf("start portal session: %w", err)
		}

		// 4. Optional: Create RemoteDesktop session for input (Sway may use uinput instead)
		if err := s.createPortalRemoteDesktopSession(ctx); err != nil {
			s.logger.Warn("portal RemoteDesktop not available, using uinput virtual input",
				"err", err)
			// Create uinput virtual devices for keyboard/mouse input
			vi, err := NewVirtualInput(s.logger)
			if err != nil {
				s.logger.Warn("failed to create virtual input devices", "err", err)
			} else {
				s.virtualInput = vi
			}
		}

		// Note: No video forwarder needed for Sway - we consume directly from PipeWire
		// via GStreamer pipewiresrc in ws_stream.go. The node ID from the portal session
		// is all we need.

		s.running.Store(true)
		s.logger.Info("Sway portal session ready",
			"node_id", s.nodeID,
			"session_handle", s.portalSessionHandle)
	} else {
		s.logger.Info("unknown compositor environment, skipping D-Bus session setup")
		// In non-GNOME/non-Sway mode, still set running for HTTP server
		s.running.Store(true)
	}

	// 9. Start HTTP server

	httpServer := &http.Server{
		Addr:    ":" + s.config.HTTPPort,
		Handler: s.httpHandler(),
	}

	errCh := make(chan error, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("HTTP server starting", "port", s.config.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http: %w", err)
		}
	}()

	// Wait for shutdown or error
	select {
	case <-ctx.Done():
		s.logger.Info("shutting down...")
	case err := <-errCh:
		return err
	}

	s.running.Store(false)

	// Stop video forwarder
	if s.videoForwarder != nil {
		s.videoForwarder.Stop()
	}

	// Graceful HTTP shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)

	// Stop D-Bus session
	if s.rdSessionPath != "" && s.conn != nil {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		rdSession.Call(remoteDesktopSessionIface+".Stop", 0)
	}

	// Close virtual input devices
	if s.virtualInput != nil {
		s.virtualInput.Close()
	}

	s.wg.Wait()
	return ctx.Err()
}

// httpHandler returns the HTTP handler with all routes.
func (s *Server) httpHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/screenshot", s.handleScreenshot)
	mux.HandleFunc("/clipboard", s.handleClipboard)
	mux.HandleFunc("/keyboard-state", s.handleKeyboardState)
	mux.HandleFunc("/keyboard-reset", s.handleKeyboardReset)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/input", s.handleInput)
	mux.HandleFunc("/ws/input", s.handleWSInput)   // Direct WebSocket input (bypasses Moonlight/Wolf)
	mux.HandleFunc("/ws/stream", s.handleWSStream) // Direct WebSocket video streaming (bypasses Wolf)
	mux.HandleFunc("/exec", s.handleExec)          // Execute command in container (for benchmarking)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return mux
}

// isRunning returns whether the server is still running.
func (s *Server) isRunning() bool {
	return s.running.Load()
}

// handleWSStream handles the combined WebSocket video+input streaming endpoint.
// This streams H.264 encoded video directly from PipeWire to the browser
// and handles input events in the same connection, bypassing Wolf and Moonlight-Web entirely.
// Uses the exact same protocol as moonlight-web-stream for frontend compatibility.
func (s *Server) handleWSStream(w http.ResponseWriter, r *http.Request) {
	if s.nodeID == 0 {
		s.logger.Error("cannot stream video: no PipeWire node ID available")
		http.Error(w, "Video streaming not available - no screen capture session", http.StatusServiceUnavailable)
		return
	}
	s.handleStreamWebSocketWithServer(w, r)
}
