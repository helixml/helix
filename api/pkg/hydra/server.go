package hydra

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	manager             *Manager
	devContainerManager *DevContainerManager // Dev container management (desktop container lifecycle)
	socketPath          string
	listener            net.Listener
	server              *http.Server
	dnsServer           *DNSServer // DNS server for container name resolution

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

	// Create dev container manager for desktop container lifecycle functionality
	devContainerManager := NewDevContainerManager(manager)

	return &Server{
		manager:               manager,
		devContainerManager:   devContainerManager,
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

	// Dev container management (desktop container lifecycle - container lifecycle)
	api.HandleFunc("/dev-containers", s.handleCreateDevContainer).Methods("POST")
	api.HandleFunc("/dev-containers", s.handleListDevContainers).Methods("GET")
	api.HandleFunc("/dev-containers/{session_id}", s.handleGetDevContainer).Methods("GET")
	api.HandleFunc("/dev-containers/{session_id}", s.handleDeleteDevContainer).Methods("DELETE")
	api.HandleFunc("/dev-containers/{session_id}/clients", s.handleGetDevContainerClients).Methods("GET")

	// System stats (GPU info, active sessions)
	api.HandleFunc("/system/stats", s.handleSystemStats).Methods("GET")
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

// handleBridgeDesktop bridges a desktop container (on sandbox dockerd) to a Hydra network
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

// handleCreateDevContainer creates a new dev container (desktop container lifecycle)
func (s *Server) handleCreateDevContainer(w http.ResponseWriter, r *http.Request) {
	var req CreateDevContainerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %s", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	if req.Image == "" {
		http.Error(w, "image is required", http.StatusBadRequest)
		return
	}
	if req.ContainerName == "" {
		http.Error(w, "container_name is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	resp, err := s.devContainerManager.CreateDevContainer(ctx, &req)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", req.SessionID).
			Str("container_name", req.ContainerName).
			Msg("Failed to create dev container")
		http.Error(w, fmt.Sprintf("failed to create dev container: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// handleListDevContainers lists all active dev containers
func (s *Server) handleListDevContainers(w http.ResponseWriter, r *http.Request) {
	resp := s.devContainerManager.ListDevContainers()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGetDevContainer returns status of a specific dev container
func (s *Server) handleGetDevContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp, err := s.devContainerManager.GetDevContainer(ctx, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleDeleteDevContainer stops and removes a dev container
func (s *Server) handleDeleteDevContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := s.devContainerManager.DeleteDevContainer(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", sessionID).
			Msg("Failed to delete dev container")
		http.Error(w, fmt.Sprintf("failed to delete dev container: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGetDevContainerClients proxies a request to the desktop container's /clients endpoint
// to get the list of connected WebSocket users for multi-player visibility
func (s *Server) handleGetDevContainerClients(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get the dev container to find its IP address
	container, err := s.devContainerManager.GetDevContainer(ctx, sessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("dev container not found: %s", err), http.StatusNotFound)
		return
	}

	if container.IPAddress == "" {
		http.Error(w, "dev container has no IP address", http.StatusServiceUnavailable)
		return
	}

	if container.Status != DevContainerStatusRunning {
		http.Error(w, fmt.Sprintf("dev container not running (status: %s)", container.Status), http.StatusServiceUnavailable)
		return
	}

	// Proxy request to the desktop container's /clients endpoint
	desktopURL := fmt.Sprintf("http://%s:9876/clients", container.IPAddress)

	req, err := http.NewRequestWithContext(ctx, "GET", desktopURL, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create request: %s", err), http.StatusInternalServerError)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn().Err(err).
			Str("session_id", sessionID).
			Str("desktop_url", desktopURL).
			Msg("Failed to query desktop /clients endpoint")
		// Return empty clients list instead of error (desktop may not have endpoint yet)
		emptyResp := struct {
			SessionID string        `json:"session_id"`
			Clients   []interface{} `json:"clients"`
		}{
			SessionID: sessionID,
			Clients:   []interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(emptyResp)
		return
	}
	defer resp.Body.Close()

	// Forward the response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
}

// handleSystemStats returns GPU stats and session counts
func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	gpus := getGPUInfo()
	containers := s.devContainerManager.ListDevContainers()

	resp := &SystemStatsResponse{
		GPUs:             gpus,
		ActiveContainers: len(containers.Containers),
		ActiveSessions:   len(containers.Containers),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// getGPUInfo detects and queries GPU information
func getGPUInfo() []GPUInfo {
	var gpus []GPUInfo

	// Try NVIDIA first (nvidia-smi)
	gpus = append(gpus, getNvidiaGPUs()...)

	// Try AMD (rocm-smi) if no NVIDIA GPUs found
	if len(gpus) == 0 {
		gpus = append(gpus, getAMDGPUs()...)
	}

	return gpus
}

// getNvidiaGPUs queries NVIDIA GPUs using nvidia-smi
func getNvidiaGPUs() []GPUInfo {
	var gpus []GPUInfo

	// Check if nvidia-smi exists
	if _, err := os.Stat("/usr/bin/nvidia-smi"); err != nil {
		return gpus
	}

	// Query GPU info in CSV format
	// nvidia-smi --query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu --format=csv,noheader,nounits
	cmd := fmt.Sprintf("nvidia-smi --query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu --format=csv,noheader,nounits 2>/dev/null")

	// Execute via /bin/sh
	out, err := execCommand("sh", "-c", cmd)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to query NVIDIA GPUs")
		return gpus
	}

	// Parse output (one line per GPU)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ", ")
		if len(fields) < 7 {
			continue
		}

		var index, memTotal, memUsed, memFree, util, temp int64
		fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &index)
		fmt.Sscanf(strings.TrimSpace(fields[2]), "%d", &memTotal)
		fmt.Sscanf(strings.TrimSpace(fields[3]), "%d", &memUsed)
		fmt.Sscanf(strings.TrimSpace(fields[4]), "%d", &memFree)
		fmt.Sscanf(strings.TrimSpace(fields[5]), "%d", &util)
		fmt.Sscanf(strings.TrimSpace(fields[6]), "%d", &temp)

		gpus = append(gpus, GPUInfo{
			Index:       int(index),
			Name:        strings.TrimSpace(fields[1]),
			Vendor:      "nvidia",
			MemoryTotal: memTotal * 1024 * 1024, // MiB to bytes
			MemoryUsed:  memUsed * 1024 * 1024,
			MemoryFree:  memFree * 1024 * 1024,
			Utilization: int(util),
			Temperature: int(temp),
		})
	}

	return gpus
}

// getAMDGPUs queries AMD GPUs using rocm-smi or fallback detection
func getAMDGPUs() []GPUInfo {
	var gpus []GPUInfo

	// Check if rocm-smi exists
	hasRocmSmi := false
	if _, err := os.Stat("/usr/local/bin/rocm-smi"); err == nil {
		hasRocmSmi = true
	} else if _, err := os.Stat("/opt/rocm/bin/rocm-smi"); err == nil {
		hasRocmSmi = true
	}

	if hasRocmSmi {
		// Query GPU info via rocm-smi
		cmd := "rocm-smi --showid --showtemp --showuse --showmeminfo vram --csv 2>/dev/null"
		out, err := execCommand("sh", "-c", cmd)
		if err != nil {
			log.Debug().Err(err).Msg("Failed to query AMD GPUs via rocm-smi")
		} else {
			// Parse rocm-smi CSV output
			lines := strings.Split(strings.TrimSpace(out), "\n")
			for i, line := range lines {
				if i == 0 || line == "" { // Skip header
					continue
				}
				fields := strings.Split(line, ",")
				if len(fields) < 4 {
					continue
				}

				var index int
				fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &index)

				gpus = append(gpus, GPUInfo{
					Index:  index,
					Name:   "AMD GPU",
					Vendor: "amd",
				})
			}
			if len(gpus) > 0 {
				return gpus
			}
		}
	}

	// Fallback: detect AMD GPU via /dev/kfd (AMD's kernel fusion driver)
	// This works even without rocm-smi installed (e.g., on Azure AMD VMs)
	if _, err := os.Stat("/dev/kfd"); err == nil {
		// /dev/kfd exists, which indicates AMD GPU with ROCm/compute support
		// Count renderD* devices to estimate GPU count
		matches, _ := filepath.Glob("/dev/dri/renderD*")
		gpuCount := len(matches)
		if gpuCount == 0 {
			gpuCount = 1 // At least one GPU if /dev/kfd exists
		}

		for i := 0; i < gpuCount; i++ {
			gpus = append(gpus, GPUInfo{
				Index:  i,
				Name:   "AMD GPU (detected via /dev/kfd)",
				Vendor: "amd",
			})
		}
		log.Info().Int("count", gpuCount).Msg("Detected AMD GPU(s) via /dev/kfd fallback")
	}

	return gpus
}

// execCommand runs a command and returns its output
func execCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := &exec.Cmd{
		Path: name,
		Args: append([]string{name}, args...),
	}
	if filepath.Base(name) == name {
		if lp, err := exec.LookPath(name); err == nil {
			cmd.Path = lp
		}
	}

	out, err := cmd.Output()
	_ = ctx // Context used for timeout documentation
	return string(out), err
}

// getUpstreamDNS reads /etc/resolv.conf and returns the nameservers configured there.
// This ensures DNS works in both Docker and Kubernetes environments:
// - Docker: returns 127.0.0.11 (Docker's internal DNS) or host DNS
// - Kubernetes: returns CoreDNS IP (e.g., 10.96.0.10)
//
// In enterprise environments, the sandbox container's resolv.conf should contain
// the enterprise DNS servers (configured via Docker's --dns flag or daemon.json).
// If only systemd-resolved stub (127.0.0.53) is found, we fall back to Docker's
// internal DNS (127.0.0.11) which forwards to the host's DNS server.
func getUpstreamDNS() []string {
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		// Fallback to Docker's internal DNS, which forwards to host DNS
		// This preserves enterprise DNS configuration
		log.Warn().Err(err).Msg("Failed to read /etc/resolv.conf, using Docker internal DNS (127.0.0.11)")
		return []string{"127.0.0.11:53"}
	}
	defer file.Close()

	var nameservers []string
	var skippedLoopback []string
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
				// Skip loopback addresses that don't work from inside containers:
				// - 127.0.0.53: systemd-resolved stub (doesn't work in containers)
				// - 127.0.0.1: generic loopback (doesn't work in containers)
				// Note: 127.0.0.11 is Docker's internal DNS and DOES work in containers
				if ip == "127.0.0.53" || ip == "127.0.0.1" {
					skippedLoopback = append(skippedLoopback, ip)
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
		// Fallback to Docker's internal DNS (127.0.0.11) which forwards to host DNS
		// This is critical for enterprise environments where:
		// 1. The host uses systemd-resolved (127.0.0.53) which doesn't work in containers
		// 2. Docker's internal DNS properly resolves to the host's configured DNS
		// 3. Enterprise internal DNS servers are configured on the host
		if len(skippedLoopback) > 0 {
			log.Info().
				Strs("skipped", skippedLoopback).
				Msg("Only loopback nameservers found in /etc/resolv.conf, using Docker internal DNS (127.0.0.11)")
		} else {
			log.Warn().Msg("No nameservers found in /etc/resolv.conf, using Docker internal DNS (127.0.0.11)")
		}
		return []string{"127.0.0.11:53"}
	}

	return nameservers
}
