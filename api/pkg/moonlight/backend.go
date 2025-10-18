package moonlight

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// MoonlightBackend handles UDP packet decapsulation on the runner side
type MoonlightBackend struct {
	// Wolf connection info
	wolfHost string
	wolfPort int
	
	// UDP sockets for communicating with Wolf
	videoSocket   net.Conn // Connects to Wolf's video port (48100)
	audioSocket   net.Conn // Connects to Wolf's audio port (48200)
	controlSocket net.Conn // Connects to Wolf's control port (47999)
	
	// TCP connection from proxy (reverse dial)
	proxyConn net.Conn
	
	// Session info
	sessionID uint64
	running   bool
	mu        sync.RWMutex
	
	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMoonlightBackend creates a new backend UDP handler
func NewMoonlightBackend(wolfHost string, wolfPort int) *MoonlightBackend {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &MoonlightBackend{
		wolfHost: wolfHost,
		wolfPort: wolfPort,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins handling UDP packet decapsulation
func (mb *MoonlightBackend) Start(proxyConn net.Conn) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	
	if mb.running {
		return fmt.Errorf("backend already running")
	}
	
	mb.proxyConn = proxyConn
	mb.running = true
	
	// Connect to Wolf's UDP ports via TCP (since Wolf expects TCP for non-UDP)
	// Note: This assumes Wolf is modified to accept TCP connections for UDP traffic
	// In practice, you might need to use UDP sockets here
	if err := mb.connectToWolf(); err != nil {
		return fmt.Errorf("failed to connect to Wolf: %w", err)
	}
	
	// Start packet processing goroutines
	go mb.handleProxyToWolf()
	go mb.handleWolfToProxy()
	
	log.Info().
		Str("wolf_host", mb.wolfHost).
		Int("wolf_port", mb.wolfPort).
		Msg("Moonlight backend started")
	
	return nil
}

// Stop shuts down the backend
func (mb *MoonlightBackend) Stop() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	
	if !mb.running {
		return
	}
	
	mb.cancel()
	mb.running = false
	
	// Close all connections
	if mb.proxyConn != nil {
		mb.proxyConn.Close()
	}
	if mb.videoSocket != nil {
		mb.videoSocket.Close()
	}
	if mb.audioSocket != nil {
		mb.audioSocket.Close()
	}
	if mb.controlSocket != nil {
		mb.controlSocket.Close()
	}
	
	log.Info().Msg("Moonlight backend stopped")
}

// connectToWolf establishes connections to Wolf's UDP ports
func (mb *MoonlightBackend) connectToWolf() error {
	var err error
	
	// Connect to Wolf's video port (48100)
	mb.videoSocket, err = net.Dial("tcp", fmt.Sprintf("%s:%d", mb.wolfHost, 48100))
	if err != nil {
		return fmt.Errorf("failed to connect to Wolf video port: %w", err)
	}
	
	// Connect to Wolf's audio port (48200)
	mb.audioSocket, err = net.Dial("tcp", fmt.Sprintf("%s:%d", mb.wolfHost, 48200))
	if err != nil {
		return fmt.Errorf("failed to connect to Wolf audio port: %w", err)
	}
	
	// Connect to Wolf's control port (47999)
	mb.controlSocket, err = net.Dial("tcp", fmt.Sprintf("%s:%d", mb.wolfHost, 47999))
	if err != nil {
		return fmt.Errorf("failed to connect to Wolf control port: %w", err)
	}
	
	log.Info().
		Str("wolf_host", mb.wolfHost).
		Msg("Connected to Wolf UDP ports via TCP")
	
	return nil
}

// handleProxyToWolf reads encapsulated UDP packets from proxy and forwards to Wolf
func (mb *MoonlightBackend) handleProxyToWolf() {
	defer mb.Stop()
	
	for {
		select {
		case <-mb.ctx.Done():
			return
		default:
		}
		
		// Read UDP packet header from proxy
		headerBytes := make([]byte, UDP_HEADER_SIZE)
		if _, err := io.ReadFull(mb.proxyConn, headerBytes); err != nil {
			if err != io.EOF {
				log.Error().Err(err).Msg("Failed to read UDP packet header from proxy")
			}
			return
		}
		
		// Parse header
		magic := binary.BigEndian.Uint32(headerBytes[0:4])
		if magic != UDP_PACKET_MAGIC {
			log.Error().
				Uint32("magic", magic).
				Msg("Invalid UDP packet magic from proxy")
			return
		}
		
		length := binary.BigEndian.Uint32(headerBytes[4:8])
		if length > MAX_UDP_PACKET_SIZE {
			log.Error().
				Uint32("length", length).
				Msg("UDP packet too large from proxy")
			return
		}
		
		sessionID := binary.BigEndian.Uint64(headerBytes[8:16])
		port := binary.BigEndian.Uint16(headerBytes[16:18])
		
		// Store session ID on first packet
		if mb.sessionID == 0 {
			mb.sessionID = sessionID
		}
		
		// Read UDP payload
		payload := make([]byte, length)
		if _, err := io.ReadFull(mb.proxyConn, payload); err != nil {
			log.Error().Err(err).Msg("Failed to read UDP packet payload from proxy")
			return
		}
		
		// Forward to appropriate Wolf port
		if err := mb.forwardToWolf(payload, port); err != nil {
			log.Error().Err(err).
				Uint16("port", port).
				Msg("Failed to forward packet to Wolf")
		}
		
		log.Debug().
			Uint64("session_id", sessionID).
			Uint16("port", port).
			Int("payload_size", len(payload)).
			Msg("Forwarded UDP packet to Wolf")
	}
}

// handleWolfToProxy reads UDP packets from Wolf and sends to proxy
func (mb *MoonlightBackend) handleWolfToProxy() {
	// Start readers for each Wolf connection
	go mb.readFromWolfSocket(mb.videoSocket, 48100)
	go mb.readFromWolfSocket(mb.audioSocket, 48200)
	go mb.readFromWolfSocket(mb.controlSocket, 47999)
}

// readFromWolfSocket reads packets from a Wolf socket and forwards to proxy
func (mb *MoonlightBackend) readFromWolfSocket(socket net.Conn, port uint16) {
	defer socket.Close()
	
	buffer := make([]byte, MAX_UDP_PACKET_SIZE)
	
	for {
		select {
		case <-mb.ctx.Done():
			return
		default:
		}
		
		// Set read timeout to detect connection issues
		socket.SetReadDeadline(time.Now().Add(30 * time.Second))
		
		n, err := socket.Read(buffer)
		if err != nil {
			if err != io.EOF && !isTimeoutError(err) {
				log.Error().Err(err).
					Uint16("port", port).
					Msg("Error reading from Wolf socket")
			}
			return
		}
		
		// Forward packet to proxy
		if err := mb.forwardToProxy(buffer[:n], port); err != nil {
			log.Error().Err(err).
				Uint16("port", port).
				Msg("Failed to forward packet to proxy")
			return
		}
		
		log.Debug().
			Uint16("port", port).
			Int("packet_size", n).
			Msg("Forwarded packet from Wolf to proxy")
	}
}

// forwardToWolf sends UDP packet to the appropriate Wolf port
func (mb *MoonlightBackend) forwardToWolf(payload []byte, port uint16) error {
	var socket net.Conn
	
	switch port {
	case 47999:
		socket = mb.controlSocket
	case 48100:
		socket = mb.videoSocket
	case 48200:
		socket = mb.audioSocket
	default:
		return fmt.Errorf("unknown UDP port: %d", port)
	}
	
	if socket == nil {
		return fmt.Errorf("no connection to Wolf port %d", port)
	}
	
	// Send raw UDP payload to Wolf
	if _, err := socket.Write(payload); err != nil {
		return fmt.Errorf("failed to write to Wolf socket: %w", err)
	}
	
	return nil
}

// forwardToProxy encapsulates UDP packet and sends to proxy
func (mb *MoonlightBackend) forwardToProxy(payload []byte, port uint16) error {
	if mb.proxyConn == nil {
		return fmt.Errorf("no proxy connection")
	}
	
	// Create UDP packet header
	header := UDPPacketHeader{
		Magic:     UDP_PACKET_MAGIC,
		Length:    uint32(len(payload)),
		SessionID: mb.sessionID,
		Port:      port,
	}
	
	// Serialize header
	headerBytes := make([]byte, UDP_HEADER_SIZE)
	binary.BigEndian.PutUint32(headerBytes[0:4], header.Magic)
	binary.BigEndian.PutUint32(headerBytes[4:8], header.Length)
	binary.BigEndian.PutUint64(headerBytes[8:16], header.SessionID)
	binary.BigEndian.PutUint16(headerBytes[16:18], header.Port)
	
	// Send header + payload to proxy
	if _, err := mb.proxyConn.Write(headerBytes); err != nil {
		return fmt.Errorf("failed to write header to proxy: %w", err)
	}
	
	if _, err := mb.proxyConn.Write(payload); err != nil {
		return fmt.Errorf("failed to write payload to proxy: %w", err)
	}
	
	return nil
}

// isTimeoutError checks if an error is a network timeout
func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// StartMoonlightBackendServer starts a server that listens for proxy connections
func StartMoonlightBackendServer(ctx context.Context, listenAddr, wolfHost string, wolfPort int) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}
	defer listener.Close()
	
	log.Info().
		Str("listen_addr", listenAddr).
		Str("wolf_host", wolfHost).
		Int("wolf_port", wolfPort).
		Msg("Moonlight backend server started")
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				log.Error().Err(err).Msg("Failed to accept connection")
				continue
			}
		}
		
		// Handle each connection in a goroutine
		go func(conn net.Conn) {
			defer conn.Close()
			
			backend := NewMoonlightBackend(wolfHost, wolfPort)
			if err := backend.Start(conn); err != nil {
				log.Error().Err(err).Msg("Failed to start Moonlight backend")
				return
			}
			
			// Wait for context cancellation or connection close
			<-ctx.Done()
			backend.Stop()
		}(conn)
	}
}