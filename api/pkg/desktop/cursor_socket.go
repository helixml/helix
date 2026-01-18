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
)

const (
	// CursorSocketPath is the Unix socket path for cursor updates from GNOME Shell extension
	CursorSocketPath = "/run/user/1000/helix-cursor.sock"
)

// CursorSocketMessage is the JSON message format from the GNOME Shell extension
type CursorSocketMessage struct {
	HotspotX int    `json:"hotspot_x"`
	HotspotY int    `json:"hotspot_y"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Pixels   string `json:"pixels"` // Base64 encoded ARGB pixel data
}

// CursorSocketListener listens for cursor updates from the GNOME Shell extension
type CursorSocketListener struct {
	listener net.Listener
	logger   *slog.Logger
	callback func(hotspotX, hotspotY, width, height int, pixels []byte)
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
func (c *CursorSocketListener) SetCallback(callback func(hotspotX, hotspotY, width, height int, pixels []byte)) {
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
			"pixels_len", len(pixels))

		// Call callback
		c.mu.Lock()
		callback := c.callback
		c.mu.Unlock()

		if callback != nil {
			callback(msg.HotspotX, msg.HotspotY, msg.Width, msg.Height, pixels)
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
