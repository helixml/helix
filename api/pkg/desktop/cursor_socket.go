// Package desktop provides cursor tracking via Unix socket.
// The GNOME Shell extension sends cursor updates to this socket.
package desktop

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const (
	// CursorSocketPath is the Unix socket path for cursor updates from GNOME Shell extension
	CursorSocketPath = "/run/user/1000/helix-cursor.sock"
)

// CursorSocketCallback is called when cursor data is received from the GNOME extension socket
type CursorSocketCallback func(hotspotX, hotspotY, width, height int, pixels []byte, cursorName string)

// SharedCursorBroadcaster manages a single cursor socket listener and broadcasts to multiple VideoStreamers.
// This is necessary because the Unix socket path is shared - only one listener can own it at a time.
// When multiple clients connect, they all receive cursor updates via this broadcaster.
type SharedCursorBroadcaster struct {
	mu        sync.RWMutex
	callbacks map[uint64]CursorSocketCallback
	nextID    atomic.Uint64
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	logger    *slog.Logger
}

var (
	sharedCursorBroadcaster     *SharedCursorBroadcaster
	sharedCursorBroadcasterOnce sync.Once
	sharedCursorBroadcasterMu   sync.Mutex
)

// GetSharedCursorBroadcaster returns the global shared cursor broadcaster.
// It lazily starts the broadcaster on first use.
func GetSharedCursorBroadcaster(logger *slog.Logger) *SharedCursorBroadcaster {
	sharedCursorBroadcasterMu.Lock()
	defer sharedCursorBroadcasterMu.Unlock()

	if sharedCursorBroadcaster == nil {
		ctx, cancel := context.WithCancel(context.Background())
		sharedCursorBroadcaster = &SharedCursorBroadcaster{
			callbacks: make(map[uint64]CursorSocketCallback),
			ctx:       ctx,
			cancel:    cancel,
			logger:    logger,
		}
		go sharedCursorBroadcaster.run()
	}
	return sharedCursorBroadcaster
}

// Register adds a callback to receive cursor updates. Returns an ID for unregistering.
func (b *SharedCursorBroadcaster) Register(callback CursorSocketCallback) uint64 {
	id := b.nextID.Add(1)
	b.mu.Lock()
	b.callbacks[id] = callback
	b.mu.Unlock()
	b.logger.Info("[SharedCursorBroadcaster] registered callback", "id", id, "total", len(b.callbacks))
	return id
}

// Unregister removes a callback by ID
func (b *SharedCursorBroadcaster) Unregister(id uint64) {
	b.mu.Lock()
	delete(b.callbacks, id)
	remaining := len(b.callbacks)
	b.mu.Unlock()
	b.logger.Info("[SharedCursorBroadcaster] unregistered callback", "id", id, "remaining", remaining)
}

// broadcast sends cursor data to all registered callbacks
func (b *SharedCursorBroadcaster) broadcast(hotspotX, hotspotY, width, height int, pixels []byte, cursorName string) {
	b.mu.RLock()
	callbacks := make([]CursorSocketCallback, 0, len(b.callbacks))
	for _, cb := range b.callbacks {
		callbacks = append(callbacks, cb)
	}
	b.mu.RUnlock()

	for _, cb := range callbacks {
		cb(hotspotX, hotspotY, width, height, pixels, cursorName)
	}
}

// run starts the cursor socket listener and broadcasts events
func (b *SharedCursorBroadcaster) run() {
	b.logger.Info("[SharedCursorBroadcaster] starting shared cursor listener")

	listener, err := NewCursorSocketListener(b.logger)
	if err != nil {
		b.logger.Error("[SharedCursorBroadcaster] failed to create cursor socket listener", "err", err)
		return
	}
	defer listener.Close()

	// Set callback to broadcast to all registered callbacks
	listener.SetCallback(func(hotspotX, hotspotY, width, height int, pixels []byte, cursorName string) {
		b.broadcast(hotspotX, hotspotY, width, height, pixels, cursorName)
	})

	// Run the listener
	listener.Run(b.ctx)
}

