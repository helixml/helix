package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testServer simulates the desktop-side server.
// It can be "killed" and restarted to simulate RevDial disconnection.
type testServer struct {
	listener   net.Listener
	addr       string
	conns      []net.Conn
	connsMu    sync.Mutex
	acceptCh   chan net.Conn
	closed     atomic.Bool
	echoData   bool // If true, echo received data back
	onReceive  func([]byte)
}

func newTestServer(t *testing.T, echoData bool) *testServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	s := &testServer{
		listener: listener,
		addr:     listener.Addr().String(),
		acceptCh: make(chan net.Conn, 10),
		echoData: echoData,
	}

	go s.acceptLoop()
	return s
}

func (s *testServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			continue
		}
		s.connsMu.Lock()
		s.conns = append(s.conns, conn)
		s.connsMu.Unlock()

		s.acceptCh <- conn

		if s.echoData {
			go s.handleEcho(conn)
		}
	}
}

func (s *testServer) handleEcho(conn net.Conn) {
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		if s.onReceive != nil {
			s.onReceive(buf[:n])
		}
		_, err = conn.Write(buf[:n])
		if err != nil {
			return
		}
	}
}

// killAllConns closes all active connections (simulates RevDial disconnect)
func (s *testServer) killAllConns() {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	for _, conn := range s.conns {
		conn.Close()
	}
	s.conns = nil
}

// waitForConn waits for a new connection with timeout
func (s *testServer) waitForConn(timeout time.Duration) (net.Conn, error) {
	select {
	case conn := <-s.acceptCh:
		return conn, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for connection")
	}
}

func (s *testServer) close() {
	s.closed.Store(true)
	s.listener.Close()
	s.killAllConns()
}

// TestResilientProxy_BasicBidirectional tests basic bidirectional data flow
func TestResilientProxy_BasicBidirectional(t *testing.T) {
	// Create test server (echo mode)
	server := newTestServer(t, true)
	defer server.close()

	// Create client connection pair (simulates browser <-> API)
	clientConn, proxyClientConn := net.Pipe()
	defer clientConn.Close()
	defer proxyClientConn.Close()

	// Initial server connection
	serverConn, err := net.Dial("tcp", server.addr)
	if err != nil {
		t.Fatalf("Failed to dial server: %v", err)
	}

	// Wait for server to accept
	_, err = server.waitForConn(time.Second)
	if err != nil {
		t.Fatalf("Server didn't accept: %v", err)
	}

	// Create resilient proxy
	proxy := NewResilientProxy(ResilientProxyConfig{
		SessionID:  "test-session",
		ClientConn: proxyClientConn,
		ServerConn: serverConn,
		DialFunc: func(ctx context.Context) (net.Conn, error) {
			return net.Dial("tcp", server.addr)
		},
		UpgradeFunc: nil, // No WebSocket upgrade for this test
	})

	// Run proxy in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- proxy.Run(ctx)
	}()

	// Test: Send data from client, should echo back
	testData := []byte("hello world")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write to client conn: %v", err)
	}

	// Read echo response
	buf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read echo: %v", err)
	}

	if string(buf[:n]) != string(testData) {
		t.Errorf("Expected %q, got %q", testData, buf[:n])
	}

	// Clean shutdown
	cancel()
	proxy.Close()
}

