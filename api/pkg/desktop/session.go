package desktop

import (
	"context"
	"fmt"
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
		"cursor-mode": dbus.MakeVariant(uint32(2)), // Metadata - cursor sent as PipeWire metadata, not rendered into video
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
	s.logger.Info("stream created (cursor as metadata for client-side rendering)", "path", streamPath)

	return nil
}

// createScreenshotSession creates a standalone ScreenCast session for screenshots.
// This is SEPARATE from the video streaming session to avoid buffer renegotiation conflicts.
// Unlike the video session, this is NOT linked to RemoteDesktop (no input needed).
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
						"video_node_id", s.nodeID)
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for screenshot PipeWireStreamAdded signal")
		}
	}
}

// createNoCursorScreenshotSession creates a standalone ScreenCast session for no-cursor screenshots.
// This is used for video polling mode where we don't want cursors in the screenshots
// (the frontend renders its own cursor overlay based on cursor metadata).
// Unlike the regular screenshot session, this uses cursor-mode=0 (Hidden).
func (s *Server) createNoCursorScreenshotSession(ctx context.Context) error {
	s.logger.Info("creating standalone ScreenCast session for no-cursor screenshots...")

	scObj := s.conn.Object(screenCastBus, screenCastPath)

	// Create standalone session (no remote-desktop-session-id = not linked)
	var scSessionPath dbus.ObjectPath
	if err := scObj.Call(screenCastIface+".CreateSession", 0, map[string]dbus.Variant{}).Store(&scSessionPath); err != nil {
		return fmt.Errorf("create no-cursor screenshot ScreenCast session: %w", err)
	}
	s.ssNoCursorScSessionPath = scSessionPath
	s.logger.Info("no-cursor screenshot ScreenCast session created", "path", scSessionPath)

	// Record the virtual monitor Meta-0 with cursor HIDDEN
	scSession := s.conn.Object(screenCastBus, scSessionPath)
	recordOptions := map[string]dbus.Variant{
		"cursor-mode": dbus.MakeVariant(uint32(0)), // Hidden - no cursor in output
	}

	var streamPath dbus.ObjectPath
	if err := scSession.Call(screenCastSessionIface+".RecordMonitor", 0, "Meta-0", recordOptions).Store(&streamPath); err != nil {
		return fmt.Errorf("no-cursor screenshot RecordMonitor: %w", err)
	}
	s.ssNoCursorScStreamPath = streamPath
	s.logger.Info("no-cursor screenshot stream created", "path", streamPath)

	// Subscribe to PipeWireStreamAdded signal for this stream
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(s.ssNoCursorScStreamPath),
		dbus.WithMatchInterface(screenCastStreamIface),
		dbus.WithMatchMember("PipeWireStreamAdded"),
	); err != nil {
		return fmt.Errorf("add no-cursor screenshot signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)

	// Start the standalone ScreenCast session
	s.logger.Info("starting no-cursor screenshot ScreenCast session...")
	if err := scSession.Call(screenCastSessionIface+".Start", 0).Err; err != nil {
		return fmt.Errorf("start no-cursor screenshot ScreenCast session: %w", err)
	}

	// Wait for PipeWire node ID
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signalChan:
			if sig.Name == screenCastStreamIface+".PipeWireStreamAdded" &&
				sig.Path == s.ssNoCursorScStreamPath && len(sig.Body) > 0 {
				if nodeID, ok := sig.Body[0].(uint32); ok {
					s.ssNoCursorNodeID = nodeID
					s.logger.Info("no-cursor screenshot PipeWireStreamAdded signal received",
						"node_id", nodeID,
						"cursor_screenshot_node_id", s.ssNodeID)
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for no-cursor screenshot PipeWireStreamAdded signal")
		}
	}
}

// createVideoSession creates a standalone ScreenCast session for video streaming.
// CRITICAL: This is NOT linked to RemoteDesktop on purpose!
//
// Why: Linked ScreenCast sessions in GNOME headless mode don't offer DmaBuf modifiers.
// Without DmaBuf modifiers, pipewirezerocopysrc can only use SHM (MemFd) buffers,
// resulting in ~10 FPS instead of 60 FPS due to CPU memory copies.
//
// Standalone sessions DO offer DmaBuf with NVIDIA tiled modifiers, enabling true
// zero-copy GPU capture at 60 FPS.
//
// Input still works because NotifyPointerMotionAbsolute uses the LINKED session's
// stream path (s.scStreamPath) for coordinate reference - both sessions target
// the same virtual monitor (Meta-0).
func (s *Server) createVideoSession(ctx context.Context) error {
	s.logger.Info("creating standalone ScreenCast session for VIDEO (enables DmaBuf zero-copy)...")

	scObj := s.conn.Object(screenCastBus, screenCastPath)

	// Create STANDALONE session (no remote-desktop-session-id)
	// This is the key difference from createSession() which creates a LINKED session
	var scSessionPath dbus.ObjectPath
	if err := scObj.Call(screenCastIface+".CreateSession", 0, map[string]dbus.Variant{}).Store(&scSessionPath); err != nil {
		return fmt.Errorf("create video ScreenCast session: %w", err)
	}
	s.videoScSessionPath = scSessionPath
	s.logger.Info("video ScreenCast session created (standalone)", "path", scSessionPath)

	// Record the virtual monitor Meta-0
	scSession := s.conn.Object(screenCastBus, scSessionPath)
	recordOptions := map[string]dbus.Variant{
		"cursor-mode": dbus.MakeVariant(uint32(2)), // Metadata - cursor sent as PipeWire metadata, not rendered into video
		// NOTE: Do NOT use is-platform=true here!
		// While the docs suggest it "bypasses screen sharing optimizations", it actually
		// forces GNOME to use SHM-only formats instead of DmaBuf with NVIDIA modifiers.
	}

	var streamPath dbus.ObjectPath
	if err := scSession.Call(screenCastSessionIface+".RecordMonitor", 0, "Meta-0", recordOptions).Store(&streamPath); err != nil {
		return fmt.Errorf("video RecordMonitor: %w", err)
	}
	s.videoScStreamPath = streamPath
	s.logger.Info("video stream created (cursor as metadata for client-side rendering)", "path", streamPath)

	// Subscribe to PipeWireStreamAdded signal for this stream
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(s.videoScStreamPath),
		dbus.WithMatchInterface(screenCastStreamIface),
		dbus.WithMatchMember("PipeWireStreamAdded"),
	); err != nil {
		return fmt.Errorf("add video signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)

	// Start the standalone ScreenCast session
	s.logger.Info("starting video ScreenCast session...")
	if err := scSession.Call(screenCastSessionIface+".Start", 0).Err; err != nil {
		return fmt.Errorf("start video ScreenCast session: %w", err)
	}

	// Wait for PipeWire node ID
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig := <-signalChan:
			if sig.Name == screenCastStreamIface+".PipeWireStreamAdded" &&
				sig.Path == s.videoScStreamPath && len(sig.Body) > 0 {
				if nodeID, ok := sig.Body[0].(uint32); ok {
					s.videoNodeID = nodeID
					s.logger.Info("video PipeWireStreamAdded signal received (standalone session)",
						"video_node_id", nodeID,
						"input_node_id", s.nodeID,
						"note", "video uses standalone for DmaBuf, input uses linked for coordinates")
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for video PipeWireStreamAdded signal")
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
// which causes video streaming clients to timeout waiting for frames.
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

		case sig, ok := <-signalChan:
			// Channel closed or nil signal
			if !ok || sig == nil {
				s.logger.Debug("signal channel closed or nil signal received")
				continue
			}
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

			// Also check screenshot sessions - these can die independently
			s.checkScreenshotSessionHealth(ctx)
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

	s.logger.Info("session recreated successfully", "new_node_id", s.nodeID)
}

// checkScreenshotSessionHealth checks if screenshot sessions are still valid.
// If a screenshot session has died, recreate it. This prevents gst-launch from
// hanging on stale PipeWire node IDs.
func (s *Server) checkScreenshotSessionHealth(ctx context.Context) {
	// Check screenshot session with cursor (ssScSessionPath)
	if s.ssScSessionPath != "" {
		ssSession := s.conn.Object(screenCastBus, s.ssScSessionPath)
		if err := ssSession.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
			s.logger.Warn("screenshot session health check failed, recreating",
				"err", err,
				"path", s.ssScSessionPath)
			s.ssScSessionPath = ""
			s.ssScStreamPath = ""
			s.ssNodeID = 0

			if err := s.createScreenshotSession(ctx); err != nil {
				s.logger.Error("failed to recreate screenshot session", "err", err)
			} else {
				s.logger.Info("screenshot session recreated", "node_id", s.ssNodeID)
			}
		}
	}

	// Check no-cursor screenshot session (ssNoCursorScSessionPath)
	if s.ssNoCursorScSessionPath != "" {
		ssNoCursorSession := s.conn.Object(screenCastBus, s.ssNoCursorScSessionPath)
		if err := ssNoCursorSession.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
			s.logger.Warn("no-cursor screenshot session health check failed, recreating",
				"err", err,
				"path", s.ssNoCursorScSessionPath)
			s.ssNoCursorScSessionPath = ""
			s.ssNoCursorScStreamPath = ""
			s.ssNoCursorNodeID = 0

			if err := s.createNoCursorScreenshotSession(ctx); err != nil {
				s.logger.Error("failed to recreate no-cursor screenshot session", "err", err)
			} else {
				s.logger.Info("no-cursor screenshot session recreated", "node_id", s.ssNoCursorNodeID)
			}
		}
	}

	// Check standalone video session (videoScSessionPath)
	if s.videoScSessionPath != "" {
		videoSession := s.conn.Object(screenCastBus, s.videoScSessionPath)
		if err := videoSession.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
			s.logger.Warn("video session health check failed, recreating",
				"err", err,
				"path", s.videoScSessionPath)
			s.videoScSessionPath = ""
			s.videoScStreamPath = ""
			s.videoNodeID = 0

			if err := s.createVideoSession(ctx); err != nil {
				s.logger.Error("failed to recreate video session", "err", err)
			} else {
				s.logger.Info("video session recreated", "node_id", s.videoNodeID)
			}
		}
	}
}