// CursorSocketMessage is the JSON message format from the GNOME Shell extension
type CursorSocketMessage struct {
	HotspotX   int    `json:"hotspot_x"`
	HotspotY   int    `json:"hotspot_y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Pixels     string `json:"pixels"`      // Base64 encoded RGBA pixel data
	CursorName string `json:"cursor_name"` // CSS cursor name (fallback when pixels unavailable)
}

// CursorSocketListener listens for cursor updates from the GNOME Shell extension
type CursorSocketListener struct {
	listener net.Listener
	logger   *slog.Logger
	callback func(hotspotX, hotspotY, width, height int, pixels []byte, cursorName string)
	mu       sync.Mutex
	running  bool
}

// NewCursorSocketListener creates a new cursor socket listener
func NewCursorSocketListener(logger *slog.Logger) (*CursorSocketListener, error) {
	// Remove existing socket file if present
	if err := os.Remove(CursorSocketPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove existing cursor socket", "err", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(CursorSocketPath), 0755); err != nil {
		return nil, err
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", CursorSocketPath)
	if err != nil {
		return nil, err
	}

	// Make socket writable by all (for GNOME Shell running as user)
	if err := os.Chmod(CursorSocketPath, 0666); err != nil {
		logger.Warn("failed to chmod cursor socket", "err", err)
	}

	logger.Info("cursor socket listener created", "path", CursorSocketPath)

	return &CursorSocketListener{
		listener: listener,
		logger:   logger,
		running:  true,
	}, nil
}

// SetCallback sets the callback for cursor updates
func (c *CursorSocketListener) SetCallback(callback func(hotspotX, hotspotY, width, height int, pixels []byte, cursorName string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callback = callback
}

// Run starts accepting connections and processing cursor updates
func (c *CursorSocketListener) Run(ctx context.Context) {
	c.logger.Info("starting cursor socket listener")

	// Use a goroutine to close listener when context is cancelled
	// This unblocks the blocking Accept() call
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		if c.listener != nil {
			c.listener.Close()
		}
	}()

	for {
		conn, err := c.listener.Accept()
		if err != nil {
			c.mu.Lock()
			running := c.running
			c.mu.Unlock()
			if !running {
				c.logger.Info("cursor socket listener stopping (context done)")
				return
			}
			c.logger.Debug("cursor socket accept error", "err", err)
			continue
		}

		// Handle connection in goroutine
		go c.handleConnection(conn)
	}
}

// handleConnection processes a single connection from the GNOME Shell extension
func (c *CursorSocketListener) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg CursorSocketMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			c.logger.Debug("failed to parse cursor message", "err", err, "line", line[:min(100, len(line))])
			continue
		}

		// Decode base64 pixel data
		var pixels []byte
		if msg.Pixels != "" {
			var err error
			pixels, err = base64.StdEncoding.DecodeString(msg.Pixels)
			if err != nil {
				c.logger.Debug("failed to decode cursor pixels", "err", err)
				pixels = nil
			}
		}

		c.logger.Info("cursor update from extension",
			"hotspot", [2]int{msg.HotspotX, msg.HotspotY},
			"size", [2]int{msg.Width, msg.Height},
			"pixels_len", len(pixels),
			"cursor_name", msg.CursorName)

		// Call callback
		c.mu.Lock()
		callback := c.callback
		c.mu.Unlock()

		if callback != nil {
			callback(msg.HotspotX, msg.HotspotY, msg.Width, msg.Height, pixels, msg.CursorName)
		}
	}
}

// Close closes the listener
func (c *CursorSocketListener) Close() {
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	if c.listener != nil {
		c.listener.Close()
	}

	// Clean up socket file
	os.Remove(CursorSocketPath)

	c.logger.Info("cursor socket listener closed")
}
