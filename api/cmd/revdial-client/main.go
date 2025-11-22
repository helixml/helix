package main

import (
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
	localAddr    = flag.String("local", "localhost:9876", "Local address to proxy (screenshot/clipboard server)")
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
	// Connect to API's /revdial endpoint with authentication
	dialURL := fmt.Sprintf("%s?runnerid=%s", *serverURL, *runnerID)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+*runnerToken)

	log.Printf("Connecting to RevDial server: %s", dialURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	wsConn, resp, err := dialer.Dial(dialURL, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to connect: %v (status: %d)", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer wsConn.Close()

	log.Printf("✅ Connected to RevDial server")

	// Upgrade websocket to net.Conn
	conn := &wsConnAdapter{wsConn}

	// Create RevDial listener (reverse proxy)
	listener := revdial.NewListener(conn, func(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
		// Dial back to server for data connections
		dataURL := "ws://" + extractHost(*serverURL) + path
		header := http.Header{}
		header.Set("Authorization", "Bearer "+*runnerToken)

		return dialer.DialContext(ctx, dataURL, header)
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

		// Connect to local screenshot/clipboard server
		localConn, err := net.DialTimeout("tcp", *localAddr, 5*time.Second)
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
