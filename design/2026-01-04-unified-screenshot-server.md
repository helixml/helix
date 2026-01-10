# Unified Screenshot Server Architecture

**Date:** 2026-01-04
**Status:** Proposed

## Problem

Currently we have three separate components for GNOME desktop integration:

```
remotedesktop-session (Go)     screenshot-server (Go)        gnome-screenshot.py
         │                            │                              │
         │ saves node ID to file      │ reads node ID from file      │ reads node ID from file
         ├──────────────────────────>│                              │
         │                            │ forks gst-launch-1.0 ────────┤
         │                            │                              │
    D-Bus session                HTTP :9876                    (forked by screenshot-server)
    + input socket
```

**Issues:**
1. File-based coordination for PipeWire node ID is fragile
2. Three separate processes/binaries to manage
3. gnome-screenshot.py adds Python runtime dependency
4. Forking Python on every screenshot request adds latency

## Solution

Merge everything into a single `screenshot-server` binary:

```
screenshot-server (Go)
         │
         ├── D-Bus session management (RemoteDesktop + ScreenCast)
         ├── Input handling via Unix socket
         ├── HTTP server (:9876) for screenshots/clipboard/keyboard/upload
         │   └── Has PipeWire node ID in memory
         │   └── Forks gst-launch-1.0 for frame capture (CGO-free)
         │
    Single binary, single process
```

**Benefits:**
- Node ID in memory, no temp file coordination
- One process instead of three
- Delete gnome-screenshot.py entirely
- Simpler container startup
- CGO-free (keeps forking to gst-launch-1.0, avoids go-gst complexity)

## Package Structure

```
api/pkg/desktop/
├── desktop.go      # Package doc, shared types, Server struct
├── session.go      # RemoteDesktop/ScreenCast D-Bus session
├── input.go        # Wolf input socket handling
├── screenshot.go   # Screenshot capture (PipeWire, KDE, X11)
├── clipboard.go    # Clipboard get/set
├── keyboard.go     # Keyboard state
├── upload.go       # File upload
└── env.go          # Environment detection (GNOME, KDE, X11)

api/cmd/screenshot-server/
└── main.go         # Entry point, wires everything together
```

## Architecture

### Core Types

```go
// desktop.go

// Server is the main desktop integration server.
// It manages D-Bus sessions for video/input and serves HTTP APIs.
type Server struct {
    // D-Bus session state
    conn          *dbus.Conn
    rdSessionPath dbus.ObjectPath
    scStreamPath  dbus.ObjectPath
    nodeID        uint32

    // Input socket
    inputListener net.Listener
    inputSocketPath string

    // Configuration
    config Config

    // Lifecycle
    running   atomic.Bool
    wg        sync.WaitGroup
    logger    *slog.Logger
}

// Config holds server configuration.
type Config struct {
    HTTPPort        string
    WolfSocketPath  string
    XDGRuntimeDir   string
    SessionID       string
}

// NewServer creates a new desktop server with the given config.
func NewServer(cfg Config, logger *slog.Logger) *Server
```

### Interface-Based Design for Testability

```go
// screenshot.go

// Capturer captures screenshots from the desktop.
type Capturer interface {
    Capture(format string, quality int) (data []byte, actualFormat string, err error)
}

// PipeWireCapturer captures from PipeWire via gst-launch-1.0.
type PipeWireCapturer struct {
    nodeID uint32
    exec   CommandExecutor  // Interface for testing
}

// CommandExecutor abstracts exec.Command for testing.
type CommandExecutor interface {
    Run(name string, args ...string) ([]byte, error)
}

// realExecutor is the production implementation.
type realExecutor struct{}

func (e *realExecutor) Run(name string, args ...string) ([]byte, error) {
    cmd := exec.Command(name, args...)
    return cmd.CombinedOutput()
}
```

### Clean HTTP Handlers

```go
// main.go

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

    cfg := desktop.Config{
        HTTPPort:       getEnv("SCREENSHOT_PORT", "9876"),
        WolfSocketPath: getEnv("WOLF_LOBBY_SOCKET_PATH", "/var/run/wolf/lobby.sock"),
        XDGRuntimeDir:  getEnv("XDG_RUNTIME_DIR", "/run/user/1000"),
        SessionID:      os.Getenv("HELIX_SESSION_ID"),
    }

    server := desktop.NewServer(cfg, logger)

    // Graceful shutdown
    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    if err := server.Run(ctx); err != nil && err != context.Canceled {
        logger.Error("server error", "err", err)
        os.Exit(1)
    }
}
```

