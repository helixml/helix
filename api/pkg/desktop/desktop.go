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
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

// Config holds server configuration.
type Config struct {
	HTTPPort      string // HTTP server port (default: 9876)
	XDGRuntimeDir string // XDG_RUNTIME_DIR for sockets
	SessionID     string // HELIX_SESSION_ID for session identification
}

// Server is the main desktop integration server.
// It manages D-Bus sessions for video/input and serves HTTP APIs.
type Server struct {
	// D-Bus session state (for video streaming + input)
	conn          *dbus.Conn
	rdSessionPath dbus.ObjectPath
	scSessionPath dbus.ObjectPath
	scStreamPath  dbus.ObjectPath
	nodeID        uint32
	pipeWireFd    int // PipeWire FD from OpenPipeWireRemote portal call

	// Screenshot-dedicated ScreenCast session (separate from video streaming)
	// This avoids buffer renegotiation conflicts when capturing screenshots
	ssScSessionPath dbus.ObjectPath
	ssScStreamPath  dbus.ObjectPath
	ssNodeID        uint32

	// Standalone video streaming ScreenCast session (NOT linked to RemoteDesktop)
	// CRITICAL: Linked sessions don't offer DmaBuf modifiers in GNOME headless mode!
	// This standalone session offers DmaBuf with NVIDIA tiled modifiers for zero-copy GPU capture.
	// Input still uses the linked session's stream path for NotifyPointerMotionAbsolute.
	videoScSessionPath dbus.ObjectPath
	videoScStreamPath  dbus.ObjectPath
	videoNodeID        uint32

	// Portal session state (for Sway/wlroots via xdg-desktop-portal-wlr)
	portalSessionHandle   string // ScreenCast session handle
	portalRDSessionHandle string // RemoteDesktop session handle (optional)
	compositorType        string // "gnome", "sway", or "unknown"

	// Wayland-native input for Sway (uses zwlr_virtual_pointer_v1 and zwp_virtual_keyboard_v1)
	waylandInput *WaylandInput

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
	inputCallCount uint64 // For D-Bus latency sampling

	// Scroll finish detection - send "scroll finished" to Mutter after timeout
	scrollFinishTimer *time.Timer
	scrollFinishMu    sync.Mutex

	// Screen dimensions for mouse coordinate scaling
	// Initialized from GAMESCOPE_WIDTH/HEIGHT env vars (default: 1920x1080)
	screenWidth  int
	screenHeight int

	// Display scale factor for Sway (from HELIX_ZOOM_LEVEL, default: 1.0)
	// With scale 2.0, physical 3840x2160 becomes logical 1920x1080
	// GNOME handles scaling internally so this is only used for Sway
	displayScale float64

	// Screenshot serialization - GNOME D-Bus Screenshot API only allows
	// one operation at a time per sender. Concurrent calls return
	// "There is an ongoing operation for this sender" error.
	screenshotMu sync.Mutex
}

// NewServer creates a new desktop server with the given config.
func NewServer(cfg Config, logger *slog.Logger) *Server {
	if cfg.HTTPPort == "" {
		cfg.HTTPPort = "9876"
	}
	if cfg.XDGRuntimeDir == "" {
		cfg.XDGRuntimeDir = "/run/user/1000"
	}

	// Read screen dimensions from environment (set by Dockerfile)
	// These should match the video resolution being captured/streamed
	screenWidth := 1920
	screenHeight := 1080
	if w := os.Getenv("GAMESCOPE_WIDTH"); w != "" {
		if parsed, err := strconv.Atoi(w); err == nil && parsed > 0 {
			screenWidth = parsed
		}
	}
	if h := os.Getenv("GAMESCOPE_HEIGHT"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil && parsed > 0 {
			screenHeight = parsed
		}
	}

	// Read display scale from HELIX_ZOOM_LEVEL (percentage, default 100)
	// Used for Sway to convert physical coords to logical coords
	displayScale := 1.0
	if z := os.Getenv("HELIX_ZOOM_LEVEL"); z != "" {
		if parsed, err := strconv.Atoi(z); err == nil && parsed > 0 {
			displayScale = float64(parsed) / 100.0
		}
	}

	logger.Info("screen dimensions for mouse scaling",
		"width", screenWidth,
		"height", screenHeight,
		"display_scale", displayScale)

	return &Server{
		config:          cfg,
		inputSocketPath: cfg.XDGRuntimeDir + "/helix-input.sock",
		logger:          logger,
		screenWidth:     screenWidth,
		screenHeight:    screenHeight,
		displayScale:    displayScale,
	}
}

