// remotedesktop-session creates and maintains a GNOME RemoteDesktop session
// that provides ScreenCast for video (PipeWire stream) and input injection via D-Bus.
//
// This is a Go rewrite of remotedesktop-session.py with proper line-based socket reading.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

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

	dbusPropIface = "org.freedesktop.DBus.Properties"
)

// InputEvent represents an input event from Wolf
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

// Session manages a GNOME RemoteDesktop session
type Session struct {
	conn            *dbus.Conn
	rdSessionPath   dbus.ObjectPath
	scSessionPath   dbus.ObjectPath
	scStreamPath    dbus.ObjectPath
	nodeID          uint32
	inputSocketPath string
	wolfSocketPath  string
	sessionID       string

	listener   net.Listener
	running    bool
	runningMu  sync.RWMutex
	wg         sync.WaitGroup
	moveCount  int
}

func logMsg(format string, args ...interface{}) {
	log.Printf("[remotedesktop-go] "+format, args...)
}

func NewSession() *Session {
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/run/user/1000"
	}

	return &Session{
		inputSocketPath: xdgRuntimeDir + "/wolf-input.sock",
		wolfSocketPath:  getEnvOrDefault("WOLF_LOBBY_SOCKET_PATH", "/var/run/wolf/lobby.sock"),
		sessionID:       os.Getenv("HELIX_SESSION_ID"),
		running:         true,
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func (s *Session) isRunning() bool {
	s.runningMu.RLock()
	defer s.runningMu.RUnlock()
	return s.running
}

func (s *Session) stop() {
	s.runningMu.Lock()
	s.running = false
	s.runningMu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}
}

// Connect establishes D-Bus connection with retry
func (s *Session) Connect() error {
	logMsg("Waiting for gnome-shell D-Bus services...")

	var err error
	for attempt := 0; attempt < 60; attempt++ {
		logMsg("Connection attempt %d/60...", attempt+1)

		s.conn, err = dbus.ConnectSessionBus()
		if err != nil {
			logMsg("D-Bus not ready (%v), retrying in 1s...", err)
			time.Sleep(time.Second)
			continue
		}

		// Verify RemoteDesktop service is available
		obj := s.conn.Object(remoteDesktopBus, remoteDesktopPath)
		if err := obj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
			logMsg("RemoteDesktop service not ready (%v), retrying in 1s...", err)
			s.conn.Close()
			time.Sleep(time.Second)
			continue
		}

		logMsg("D-Bus connected")
		return nil
	}

	return fmt.Errorf("failed to connect to D-Bus after 60 attempts: %w", err)
}

// CreateSession creates RemoteDesktop and linked ScreenCast sessions
func (s *Session) CreateSession() error {
	// Create RemoteDesktop session
	logMsg("Creating RemoteDesktop session...")
	rdObj := s.conn.Object(remoteDesktopBus, remoteDesktopPath)

	var rdSessionPath dbus.ObjectPath
	if err := rdObj.Call(remoteDesktopIface+".CreateSession", 0).Store(&rdSessionPath); err != nil {
		return fmt.Errorf("failed to create RemoteDesktop session: %w", err)
	}
	s.rdSessionPath = rdSessionPath
	logMsg("RemoteDesktop session: %s", rdSessionPath)

	// Get session ID from path (last component)
	sessionID := string(rdSessionPath)[len("/org/gnome/Mutter/RemoteDesktop/Session/"):]
	logMsg("Session ID: %s", sessionID)

	// Create linked ScreenCast session
	logMsg("Creating linked ScreenCast session...")
	scObj := s.conn.Object(screenCastBus, screenCastPath)

	options := map[string]dbus.Variant{
		"remote-desktop-session-id": dbus.MakeVariant(sessionID),
	}

	var scSessionPath dbus.ObjectPath
	if err := scObj.Call(screenCastIface+".CreateSession", 0, options).Store(&scSessionPath); err != nil {
		return fmt.Errorf("failed to create ScreenCast session: %w", err)
	}
	s.scSessionPath = scSessionPath
	logMsg("ScreenCast session: %s", scSessionPath)

	// Record the virtual monitor Meta-0
	logMsg("Recording virtual monitor Meta-0...")
	scSession := s.conn.Object(screenCastBus, scSessionPath)

	recordOptions := map[string]dbus.Variant{
		"cursor-mode": dbus.MakeVariant(uint32(1)), // Embedded cursor
	}

	var streamPath dbus.ObjectPath
	if err := scSession.Call(screenCastSessionIface+".RecordMonitor", 0, "Meta-0", recordOptions).Store(&streamPath); err != nil {
		return fmt.Errorf("failed to RecordMonitor Meta-0: %w", err)
	}
	s.scStreamPath = streamPath
	logMsg("Stream: %s", streamPath)

	return nil
}

