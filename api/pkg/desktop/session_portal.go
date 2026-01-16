package desktop

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// XDG Desktop Portal D-Bus constants (for Sway/wlroots via xdg-desktop-portal-wlr)
const (
	portalBus  = "org.freedesktop.portal.Desktop"
	portalPath = "/org/freedesktop/portal/desktop"

	portalScreenCastIface     = "org.freedesktop.portal.ScreenCast"
	portalRemoteDesktopIface  = "org.freedesktop.portal.RemoteDesktop"
	portalRequestIface        = "org.freedesktop.portal.Request"
	portalSessionIface        = "org.freedesktop.portal.Session"
)

// ScreenCast source types
const (
	portalSourceMonitor = uint32(1)
	portalSourceWindow  = uint32(2)
	portalSourceVirtual = uint32(4)
)

// Cursor modes
const (
	portalCursorHidden   = uint32(1)
	portalCursorEmbedded = uint32(2)
	portalCursorMetadata = uint32(4)
)

// detectCompositor detects which compositor is running.
// Returns "gnome" for GNOME/Mutter, "sway" for Sway/wlroots, or "unknown".
func (s *Server) detectCompositor() string {
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	sessionType := os.Getenv("XDG_SESSION_TYPE")

	s.logger.Info("detecting compositor",
		"XDG_CURRENT_DESKTOP", desktop,
		"XDG_SESSION_TYPE", sessionType)

	switch desktop {
	case "sway", "Sway":
		return "sway"
	case "GNOME", "gnome", "ubuntu:GNOME":
		return "gnome"
	default:
		// Try to detect by checking which D-Bus service is available
		if s.conn != nil {
			// Check for GNOME Mutter
			mutterObj := s.conn.Object(screenCastBus, screenCastPath)
			if err := mutterObj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err == nil {
				return "gnome"
			}
		}
		return "unknown"
	}
}

// connectDBusPortal connects to session D-Bus and waits for portal service.
func (s *Server) connectDBusPortal(ctx context.Context) error {
	s.logger.Info("connecting to D-Bus for portal...")

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

		// Verify portal service is available
		portalObj := s.conn.Object(portalBus, portalPath)
		if err := portalObj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
			s.logger.Debug("Portal service not ready", "err", err)
			s.conn.Close()
			time.Sleep(time.Second)
			continue
		}

		s.logger.Info("D-Bus connected (portal mode)")
		return nil
	}

	return fmt.Errorf("failed to connect to portal after 60 attempts: %w", err)
}

