package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// D-Bus constants for GNOME Mutter.
const (
	remoteDesktopBus          = "org.gnome.Mutter.RemoteDesktop"
	remoteDesktopPath         = "/org/gnome/Mutter/RemoteDesktop"
	remoteDesktopIface        = "org.gnome.Mutter.RemoteDesktop"
	remoteDesktopSessionIface = "org.gnome.Mutter.RemoteDesktop.Session"

	screenCastBus          = "org.gnome.Mutter.ScreenCast"
	screenCastPath         = "/org/gnome/Mutter/ScreenCast"
	screenCastIface        = "org.gnome.Mutter.ScreenCast"
	screenCastSessionIface = "org.gnome.Mutter.ScreenCast.Session"
	screenCastStreamIface  = "org.gnome.Mutter.ScreenCast.Stream"
)

// connectDBus connects to session D-Bus with retry.
func (s *Server) connectDBus(ctx context.Context) error {
	s.logger.Info("connecting to D-Bus...")

	var err error
	for attempt := 0; attempt < 60; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.logger.Debug("D-Bus connection attempt", "attempt", attempt+1)

		s.conn, err = dbus.ConnectSessionBus()
		if err != nil {
			s.logger.Debug("D-Bus not ready", "err", err)
			time.Sleep(time.Second)
			continue
		}

		// Verify RemoteDesktop service is available
		rdObj := s.conn.Object(remoteDesktopBus, remoteDesktopPath)
		if err := rdObj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
			s.logger.Debug("RemoteDesktop service not ready", "err", err)
			s.conn.Close()
			time.Sleep(time.Second)
			continue
		}

		// Also verify ScreenCast service is available
		scObj := s.conn.Object(screenCastBus, screenCastPath)
		if err := scObj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
			s.logger.Debug("ScreenCast service not ready", "err", err)
			s.conn.Close()
			time.Sleep(time.Second)
			continue
		}

		s.logger.Info("D-Bus connected")
		return nil
	}

	return fmt.Errorf("failed to connect after 60 attempts: %w", err)
}

// createSession creates linked RemoteDesktop and ScreenCast sessions.
// Both sessions must be created and linked for input injection to work properly.
func (s *Server) createSession(ctx context.Context) error {
	// Create RemoteDesktop session (required for input forwarding)
	s.logger.Info("creating RemoteDesktop session...")
	rdObj := s.conn.Object(remoteDesktopBus, remoteDesktopPath)

	var rdSessionPath dbus.ObjectPath
	if err := rdObj.Call(remoteDesktopIface+".CreateSession", 0).Store(&rdSessionPath); err != nil {
		return fmt.Errorf("create RemoteDesktop session: %w", err)
	}
	s.rdSessionPath = rdSessionPath
	s.logger.Info("RemoteDesktop session created", "path", rdSessionPath)

	// Read the SessionId property from the RemoteDesktop session
	// This is more reliable than extracting from the path
	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	var sessionID string
	var sessionIDVariant dbus.Variant
	if err := rdSession.Call("org.freedesktop.DBus.Properties.Get", 0,
		remoteDesktopSessionIface, "SessionId").Store(&sessionIDVariant); err != nil {
		s.logger.Warn("failed to read SessionId property, falling back to path extraction", "err", err)
		// Fallback: Extract session ID from path (e.g., "u1" from "/org/gnome/Mutter/RemoteDesktop/Session/u1")
		sessionID = string(s.rdSessionPath)
		if idx := strings.LastIndex(sessionID, "/"); idx >= 0 {
			sessionID = sessionID[idx+1:]
		}
		s.logger.Info("extracted session ID from path", "session_id", sessionID)
	} else {
		sessionID = sessionIDVariant.Value().(string)
		s.logger.Info("got SessionId from property", "session_id", sessionID)
	}

	// Small delay to let the session fully initialize
	time.Sleep(100 * time.Millisecond)

	// Create linked ScreenCast session - this is REQUIRED for NotifyPointerMotionAbsolute to work
	s.logger.Info("creating linked ScreenCast session...", "session_id", sessionID)
	scObj := s.conn.Object(screenCastBus, screenCastPath)
	options := map[string]dbus.Variant{
		"remote-desktop-session-id": dbus.MakeVariant(sessionID),
	}

	var scSessionPath dbus.ObjectPath
	var linkErr error
	for attempt := 0; attempt < 5; attempt++ {
		linkErr = scObj.Call(screenCastIface+".CreateSession", 0, options).Store(&scSessionPath)
		if linkErr == nil {
			s.logger.Info("linked ScreenCast session created", "path", scSessionPath)
			break
		}
		s.logger.Warn("linked ScreenCast attempt failed", "attempt", attempt+1, "err", linkErr)
		time.Sleep(500 * time.Millisecond)
	}
	if linkErr != nil {
		return fmt.Errorf("create linked ScreenCast session (session_id=%s): %w", sessionID, linkErr)
	}
	s.scSessionPath = scSessionPath

	// Record the virtual monitor Meta-0
	s.logger.Info("recording virtual monitor Meta-0...")
	scSession := s.conn.Object(screenCastBus, scSessionPath)

	recordOptions := map[string]dbus.Variant{
		"cursor-mode": dbus.MakeVariant(uint32(1)), // Embedded cursor
		// NOTE: Do NOT use is-platform=true here!
		// While the docs suggest it "bypasses screen sharing optimizations", it actually
		// forces GNOME to use SHM-only formats instead of DmaBuf with NVIDIA modifiers.
		// Without is-platform, GNOME offers DmaBuf with tiled modifiers for zero-copy GPU rendering.
	}

	var streamPath dbus.ObjectPath
	if err := scSession.Call(screenCastSessionIface+".RecordMonitor", 0, "Meta-0", recordOptions).Store(&streamPath); err != nil {
		return fmt.Errorf("RecordMonitor: %w", err)
	}
	s.scStreamPath = streamPath
	s.logger.Info("stream created", "path", streamPath)

	return nil
}