// Start starts the RemoteDesktop session and gets PipeWire node ID
func (s *Session) Start() error {
	logMsg("Setting up PipeWireStreamAdded signal handler...")

	// Subscribe to signals
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(s.scStreamPath),
		dbus.WithMatchInterface(screenCastStreamIface),
		dbus.WithMatchMember("PipeWireStreamAdded"),
	); err != nil {
		return fmt.Errorf("failed to add signal match: %w", err)
	}

	signalChan := make(chan *dbus.Signal, 10)
	s.conn.Signal(signalChan)

	// Start the session
	logMsg("Starting RemoteDesktop session...")
	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
	if err := rdSession.Call(remoteDesktopSessionIface+".Start", 0).Err; err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	logMsg("Session started, waiting for PipeWireStreamAdded signal...")

	// Wait for signal with timeout
	timeout := time.After(10 * time.Second)
	for {
		select {
		case sig := <-signalChan:
			if sig.Name == screenCastStreamIface+".PipeWireStreamAdded" && len(sig.Body) > 0 {
				if nodeID, ok := sig.Body[0].(uint32); ok {
					s.nodeID = nodeID
					logMsg("Received PipeWireStreamAdded signal: node_id=%d", nodeID)

					// Save node ID to file for screenshot script
					if err := os.WriteFile("/tmp/pipewire-node-id", []byte(fmt.Sprintf("%d", nodeID)), 0644); err != nil {
						logMsg("WARNING: Failed to save node ID to file: %v", err)
					} else {
						logMsg("Saved PipeWire node ID to /tmp/pipewire-node-id")
					}
					return nil
				}
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for PipeWireStreamAdded signal after 10s")
		}
	}
}

// CreateInputSocket creates the Unix socket for input events
func (s *Session) CreateInputSocket() error {
	logMsg("Creating input socket at %s...", s.inputSocketPath)

	// Remove existing socket
	os.Remove(s.inputSocketPath)

	var err error
	s.listener, err = net.Listen("unix", s.inputSocketPath)
	if err != nil {
		return fmt.Errorf("failed to create input socket: %w", err)
	}

	// Make socket world-accessible
	if err := os.Chmod(s.inputSocketPath, 0777); err != nil {
		logMsg("WARNING: Failed to chmod socket: %v", err)
	}

	logMsg("Input socket created and listening")
	return nil
}

// ReportToWolf reports node ID and input socket to Wolf
func (s *Session) ReportToWolf() {
	logMsg("Session summary:")
	logMsg("  RemoteDesktop session: %s", s.rdSessionPath)
	logMsg("  ScreenCast stream: %s", s.scStreamPath)
	logMsg("  PipeWire node ID: %d", s.nodeID)
	logMsg("  Input socket: %s", s.inputSocketPath)

	if _, err := os.Stat(s.wolfSocketPath); os.IsNotExist(err) {
		logMsg("WARNING: Wolf socket not found at %s", s.wolfSocketPath)
		return
	}

	// Report node ID
	logMsg("Reporting node ID to Wolf...")
	s.postToWolf("/set-pipewire-node-id", map[string]interface{}{
		"node_id":      s.nodeID,
		"session_path": string(s.rdSessionPath),
	})

	// Report input socket
	logMsg("Reporting input socket to Wolf...")
	s.postToWolf("/set-input-socket", map[string]interface{}{
		"input_socket": s.inputSocketPath,
	})
}

func (s *Session) postToWolf(endpoint string, data map[string]interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		logMsg("Failed to marshal data: %v", err)
		return
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", s.wolfSocketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Post("http://localhost"+endpoint, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		logMsg("Failed to report to Wolf: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	logMsg("Wolf %s response: %s", endpoint, string(body))
}

// RunInputBridge accepts connections and handles input events
func (s *Session) RunInputBridge() {
	logMsg("Starting input bridge accept loop...")

	for s.isRunning() {
		// Set accept deadline so we can check running flag
		if ul, ok := s.listener.(*net.UnixListener); ok {
			ul.SetDeadline(time.Now().Add(time.Second))
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.isRunning() {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue // Normal timeout, check running and continue
				}
				logMsg("Accept error: %v", err)
			}
			continue
		}

		s.wg.Add(1)
		go s.handleClient(conn)
	}

	s.wg.Wait()
	s.listener.Close()
	os.Remove(s.inputSocketPath)
}

