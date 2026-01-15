// Package proxy provides resilient proxy connections that survive brief disconnections.
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	ErrReconnectFailed    = errors.New("reconnection failed")
	ErrInputBufferFull    = errors.New("input buffer full")
	ErrOutputBufferFull   = errors.New("output buffer full")
)

const (
	// DefaultBufferSize is the size of each direction's buffer (512KB)
	// At typical rates, this provides several seconds of buffering.
	// If buffer fills before reconnection completes, connection terminates cleanly.
	DefaultBufferSize = 512 * 1024

	// ReconnectTimeout is how long to wait for reconnection
	ReconnectTimeout = 30 * time.Second

	// MaxReconnectAttempts is the maximum number of reconnect attempts
	MaxReconnectAttempts = 3

	// reconnectPollInterval is how often to check if reconnection completed
	reconnectPollInterval = 10 * time.Millisecond
)

// DialFunc is a function that creates a new connection to the server
type DialFunc func(ctx context.Context) (net.Conn, error)

// UpgradeFunc is a function that performs WebSocket upgrade on a connection
type UpgradeFunc func(conn net.Conn) error

// ResilientProxy provides a bidirectional proxy that survives brief server-side disconnections.
// The client connection (browser) stays alive while the server connection (RevDial to desktop)
// can be reconnected transparently. Both directions are buffered during reconnection.
type ResilientProxy struct {
	// Configuration
	sessionID    string
	dialFunc     DialFunc    // Function to dial server (via connman)
	upgradeFunc  UpgradeFunc // Function to upgrade connection to WebSocket
	bufferSize   int

	// Connections
	clientConn net.Conn // Browser connection (stable)
	serverConn net.Conn // Server connection (may reconnect)
	serverMu   sync.Mutex

	// Input buffering (client → server direction)
	inputBuffer    []byte
	inputBufferMu  sync.Mutex
	inputBufferPos int

	// Output buffering (server → client direction)
	outputBuffer    []byte
	outputBufferMu  sync.Mutex
	outputBufferPos int

	// State
	reconnecting   atomic.Bool
	closed         atomic.Bool
	done           chan struct{}
	reconnectDone  chan struct{} // Signals reconnection completed
	reconnectMu    sync.Mutex    // Protects reconnection initiation
	serverError    chan error    // Channel to signal server errors for reconnection

	// Stats
	reconnectCount      atomic.Int64
	inputBytesBuffered  atomic.Int64
	outputBytesBuffered atomic.Int64
}

// ResilientProxyConfig contains configuration for creating a ResilientProxy
type ResilientProxyConfig struct {
	SessionID   string
	ClientConn  net.Conn
	ServerConn  net.Conn
	DialFunc    DialFunc
	UpgradeFunc UpgradeFunc
	BufferSize  int // Size of each direction's buffer (default 512KB)
}

// NewResilientProxy creates a new resilient proxy
func NewResilientProxy(cfg ResilientProxyConfig) *ResilientProxy {
	bufSize := cfg.BufferSize
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}

	return &ResilientProxy{
		sessionID:     cfg.SessionID,
		clientConn:    cfg.ClientConn,
		serverConn:    cfg.ServerConn,
		dialFunc:      cfg.DialFunc,
		upgradeFunc:   cfg.UpgradeFunc,
		bufferSize:    bufSize,
		inputBuffer:   make([]byte, bufSize),
		outputBuffer:  make([]byte, bufSize),
		done:          make(chan struct{}),
		reconnectDone: make(chan struct{}),
		serverError:   make(chan error, 2), // Buffered for both directions
	}
}