// createScreenshotSession creates a standalone ScreenCast session for screenshots.
// This is SEPARATE from the Wolf video session to avoid buffer renegotiation conflicts.
// Unlike the Wolf session, this is NOT linked to RemoteDesktop (no input needed).
func (s *Server) createScreenshotSession(ctx context.Context) error {
	s.logger.Info("creating standalone ScreenCast session for screenshots...")

	scObj := s.conn.Object(screenCastBus, screenCastPath)

	// Create standalone session (no remote-desktop-session-id = not linked)
	var scSessionPath dbus.ObjectPath
	if err := scObj.Call(screenCastIface+".CreateSession", 0, map[string]dbus.Variant{}).Store(&scSessionPath); err != nil {
		return fmt.Errorf("create screenshot ScreenCast session: %w", err)
	}
	s.ssScSessionPath = scSessionPath
	s.logger.Info("screenshot ScreenCast session created", "path", scSessionPath)

	// Record the virtual monitor Meta-0
	scSession := s.conn.Object(screenCastBus, scSessionPath)
	recordOptions := map[string]dbus.Variant{
		"cursor-mode": dbus.MakeVariant(uint32(1)), // Embedded cursor
	}

	var streamPath dbus.ObjectPath
	if err := scSession.Call(screenCastSessionIface+".RecordMonitor", 0, "Meta-0", recordOptions).Store(&streamPath); err != nil {
		return fmt.Errorf("screenshot RecordMonitor: %w", err)
	}
	s.ssScStreamPath = streamPath
	s.logger.Info("screenshot stream created", "path", streamPath)

	// Subscribe to PipeWireStreamAdded signal for this stream
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(s.ssScStreamPath),
		dbus.WithMatchInterface(screenCastStreamIface),
		dbus.WithMatchMember("PipeWireStreamAdded"),
	); err != nil {
		return fmt.Errorf("add screenshot signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)

	// Start the standalone ScreenCast session
	s.logger.Info("starting screenshot ScreenCast session...")
	if err := scSession.Call(screenCastSessionIface+".Start", 0).Err; err != nil {
		return fmt.Errorf("start screenshot ScreenCast session: %w", err)
	}

	// Wait for PipeWire node ID
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signalChan:
			if sig.Name == screenCastStreamIface+".PipeWireStreamAdded" &&
				sig.Path == s.ssScStreamPath && len(sig.Body) > 0 {
				if nodeID, ok := sig.Body[0].(uint32); ok {
					s.ssNodeID = nodeID
					s.logger.Info("screenshot PipeWireStreamAdded signal received",
						"node_id", nodeID,
						"wolf_node_id", s.nodeID)
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for screenshot PipeWireStreamAdded signal")
		}
	}
}

// startSession starts the session and waits for PipeWire node ID.
func (s *Server) startSession(ctx context.Context) error {
	s.logger.Info("setting up PipeWireStreamAdded signal handler...")

	// Subscribe to signals
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(s.scStreamPath),
		dbus.WithMatchInterface(screenCastStreamIface),
		dbus.WithMatchMember("PipeWireStreamAdded"),
	); err != nil {
		return fmt.Errorf("add signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)

	// Start the RemoteDesktop session - this also starts the linked ScreenCast
	s.logger.Info("starting RemoteDesktop session (linked mode)...")
	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	if err := rdSession.Call(remoteDesktopSessionIface+".Start", 0).Err; err != nil {
		return fmt.Errorf("start RemoteDesktop session: %w", err)
	}
	s.logger.Info("session started, waiting for PipeWireStreamAdded signal...")

	// Wait for signal with timeout
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signalChan:
			if sig.Name == screenCastStreamIface+".PipeWireStreamAdded" && len(sig.Body) > 0 {
				if nodeID, ok := sig.Body[0].(uint32); ok {
					s.nodeID = nodeID
					s.logger.Info("received PipeWireStreamAdded signal", "node_id", nodeID)

					// Save node ID to file for compatibility
					if err := os.WriteFile("/tmp/pipewire-node-id", []byte(fmt.Sprintf("%d", nodeID)), 0644); err != nil {
						s.logger.Warn("failed to save node ID to file", "err", err)
					}
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for PipeWireStreamAdded signal")
		}
	}
}

// monitorSession monitors the D-Bus session for closure and handles re-creation.
// This is critical because GNOME ScreenCast sessions can close unexpectedly,
// which causes Wolf's pipewiresrc to timeout waiting for frames.
func (s *Server) monitorSession(ctx context.Context) {
	s.logger.Info("starting D-Bus session monitor...")

	// Subscribe to Closed signal on ScreenCast session
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(s.scSessionPath),
		dbus.WithMatchInterface(screenCastSessionIface),
		dbus.WithMatchMember("Closed"),
	); err != nil {
		s.logger.Error("failed to subscribe to ScreenCast session Closed signal", "err", err)
		return
	}

	// Also subscribe to Closed signal on RemoteDesktop session if we have one
	if s.rdSessionPath != "" {
		if err := s.conn.AddMatchSignal(
			dbus.WithMatchObjectPath(s.rdSessionPath),
			dbus.WithMatchInterface(remoteDesktopSessionIface),
			dbus.WithMatchMember("Closed"),
		); err != nil {
			s.logger.Error("failed to subscribe to RemoteDesktop session Closed signal", "err", err)
		}
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)

	// Periodic health check - verify session is still valid
	healthTicker := time.NewTicker(10 * time.Second)
	defer healthTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("session monitor shutting down")
			return

		case sig := <-signalChan:
			// Check for session closed signals
			if sig.Name == screenCastSessionIface+".Closed" {
				s.logger.Warn("ScreenCast session closed unexpectedly!",
					"path", sig.Path,
					"body", sig.Body)

				// Try to recreate the session
				s.handleSessionClosed(ctx)
			}
			if sig.Name == remoteDesktopSessionIface+".Closed" {
				s.logger.Warn("RemoteDesktop session closed unexpectedly!",
					"path", sig.Path,
					"body", sig.Body)

				// Try to recreate the session
				s.handleSessionClosed(ctx)
			}

		case <-healthTicker.C:
			// Periodic health check - try to read a property from the session
			if s.scSessionPath != "" {
				scSession := s.conn.Object(screenCastBus, s.scSessionPath)
				// Introspect to check if session still exists
				if err := scSession.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
					s.logger.Error("ScreenCast session health check failed - session may have died",
						"err", err,
						"path", s.scSessionPath)

					// Session is dead, try to recreate
					s.handleSessionClosed(ctx)
				} else {
					s.logger.Debug("session health check OK",
						"sc_session", s.scSessionPath,
						"node_id", s.nodeID)
				}
			}
		}
	}
}