// Run starts the server and blocks until context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting desktop server",
		"port", s.config.HTTPPort,
		"session_id", s.config.SessionID,
	)

	// Pre-initialize GStreamer to avoid 4-second delay on first video stream connection.
	// GStreamer initialization includes scanning for plugins which is slow on first call.
	InitGStreamer()
	s.logger.Info("GStreamer initialized")

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

		// 6. Both GNOME and Sway now use pipewirezerocopysrc directly
		// The shmsink/shmsrc approach (video forwarder) has been eliminated.
		// Both compositors use the same zero-copy DMA-BUF pipeline.
		s.logger.Info("GNOME: using pipewirezerocopysrc (zero-copy DMA-BUF)")

		// 7. Create standalone ScreenCast session for VIDEO STREAMING
		// CRITICAL: Linked sessions don't offer DmaBuf modifiers in GNOME headless mode!
		// This standalone session offers DmaBuf with NVIDIA modifiers for true zero-copy.
		// Input continues to use the linked session's stream path for NotifyPointerMotionAbsolute.
		if err := s.createVideoSession(ctx); err != nil {
			// Non-fatal - fall back to linked session (SHM path)
			s.logger.Warn("failed to create standalone video session, falling back to linked session",
				"err", err,
				"note", "video will use SHM path instead of DmaBuf zero-copy")
		} else {
			s.logger.Info("video session ready (standalone, DmaBuf enabled)",
				"video_node_id", s.videoNodeID,
				"input_node_id", s.nodeID)
		}

		// 8. Create dedicated screenshot ScreenCast session (separate from video streaming)
		// This is a THIRD standalone session to avoid buffer renegotiation conflicts
		// when capturing screenshots while video is streaming.
		if err := s.createScreenshotSession(ctx); err != nil {
			// Non-fatal - fall back to D-Bus Screenshot API
			s.logger.Warn("failed to create screenshot session, will use D-Bus Screenshot API",
				"err", err)
		} else {
			s.logger.Info("screenshot session ready",
				"screenshot_node_id", s.ssNodeID,
				"video_node_id", s.videoNodeID)
		}

		// Mark as running BEFORE starting goroutines that check isRunning()
		// CRITICAL: This fixes a race condition where input bridge would exit
		// immediately because s.running was false when the goroutine started.
		s.running.Store(true)

		// 9. Start input bridge
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runInputBridge(ctx)
		}()

		// 10. Start session monitor (detects session closure and recreates)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.monitorSession(ctx)
		}()
	} else if isSwayEnvironment() {
		// Sway: Use native Wayland protocols for video capture
		// Our pipewirezerocopysrc plugin uses ext-image-copy-capture (Sway 1.10+) or
		// wlr-screencopy (legacy) directly, bypassing xdg-desktop-portal-wlr entirely.
		// This avoids the portal ScreenCast interface which isn't properly supported.
		s.compositorType = "sway"

		// 1. Connect to D-Bus portal for RemoteDesktop (optional, for input forwarding)
		// NOTE: We don't use portal ScreenCast - pipewirezerocopysrc handles capture directly
		if err := s.connectDBusPortal(ctx); err != nil {
			s.logger.Warn("D-Bus portal connection failed (non-fatal for Sway)",
				"err", err,
				"note", "pipewirezerocopysrc will use ext-image-copy-capture directly")
		} else {
			defer s.conn.Close()

			// 2. Try to create ScreenCast session via portal (non-fatal)
			// This will likely fail because xdg-desktop-portal-wlr doesn't expose ScreenCast
			// But pipewirezerocopysrc doesn't need it - it uses ext-image-copy-capture directly
			if err := s.createPortalSession(ctx); err != nil {
				s.logger.Warn("Portal ScreenCast session failed (expected for Sway)",
					"err", err,
					"note", "pipewirezerocopysrc will use ext-image-copy-capture directly")
			} else {
				// 3. Start portal session, get PipeWire node ID (only if session created)
				if err := s.startPortalSession(ctx); err != nil {
					s.logger.Warn("Portal start failed (non-fatal)",
						"err", err)
				}
			}
		}

		// 4. Create Wayland-native virtual input for Sway
		// Uses zwlr_virtual_pointer_v1 and zwp_virtual_keyboard_v1 protocols
		// No /dev/uinput or root privileges required
		//
		// IMPORTANT: Use LOGICAL dimensions (physical / scale), not physical!
		// Sway's virtual pointer operates in logical coordinate space.
		// With scale=2.0, physical 3840x2160 becomes logical 1920x1080.
		logicalWidth := int(float64(s.screenWidth) / s.displayScale)
		logicalHeight := int(float64(s.screenHeight) / s.displayScale)
		wi, err := NewWaylandInput(s.logger, logicalWidth, logicalHeight)
		if err != nil {
			s.logger.Error("failed to create Wayland virtual input", "err", err)
			// This is a critical failure for Sway - input won't work without it
		} else {
			s.waylandInput = wi
			s.logger.Info("Wayland virtual input created for Sway",
				"physical", fmt.Sprintf("%dx%d", s.screenWidth, s.screenHeight),
				"logical", fmt.Sprintf("%dx%d", logicalWidth, logicalHeight),
				"scale", s.displayScale)
		}

		// 5. pipewirezerocopysrc uses native Wayland protocols for Sway
		// It automatically detects Sway and uses:
		// - ext-image-copy-capture-v1 (Sway 1.10+) - modern protocol with damage tracking
		// - wlr-screencopy-unstable-v1 (legacy fallback)
		// Both paths bypass PipeWire entirely - no portal node ID needed.
		// Video is captured directly from Sway via Wayland, then hardware encoded.

		s.running.Store(true)
		s.logger.Info("Sway session ready (using pipewirezerocopysrc with ext-image-copy-capture)",
			"note", "bypasses xdg-desktop-portal, uses native Wayland protocols")
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

	// Graceful HTTP shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)

	// Stop D-Bus session
	if s.rdSessionPath != "" && s.conn != nil {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		rdSession.Call(remoteDesktopSessionIface+".Stop", 0)
	}

	// Close Wayland input devices
	if s.waylandInput != nil {
		s.waylandInput.Close()
	}

	s.wg.Wait()
	return ctx.Err()
}

