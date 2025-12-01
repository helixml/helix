package turn

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pion/turn/v4"
)

// Server wraps a pion/turn server for WebRTC NAT traversal
type Server struct {
	server      *turn.Server
	config      Config
	udpConn     net.PacketConn
	tcpListener net.Listener
	cancelCtx   context.CancelFunc
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
	// TCPListener is an optional TCP listener for TURN-over-TCP/TLS
	// When set, the TURN server will accept TCP connections on this listener.
	// This enables TURNS (TURN over TLS) when used with a TLS-terminating proxy.
	TCPListener net.Listener
	// TURNSHost is the hostname to advertise for TURNS URLs (e.g., "helix.example.com")
	// If empty, TURNS URLs won't be advertised
	TURNSHost string
	// TURNSPort is the port to advertise for TURNS (default: 443)
	TURNSPort int
}

// detectPublicIP attempts to detect the public IP using api.ipify.org with a timeout
func detectPublicIP() string {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get("https://api.ipify.org?format=text")
	if err != nil {
		log.Printf("[TURN] Could not detect public IP via api.ipify.org: %v", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[TURN] Could not read public IP response: %v", err)
		return ""
	}

	publicIP := strings.TrimSpace(string(body))
	if net.ParseIP(publicIP) == nil {
		log.Printf("[TURN] Invalid public IP detected: %s", publicIP)
		return ""
	}

	log.Printf("[TURN] Detected public IP: %s", publicIP)
	return publicIP
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

	// Always try to auto-detect public IP, use configured as fallback
	configuredIP := cfg.PublicIP
	if detectedIP := detectPublicIP(); detectedIP != "" {
		cfg.PublicIP = detectedIP
		log.Printf("[TURN] Using auto-detected public IP: %s (configured was: %s)", detectedIP, configuredIP)
	} else {
		// Fallback to configured value
		if configuredIP == "" {
			return nil, fmt.Errorf("PublicIP is required and could not be auto-detected")
		}
		log.Printf("[TURN] Could not auto-detect public IP, using configured: %s", cfg.PublicIP)
	}

	// Resolve Docker hostname to IP for local relay address
	localIP := configuredIP
	if net.ParseIP(configuredIP) == nil {
		// It's a hostname, resolve it
		addrs, err := net.LookupHost(configuredIP)
		if err != nil || len(addrs) == 0 {
			log.Printf("[TURN] Could not resolve %s to IP, using public IP for all relays", configuredIP)
			localIP = cfg.PublicIP
		} else {
			localIP = addrs[0]
			log.Printf("[TURN] Resolved %s to %s for local relay", configuredIP, localIP)
		}
	}

	// Create UDP listener
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+strconv.Itoa(cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN UDP listener: %w", err)
	}

	// Generate auth key for username/password
	authKey := turn.GenerateAuthKey(cfg.Username, cfg.Realm, cfg.Password)

	// Build ListenerConfigs for TCP if a listener was provided
	var listenerConfigs []turn.ListenerConfig
	if cfg.TCPListener != nil {
		log.Printf("[TURN] Adding TCP listener for TURN-over-TCP/TLS support")
		listenerConfigs = append(listenerConfigs, turn.ListenerConfig{
			Listener: cfg.TCPListener,
			RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
				RelayAddress: net.ParseIP(cfg.PublicIP),
				Address:      "0.0.0.0",
			},
		})
	}

	// Create TURN server with both local and public relay addresses
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
		// PacketConnConfigs defines UDP listeners with multiple relay addresses
		PacketConnConfigs: []turn.PacketConnConfig{
			// Local Docker network relay for low-latency connections
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(localIP), // Use local Docker IP for local clients
					Address:      "0.0.0.0",
				},
			},
			// Public IP relay for external clients
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(cfg.PublicIP), // Use public IP for external clients
					Address:      "0.0.0.0",
				},
			},
		},
		// ListenerConfigs for TCP-based TURN (used for TURNS via TLS proxy)
		ListenerConfigs: listenerConfigs,
	})
	if err != nil {
		udpListener.Close()
		return nil, fmt.Errorf("failed to create TURN server: %w", err)
	}

	_, cancel := context.WithCancel(context.Background())

	s := &Server{
		server:      turnServer,
		config:      cfg,
		udpConn:     udpListener,
		tcpListener: cfg.TCPListener,
		cancelCtx:   cancel,
	}

	log.Printf("[TURN] Server started on 0.0.0.0:%d (public IP: %s)", cfg.Port, cfg.PublicIP)
	if cfg.TCPListener != nil {
		log.Printf("[TURN] TCP listener enabled for TURN-over-TCP/TLS (TURNS)")
	}
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
	urls := []string{
		fmt.Sprintf("turn:%s:%d?transport=udp", s.config.PublicIP, s.config.Port),
		fmt.Sprintf("turn:%s:%d?transport=tcp", s.config.PublicIP, s.config.Port),
	}

	// Add TURNS URL if configured (for TLS-terminated connections)
	if s.config.TURNSHost != "" {
		port := s.config.TURNSPort
		if port == 0 {
			port = 443
		}
		urls = append(urls, fmt.Sprintf("turns:%s:%d?transport=tcp", s.config.TURNSHost, port))
	}

	return urls
}