### Server Lifecycle

```go
// desktop.go

// Run starts the server and blocks until context is cancelled.
func (s *Server) Run(ctx context.Context) error {
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

    // 5. Report to Wolf
    s.reportToWolf()

    // 6. Start subsystems
    s.running.Store(true)

    errCh := make(chan error, 2)

    // HTTP server
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        if err := s.serveHTTP(ctx); err != nil {
            errCh <- fmt.Errorf("http: %w", err)
        }
    }()

    // Input bridge
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        s.runInputBridge(ctx)
    }()

    // Wait for shutdown or error
    select {
    case <-ctx.Done():
        s.logger.Info("shutting down...")
    case err := <-errCh:
        return err
    }

    s.running.Store(false)
    s.wg.Wait()
    return ctx.Err()
}
```

## File Details

### session.go (~200 lines)

```go
package desktop

// D-Bus constants
const (
    remoteDesktopBus   = "org.gnome.Mutter.RemoteDesktop"
    screenCastBus      = "org.gnome.Mutter.ScreenCast"
    // ...
)

// connectDBus connects to session D-Bus with retry.
func (s *Server) connectDBus(ctx context.Context) error

// createSession creates RemoteDesktop and linked ScreenCast sessions.
func (s *Server) createSession(ctx context.Context) error

// startSession starts the session and waits for PipeWire node ID.
func (s *Server) startSession(ctx context.Context) error

// reportToWolf reports node ID and input socket to Wolf.
func (s *Server) reportToWolf()
```

### input.go (~150 lines)

```go
package desktop

// InputEvent represents an input event from Wolf.
type InputEvent struct {
    Type    string  `json:"type"`
    X       float64 `json:"x,omitempty"`
    Y       float64 `json:"y,omitempty"`
    // ...
}

// createInputSocket creates the Unix socket for input events.
func (s *Server) createInputSocket() error

// runInputBridge accepts connections and handles input events.
func (s *Server) runInputBridge(ctx context.Context)

// handleInputClient handles a single input client connection.
func (s *Server) handleInputClient(ctx context.Context, conn net.Conn)

// injectInput sends an input event to GNOME via D-Bus.
func (s *Server) injectInput(event *InputEvent) error
```

### screenshot.go (~300 lines)

```go
package desktop

// CaptureScreenshot captures a screenshot with the specified format and quality.
func (s *Server) CaptureScreenshot(format string, quality int) ([]byte, string, error) {
    switch {
    case s.nodeID != 0:
        return s.capturePipeWire(format, quality)
    case isKDEEnvironment():
        return s.captureKDE(format, quality)
    default:
        return s.captureX11(format, quality)
    }
}

// capturePipeWire captures from the PipeWire stream via gst-launch-1.0.
func (s *Server) capturePipeWire(format string, quality int) ([]byte, string, error)

// captureKDE captures via KWin D-Bus API.
func (s *Server) captureKDE(format string, quality int) ([]byte, string, error)

// captureX11 captures via scrot.
func (s *Server) captureX11(format string, quality int) ([]byte, string, error)

// convertPNGtoJPEG converts PNG to JPEG with specified quality.
func convertPNGtoJPEG(pngData []byte, quality int) ([]byte, error)
```

### clipboard.go (~200 lines)

```go
package desktop

// ClipboardData represents clipboard content.
type ClipboardData struct {
    Type string `json:"type"` // "text" or "image"
    Data string `json:"data"` // text or base64-encoded image
}

// GetClipboard reads from the clipboard.
func (s *Server) GetClipboard() (*ClipboardData, error)

// SetClipboard writes to the clipboard.
func (s *Server) SetClipboard(data *ClipboardData) error
```

### keyboard.go (~150 lines)