// httpHandler returns the HTTP handler with all routes.
func (s *Server) httpHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/screenshot", s.handleScreenshot)
	mux.HandleFunc("/clipboard", s.handleClipboard)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/input", s.handleInput)
	mux.HandleFunc("/ws/input", s.handleWSInput)   // Direct WebSocket input
	mux.HandleFunc("/ws/stream", s.handleWSStream) // Direct WebSocket video streaming
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
// and handles input events in the same connection.
//
// For GNOME: Uses PipeWire ScreenCast with nodeID from portal session.
// For Sway: Uses pipewirezerocopysrc with ext-image-copy-capture (no PipeWire needed).
func (s *Server) handleWSStream(w http.ResponseWriter, r *http.Request) {
	nodeID := s.nodeID // Use linked session
	if nodeID == 0 {
		nodeID = s.videoNodeID // Fallback to standalone
		if nodeID != 0 {
			s.logger.Warn("using standalone session for video (linked unavailable)",
				"fallback_node_id", nodeID)
		}
	} else {
		s.logger.Info("using linked session for video",
			"linked_node_id", nodeID,
			"standalone_node_id", s.videoNodeID)
	}

	// For Sway: nodeID can be 0 because pipewirezerocopysrc uses ext-image-copy-capture
	// directly via native Wayland protocols (bypasses PipeWire entirely).
	// The plugin detects Sway via XDG_CURRENT_DESKTOP and uses the appropriate capture method.
	if nodeID == 0 && s.compositorType != "sway" {
		s.logger.Error("cannot stream video: no PipeWire node ID available (GNOME requires portal)")
		http.Error(w, "Video streaming not available - no screen capture session", http.StatusServiceUnavailable)
		return
	}

	if nodeID == 0 {
		// Sway: use dummy node ID (pipewirezerocopysrc will ignore it and use ext-image-copy-capture)
		s.logger.Info("Sway mode: using ext-image-copy-capture (no PipeWire node needed)")
		nodeID = 1 // Dummy value - ignored by pipewirezerocopysrc for Sway
	}

	// Call handleStreamWebSocketInternal directly with our selected nodeID
	// (handleStreamWebSocketWithServer has its own logic that would override our choice)
	handleStreamWebSocketInternal(w, r, nodeID, s.logger, s)
}
