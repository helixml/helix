package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/revdial"
)

var (
	apiURL             = flag.String("api-url", "", "Control plane API URL (e.g., http://api.example.com:8080)")
	wolfID             = flag.String("wolf-id", "", "Unique Wolf instance ID")
	runnerToken        = flag.String("token", "", "Runner authenNewClienttication token")
	localAddr          = flag.String("local", "localhost:8080", "Local Wolf API address")
	reconnectSec       = flag.Int("reconnect", 5, "Reconnect interval in seconds if connection drops")
	insecureSkipVerify = flag.Bool("insecure", false, "Skip TLS certificate verification (env: HELIX_INSECURE_TLS)")
)

func main() {
	flag.Parse()

	// Allow environment variable overrides
	if *apiURL == "" {
		*apiURL = os.Getenv("HELIX_API_URL")
	}
	if *wolfID == "" {
		*wolfID = os.Getenv("WOLF_ID")
	}
	if *runnerToken == "" {
		*runnerToken = os.Getenv("RUNNER_TOKEN")
	}

	if *apiURL == "" || *wolfID == "" || *runnerToken == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -api-url <url> -wolf-id <id> -token <token>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  HELIX_API_URL - Control plane API URL\n")
		fmt.Fprintf(os.Stderr, "  WOLF_ID       - Unique Wolf instance ID\n")
		fmt.Fprintf(os.Stderr, "  RUNNER_TOKEN  - Runner authentication token\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -api-url http://api.example.com:8080 -wolf-id wolf-1 -token xyz\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("Wolf RevDial client starting...")
	log.Printf("  API URL: %s", *apiURL)
	log.Printf("  Wolf ID: %s", *wolfID)
	log.Printf("  Local Wolf API: %s", *localAddr)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, closing Wolf RevDial connection...")
		cancel()
	}()

	// Main reconnection loop
	for {
		select {
		case <-ctx.Done():
			log.Println("Wolf RevDial client shutting down")
			return
		default:
		}

		if err := runRevDialClient(ctx); err != nil {
			log.Printf("Wolf RevDial connection error: %v", err)
			log.Printf("Reconnecting in %d seconds...", *reconnectSec)

			select {
			case <-time.After(time.Duration(*reconnectSec) * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}
	}
}

func runRevDialClient(ctx context.Context) error {
	// Convert http:// to ws:// for WebSocket connection
	wsURL := strings.Replace(*apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

	// Wolf instances connect to /api/v1/revdial with runnerid=wolf-{wolf_id}
	dialURL := fmt.Sprintf("%s/api/v1/revdial?runnerid=wolf-%s", wsURL, *wolfID)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+*runnerToken)

	log.Printf("Connecting to control plane via RevDial: %s", dialURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // TODO: make configurable
		},
	}

	wsConn, resp, err := dialer.Dial(dialURL, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to connect: %v (status: %d)", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer wsConn.Close()

	log.Printf("✅ Connected to control plane via RevDial")

	// Start ping keepalive goroutine to keep connection alive through proxies/load balancers
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := wsConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
				if err != nil {
					log.Printf("Failed to send WebSocket ping, closing connection: %v", err)
					wsConn.Close()
					return
				}
				log.Printf("Sent WebSocket ping keepalive")
			}
		}
	}()

	// Upgrade websocket to net.Conn
	conn := &wsConnAdapter{wsConn}

	// Determine WebSocket scheme for data connections
	wsScheme := "ws://"
	if strings.HasPrefix(*apiURL, "https://") || strings.HasPrefix(*apiURL, "wss://") {
		wsScheme = "wss://"
	}

	// Create RevDial listener (reverse proxy)
	listener := revdial.NewListener(conn, func(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
		// Dial back to server for data connections
		dataURL := wsScheme + extractHost(*apiURL) + path
		header := http.Header{}
		header.Set("Authorization", "Bearer "+*runnerToken)

		return dialer.DialContext(ctx, dataURL, header)
	})
	defer listener.Close()

	log.Printf("✅ RevDial listener ready, proxying connections to local Wolf API at %s", *localAddr)

	// Accept incoming connections and proxy to local Wolf API
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Accept connection from API (via RevDial tunnel)
		remoteConn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %v", err)
		}

		log.Printf("Accepted RevDial connection, proxying to local Wolf API at %s", *localAddr)

		// Handle connection in goroutine
		go proxyConnection(remoteConn, *localAddr)
	}
}

func proxyConnection(remoteConn net.Conn, localAddr string) {
	defer remoteConn.Close()

	// Connect to local Wolf API
	localConn, err := net.DialTimeout("tcp", localAddr, 5*time.Second)
	if err != nil {
		log.Printf("Failed to connect to local Wolf API at %s: %v", localAddr, err)
		return
	}
	defer localConn.Close()

	log.Printf("Established proxy connection: RevDial ↔ Wolf API")

	// Bidirectional proxy
	errChan := make(chan error, 2)

	// Remote → Local
	go func() {
		_, err := io.Copy(localConn, remoteConn)
		errChan <- err
	}()

	// Local → Remote
	go func() {
		_, err := io.Copy(remoteConn, localConn)
		errChan <- err
	}()

	// Wait for either direction to finish
	err = <-errChan
	if err != nil && err != io.EOF {
		log.Printf("Proxy error: %v", err)
	}

	log.Printf("Proxy connection closed")
}

func extractHost(url string) string {
	// Extract host from URL (e.g., "http://api:8080/revdial" → "api:8080")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "ws://")
	url = strings.TrimPrefix(url, "wss://")

	parts := strings.Split(url, "/")
	return parts[0]
}

// wsConnAdapter adapts websocket.Conn to net.Conn interface
type wsConnAdapter struct {
	*websocket.Conn
}

func (w *wsConnAdapter) Read(p []byte) (n int, err error) {
	msgType, r, err := w.Conn.NextReader()
	if err != nil {
		return 0, err
	}
	if msgType != websocket.BinaryMessage {
		return 0, fmt.Errorf("unexpected websocket message type: %d", msgType)
	}
	return r.Read(p)
}

func (w *wsConnAdapter) Write(p []byte) (n int, err error) {
	err = w.Conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *wsConnAdapter) SetDeadline(t time.Time) error {
	if err := w.SetReadDeadline(t); err != nil {
		return err
	}
	return w.SetWriteDeadline(t)
}

func (w *wsConnAdapter) SetReadDeadline(t time.Time) error {
	return w.Conn.SetReadDeadline(t)
}

func (w *wsConnAdapter) SetWriteDeadline(t time.Time) error {
	return w.Conn.SetWriteDeadline(t)
}
