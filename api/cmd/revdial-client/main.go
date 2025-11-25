package main

import (
	"bufio"
	"context"
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
	serverURL    = flag.String("server", "", "RevDial server URL (e.g., http://api:8080/revdial)")
	runnerID     = flag.String("runner-id", "", "Unique runner/sandbox ID")
	runnerToken  = flag.String("token", "", "Runner authentication token")
	localAddr    = flag.String("local", "localhost:9876", "Local address to proxy (e.g., localhost:9876 for TCP or unix:///path/to/socket for Unix socket)")
	reconnectSec = flag.Int("reconnect", 5, "Reconnect interval in seconds if connection drops")
)

func main() {
	flag.Parse()

	if *serverURL == "" || *runnerID == "" || *runnerToken == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -server <url> -runner-id <id> -token <token>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -server http://api:8080/revdial -runner-id sandbox-123 -token xyz\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("RevDial client starting...")
	log.Printf("  Server: %s", *serverURL)
	log.Printf("  Runner ID: %s", *runnerID)
	log.Printf("  Local proxy: %s", *localAddr)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, closing RevDial connection...")
		cancel()
	}()

	// Main reconnection loop
	for {
		select {
		case <-ctx.Done():
			log.Println("RevDial client shutting down")
			return
		default:
		}

		if err := runRevDialClient(ctx); err != nil {
			log.Printf("RevDial connection error: %v", err)
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
	// Parse server URL to extract host:port
	host := extractHost(*serverURL)
	dialURL := fmt.Sprintf("%s?runnerid=%s", *serverURL, *runnerID)

	log.Printf("Connecting to RevDial server: %s", dialURL)

	// Dial TCP connection directly (no http.Client - we need raw connection)
	conn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to dial server: %v", err)
	}

	// Send HTTP request that will be hijacked by server
	httpReq, err := http.NewRequest("GET", dialURL, nil)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create request: %v", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+*runnerToken)
	httpReq.Header.Set("Connection", "Upgrade") // Signal that we expect hijacking

	// Write HTTP request to raw TCP connection
	if err := httpReq.Write(conn); err != nil {
		conn.Close()
		return fmt.Errorf("failed to write request: %v", err)
	}

	// Read HTTP response (should be 200 OK before hijacking)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, httpReq)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		conn.Close()
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("✅ Connected to RevDial server (HTTP hijacked connection)")

	// After the 200 OK response, the connection is hijacked and we have raw TCP
	// Create RevDial listener (reverse proxy)
	// For DATA connections, we use WebSocket
	wsDialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	listener := revdial.NewListener(conn, func(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
		// Dial back to server for DATA connections (these use WebSocket)
		// Path comes from server like "/revdial?revdial.dialer=abc123"
		// But our API is at /api/v1/revdial, so we need to rewrite the path
		dataPath := strings.Replace(path, "/revdial", "/api/v1/revdial", 1)
		dataURL := "ws://" + host + dataPath
		header := http.Header{}
		header.Set("Authorization", "Bearer "+*runnerToken)

		log.Printf("DATA connection to: %s", dataURL)
		return wsDialer.DialContext(ctx, dataURL, header)
	})
	defer listener.Close()

	log.Printf("✅ RevDial listener ready, proxying connections to %s", *localAddr)

	// Accept incoming connections and proxy to local server
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

		log.Printf("Accepted RevDial connection, proxying to %s", *localAddr)

		// Connect to local server (supports both TCP and Unix sockets)
		localConn, err := dialLocal(*localAddr)
		if err != nil {
			log.Printf("Failed to connect to local server %s: %v", *localAddr, err)
			remoteConn.Close()
			continue
		}

		// Proxy bidirectionally
		go proxyConn(remoteConn, localConn)
	}
}

func proxyConn(remote, local net.Conn) {
	defer remote.Close()
	defer local.Close()

	errChan := make(chan error, 2)

	// Remote → Local
	go func() {
		_, err := io.Copy(local, remote)
		errChan <- err
	}()

	// Local → Remote
	go func() {
		_, err := io.Copy(remote, local)
		errChan <- err
	}()

	// Wait for either direction to finish
	err := <-errChan
	if err != nil && err != io.EOF {
		log.Printf("Proxy error: %v", err)
	}
}

// dialLocal connects to a local address, supporting both TCP and Unix sockets
func dialLocal(addr string) (net.Conn, error) {
	// Check if it's a Unix socket (starts with "unix://")
	if strings.HasPrefix(addr, "unix://") {
		socketPath := strings.TrimPrefix(addr, "unix://")
		return net.DialTimeout("unix", socketPath, 5*time.Second)
	}

	// Default: TCP connection
	return net.DialTimeout("tcp", addr, 5*time.Second)
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