// createPortalSession creates a ScreenCast session via XDG Portal.
// This is used for Sway/wlroots compositors via xdg-desktop-portal-wlr.
func (s *Server) createPortalSession(ctx context.Context) error {
	s.logger.Info("creating portal ScreenCast session...")

	// Generate a unique session handle token
	sessionToken := fmt.Sprintf("helix_%d", time.Now().UnixNano())
	requestToken := fmt.Sprintf("req_%d", time.Now().UnixNano())

	// Get our D-Bus connection name for the request path
	senderName := s.conn.Names()[0]
	// Convert ":" and "." to "_" for the path
	senderPath := ""
	for _, c := range senderName[1:] { // Skip leading ":"
		if c == '.' {
			senderPath += "_"
		} else {
			senderPath += string(c)
		}
	}
	requestPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", senderPath, requestToken))

	s.logger.Debug("portal request path", "path", requestPath)

	// Subscribe to Response signal before making the call
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(requestPath),
		dbus.WithMatchInterface(portalRequestIface),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return fmt.Errorf("add signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)
	defer s.conn.RemoveSignal(signalChan)

	// Call CreateSession
	portalObj := s.conn.Object(portalBus, portalPath)
	options := map[string]dbus.Variant{
		"handle_token":  dbus.MakeVariant(requestToken),
		"session_handle_token": dbus.MakeVariant(sessionToken),
	}

	var returnedRequestPath dbus.ObjectPath
	if err := portalObj.Call(portalScreenCastIface+".CreateSession", 0, options).Store(&returnedRequestPath); err != nil {
		return fmt.Errorf("CreateSession call: %w", err)
	}

	s.logger.Debug("CreateSession called", "request_path", returnedRequestPath)

	// Wait for Response signal
	sessionHandle, err := s.waitForPortalResponse(ctx, signalChan, "session_handle")
	if err != nil {
		return fmt.Errorf("CreateSession response: %w", err)
	}
	s.portalSessionHandle = sessionHandle
	s.logger.Info("portal session created", "handle", sessionHandle)

	// Now call SelectSources to choose what to capture
	return s.selectPortalSources(ctx)
}

// selectPortalSources calls SelectSources on the portal session.
func (s *Server) selectPortalSources(ctx context.Context) error {
	s.logger.Info("selecting portal sources...")

	requestToken := fmt.Sprintf("req_%d", time.Now().UnixNano())
	senderName := s.conn.Names()[0]
	senderPath := ""
	for _, c := range senderName[1:] {
		if c == '.' {
			senderPath += "_"
		} else {
			senderPath += string(c)
		}
	}
	requestPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", senderPath, requestToken))

	// Subscribe to Response signal
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(requestPath),
		dbus.WithMatchInterface(portalRequestIface),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return fmt.Errorf("add signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)
	defer s.conn.RemoveSignal(signalChan)

	// Call SelectSources
	portalObj := s.conn.Object(portalBus, portalPath)
	options := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(requestToken),
		"types":        dbus.MakeVariant(portalSourceMonitor), // Capture monitor
		"cursor_mode":  dbus.MakeVariant(portalCursorHidden),  // Hidden - cursor rendered client-side
		"persist_mode": dbus.MakeVariant(uint32(0)),           // Don't persist (session-only)
	}

	sessionPath := dbus.ObjectPath(s.portalSessionHandle)
	var returnedRequestPath dbus.ObjectPath
	if err := portalObj.Call(portalScreenCastIface+".SelectSources", 0, sessionPath, options).Store(&returnedRequestPath); err != nil {
		return fmt.Errorf("SelectSources call: %w", err)
	}

	// Wait for Response signal (no result data expected, just success)
	_, err := s.waitForPortalResponse(ctx, signalChan, "")
	if err != nil {
		return fmt.Errorf("SelectSources response: %w", err)
	}

	s.logger.Info("portal sources selected")
	return nil
}

// startPortalSession starts the portal ScreenCast session and gets PipeWire node ID.
func (s *Server) startPortalSession(ctx context.Context) error {
	s.logger.Info("starting portal ScreenCast session...")

	requestToken := fmt.Sprintf("req_%d", time.Now().UnixNano())
	senderName := s.conn.Names()[0]
	senderPath := ""
	for _, c := range senderName[1:] {
		if c == '.' {
			senderPath += "_"
		} else {
			senderPath += string(c)
		}
	}
	requestPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", senderPath, requestToken))

	// Subscribe to Response signal
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(requestPath),
		dbus.WithMatchInterface(portalRequestIface),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return fmt.Errorf("add signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)
	defer s.conn.RemoveSignal(signalChan)

	// Call Start
	portalObj := s.conn.Object(portalBus, portalPath)
	options := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(requestToken),
	}

	sessionPath := dbus.ObjectPath(s.portalSessionHandle)
	// parent_window is empty string for headless
	var returnedRequestPath dbus.ObjectPath
	if err := portalObj.Call(portalScreenCastIface+".Start", 0, sessionPath, "", options).Store(&returnedRequestPath); err != nil {
		return fmt.Errorf("Start call: %w", err)
	}

	// Wait for Response signal with streams data
	streamsData, err := s.waitForPortalResponseStreams(ctx, signalChan)
	if err != nil {
		return fmt.Errorf("Start response: %w", err)
	}

	// Extract PipeWire node ID from streams
	// streams is an array of (node_id uint32, properties dict)
	if len(streamsData) == 0 {
		return fmt.Errorf("no streams returned from portal")
	}

	// First element is node_id
	nodeID, ok := streamsData[0].(uint32)
	if !ok {
		// Try to extract from struct
		if streamStruct, ok := streamsData[0].([]interface{}); ok && len(streamStruct) > 0 {
			if nid, ok := streamStruct[0].(uint32); ok {
				nodeID = nid
			}
		}
	}

	if nodeID == 0 {
		return fmt.Errorf("failed to extract node ID from streams: %v", streamsData)
	}

	s.nodeID = nodeID
	s.logger.Info("portal session started", "node_id", nodeID)

	// Save node ID to file for compatibility
	if err := os.WriteFile("/tmp/pipewire-node-id", []byte(fmt.Sprintf("%d", nodeID)), 0644); err != nil {
		s.logger.Warn("failed to save node ID to file", "err", err)
	}

	// Call OpenPipeWireRemote to get the PipeWire FD
	// This FD is required for pipewiresrc to connect to ScreenCast nodes
	if err := s.openPipeWireRemote(); err != nil {
		s.logger.Warn("failed to open PipeWire remote (zerocopy may not work)", "err", err)
		// Don't fail - we can still try without the FD (some setups work without it)
	}

	return nil
}

// openPipeWireRemote calls the portal's OpenPipeWireRemote method to get a FD
// for connecting to the PipeWire session that has ScreenCast access.
func (s *Server) openPipeWireRemote() error {
	if s.portalSessionHandle == "" {
		return fmt.Errorf("no portal session handle")
	}

	portalObj := s.conn.Object(portalBus, portalPath)

	// OpenPipeWireRemote takes session handle and options dict
	// Returns a Unix file descriptor via D-Bus fd passing
	options := map[string]dbus.Variant{}

	var pipeWireFd dbus.UnixFD
	err := portalObj.Call(
		portalScreenCastIface+".OpenPipeWireRemote",
		0,
		dbus.ObjectPath(s.portalSessionHandle),
		options,
	).Store(&pipeWireFd)

	if err != nil {
		return fmt.Errorf("OpenPipeWireRemote call failed: %w", err)
	}

	// Duplicate the FD to prevent it from being closed when D-Bus message is garbage collected.
	// D-Bus passes FDs via SCM_RIGHTS, but the library may close the original FD after processing.
	dupFd, dupErr := syscall.Dup(int(pipeWireFd))
	if dupErr != nil {
		s.logger.Warn("failed to dup PipeWire FD, using original", "err", dupErr)
		s.pipeWireFd = int(pipeWireFd)
	} else {
		s.pipeWireFd = dupFd
		s.logger.Debug("duplicated PipeWire FD", "original", int(pipeWireFd), "dup", dupFd)
	}
	s.logger.Info("opened PipeWire remote", "fd", s.pipeWireFd)

	// Save FD to file so other processes can use it
	if err := os.WriteFile("/tmp/pipewire-fd", []byte(fmt.Sprintf("%d", s.pipeWireFd)), 0644); err != nil {
		s.logger.Warn("failed to save PipeWire FD to file", "err", err)
	}

	return nil
}

// waitForPortalResponse waits for a portal Response signal and extracts a string result.
func (s *Server) waitForPortalResponse(ctx context.Context, signalChan chan *dbus.Signal, resultKey string) (string, error) {
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case sig := <-signalChan:
			if sig.Name == portalRequestIface+".Response" && len(sig.Body) >= 2 {
				response, ok := sig.Body[0].(uint32)
				if !ok {
					continue
				}
				if response != 0 {
					return "", fmt.Errorf("portal returned error response: %d", response)
				}

				results, ok := sig.Body[1].(map[string]dbus.Variant)
				if !ok {
					// No results but success
					return "", nil
				}

				if resultKey == "" {
					return "", nil
				}

				if val, ok := results[resultKey]; ok {
					if strVal, ok := val.Value().(string); ok {
						return strVal, nil
					}
				}

				return "", nil
			}
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for portal response")
		}
	}
}