// handleSessionClosed handles session closure by attempting to recreate it.
func (s *Server) handleSessionClosed(ctx context.Context) {
	s.logger.Info("attempting to recreate D-Bus session...")

	// Clear old session state
	s.rdSessionPath = ""
	s.scSessionPath = ""
	s.scStreamPath = ""
	s.nodeID = 0

	// Recreate session
	if err := s.createSession(ctx); err != nil {
		s.logger.Error("failed to recreate session", "err", err)
		return
	}

	// Restart session to get new PipeWire node ID
	if err := s.startSession(ctx); err != nil {
		s.logger.Error("failed to restart session", "err", err)
		return
	}

	// Report new node ID to Wolf
	s.reportToWolf()

	s.logger.Info("session recreated successfully", "new_node_id", s.nodeID)
}

// reportToWolf reports node ID, SHM socket path, and input socket to Wolf with retry.
// Wolf's lobby socket may not be ready immediately, so we retry with backoff.
//
// Video capture modes:
// 1. SHM mode (preferred): If video forwarder is running, Wolf reads from shmsrc
// 2. Direct mode (fallback): Wolf tries to connect directly to PipeWire via node_id
//
// Direct mode fails for cross-container access due to PipeWire authorization.
// SHM mode works because the forwarder runs inside the container with PipeWire.
func (s *Server) reportToWolf() {
	// Determine SHM socket path if video forwarder is running
	var shmSocketPath string
	if s.videoForwarder != nil && s.videoForwarder.IsRunning() {
		shmSocketPath = s.videoForwarder.ShmSocketPath()
	}

	s.logger.Info("session summary",
		"rd_session", s.rdSessionPath,
		"sc_stream", s.scStreamPath,
		"node_id", s.nodeID,
		"shm_socket", shmSocketPath,
		"input_socket", s.inputSocketPath,
	)

	// Retry reporting to Wolf with exponential backoff
	// Wolf's lobby socket may not be ready when the container starts
	maxRetries := 30
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Check if socket file exists
		if _, err := os.Stat(s.config.WolfSocketPath); os.IsNotExist(err) {
			s.logger.Debug("Wolf socket not found, retrying...",
				"path", s.config.WolfSocketPath,
				"attempt", attempt+1)
			time.Sleep(time.Second)
			continue
		}

		// Try to report node ID and SHM socket path
		s.logger.Info("reporting video config to Wolf...",
			"node_id", s.nodeID,
			"shm_socket", shmSocketPath,
			"attempt", attempt+1)

		// Include SHM socket path in the request - Wolf will use this for SHM mode
		// if available, otherwise fall back to direct PipeWire access
		requestData := map[string]interface{}{
			"node_id":      s.nodeID,
			"session_path": string(s.rdSessionPath),
		}
		if shmSocketPath != "" {
			requestData["shm_socket_path"] = shmSocketPath
		}

		lastErr = s.postToWolfWithError("/set-pipewire-node-id", requestData)

		if lastErr != nil {
			s.logger.Debug("Wolf not ready, retrying...", "err", lastErr, "attempt", attempt+1)
			time.Sleep(time.Second)
			continue
		}

		// Node ID reported successfully, now report input socket
		s.logger.Info("reporting input socket to Wolf...")
		if err := s.postToWolfWithError("/set-input-socket", map[string]interface{}{
			"input_socket": s.inputSocketPath,
		}); err != nil {
			s.logger.Warn("failed to report input socket", "err", err)
		}

		s.logger.Info("successfully reported to Wolf",
			"node_id", s.nodeID,
			"shm_socket", shmSocketPath)
		return
	}

	s.logger.Error("failed to report to Wolf after retries",
		"attempts", maxRetries,
		"last_err", lastErr)
}

// postToWolfWithError posts to Wolf and returns any error for retry handling.
func (s *Server) postToWolfWithError(endpoint string, data map[string]interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", s.config.WolfSocketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Post("http://localhost"+endpoint, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	s.logger.Info("Wolf response", "endpoint", endpoint, "status", resp.Status)
	return nil
}
