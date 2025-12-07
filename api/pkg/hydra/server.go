package hydra

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultSocketPath is the default Unix socket path for Hydra API
	DefaultSocketPath = "/var/run/hydra/hydra.sock"

	// Version is the Hydra version
	Version = "1.0.0"
)

// Server is the Hydra HTTP server
type Server struct {
	manager    *Manager
	socketPath string
	listener   net.Listener
	server     *http.Server
	dnsServer  *DNSServer // DNS server for container name resolution

	// Privileged mode settings
	privilegedModeEnabled bool // Controlled by HYDRA_PRIVILEGED_MODE_ENABLED env var
}

// NewServer creates a new Hydra server
func NewServer(manager *Manager, socketPath string) *Server {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	// Check if privileged mode is enabled via environment variable
	privilegedModeEnabled := os.Getenv("HYDRA_PRIVILEGED_MODE_ENABLED") == "true"
	if privilegedModeEnabled {
		log.Warn().Msg("⚠️ HYDRA_PRIVILEGED_MODE_ENABLED=true - Host Docker access available for development")
	}

	// Pass privileged mode setting to manager for BridgeDesktop to use
	manager.SetPrivilegedMode(privilegedModeEnabled)

	// Create DNS server for container name resolution
	// The DNS proxy runs in the outer sandbox and forwards to the sandbox's upstream DNS.
	// This works in both Docker and Kubernetes environments:
	// - Docker: /etc/resolv.conf has 127.0.0.11 (Docker's internal DNS)
	// - Kubernetes: /etc/resolv.conf has CoreDNS IP (e.g., 10.96.0.10)
	//
	// DNS chain:
	// 1. Inner containers use 10.200.X.1:53 (Hydra DNS proxy on bridge gateway)
	// 2. Hydra DNS proxy forwards to sandbox's upstream DNS (from /etc/resolv.conf)
	// 3. Upstream DNS forwards to host/cluster DNS (enterprise DNS if configured)
	upstreamDNS := getUpstreamDNS()
	log.Info().Strs("upstream_dns", upstreamDNS).Msg("Configured Hydra DNS proxy (using sandbox resolv.conf)")
	dnsServer := NewDNSServer(manager, upstreamDNS)
	manager.SetDNSServer(dnsServer)

	return &Server{
		manager:               manager,
		socketPath:            socketPath,
		dnsServer:             dnsServer,
		privilegedModeEnabled: privilegedModeEnabled,
	}
}

// Start starts the HTTP server on Unix socket
func (s *Server) Start(ctx context.Context) error {
	// Remove stale socket
	os.Remove(s.socketPath)

	// Create socket directory
	socketDir := strings.TrimSuffix(s.socketPath, "/hydra.sock")
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket: %w", err)
	}
	s.listener = listener

	// Set socket permissions
	if err := os.Chmod(s.socketPath, 0660); err != nil {
		log.Warn().Err(err).Msg("Failed to set socket permissions")
	}

	// Create router
	router := mux.NewRouter()
	s.registerRoutes(router)

	// Create HTTP server
	s.server = &http.Server{
		Handler: router,
	}

	// Start manager
	if err := s.manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	log.Info().
		Str("socket", s.socketPath).
		Bool("privileged_mode", s.privilegedModeEnabled).
		Msg("Hydra server started")

	// Start serving (blocks)
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Hydra server error")
		}
	}()

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	log.Info().Msg("Stopping Hydra server...")

	// Stop DNS server first
	if s.dnsServer != nil {
		log.Info().Msg("Stopping Hydra DNS servers...")
		s.dnsServer.StopAll()
	}

	// Stop manager (stops all dockerd instances)
	if err := s.manager.Stop(ctx); err != nil {
		log.Error().Err(err).Msg("Error stopping manager")
	}

	// Shutdown HTTP server
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Error shutting down HTTP server")
		}
	}

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Remove socket file
	os.Remove(s.socketPath)

	log.Info().Msg("Hydra server stopped")
	return nil
}