// Run starts the bidirectional proxy with reconnection support.
// It blocks until one of the connections is closed or an unrecoverable error occurs.
// Both directions are buffered during server reconnection.
func (p *ResilientProxy) Run(ctx context.Context) error {
	log.Info().
		Str("session_id", p.sessionID).
		Msg("Starting resilient proxy")

	// Error channels for both directions
	clientToServerErr := make(chan error, 1)
	serverToClientErr := make(chan error, 1)

	// Start both copy goroutines - they run for the lifetime of the proxy
	go func() {
		clientToServerErr <- p.copyClientToServer(ctx)
	}()

	go func() {
		serverToClientErr <- p.copyServerToClient(ctx)
	}()

	// Main loop handles reconnection
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.done:
			return nil
		case err := <-clientToServerErr:
			// Client->Server goroutine exited
			if p.closed.Load() {
				return nil
			}
			if p.isClientError(err) {
				log.Info().
					Str("session_id", p.sessionID).
					Err(err).
					Msg("Client connection closed (input direction)")
				return nil
			}
			// Buffer overflow or other fatal error
			log.Error().
				Str("session_id", p.sessionID).
				Err(err).
				Msg("Input direction failed")
			return err

		case err := <-serverToClientErr:
			// Server->Client goroutine exited
			if p.closed.Load() {
				return nil
			}
			if p.isClientError(err) {
				log.Info().
					Str("session_id", p.sessionID).
					Err(err).
					Msg("Client connection closed (output direction)")
				return nil
			}
			// Buffer overflow or other fatal error
			log.Error().
				Str("session_id", p.sessionID).
				Err(err).
				Msg("Output direction failed")
			return err

		case err := <-p.serverError:
			// Server error detected by one of the copy goroutines
			if p.closed.Load() {
				return nil
			}

			log.Warn().
				Str("session_id", p.sessionID).
				Err(err).
				Msg("Server connection error, attempting reconnection")

			if err := p.reconnect(ctx); err != nil {
				log.Error().
					Str("session_id", p.sessionID).
					Err(err).
					Msg("Reconnection failed")
				return fmt.Errorf("reconnection failed: %w", err)
			}

			p.reconnectCount.Add(1)
			log.Info().
				Str("session_id", p.sessionID).
				Int64("reconnect_count", p.reconnectCount.Load()).
				Msg("Reconnected successfully, resuming proxy")
		}
	}
}

// copyClientToServer copies data from client to server with buffering during reconnection.
// This goroutine runs for the lifetime of the proxy, handling reconnections internally.
func (p *ResilientProxy) copyClientToServer(ctx context.Context) error {
	buf := make([]byte, 32*1024) // 32KB read buffer

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.done:
			return nil
		default:
		}

		// Read from client
		n, err := p.clientConn.Read(buf)
		if err != nil {
			return err // Client error - fatal
		}

		if n == 0 {
			continue
		}

		// If reconnecting, buffer the data and wait
		if p.reconnecting.Load() {
			if err := p.bufferInput(buf[:n]); err != nil {
				return fmt.Errorf("input buffer overflow during reconnection: %w", err)
			}
			// Wait for reconnection to complete
			if err := p.waitForReconnect(ctx); err != nil {
				return err
			}
			continue
		}

		// Write to server
		p.serverMu.Lock()
		server := p.serverConn
		p.serverMu.Unlock()

		_, err = server.Write(buf[:n])
		if err != nil {
			// Server write failed - buffer this data and signal for reconnection
			if bufErr := p.bufferInput(buf[:n]); bufErr != nil {
				return fmt.Errorf("input buffer overflow: %w", bufErr)
			}
			p.signalServerError(err)
			// Wait for reconnection to complete
			if err := p.waitForReconnect(ctx); err != nil {
				return err
			}
		}
	}
}

// copyServerToClient copies data from server to client with buffering during reconnection.
// This goroutine runs for the lifetime of the proxy, handling reconnections internally.
func (p *ResilientProxy) copyServerToClient(ctx context.Context) error {
	buf := make([]byte, 32*1024) // 32KB read buffer

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.done:
			return nil
		default:
		}

		// If reconnecting, wait for it to complete
		if p.reconnecting.Load() {
			if err := p.waitForReconnect(ctx); err != nil {
				return err
			}
			continue
		}

		// Read from server
		p.serverMu.Lock()
		server := p.serverConn
		p.serverMu.Unlock()

		n, err := server.Read(buf)
		if err != nil {
			if p.closed.Load() {
				return nil
			}
			// Server read failed - signal for reconnection and wait
			p.signalServerError(err)
			if waitErr := p.waitForReconnect(ctx); waitErr != nil {
				return waitErr
			}
			continue
		}

		if n == 0 {
			continue
		}

		// Write to client
		_, err = p.clientConn.Write(buf[:n])
		if err != nil {
			return err // Client error - fatal
		}
	}
}

// signalServerError notifies the main loop that a server error occurred.
// Only the first error triggers reconnection; subsequent calls are ignored.
func (p *ResilientProxy) signalServerError(err error) {
	p.reconnectMu.Lock()
	defer p.reconnectMu.Unlock()

	// Only signal if not already reconnecting
	if !p.reconnecting.Load() {
		p.reconnecting.Store(true)
		// Create new channel for this reconnection cycle
		p.reconnectDone = make(chan struct{})
		// Non-blocking send
		select {
		case p.serverError <- err:
		default:
		}
	}
}