```go
package desktop

// KeyboardState represents the current keyboard state.
type KeyboardState struct {
    Timestamp     int64    `json:"timestamp"`
    PressedKeys   []int    `json:"pressed_keys"`
    KeyNames      []string `json:"key_names"`
    ModifierState struct {
        Shift bool `json:"shift"`
        Ctrl  bool `json:"ctrl"`
        Alt   bool `json:"alt"`
        Meta  bool `json:"meta"`
    } `json:"modifier_state"`
}

// GetKeyboardState returns the current keyboard state.
func (s *Server) GetKeyboardState() (*KeyboardState, error)

// ResetKeyboard releases all stuck modifier keys.
func (s *Server) ResetKeyboard() ([]string, error)
```

### upload.go (~80 lines)

```go
package desktop

// UploadResult contains the result of a file upload.
type UploadResult struct {
    Path     string `json:"path"`
    Size     int64  `json:"size"`
    Filename string `json:"filename"`
}

// UploadFile saves an uploaded file to ~/work/incoming.
func (s *Server) UploadFile(filename string, data io.Reader) (*UploadResult, error)
```

### env.go (~50 lines)

```go
package desktop

// Environment detection with caching.

var (
    envOnce   sync.Once
    envGNOME  bool
    envKDE    bool
    envX11    bool
)

func isGNOMEEnvironment() bool
func isKDEEnvironment() bool
func isX11Mode() bool
```

## HTTP Handlers (in main.go)

```go
// HTTP handlers are thin wrappers around Server methods.

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
    format := r.URL.Query().Get("format")
    if format == "" {
        format = "jpeg"
    }
    quality := parseQuality(r.URL.Query().Get("quality"), 70)

    data, actualFormat, err := s.CaptureScreenshot(format, quality)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "image/"+actualFormat)
    w.Write(data)
}

func (s *Server) handleClipboard(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        data, err := s.GetClipboard()
        // ...
    case http.MethodPost:
        // ...
    }
}

// etc.
```

## Testing Strategy

### Unit Tests

```go
// screenshot_test.go

func TestConvertPNGtoJPEG(t *testing.T) {
    // Test with various quality levels
    pngData := createTestPNG(100, 100)

    for _, quality := range []int{10, 50, 90} {
        jpegData, err := convertPNGtoJPEG(pngData, quality)
        require.NoError(t, err)
        assert.Greater(t, len(jpegData), 0)
    }
}

// Mock executor for testing capturePipeWire without gst-launch-1.0
type mockExecutor struct {
    output []byte
    err    error
}

func (m *mockExecutor) Run(name string, args ...string) ([]byte, error) {
    return m.output, m.err
}
```

### Integration Tests

```go
// server_test.go

func TestServerLifecycle(t *testing.T) {
    if testing.Short() {
        t.Skip("integration test")
    }

    cfg := Config{
        HTTPPort: "0", // Random port
        // ...
    }

    server := NewServer(cfg, slog.Default())

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    errCh := make(chan error, 1)
    go func() {
        errCh <- server.Run(ctx)
    }()

    // Wait for server to be ready
    // Make test requests
    // Cancel context
    // Check clean shutdown
}
```

## Files to Delete

- `wolf/ubuntu-config/remotedesktop-session.go`
- `wolf/ubuntu-config/gnome-screenshot.py`

## Dockerfile Changes

```dockerfile
# Before: built 4 binaries
CGO_ENABLED=0 go build -o /screenshot-server ./cmd/screenshot-server && \
CGO_ENABLED=0 go build -o /remotedesktop-session ../wolf/ubuntu-config/remotedesktop-session.go

# After: only 3 binaries
CGO_ENABLED=0 go build -o /screenshot-server ./cmd/screenshot-server && \
CGO_ENABLED=0 go build -o /settings-sync-daemon ./cmd/settings-sync-daemon && \
CGO_ENABLED=0 go build -o /revdial-client ./cmd/revdial-client
```

## Startup Script Changes

```bash
# Before (Dockerfile.ubuntu-helix):
/opt/gow/remotedesktop-session >> /tmp/remotedesktop-session.log 2>&1 &
/usr/local/bin/screenshot-server >> /tmp/screenshot-server.log 2>&1 &

# After:
/usr/local/bin/screenshot-server >> /tmp/screenshot-server.log 2>&1 &
# screenshot-server now handles both D-Bus session and HTTP API
```
