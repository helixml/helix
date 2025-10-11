package turn

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/pion/turn/v4"
)

// Server wraps a pion/turn server for WebRTC NAT traversal
type Server struct {
	server    *turn.Server
	config    Config
	udpConn   net.PacketConn
	cancelCtx context.CancelFunc
}

// Config holds TURN server configuration
type Config struct {
	// PublicIP is the IP address that TURN clients can reach
	PublicIP string
	// Port is the UDP port to listen on (default: 3478)
	Port int
	// Realm is the authentication realm (default: "helix.ai")
	Realm string
	// Username for TURN authentication
	Username string
	// Password for TURN authentication
	Password string
}

// New creates and starts a new TURN server
func New(cfg Config) (*Server, error) {
	// Set defaults
	if cfg.Port == 0 {
		cfg.Port = 3478
	}
	if cfg.Realm == "" {
		cfg.Realm = "helix.ai"
	}
	if cfg.Username == "" {
		cfg.Username = "helix"
	}
	if cfg.Password == "" {
		cfg.Password = "helix-turn-secret"
	}

	// Validate required config
	if cfg.PublicIP == "" {
		return nil, fmt.Errorf("PublicIP is required")
	}

	// Create UDP listener
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+strconv.Itoa(cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN UDP listener: %w", err)
	}

	// Generate auth key for username/password
	authKey := turn.GenerateAuthKey(cfg.Username, cfg.Realm, cfg.Password)

	// Create TURN server
	turnServer, err := turn.NewServer(turn.ServerConfig{
		Realm: cfg.Realm,
		// AuthHandler validates credentials
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			if username == cfg.Username {
				return authKey, true
			}
			log.Printf("[TURN] Authentication failed for user: %s from %s", username, srcAddr)
			return nil, false
		},
		// PacketConnConfigs defines UDP listeners
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(cfg.PublicIP),
					Address:      "0.0.0.0",
				},
			},
		},
	})
	if err != nil {
		udpListener.Close()
		return nil, fmt.Errorf("failed to create TURN server: %w", err)
	}

	_, cancel := context.WithCancel(context.Background())

	s := &Server{
		server:    turnServer,
		config:    cfg,
		udpConn:   udpListener,
		cancelCtx: cancel,
	}

	log.Printf("[TURN] Server started on 0.0.0.0:%d (public IP: %s)", cfg.Port, cfg.PublicIP)
	log.Printf("[TURN] Realm: %s, Username: %s", cfg.Realm, cfg.Username)

	return s, nil
}

// Close shuts down the TURN server gracefully
func (s *Server) Close() error {
	if s.cancelCtx != nil {
		s.cancelCtx()
	}

	if s.server != nil {
		if err := s.server.Close(); err != nil {
			return fmt.Errorf("failed to close TURN server: %w", err)
		}
	}

	if s.udpConn != nil {
		if err := s.udpConn.Close(); err != nil {
			return fmt.Errorf("failed to close UDP connection: %w", err)
		}
	}

	log.Printf("[TURN] Server stopped")
	return nil
}

// GetConfig returns the server configuration (useful for clients)
func (s *Server) GetConfig() Config {
	return s.config
}

// GetCredentials returns TURN credentials for WebRTC clients
func (s *Server) GetCredentials() (username, password, realm string) {
	return s.config.Username, s.config.Password, s.config.Realm
}

// GetURLs returns TURN server URLs for WebRTC configuration
func (s *Server) GetURLs() []string {
	return []string{
		fmt.Sprintf("turn:%s:%d", s.config.PublicIP, s.config.Port),
		fmt.Sprintf("turn:%s:%d?transport=udp", s.config.PublicIP, s.config.Port),
	}
}