func (s *Session) handleClient(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	logMsg("Input client connected")

	// Use bufio.Scanner for proper line-based reading
	scanner := bufio.NewScanner(conn)
	// Set a reasonable max line size (64KB should be plenty for JSON input events)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for s.isRunning() {
		// Set read deadline so we can check running flag
		conn.SetReadDeadline(time.Now().Add(time.Second))

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue // Normal timeout, check running and continue
				}
				logMsg("Client read error: %v", err)
			}
			break // EOF or error
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var event InputEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Invalid JSON, skip
		}

		s.handleInput(&event)
	}

	logMsg("Input client disconnected")
}

func (s *Session) handleInput(event *InputEvent) {
	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

	// Debug log for non-move events
	if event.Type != "mouse_move_abs" && event.Type != "mouse_move_rel" {
		logMsg("[INPUT_DEBUG] Received: %s data=%+v", event.Type, event)
	}

	// Use 1 second timeout for D-Bus calls (passed as microseconds to dbus)
	// Note: godbus doesn't support per-call timeouts directly, but calls are generally fast

	switch event.Type {
	case "mouse_move_abs":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		s.moveCount++
		if s.moveCount == 1 || s.moveCount%100 == 0 {
			logMsg("[INPUT_DEBUG] mouse_move_abs #%d: x=%.1f y=%.1f stream=%s", s.moveCount, event.X, event.Y, stream)
		}
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotionAbsolute", 0, stream, event.X, event.Y).Err; err != nil {
			logMsg("Input error for mouse_move_abs: %v", err)
		}

	case "mouse_move_rel":
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotion", 0, event.DX, event.DY).Err; err != nil {
			logMsg("Input error for mouse_move_rel: %v", err)
		}

	case "button":
		logMsg("[INPUT_DEBUG] Button event: evdev_button=%d state=%v", event.Button, event.State)
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerButton", 0, event.Button, event.State).Err; err != nil {
			logMsg("Input error for button: %v", err)
		}

	case "scroll":
		dy := int32(event.DY)
		dx := int32(event.DX)
		if dy != 0 {
			if err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxisDiscrete", 0, uint32(0), dy).Err; err != nil {
				logMsg("Input error for scroll (vertical): %v", err)
			}
		}
		if dx != 0 {
			if err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxisDiscrete", 0, uint32(1), dx).Err; err != nil {
				logMsg("Input error for scroll (horizontal): %v", err)
			}
		}

	case "scroll_smooth":
		flags := uint32(4) // SOURCE_FINGER
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerAxis", 0, event.DX, event.DY, flags).Err; err != nil {
			logMsg("Input error for scroll_smooth: %v", err)
		}

	case "key":
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeycode", 0, event.Keycode, event.State).Err; err != nil {
			logMsg("Input error for key: %v", err)
		}

	case "touch_down":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyTouchDown", 0, stream, event.Slot, event.X, event.Y).Err; err != nil {
			logMsg("Input error for touch_down: %v", err)
		}

	case "touch_motion":
		stream := event.Stream
		if stream == "" {
			stream = string(s.scStreamPath)
		}
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyTouchMotion", 0, stream, event.Slot, event.X, event.Y).Err; err != nil {
			logMsg("Input error for touch_motion: %v", err)
		}

	case "touch_up":
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyTouchUp", 0, event.Slot).Err; err != nil {
			logMsg("Input error for touch_up: %v", err)
		}
	}
}

func (s *Session) Stop() {
	logMsg("Stopping...")
	s.stop()

	if s.rdSessionPath != "" && s.conn != nil {
		rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
		rdSession.Call(remoteDesktopSessionIface+".Stop", 0)
	}

	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *Session) Run() error {
	if err := s.Connect(); err != nil {
		return err
	}

	if err := s.CreateSession(); err != nil {
		return err
	}

	if err := s.Start(); err != nil {
		return err
	}

	// Create socket BEFORE reporting to Wolf
	if err := s.CreateInputSocket(); err != nil {
		return err
	}

	s.ReportToWolf()

	// Run input bridge (blocks until stopped)
	s.RunInputBridge()

	return nil
}

func main() {
	log.SetFlags(0) // No timestamp prefix, we add our own

	session := NewSession()

	if session.sessionID == "" {
		logMsg("ERROR: HELIX_SESSION_ID not set (required for Wolf session management)")
		logMsg("WARNING: Continuing without Wolf session ID - input may not work")
	}

	logMsg("Session ID: %s", session.sessionID)
	logMsg("Wolf socket: %s", session.wolfSocketPath)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logMsg("Received signal %v", sig)
		session.Stop()
	}()

	if err := session.Run(); err != nil {
		logMsg("ERROR: %v", err)
		os.Exit(1)
	}

	session.Stop()
}
