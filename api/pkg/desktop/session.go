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
	}

	var streamPath dbus.ObjectPath
	if err := scSession.Call(screenCastSessionIface+".RecordMonitor", 0, "Meta-0", recordOptions).Store(&streamPath); err != nil {
		return fmt.Errorf("RecordMonitor: %w", err)
	}
	s.scStreamPath = streamPath
	s.logger.Info("stream created", "path", streamPath)

	return nil
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

// reportToWolf reports node ID and input socket to Wolf.
func (s *Server) reportToWolf() {
	s.logger.Info("session summary",
		"rd_session", s.rdSessionPath,
		"sc_stream", s.scStreamPath,
		"node_id", s.nodeID,
		"input_socket", s.inputSocketPath,
	)

	if _, err := os.Stat(s.config.WolfSocketPath); os.IsNotExist(err) {
		s.logger.Warn("Wolf socket not found", "path", s.config.WolfSocketPath)
		return
	}

	// Report node ID
	s.logger.Info("reporting node ID to Wolf...")
	s.postToWolf("/set-pipewire-node-id", map[string]interface{}{
		"node_id":      s.nodeID,
		"session_path": string(s.rdSessionPath),
	})

	// Report input socket
	s.logger.Info("reporting input socket to Wolf...")
	s.postToWolf("/set-input-socket", map[string]interface{}{
		"input_socket": s.inputSocketPath,
	})
}

func (s *Server) postToWolf(endpoint string, data map[string]interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		s.logger.Error("failed to marshal data", "err", err)
		return
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
		s.logger.Error("failed to report to Wolf", "endpoint", endpoint, "err", err)
		return
	}
	defer resp.Body.Close()

	s.logger.Info("Wolf response", "endpoint", endpoint, "status", resp.Status)
}