// TestResilientProxy_SurvivesDisconnect tests that the proxy survives server disconnection
func TestResilientProxy_SurvivesDisconnect(t *testing.T) {
	// Create test server (echo mode)
	server := newTestServer(t, true)
	defer server.close()

	// Create client connection pair
	clientConn, proxyClientConn := net.Pipe()
	defer clientConn.Close()
	defer proxyClientConn.Close()

	// Initial server connection
	serverConn, err := net.Dial("tcp", server.addr)
	if err != nil {
		t.Fatalf("Failed to dial server: %v", err)
	}

	_, err = server.waitForConn(time.Second)
	if err != nil {
		t.Fatalf("Server didn't accept initial connection: %v", err)
	}

	// Track reconnection attempts
	var dialCount atomic.Int32

	proxy := NewResilientProxy(ResilientProxyConfig{
		SessionID:  "test-session",
		ClientConn: proxyClientConn,
		ServerConn: serverConn,
		DialFunc: func(ctx context.Context) (net.Conn, error) {
			dialCount.Add(1)
			return net.Dial("tcp", server.addr)
		},
		UpgradeFunc: nil,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- proxy.Run(ctx)
	}()

	// Step 1: Verify initial connection works
	testData1 := []byte("before disconnect")
	_, err = clientConn.Write(testData1)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	buf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read echo before disconnect: %v", err)
	}
	if string(buf[:n]) != string(testData1) {
		t.Errorf("Before disconnect: expected %q, got %q", testData1, buf[:n])
	}
	t.Logf("Before disconnect: echo works")

	// Step 2: Kill server connections (simulate RevDial disconnect)
	t.Log("Killing server connections...")
	server.killAllConns()

	// Give the proxy a moment to detect the disconnect
	time.Sleep(100 * time.Millisecond)

	// Step 3: Send data while disconnected (should be buffered)
	testData2 := []byte("during disconnect")
	_, err = clientConn.Write(testData2)
	if err != nil {
		t.Fatalf("Failed to write during disconnect: %v", err)
	}
	t.Logf("Wrote data during disconnect (should be buffered)")

	// Step 4: Wait for reconnection and verify data flows
	// The proxy should reconnect and the echo server should respond
	_, err = server.waitForConn(5 * time.Second)
	if err != nil {
		t.Fatalf("Server didn't accept reconnection: %v", err)
	}
	t.Logf("Server accepted reconnection")

	// Read the echo of buffered data
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read echo after reconnect: %v", err)
	}
	if string(buf[:n]) != string(testData2) {
		t.Errorf("After reconnect: expected %q, got %q", testData2, buf[:n])
	}
	t.Logf("After reconnect: buffered data echoed successfully")

	// Verify reconnection happened
	if dialCount.Load() < 1 {
		t.Error("Expected at least one reconnection dial")
	}
	t.Logf("Dial count: %d", dialCount.Load())

	// Step 5: Verify connection still works after reconnect
	testData3 := []byte("after reconnect")
	_, err = clientConn.Write(testData3)
	if err != nil {
		t.Fatalf("Failed to write after reconnect: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read final echo: %v", err)
	}
	if string(buf[:n]) != string(testData3) {
		t.Errorf("Final echo: expected %q, got %q", testData3, buf[:n])
	}
	t.Logf("Post-reconnect: new data flows correctly")

	// Check stats
	stats := proxy.Stats()
	t.Logf("Stats: reconnects=%d, input_buffered=%d, output_buffered=%d",
		stats.ReconnectCount, stats.InputBytesBuffered, stats.OutputBytesBuffered)

	if stats.ReconnectCount < 1 {
		t.Error("Expected ReconnectCount >= 1")
	}

	cancel()
	proxy.Close()
}

