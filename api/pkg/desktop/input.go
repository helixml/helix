package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
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
	s.logger.Info("starting input bridge...")

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
	s.logger.Info("input client connected")

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

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
			continue
		}

		s.injectInput(&event)
	}

	s.logger.Info("input client disconnected")
}

// injectInput sends an input event to GNOME via D-Bus.
func (s *Server) injectInput(event *InputEvent) {
	if s.conn == nil || s.rdSessionPath == "" {
		return
	}

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

	switch event.Type {
	case "mouse_move_abs":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		s.moveCount++
		if s.moveCount == 1 || s.moveCount%100 == 0 {
			s.logger.Debug("mouse_move_abs", "count", s.moveCount, "x", event.X, "y", event.Y)
		}
		rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotionAbsolute", 0, stream, event.X, event.Y)

	case "mouse_move_rel":
		rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotion", 0, event.DX, event.DY)

	case "button":
		s.logger.Debug("button", "button", event.Button, "state", event.State)
		rdSession.Call(remoteDesktopSessionIface+".NotifyPointerButton", 0, event.Button, event.State)

	case "scroll":
		if event.DY != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxisDiscrete", 0, uint32(0), int32(event.DY))
		}
		if event.DX != 0 {
			rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxisDiscrete", 0, uint32(1), int32(event.DX))
		}

	case "scroll_smooth":
		rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxis", 0, event.DX, event.DY, uint32(4))

	case "key":
		rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeycode", 0, event.Keycode, event.State)

	case "touch_down":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		rdSession.Call(remoteDesktopSessionIface+".NotifyTouchDown", 0, stream, event.Slot, event.X, event.Y)

	case "touch_motion":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		rdSession.Call(remoteDesktopSessionIface+".NotifyTouchMotion", 0, stream, event.Slot, event.X, event.Y)

	case "touch_up":
		rdSession.Call(remoteDesktopSessionIface+".NotifyTouchUp", 0, event.Slot)
	}
}