// waitForReconnect blocks until reconnection completes or context is cancelled
func (p *ResilientProxy) waitForReconnect(ctx context.Context) error {
	p.reconnectMu.Lock()
	done := p.reconnectDone
	p.reconnectMu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	case <-done:
		return nil
	}
}

// bufferInput adds data to the input buffer (client → server direction).
// Returns ErrInputBufferFull if buffer would overflow.
func (p *ResilientProxy) bufferInput(data []byte) error {
	p.inputBufferMu.Lock()
	defer p.inputBufferMu.Unlock()

	if p.inputBufferPos+len(data) > p.bufferSize {
		log.Error().
			Str("session_id", p.sessionID).
			Int("buffer_used", p.inputBufferPos).
			Int("buffer_size", p.bufferSize).
			Int("data_size", len(data)).
			Msg("Input buffer full, terminating connection")
		return ErrInputBufferFull
	}

	copy(p.inputBuffer[p.inputBufferPos:], data)
	p.inputBufferPos += len(data)
	p.inputBytesBuffered.Add(int64(len(data)))
	return nil
}

// bufferOutput adds data to the output buffer (server → client direction).
// Returns ErrOutputBufferFull if buffer would overflow.
func (p *ResilientProxy) bufferOutput(data []byte) error {
	p.outputBufferMu.Lock()
	defer p.outputBufferMu.Unlock()

	if p.outputBufferPos+len(data) > p.bufferSize {
		log.Error().
			Str("session_id", p.sessionID).
			Int("buffer_used", p.outputBufferPos).
			Int("buffer_size", p.bufferSize).
			Int("data_size", len(data)).
			Msg("Output buffer full, terminating connection")
		return ErrOutputBufferFull
	}

	copy(p.outputBuffer[p.outputBufferPos:], data)
	p.outputBufferPos += len(data)
	p.outputBytesBuffered.Add(int64(len(data)))
	return nil
}

// flushInputBuffer writes buffered input to the server and clears the buffer
func (p *ResilientProxy) flushInputBuffer() error {
	p.inputBufferMu.Lock()
	defer p.inputBufferMu.Unlock()

	if p.inputBufferPos == 0 {
		return nil
	}

	p.serverMu.Lock()
	server := p.serverConn
	p.serverMu.Unlock()

	log.Debug().
		Str("session_id", p.sessionID).
		Int("buffered_bytes", p.inputBufferPos).
		Msg("Flushing input buffer after reconnection")

	_, err := server.Write(p.inputBuffer[:p.inputBufferPos])
	if err != nil {
		return fmt.Errorf("failed to flush input buffer: %w", err)
	}

	p.inputBufferPos = 0
	return nil
}

// flushOutputBuffer writes buffered output to the client and clears the buffer
func (p *ResilientProxy) flushOutputBuffer() error {
	p.outputBufferMu.Lock()
	defer p.outputBufferMu.Unlock()

	if p.outputBufferPos == 0 {
		return nil
	}

	log.Debug().
		Str("session_id", p.sessionID).
		Int("buffered_bytes", p.outputBufferPos).
		Msg("Flushing output buffer after reconnection")

	_, err := p.clientConn.Write(p.outputBuffer[:p.outputBufferPos])
	if err != nil {
		return fmt.Errorf("failed to flush output buffer: %w", err)
	}

	p.outputBufferPos = 0
	return nil
}

