package revdial

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/function61/holepunch-server/pkg/wsconnadapter"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// ClientConfig contains configuration for a RevDial client
type ClientConfig struct {
	ServerURL          string        // API URL (e.g., http://api:8080)
	RunnerID           string        // Unique runner ID
	RunnerToken        string        // Authentication token
	LocalAddr          string        // Local address to proxy (TCP or unix:// socket)
	ReconnectDelay     time.Duration // Delay between reconnection attempts
	InsecureSkipVerify bool          // Skip TLS certificate verification (for self-signed certs)
	ConnectionLogger   func(msg string, args ...interface{}) // Optional logger for connection events
}

// Client is a reusable RevDial client that can be embedded in other services
type Client struct {
	config *ClientConfig
	cancel context.CancelFunc
}

// NewClient creates a new RevDial client
func NewClient(config *ClientConfig) *Client {
	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = 5 * time.Second
	}
	return &Client{config: config}
}

// Start starts the RevDial client in a background goroutine
// Returns immediately; use Stop() to shut down
func (c *Client) Start(ctx context.Context) {
	if c.config.ServerURL == "" || c.config.RunnerToken == "" {
		log.Info().Msg("RevDial not configured (no server URL or token), skipping")
		return
	}

	childCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	log.Info().
		Str("server", c.config.ServerURL).
		Str("runner_id", c.config.RunnerID).
		Str("local_addr", c.config.LocalAddr).
		Msg("Starting RevDial client")

	go c.runLoop(childCtx)
}

// Stop stops the RevDial client
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// runLoop runs the client with auto-reconnect
func (c *Client) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("RevDial client shutting down")
			return
		default:
		}

		if err := c.runConnection(ctx); err != nil {
			log.Error().Err(err).Msg("RevDial connection error")
			log.Info().Dur("reconnect_in", c.config.ReconnectDelay).Msg("Reconnecting...")

			select {
			case <-time.After(c.config.ReconnectDelay):
				continue
			case <-ctx.Done():
				return
			}
		}
	}
}

// runConnection establishes and maintains a single RevDial connection
func (c *Client) runConnection(ctx context.Context) error {
	serverURL := strings.TrimSuffix(c.config.ServerURL, "/") + "/api/v1/revdial"
	host, useTLS := ExtractHostAndTLS(serverURL)

	// Build WebSocket URL for control connection
	wsScheme := "ws://"
	if useTLS {
		wsScheme = "wss://"
	}
	controlURL := fmt.Sprintf("%s%s/api/v1/revdial?runnerid=%s", wsScheme, host, c.config.RunnerID)

	log.Trace().
		Str("control_url", controlURL).
		Bool("tls", useTLS).
		Msg("Connecting to RevDial server via WebSocket")

	// Create WebSocket dialer with TLS config
	wsDialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.config.InsecureSkipVerify,
		},
	}

	// Dial control connection as WebSocket
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.config.RunnerToken)

	controlWS, resp, err := wsDialer.DialContext(ctx, controlURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("failed to connect control WebSocket (status %d): %s: %w", resp.StatusCode, string(body), err)
		}
		return fmt.Errorf("failed to connect control WebSocket: %w", err)
	}

	log.Info().Msg("✅ RevDial control connection established (WebSocket)")

	// Start ping keepalive goroutine to keep connection alive through proxies/load balancers
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := controlWS.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
				if err != nil {
					log.Error().Err(err).Msg("Failed to send WebSocket ping, closing connection")
					controlWS.Close()
					return
				}
				log.Trace().Msg("Sent WebSocket ping keepalive")
			}
		}
	}()

	// Wrap WebSocket as net.Conn for the Listener
	controlConn := wsconnadapter.New(controlWS)

	// Create listener with the WebSocket-wrapped control connection
	listener := NewListener(controlConn, func(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
		dataPath := strings.Replace(path, "/revdial", "/api/v1/revdial", 1)
		dataURL := wsScheme + host + dataPath
		dataHeader := http.Header{}
		dataHeader.Set("Authorization", "Bearer "+c.config.RunnerToken)

		log.Trace().Str("data_url", dataURL).Msg("DATA connection")
		return wsDialer.DialContext(ctx, dataURL, dataHeader)
	})
	defer listener.Close()

	log.Info().Str("local_addr", c.config.LocalAddr).Msg("RevDial listener ready")

	// Accept incoming connections
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		remoteConn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %w", err)
		}

		log.Trace().Msg("Accepted RevDial connection")

		localConn, err := DialLocal(c.config.LocalAddr)
		if err != nil {
			log.Error().Err(err).Str("addr", c.config.LocalAddr).Msg("Failed to connect to local address")
			remoteConn.Close()
			continue
		}

		go ProxyConn(remoteConn, localConn)
	}
}

// DialLocal connects to a local address (Unix socket or TCP)
func DialLocal(addr string) (net.Conn, error) {
	if strings.HasPrefix(addr, "unix://") {
		socketPath := strings.TrimPrefix(addr, "unix://")
		return net.DialTimeout("unix", socketPath, 5*time.Second)
	}
	return net.DialTimeout("tcp", addr, 5*time.Second)
}

// ProxyConn bidirectionally proxies between two connections
func ProxyConn(remote, local net.Conn) {
	defer remote.Close()
	defer local.Close()

	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(local, remote)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(remote, local)
		errChan <- err
	}()

	err := <-errChan
	if err != nil && err != io.EOF {
		log.Trace().Err(err).Msg("Proxy connection ended")
	}
}

// ExtractHostAndTLS extracts host:port and TLS flag from URL
func ExtractHostAndTLS(rawURL string) (host string, useTLS bool) {
	useTLS = strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "wss://")

	rawURL = strings.TrimPrefix(rawURL, "http://")
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "ws://")
	rawURL = strings.TrimPrefix(rawURL, "wss://")

	parts := strings.Split(rawURL, "/")
	host = parts[0]

	if !strings.Contains(host, ":") {
		if useTLS {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}

	return host, useTLS
}
