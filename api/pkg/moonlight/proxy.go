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

	"github.com/helixml/helix/api/pkg/connman"
)

// UDP-over-TCP encapsulation protocol constants
const (
	// Magic bytes to identify our UDP packets
	UDP_PACKET_MAGIC = uint32(0xDEADBEEF)
	// Maximum UDP packet size we'll handle (Moonlight RTP packets are typically < 2KB)
	MAX_UDP_PACKET_SIZE = 8192
	// Header size: magic(4) + length(4) + session_id(8) + port(2) = 18 bytes
	UDP_HEADER_SIZE = 18
)

// UDPPacketHeader represents the header for UDP packets over TCP
type UDPPacketHeader struct {
	Magic     uint32 // 0xDEADBEEF
	Length    uint32 // Length of UDP payload
	SessionID uint64 // Moonlight session ID for routing
	Port      uint16 // Original UDP port (47999, 48100, 48200)
}

// MoonlightSession represents an active Moonlight streaming session
type MoonlightSession struct {
	SessionID      uint64            `json:"session_id"`
	AppID          uint64            `json:"app_id"`
	HelixSessionID string            `json:"helix_session_id"`
	RunnerID       string            `json:"runner_id"`
	ClientIP       string            `json:"client_ip"`
	SecretPayload  [16]byte          `json:"secret_payload"`
	CreatedAt      time.Time         `json:"created_at"`
	LastActivity   time.Time         `json:"last_activity"`
	TCPConn        net.Conn          `json:"-"` // TCP connection to Wolf server
	UDPPorts       map[uint16]string `json:"udp_ports"` // port -> purpose mapping
}

