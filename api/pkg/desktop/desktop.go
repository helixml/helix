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
}

// Server is the main desktop integration server.
// It manages D-Bus sessions for video/input and serves HTTP APIs.
type Server struct {
	// D-Bus session state
	conn                 *dbus.Conn
	rdSessionPath        dbus.ObjectPath
	scSessionPath        dbus.ObjectPath
	scStreamPath         dbus.ObjectPath
	nodeID               uint32
	standaloneScreenCast bool // true when ScreenCast is not linked to RemoteDesktop (GNOME 49+)

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
	moveCount int
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

	// Only do D-Bus session setup if we're in GNOME environment
	if isGNOMEEnvironment() {
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

		// 4. Create input socket
		if err := s.createInputSocket(); err != nil {
			return fmt.Errorf("create input socket: %w", err)
		}
		defer s.inputListener.Close()
		defer os.Remove(s.inputSocketPath)

		// 5. Report to Wolf
		s.reportToWolf()

		// Start input bridge
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runInputBridge(ctx)
		}()

		// 6. Start session monitor (detects session closure and recreates)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.monitorSession(ctx)
		}()
	} else {
		s.logger.Info("not GNOME environment, skipping D-Bus session setup")
	}

	// 6. Start HTTP server
	s.running.Store(true)

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