// waitForPortalResponseStreams waits for a portal Response signal with streams data.
func (s *Server) waitForPortalResponseStreams(ctx context.Context, signalChan chan *dbus.Signal) ([]interface{}, error) {
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sig := <-signalChan:
			s.logger.Debug("received signal", "name", sig.Name, "body", sig.Body)
			if sig.Name == portalRequestIface+".Response" && len(sig.Body) >= 2 {
				response, ok := sig.Body[0].(uint32)
				if !ok {
					continue
				}
				if response != 0 {
					return nil, fmt.Errorf("portal returned error response: %d", response)
				}

				results, ok := sig.Body[1].(map[string]dbus.Variant)
				if !ok {
					return nil, fmt.Errorf("invalid response format")
				}

				if streams, ok := results["streams"]; ok {
					// streams is a(ua{sv}) - array of (node_id, properties)
					if streamArray, ok := streams.Value().([][]interface{}); ok && len(streamArray) > 0 {
						return streamArray[0], nil
					}
					// Try another format
					if streamArray, ok := streams.Value().([]interface{}); ok {
						return streamArray, nil
					}
				}

				return nil, fmt.Errorf("no streams in response: %v", results)
			}
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for portal streams response")
		}
	}
}

// createPortalRemoteDesktopSession creates a RemoteDesktop session via portal for input.
// This is optional - Sway can receive input directly via wlroots virtual keyboard/pointer.
func (s *Server) createPortalRemoteDesktopSession(ctx context.Context) error {
	s.logger.Info("creating portal RemoteDesktop session...")

	sessionToken := fmt.Sprintf("helix_rd_%d", time.Now().UnixNano())
	requestToken := fmt.Sprintf("req_rd_%d", time.Now().UnixNano())

	senderName := s.conn.Names()[0]
	senderPath := ""
	for _, c := range senderName[1:] {
		if c == '.' {
			senderPath += "_"
		} else {
			senderPath += string(c)
		}
	}
	requestPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", senderPath, requestToken))

	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(requestPath),
		dbus.WithMatchInterface(portalRequestIface),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return fmt.Errorf("add signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)
	defer s.conn.RemoveSignal(signalChan)

	portalObj := s.conn.Object(portalBus, portalPath)
	options := map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(requestToken),
		"session_handle_token": dbus.MakeVariant(sessionToken),
	}

	var returnedRequestPath dbus.ObjectPath
	if err := portalObj.Call(portalRemoteDesktopIface+".CreateSession", 0, options).Store(&returnedRequestPath); err != nil {
		// Portal RemoteDesktop may not be supported - that's OK, we use wlroots virtual input
		s.logger.Warn("portal RemoteDesktop not available, using wlroots virtual input", "err", err)
		return nil
	}

	sessionHandle, err := s.waitForPortalResponse(ctx, signalChan, "session_handle")
	if err != nil {
		s.logger.Warn("portal RemoteDesktop session failed", "err", err)
		return nil
	}

	s.portalRDSessionHandle = sessionHandle
	s.logger.Info("portal RemoteDesktop session created", "handle", sessionHandle)
	return nil
}