// TestResilientProxy_BufferOverflow tests clean termination on buffer overflow
func TestResilientProxy_BufferOverflow(t *testing.T) {
	// Create a server that accepts once then closes
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	addr := listener.Addr().String()

	// Accept initial connection then close it and stop accepting
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			time.Sleep(50 * time.Millisecond)
			conn.Close()
		}
		listener.Close() // Stop accepting new connections
	}()

	clientConn, proxyClientConn := net.Pipe()

	serverConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	// Small buffer size to trigger overflow quickly
	smallBufferSize := 256

	// Track dial attempts
	var dialAttempts atomic.Int32

	proxy := NewResilientProxy(ResilientProxyConfig{
		SessionID:  "test-overflow",
		ClientConn: proxyClientConn,
		ServerConn: serverConn,
		DialFunc: func(ctx context.Context) (net.Conn, error) {
			dialAttempts.Add(1)
			// Always fail immediately - server is gone
			return nil, fmt.Errorf("server unavailable")
		},
		BufferSize: smallBufferSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- proxy.Run(ctx)
	}()

	// Wait for server connection to close
	time.Sleep(100 * time.Millisecond)

	// Write data in a goroutine (non-blocking) to avoid pipe deadlock
	writeErr := make(chan error, 1)
	go func() {
		bigData := make([]byte, smallBufferSize*2)
		for i := range bigData {
			bigData[i] = byte(i % 256)
		}

		for i := 0; i < 20; i++ {
			clientConn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
			_, err := clientConn.Write(bigData)
			if err != nil {
				writeErr <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		writeErr <- nil
	}()

	// Wait for proxy to terminate (should fail due to reconnection failure or buffer overflow)
	select {
	case err := <-proxyDone:
		t.Logf("Proxy terminated with: %v (dial attempts: %d)", err, dialAttempts.Load())
		if err == nil {
			t.Error("Expected proxy to terminate with error")
		}
	case <-time.After(12 * time.Second):
		t.Error("Proxy didn't terminate within timeout")
	}

	// Cleanup
	proxy.Close()
	clientConn.Close()
	proxyClientConn.Close()

	// Drain write goroutine
	select {
	case <-writeErr:
	case <-time.After(time.Second):
	}
}

// TestResilientProxy_MultipleReconnects tests surviving multiple disconnections
func TestResilientProxy_MultipleReconnects(t *testing.T) {
	server := newTestServer(t, true)
	defer server.close()

	clientConn, proxyClientConn := net.Pipe()
	defer clientConn.Close()
	defer proxyClientConn.Close()

	serverConn, err := net.Dial("tcp", server.addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	_, err = server.waitForConn(time.Second)
	if err != nil {
		t.Fatalf("Server didn't accept: %v", err)
	}

	var dialCount atomic.Int32

	proxy := NewResilientProxy(ResilientProxyConfig{
		SessionID:  "test-multi-reconnect",
		ClientConn: proxyClientConn,
		ServerConn: serverConn,
		DialFunc: func(ctx context.Context) (net.Conn, error) {
			dialCount.Add(1)
			return net.Dial("tcp", server.addr)
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go proxy.Run(ctx)

	buf := make([]byte, 1024)

	// Perform multiple disconnect/reconnect cycles
	for cycle := 1; cycle <= 3; cycle++ {
		t.Logf("=== Cycle %d ===", cycle)

		// Send and verify echo
		testData := []byte(fmt.Sprintf("cycle %d data", cycle))
		_, err = clientConn.Write(testData)
		if err != nil {
			t.Fatalf("Cycle %d: write failed: %v", cycle, err)
		}

		clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := clientConn.Read(buf)
		if err != nil {
			t.Fatalf("Cycle %d: read failed: %v", cycle, err)
		}
		if string(buf[:n]) != string(testData) {
			t.Errorf("Cycle %d: expected %q, got %q", cycle, testData, buf[:n])
		}
		t.Logf("Cycle %d: echo works", cycle)

		// Kill connections
		server.killAllConns()
		time.Sleep(50 * time.Millisecond)

		// Wait for reconnection
		_, err = server.waitForConn(5 * time.Second)
		if err != nil {
			t.Fatalf("Cycle %d: reconnection failed: %v", cycle, err)
		}
		t.Logf("Cycle %d: reconnected", cycle)
	}

	stats := proxy.Stats()
	t.Logf("Final stats: reconnects=%d", stats.ReconnectCount)

	if stats.ReconnectCount < 3 {
		t.Errorf("Expected at least 3 reconnects, got %d", stats.ReconnectCount)
	}

	cancel()
	proxy.Close()
}

// TestResilientProxy_ServerToClientFlow tests data flowing from server to client
func TestResilientProxy_ServerToClientFlow(t *testing.T) {
	// Server that sends data to client
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverSendCh := make(chan []byte, 10)
	serverConnCh := make(chan net.Conn, 1)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			serverConnCh <- conn

			// Send data from server to client
			go func(c net.Conn) {
				for data := range serverSendCh {
					c.Write(data)
				}
			}(conn)
		}
	}()

	clientConn, proxyClientConn := net.Pipe()
	defer clientConn.Close()
	defer proxyClientConn.Close()

	serverConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	// Wait for server to accept
	select {
	case <-serverConnCh:
	case <-time.After(time.Second):
		t.Fatal("Server didn't accept")
	}

	proxy := NewResilientProxy(ResilientProxyConfig{
		SessionID:  "test-server-to-client",
		ClientConn: proxyClientConn,
		ServerConn: serverConn,
		DialFunc: func(ctx context.Context) (net.Conn, error) {
			return net.Dial("tcp", listener.Addr().String())
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go proxy.Run(ctx)

	// Send data from server, verify client receives it
	testData := []byte("server says hello")
	serverSendCh <- testData

	buf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Client failed to read: %v", err)
	}

	if string(buf[:n]) != string(testData) {
		t.Errorf("Expected %q, got %q", testData, buf[:n])
	}
	t.Log("Server-to-client data flow works")

	cancel()
	proxy.Close()
	close(serverSendCh)
}
