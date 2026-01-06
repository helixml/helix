package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// InputEvent represents an input event from Wolf.
type InputEvent struct {
	Type    string  `json:"type"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	DX      float64 `json:"dx,omitempty"`
	DY      float64 `json:"dy,omitempty"`
	Button  int32   `json:"button,omitempty"`
	State   bool    `json:"state,omitempty"`
	Keycode uint32  `json:"keycode,omitempty"`
	Slot    uint32  `json:"slot,omitempty"`
	Stream  string  `json:"stream,omitempty"`
}

// createInputSocket creates the Unix socket for input events.
func (s *Server) createInputSocket() error {
	s.logger.Info("creating input socket", "path", s.inputSocketPath)

	// Remove existing socket
	os.Remove(s.inputSocketPath)

	var err error
	s.inputListener, err = net.Listen("unix", s.inputSocketPath)
	if err != nil {
		return err
	}

	// Make socket world-accessible for Wolf
	if err := os.Chmod(s.inputSocketPath, 0777); err != nil {
		s.logger.Warn("failed to chmod socket", "err", err)
	}

	s.logger.Info("input socket created")
	return nil
}

// runInputBridge accepts connections and handles input events.
func (s *Server) runInputBridge(ctx context.Context) {
	s.logger.Info("starting input bridge",
		"socket_path", s.inputSocketPath,
		"rd_session", s.rdSessionPath,
		"dbus_connected", s.conn != nil)

	for s.isRunning() {
		// Set accept deadline so we can check context
		if ul, ok := s.inputListener.(*net.UnixListener); ok {
			ul.SetDeadline(time.Now().Add(time.Second))
		}

		conn, err := s.inputListener.Accept()
		if err != nil {
			if s.isRunning() {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				s.logger.Error("accept error", "err", err)
			}
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleInputClient(ctx, conn)
		}()
	}
}

// handleInputClient handles a single input client connection.
func (s *Server) handleInputClient(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	s.logger.Info("input client connected from Wolf socket")

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	eventCount := 0
	for s.isRunning() {
		conn.SetReadDeadline(time.Now().Add(time.Second))

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				s.logger.Debug("client read error", "err", err)
			}
			break
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var event InputEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			s.logger.Warn("failed to parse input event JSON", "err", err, "line", line[:min(100, len(line))])
			continue
		}

		eventCount++
		if eventCount <= 5 || eventCount%100 == 0 {
			s.logger.Info("received input event from Wolf", "type", event.Type, "count", eventCount)
		}

		s.injectInput(&event)
	}

	s.logger.Info("input client disconnected", "total_events", eventCount)
}

// injectInput sends an input event to GNOME via D-Bus.
func (s *Server) injectInput(event *InputEvent) {
	if s.conn == nil {
		s.logger.Warn("input event dropped: D-Bus connection is nil", "type", event.Type)
		return
	}
	if s.rdSessionPath == "" {
		s.logger.Warn("input event dropped: RemoteDesktop session path is empty", "type", event.Type)
		return
	}

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

	var err error
	switch event.Type {
	case "mouse_move_abs":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		s.moveCount++
		if s.moveCount == 1 || s.moveCount%100 == 0 {
			s.logger.Debug("mouse_move_abs", "count", s.moveCount, "x", event.X, "y", event.Y, "stream", stream)
		}
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotionAbsolute", 0, stream, event.X, event.Y).Err

	case "mouse_move_rel":
		s.logger.Debug("mouse_move_rel", "dx", event.DX, "dy", event.DY)
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotion", 0, event.DX, event.DY).Err

	case "button":
		s.logger.Debug("button", "button", event.Button, "state", event.State)
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyPointerButton", 0, event.Button, event.State).Err

	case "scroll":
		s.logger.Debug("scroll", "dx", event.DX, "dy", event.DY)
		if event.DY != 0 {
			err = rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxisDiscrete", 0, uint32(0), int32(event.DY)).Err
		}
		if event.DX != 0 && err == nil {
			err = rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxisDiscrete", 0, uint32(1), int32(event.DX)).Err
		}

	case "scroll_smooth":
		s.logger.Debug("scroll_smooth", "dx", event.DX, "dy", event.DY)
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxis", 0, event.DX, event.DY, uint32(4)).Err

	case "key":
		s.logger.Info("key event", "keycode", event.Keycode, "state", event.State)
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeycode", 0, event.Keycode, event.State).Err

	case "touch_down":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		s.logger.Debug("touch_down", "slot", event.Slot, "x", event.X, "y", event.Y)
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchDown", 0, stream, event.Slot, event.X, event.Y).Err

	case "touch_motion":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchMotion", 0, stream, event.Slot, event.X, event.Y).Err

	case "touch_up":
		s.logger.Debug("touch_up", "slot", event.Slot)
		err = rdSession.Call(remoteDesktopSessionIface+".NotifyTouchUp", 0, event.Slot).Err

	default:
		s.logger.Warn("unknown input event type", "type", event.Type)
		return
	}

	if err != nil {
		s.logger.Error("D-Bus input call failed", "type", event.Type, "err", err)
	}
}

// InputRequest represents a batch of input events from HTTP.
type InputRequest struct {
	Events []InputEvent `json:"events"`
}

// InputResponse is returned from the input endpoint.
type InputResponse struct {
	Success   bool   `json:"success"`
	Processed int    `json:"processed"`
	Message   string `json:"message,omitempty"`
}

// handleInput handles POST /input for injecting keyboard/mouse events.
func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if D-Bus session is available
	if s.conn == nil {
		s.logger.Warn("HTTP input: D-Bus connection is nil")
		http.Error(w, "D-Bus connection not available", http.StatusServiceUnavailable)
		return
	}
	if s.rdSessionPath == "" {
		s.logger.Warn("HTTP input: RemoteDesktop session path is empty")
		http.Error(w, "RemoteDesktop session not available", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	s.logger.Debug("HTTP input received", "body_length", len(body))

	var req InputRequest
	if err := json.Unmarshal(body, &req); err != nil || len(req.Events) == 0 {
		// Try single event format for convenience (also handles case where
		// unmarshal succeeded but events array was empty/missing)
		var event InputEvent
		if err := json.Unmarshal(body, &event); err != nil {
			s.logger.Warn("HTTP input: invalid JSON", "err", err)
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		// Only use single event if it has a type (valid event)
		if event.Type != "" {
			req.Events = []InputEvent{event}
		}
	}

	// Process all events
	processed := 0
	for _, event := range req.Events {
		s.injectInput(&event)
		processed++
	}

	s.logger.Info("HTTP input events processed", "count", processed)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(InputResponse{
		Success:   true,
		Processed: processed,
	})
}