// reconnect attempts to re-establish the server connection.
// Called by the main Run() loop when a server error is signaled.
func (p *ResilientProxy) reconnect(ctx context.Context) error {
	// Note: reconnecting is already true (set by signalServerError)
	defer func() {
		p.reconnecting.Store(false)
		// Signal waiting goroutines that reconnection is done
		p.reconnectMu.Lock()
		close(p.reconnectDone)
		p.reconnectMu.Unlock()
	}()

	// Close old server connection
	p.serverMu.Lock()
	if p.serverConn != nil {
		p.serverConn.Close()
	}
	p.serverMu.Unlock()

	// Try to reconnect with timeout
	reconnectCtx, cancel := context.WithTimeout(ctx, ReconnectTimeout)
	defer cancel()

	var lastErr error
	for attempt := 1; attempt <= MaxReconnectAttempts; attempt++ {
		log.Debug().
			Str("session_id", p.sessionID).
			Int("attempt", attempt).
			Msg("Attempting reconnection")

		// Dial new connection
		newConn, err := p.dialFunc(reconnectCtx)
		if err != nil {
			lastErr = err
			log.Warn().
				Str("session_id", p.sessionID).
				Int("attempt", attempt).
				Err(err).
				Msg("Dial failed, retrying...")

			select {
			case <-reconnectCtx.Done():
				return reconnectCtx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
				continue
			}
		}

		// Upgrade to WebSocket
		if p.upgradeFunc != nil {
			if err := p.upgradeFunc(newConn); err != nil {
				newConn.Close()
				lastErr = err
				log.Warn().
					Str("session_id", p.sessionID).
					Int("attempt", attempt).
					Err(err).
					Msg("WebSocket upgrade failed, retrying...")
				continue
			}
		}

		// Success - update server connection
		p.serverMu.Lock()
		p.serverConn = newConn
		p.serverMu.Unlock()

		// Flush buffered data in both directions
		if err := p.flushInputBuffer(); err != nil {
			log.Warn().
				Str("session_id", p.sessionID).
				Err(err).
				Msg("Failed to flush input buffer, continuing anyway")
		}

		if err := p.flushOutputBuffer(); err != nil {
			log.Warn().
				Str("session_id", p.sessionID).
				Err(err).
				Msg("Failed to flush output buffer, continuing anyway")
		}

		return nil
	}

	return fmt.Errorf("reconnection failed after %d attempts: %w", MaxReconnectAttempts, lastErr)
}

// isClientError returns true if the error indicates the client connection is dead
func (p *ResilientProxy) isClientError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common client-side close indicators
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "connection reset by peer") ||
		errors.Is(err, io.EOF)
}

// Close stops the proxy and closes all connections
func (p *ResilientProxy) Close() error {
	if p.closed.Swap(true) {
		return nil // Already closed
	}

	close(p.done)

	var errs []error

	if p.clientConn != nil {
		if err := p.clientConn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	p.serverMu.Lock()
	if p.serverConn != nil {
		if err := p.serverConn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	p.serverMu.Unlock()

	// Clear both buffers to release memory and avoid data leakage
	p.inputBufferMu.Lock()
	p.inputBuffer = nil
	p.inputBufferPos = 0
	p.inputBufferMu.Unlock()

	p.outputBufferMu.Lock()
	p.outputBuffer = nil
	p.outputBufferPos = 0
	p.outputBufferMu.Unlock()

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Stats returns statistics about the proxy
func (p *ResilientProxy) Stats() ProxyStats {
	p.inputBufferMu.Lock()
	inputBufferedNow := p.inputBufferPos
	p.inputBufferMu.Unlock()

	p.outputBufferMu.Lock()
	outputBufferedNow := p.outputBufferPos
	p.outputBufferMu.Unlock()

	return ProxyStats{
		SessionID:           p.sessionID,
		ReconnectCount:      p.reconnectCount.Load(),
		InputBytesBuffered:  p.inputBytesBuffered.Load(),
		OutputBytesBuffered: p.outputBytesBuffered.Load(),
		CurrentInputBuffer:  inputBufferedNow,
		CurrentOutputBuffer: outputBufferedNow,
		IsReconnecting:      p.reconnecting.Load(),
	}
}

// ProxyStats contains statistics about a ResilientProxy
type ProxyStats struct {
	SessionID           string
	ReconnectCount      int64
	InputBytesBuffered  int64
	OutputBytesBuffered int64
	CurrentInputBuffer  int
	CurrentOutputBuffer int
	IsReconnecting      bool
}

// CreateWebSocketUpgradeFunc creates an UpgradeFunc for WebSocket connections
func CreateWebSocketUpgradeFunc(path string, wsKey string) UpgradeFunc {
	return func(conn net.Conn) error {
		// Send WebSocket upgrade request
		upgradeReq := fmt.Sprintf("GET %s HTTP/1.1\r\n"+
			"Host: localhost:9876\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: %s\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"\r\n", path, wsKey)

		if _, err := conn.Write([]byte(upgradeReq)); err != nil {
			return fmt.Errorf("failed to send WebSocket upgrade: %w", err)
		}

		// Read upgrade response
		reader := bufio.NewReader(conn)
		resp, err := http.ReadResponse(reader, nil)
		if err != nil {
			return fmt.Errorf("failed to read WebSocket upgrade response: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusSwitchingProtocols {
			return fmt.Errorf("WebSocket upgrade failed with status %d", resp.StatusCode)
		}

		return nil
	}
}