// registerRoutes registers all HTTP routes
func (s *Server) registerRoutes(router *mux.Router) {
	// Health check
	router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Docker instance management
	api := router.PathPrefix("/api/v1").Subrouter()

	// Create or resume Docker instance
	api.HandleFunc("/docker-instances", s.handleCreateInstance).Methods("POST")

	// List all Docker instances
	api.HandleFunc("/docker-instances", s.handleListInstances).Methods("GET")

	// Get Docker instance status
	api.HandleFunc("/docker-instances/{scope_type}/{scope_id}", s.handleGetInstance).Methods("GET")

	// Stop Docker instance (preserves data)
	api.HandleFunc("/docker-instances/{scope_type}/{scope_id}", s.handleDeleteInstance).Methods("DELETE")

	// Purge Docker instance (deletes data)
	api.HandleFunc("/docker-instances/{scope_type}/{scope_id}/data", s.handlePurgeInstance).Methods("DELETE")

	// Privileged mode endpoint (only available when enabled)
	api.HandleFunc("/privileged-mode/status", s.handlePrivilegedModeStatus).Methods("GET")

	// Bridge desktop container to Hydra network (for desktop-to-dev-container communication)
	api.HandleFunc("/bridge-desktop", s.handleBridgeDesktop).Methods("POST")
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := &HealthResponse{
		Status:          "healthy",
		ActiveInstances: len(s.manager.ListInstances().Instances),
		Version:         Version,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleCreateInstance creates or resumes a Docker instance
func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req CreateDockerInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %s", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.ScopeID == "" {
		http.Error(w, "scope_id is required", http.StatusBadRequest)
		return
	}
	if req.ScopeType == "" {
		http.Error(w, "scope_type is required", http.StatusBadRequest)
		return
	}

	// Validate scope type
	switch req.ScopeType {
	case ScopeTypeSpecTask, ScopeTypeSession, ScopeTypeExploratory:
		// Valid
	default:
		http.Error(w, fmt.Sprintf("invalid scope_type: %s (must be spectask, session, or exploratory)", req.ScopeType), http.StatusBadRequest)
		return
	}

	// Handle privileged mode (host Docker access)
	if req.UseHostDocker {
		if !s.privilegedModeEnabled {
			http.Error(w, "privileged mode is not enabled on this sandbox (set HYDRA_PRIVILEGED_MODE_ENABLED=true)", http.StatusForbidden)
			return
		}
		// Return host Docker socket directly - no isolated dockerd
		// The host socket is mounted at /var/run/host-docker.sock to avoid conflict with DinD
		log.Warn().
			Str("scope_type", string(req.ScopeType)).
			Str("scope_id", req.ScopeID).
			Str("user_id", req.UserID).
			Msg("⚠️ Privileged mode: returning host Docker socket")

		resp := &DockerInstanceResponse{
			ScopeType:    req.ScopeType,
			ScopeID:      req.ScopeID,
			DockerSocket: "/var/run/host-docker.sock",
			DockerHost:   "unix:///var/run/host-docker.sock",
			DataRoot:     "/var/lib/docker", // Host's Docker data root
			Status:       StatusRunning,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := s.manager.CreateInstance(ctx, &req)
	if err != nil {
		log.Error().Err(err).
			Str("scope_type", string(req.ScopeType)).
			Str("scope_id", req.ScopeID).
			Msg("Failed to create Docker instance")
		http.Error(w, fmt.Sprintf("failed to create instance: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// handleListInstances lists all Docker instances
func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	resp := s.manager.ListInstances()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGetInstance returns status of a specific Docker instance
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	scopeType := ScopeType(vars["scope_type"])
	scopeID := vars["scope_id"]

	resp, err := s.manager.GetInstance(scopeType, scopeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleDeleteInstance stops a Docker instance (preserves data)
func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	scopeType := ScopeType(vars["scope_type"])
	scopeID := vars["scope_id"]

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := s.manager.DeleteInstance(ctx, scopeType, scopeID)
	if err != nil {
		log.Error().Err(err).
			Str("scope_type", string(scopeType)).
			Str("scope_id", scopeID).
			Msg("Failed to delete Docker instance")
		http.Error(w, fmt.Sprintf("failed to delete instance: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handlePurgeInstance stops Docker instance and deletes all data
func (s *Server) handlePurgeInstance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	scopeType := ScopeType(vars["scope_type"])
	scopeID := vars["scope_id"]

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := s.manager.PurgeInstance(ctx, scopeType, scopeID)
	if err != nil {
		log.Error().Err(err).
			Str("scope_type", string(scopeType)).
			Str("scope_id", scopeID).
			Msg("Failed to purge Docker instance")
		http.Error(w, fmt.Sprintf("failed to purge instance: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// PrivilegedModeStatusResponse is the response for privileged mode status
type PrivilegedModeStatusResponse struct {
	Enabled          bool   `json:"enabled"`
	HostDockerSocket string `json:"host_docker_socket,omitempty"`
	Description      string `json:"description"`
}

// handlePrivilegedModeStatus returns whether privileged mode is available
func (s *Server) handlePrivilegedModeStatus(w http.ResponseWriter, r *http.Request) {
	resp := &PrivilegedModeStatusResponse{
		Enabled: s.privilegedModeEnabled,
	}

	if s.privilegedModeEnabled {
		resp.HostDockerSocket = "/var/run/docker.sock"
		resp.Description = "Host Docker access is available for Helix development"
	} else {
		resp.Description = "Privileged mode is disabled. Set HYDRA_PRIVILEGED_MODE_ENABLED=true to enable."
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleBridgeDesktop bridges a desktop container (on Wolf's dockerd) to a Hydra network
// This enables the desktop to access dev containers started via docker compose
func (s *Server) handleBridgeDesktop(w http.ResponseWriter, r *http.Request) {
	var req BridgeDesktopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %s", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	if req.DesktopContainerID == "" {
		http.Error(w, "desktop_container_id is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := s.manager.BridgeDesktop(ctx, &req)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", req.SessionID).
			Str("desktop_container_id", req.DesktopContainerID).
			Msg("Failed to bridge desktop to Hydra network")
		http.Error(w, fmt.Sprintf("failed to bridge desktop: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// getUpstreamDNS reads /etc/resolv.conf and returns the nameservers configured there.
// This ensures DNS works in both Docker and Kubernetes environments:
// - Docker: returns 127.0.0.11 (Docker's internal DNS) or host DNS
// - Kubernetes: returns CoreDNS IP (e.g., 10.96.0.10)
func getUpstreamDNS() []string {
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to read /etc/resolv.conf, falling back to 8.8.8.8")
		return []string{"8.8.8.8:53"}
	}
	defer file.Close()

	var nameservers []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		// Parse nameserver lines
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ip := fields[1]
				// Skip loopback for systemd-resolved stub (127.0.0.53)
				// It doesn't work from inside containers
				if ip == "127.0.0.53" {
					continue
				}
				// Add port if not present
				if !strings.Contains(ip, ":") {
					ip = ip + ":53"
				}
				nameservers = append(nameservers, ip)
			}
		}
	}

	if len(nameservers) == 0 {
		log.Warn().Msg("No nameservers found in /etc/resolv.conf, falling back to 8.8.8.8")
		return []string{"8.8.8.8:53"}
	}

	return nameservers
}