// MoonlightProxy manages Moonlight streaming connections via reverse dial
type MoonlightProxy struct {
	// Connection management
	connman *connman.ConnectionManager
	
	// Session tracking
	sessions     map[uint64]*MoonlightSession // sessionID -> session
	sessionsByIP map[string]*MoonlightSession // clientIP -> session (for RTSP routing)
	mu           sync.RWMutex
	
	// UDP listeners for each Moonlight port
	videoListener   net.PacketConn // Port 48100
	audioListener   net.PacketConn // Port 48200
	controlListener net.PacketConn // Port 47999
	
	// Configuration
	basePort     int    // Base port for TCP listeners (47984, 47989, etc.)
	publicHost   string // Public hostname/IP for clients
	
	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMoonlightProxy creates a new Moonlight proxy manager
func NewMoonlightProxy(connman *connman.ConnectionManager, publicHost string) *MoonlightProxy {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &MoonlightProxy{
		connman:      connman,
		sessions:     make(map[uint64]*MoonlightSession),
		sessionsByIP: make(map[string]*MoonlightSession),
		basePort:     47984, // Standard Moonlight HTTPS port
		publicHost:   publicHost,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start initializes the Moonlight proxy servers
func (mp *MoonlightProxy) Start() error {
	// Start UDP listeners for Moonlight ports
	if err := mp.startUDPListeners(); err != nil {
		return fmt.Errorf("failed to start UDP listeners: %w", err)
	}
	
	log.Info().
		Str("public_host", mp.publicHost).
		Int("base_port", mp.basePort).
		Msg("Moonlight proxy started")
	
	return nil
}

// Stop shuts down the Moonlight proxy
func (mp *MoonlightProxy) Stop() error {
	mp.cancel()
	
	// Close UDP listeners
	if mp.videoListener != nil {
		mp.videoListener.Close()
	}
	if mp.audioListener != nil {
		mp.audioListener.Close()
	}
	if mp.controlListener != nil {
		mp.controlListener.Close()
	}
	
	// Close all session connections
	mp.mu.Lock()
	for _, session := range mp.sessions {
		if session.TCPConn != nil {
			session.TCPConn.Close()
		}
	}
	mp.mu.Unlock()
	
	log.Info().Msg("Moonlight proxy stopped")
	return nil
}

// startUDPListeners creates UDP listeners for Moonlight streaming ports
func (mp *MoonlightProxy) startUDPListeners() error {
	var err error
	
	// Video stream port (48100)
	mp.videoListener, err = net.ListenPacket("udp", ":48100")
	if err != nil {
		return fmt.Errorf("failed to start video UDP listener: %w", err)
	}
	go mp.handleUDPTraffic(mp.videoListener, 48100, "video")
	
	// Audio stream port (48200)
	mp.audioListener, err = net.ListenPacket("udp", ":48200")
	if err != nil {
		return fmt.Errorf("failed to start audio UDP listener: %w", err)
	}
	go mp.handleUDPTraffic(mp.audioListener, 48200, "audio")
	
	// Control stream port (47999)
	mp.controlListener, err = net.ListenPacket("udp", ":47999")
	if err != nil {
		return fmt.Errorf("failed to start control UDP listener: %w", err)
	}
	go mp.handleUDPTraffic(mp.controlListener, 47999, "control")
	
	log.Info().Msg("UDP listeners started for Moonlight ports 47999, 48100, 48200")
	return nil
}

// handleUDPTraffic processes incoming UDP packets and routes them to appropriate backends
func (mp *MoonlightProxy) handleUDPTraffic(listener net.PacketConn, port uint16, streamType string) {
	defer listener.Close()
	
	log.Info().
		Uint16("port", port).
		Str("stream_type", streamType).
		Msg("Started UDP packet handler")
	
	buffer := make([]byte, MAX_UDP_PACKET_SIZE)
	
	for {
		select {
		case <-mp.ctx.Done():
			return
		default:
		}
		
		// Read UDP packet
		n, clientAddr, err := listener.ReadFrom(buffer)
		if err != nil {
			select {
			case <-mp.ctx.Done():
				return
			default:
				log.Error().Err(err).
					Uint16("port", port).
					Str("stream_type", streamType).
					Msg("Error reading UDP packet")
				continue
			}
		}
		
		// Route packet based on client IP and RTP payload
		mp.routeUDPPacket(buffer[:n], clientAddr, port, streamType)
	}
}

// routeUDPPacket determines which backend should receive this UDP packet
func (mp *MoonlightProxy) routeUDPPacket(packet []byte, clientAddr net.Addr, port uint16, streamType string) {
	clientIP := clientAddr.(*net.UDPAddr).IP.String()
	
	// Find session by client IP
	mp.mu.RLock()
	session, exists := mp.sessionsByIP[clientIP]
	mp.mu.RUnlock()
	
	if !exists {
		log.Debug().
			Str("client_ip", clientIP).
			Uint16("port", port).
			Str("stream_type", streamType).
			Msg("No session found for client IP")
		return
	}
	
	// Update last activity
	session.LastActivity = time.Now()
	
	// Forward packet to backend via TCP tunnel
	mp.forwardUDPPacket(session, packet, port)
}

// forwardUDPPacket encapsulates UDP packet and sends it over TCP tunnel
func (mp *MoonlightProxy) forwardUDPPacket(session *MoonlightSession, packet []byte, port uint16) {
	if session.TCPConn == nil {
		log.Error().
			Uint64("session_id", session.SessionID).
			Str("runner_id", session.RunnerID).
			Msg("No TCP connection available for session")
		return
	}
	
	// Create UDP packet header
	header := UDPPacketHeader{
		Magic:     UDP_PACKET_MAGIC,
		Length:    uint32(len(packet)),
		SessionID: session.SessionID,
		Port:      port,
	}
	
	// Serialize header
	headerBytes := make([]byte, UDP_HEADER_SIZE)
	binary.BigEndian.PutUint32(headerBytes[0:4], header.Magic)
	binary.BigEndian.PutUint32(headerBytes[4:8], header.Length)
	binary.BigEndian.PutUint64(headerBytes[8:16], header.SessionID)
	binary.BigEndian.PutUint16(headerBytes[16:18], header.Port)
	
	// Send header + packet over TCP
	if _, err := session.TCPConn.Write(headerBytes); err != nil {
		log.Error().Err(err).
			Uint64("session_id", session.SessionID).
			Str("runner_id", session.RunnerID).
			Msg("Failed to write UDP packet header to TCP connection")
		return
	}
	
	if _, err := session.TCPConn.Write(packet); err != nil {
		log.Error().Err(err).
			Uint64("session_id", session.SessionID).
			Str("runner_id", session.RunnerID).
			Msg("Failed to write UDP packet to TCP connection")
		return
	}
	
	log.Debug().
		Uint64("session_id", session.SessionID).
		Str("runner_id", session.RunnerID).
		Uint16("port", port).
		Int("packet_size", len(packet)).
		Msg("Forwarded UDP packet over TCP tunnel")
}

// RegisterSession adds a new Moonlight session for routing
func (mp *MoonlightProxy) RegisterSession(sessionID uint64, appID uint64, helixSessionID, runnerID, clientIP string, secretPayload [16]byte) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	// Get reverse dial connection to the runner
	tcpConn, err := mp.connman.Dial(mp.ctx, runnerID)
	if err != nil {
		return fmt.Errorf("failed to dial runner %s: %w", runnerID, err)
	}
	
	session := &MoonlightSession{
		SessionID:      sessionID,
		AppID:          appID,
		HelixSessionID: helixSessionID,
		RunnerID:       runnerID,
		ClientIP:       clientIP,
		SecretPayload:  secretPayload,
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		TCPConn:        tcpConn,
		UDPPorts: map[uint16]string{
			47999: "control",
			48100: "video",
			48200: "audio",
		},
	}
	
	mp.sessions[sessionID] = session
	mp.sessionsByIP[clientIP] = session
	
	// Start goroutine to handle incoming UDP packets from backend
	go mp.handleBackendUDPPackets(session)
	
	log.Info().
		Uint64("session_id", sessionID).
		Uint64("app_id", appID).
		Str("helix_session_id", helixSessionID).
		Str("runner_id", runnerID).
		Str("client_ip", clientIP).
		Msg("Registered Moonlight session")
	
	return nil
}

// handleBackendUDPPackets reads UDP packets from backend and forwards to client
func (mp *MoonlightProxy) handleBackendUDPPackets(session *MoonlightSession) {
	defer func() {
		if session.TCPConn != nil {
			session.TCPConn.Close()
		}
		
		// Remove session from tracking
		mp.mu.Lock()
		delete(mp.sessions, session.SessionID)
		delete(mp.sessionsByIP, session.ClientIP)
		mp.mu.Unlock()
		
		log.Info().
			Uint64("session_id", session.SessionID).
			Str("runner_id", session.RunnerID).
			Msg("Cleaned up Moonlight session")
	}()
	
	for {
		select {
		case <-mp.ctx.Done():
			return
		default:
		}
		
		// Read UDP packet header from TCP
		headerBytes := make([]byte, UDP_HEADER_SIZE)
		if _, err := io.ReadFull(session.TCPConn, headerBytes); err != nil {
			if err != io.EOF {
				log.Error().Err(err).
					Uint64("session_id", session.SessionID).
					Msg("Failed to read UDP packet header from TCP")
			}
			return
		}
		
		// Parse header
		magic := binary.BigEndian.Uint32(headerBytes[0:4])
		if magic != UDP_PACKET_MAGIC {
			log.Error().
				Uint64("session_id", session.SessionID).
				Uint32("magic", magic).
				Msg("Invalid UDP packet magic from backend")
			return
		}
		
		length := binary.BigEndian.Uint32(headerBytes[4:8])
		if length > MAX_UDP_PACKET_SIZE {
			log.Error().
				Uint64("session_id", session.SessionID).
				Uint32("length", length).
				Msg("UDP packet too large from backend")
			return
		}
		
		sessionID := binary.BigEndian.Uint64(headerBytes[8:16])
		port := binary.BigEndian.Uint16(headerBytes[16:18])
		
		if sessionID != session.SessionID {
			log.Error().
				Uint64("expected_session_id", session.SessionID).
				Uint64("received_session_id", sessionID).
				Msg("Session ID mismatch in UDP packet from backend")
			return
		}
		
		// Read UDP payload
		payload := make([]byte, length)
		if _, err := io.ReadFull(session.TCPConn, payload); err != nil {
			log.Error().Err(err).
				Uint64("session_id", session.SessionID).
				Msg("Failed to read UDP packet payload from TCP")
			return
		}
		
		// Forward to appropriate UDP listener
		mp.forwardToClient(session, payload, port)
	}
}

// forwardToClient sends UDP packet to the Moonlight client
func (mp *MoonlightProxy) forwardToClient(session *MoonlightSession, payload []byte, port uint16) {
	var listener net.PacketConn
	
	switch port {
	case 47999:
		listener = mp.controlListener
	case 48100:
		listener = mp.videoListener
	case 48200:
		listener = mp.audioListener
	default:
		log.Error().
			Uint64("session_id", session.SessionID).
			Uint16("port", port).
			Msg("Unknown UDP port from backend")
		return
	}
	
	// Parse client address
	clientAddr, err := net.ResolveUDPAddr("udp", session.ClientIP+":0")
	if err != nil {
		log.Error().Err(err).
			Uint64("session_id", session.SessionID).
			Str("client_ip", session.ClientIP).
			Msg("Failed to resolve client UDP address")
		return
	}
	
	// Send UDP packet to client
	if _, err := listener.WriteTo(payload, clientAddr); err != nil {
		log.Error().Err(err).
			Uint64("session_id", session.SessionID).
			Uint16("port", port).
			Msg("Failed to send UDP packet to client")
		return
	}
	
	log.Debug().
		Uint64("session_id", session.SessionID).
		Uint16("port", port).
		Int("payload_size", len(payload)).
		Msg("Forwarded UDP packet to client")
}

// UnregisterSession removes a session from routing
func (mp *MoonlightProxy) UnregisterSession(sessionID uint64) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	if session, exists := mp.sessions[sessionID]; exists {
		if session.TCPConn != nil {
			session.TCPConn.Close()
		}
		
		delete(mp.sessions, sessionID)
		delete(mp.sessionsByIP, session.ClientIP)
		
		log.Info().
			Uint64("session_id", sessionID).
			Str("runner_id", session.RunnerID).
			Msg("Unregistered Moonlight session")
	}
}

// GetSessionInfo returns information about an active session
func (mp *MoonlightProxy) GetSessionInfo(sessionID uint64) (*MoonlightSession, bool) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	
	session, exists := mp.sessions[sessionID]
	return session, exists
}

// ListSessions returns all active sessions
func (mp *MoonlightProxy) ListSessions() []*MoonlightSession {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	
	sessions := make([]*MoonlightSession, 0, len(mp.sessions))
	for _, session := range mp.sessions {
		sessions = append(sessions, session)
	}
	
	return sessions
}