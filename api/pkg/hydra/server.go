package hydra

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
}

// NewServer creates a new Hydra server
func NewServer(manager *Manager, socketPath string) *Server {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	// Create dev container manager for desktop container lifecycle functionality
	devContainerManager := NewDevContainerManager(manager)

	return &Server{
		manager:             manager,
		devContainerManager: devContainerManager,
		socketPath:          socketPath,
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

	// Recover any dev containers that are still running from before Hydra restarted
	// This MUST complete before we start accepting requests to avoid state corruption
	var dockerErr error
	for i := 0; i < 30; i++ {
		dockerErr = s.devContainerManager.RecoverDevContainersFromDocker(ctx, "")
		if dockerErr == nil {
			break
		}
		log.Info().Err(dockerErr).Int("attempt", i+1).Int("max_attempts", 30).Msg("Waiting for Docker to be ready...")
		time.Sleep(2 * time.Second)
	}
	if dockerErr != nil {
		return fmt.Errorf("Docker not available after 60s, Hydra cannot function without Docker: %w", dockerErr)
	}

	log.Info().
		Str("socket", s.socketPath).
		Msg("Hydra server started (docker-in-desktop mode)")

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

	// Stop manager
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

	api := router.PathPrefix("/api/v1").Subrouter()

	// Dev container management (desktop container lifecycle)
	api.HandleFunc("/dev-containers", s.handleCreateDevContainer).Methods("POST")
	api.HandleFunc("/dev-containers", s.handleListDevContainers).Methods("GET")
	api.HandleFunc("/dev-containers/{session_id}", s.handleGetDevContainer).Methods("GET")
	api.HandleFunc("/dev-containers/{session_id}", s.handleDeleteDevContainer).Methods("DELETE")
	api.HandleFunc("/dev-containers/{session_id}/clients", s.handleGetDevContainerClients).Methods("GET")
	api.HandleFunc("/dev-containers/{session_id}/video/stats", s.handleGetDevContainerVideoStats).Methods("GET")

	// Port proxy - forward HTTP requests to a port on the desktop container's network
	api.PathPrefix("/dev-containers/{session_id}/proxy/{port}").HandlerFunc(s.handleDevContainerProxy)

	// Golden cache management
	api.HandleFunc("/dev-containers/{session_id}/blkio", s.handleGetDevContainerBlkio).Methods("GET")

	api.HandleFunc("/golden-cache/{project_id}", s.handleDeleteGoldenCache).Methods("DELETE")
	api.HandleFunc("/golden-cache/{project_id}/build-result", s.handleGetGoldenBuildResult).Methods("GET")
	api.HandleFunc("/golden-cache/{project_id}/copy-progress", s.handleGetGoldenCopyProgress).Methods("GET")

	// System stats (GPU info, active sessions)
	api.HandleFunc("/system/stats", s.handleSystemStats).Methods("GET")
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := &HealthResponse{
		Status:          "healthy",
		ActiveInstances: 0,
		Version:         Version,
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
func (s *Server) handleGetDevContainerClients(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

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

// handleGetDevContainerVideoStats proxies a request to the desktop container's /video/stats endpoint
func (s *Server) handleGetDevContainerVideoStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

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

	desktopURL := fmt.Sprintf("http://%s:9876/video/stats", container.IPAddress)

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
			Msg("Failed to query desktop /video/stats endpoint")
		emptyResp := struct {
			SessionID string        `json:"session_id"`
			Sources   []interface{} `json:"sources"`
		}{
			SessionID: sessionID,
			Sources:   []interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(emptyResp)
		return
	}
	defer resp.Body.Close()

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

// handleDevContainerProxy forwards HTTP requests to a port on the desktop container's network.
func (s *Server) handleDevContainerProxy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]
	portStr := vars["port"]

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port < 1 || port > 65535 {
		http.Error(w, fmt.Sprintf("invalid port: %s", portStr), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

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

	proxyPrefix := fmt.Sprintf("/api/v1/dev-containers/%s/proxy/%s", sessionID, portStr)
	targetPath := strings.TrimPrefix(r.URL.Path, proxyPrefix)
	if targetPath == "" {
		targetPath = "/"
	}

	targetHost := container.IPAddress
	if targetHost == "host" {
		targetHost = "localhost"
	}

	targetURL := fmt.Sprintf("http://%s:%d%s", targetHost, port, targetPath)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Debug().
		Str("session_id", sessionID).
		Int("port", port).
		Str("target_url", targetURL).
		Str("method", r.Method).
		Msg("Proxying request to dev container")

	if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
		s.handleDevContainerWebSocketProxy(w, r, targetHost, port, targetPath)
		return
	}

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create proxy request: %s", err), http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		switch strings.ToLower(key) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailers", "transfer-encoding", "upgrade":
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)
	proxyReq.Header.Set("X-Forwarded-Proto", "http")
	if r.TLS != nil {
		proxyReq.Header.Set("X-Forwarded-Proto", "https")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Warn().Err(err).
			Str("session_id", sessionID).
			Int("port", port).
			Str("target_url", targetURL).
			Msg("Failed to proxy request to dev container")
		http.Error(w, fmt.Sprintf("failed to connect to service: %s", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				break
			}
		}
		if readErr != nil {
			break
		}
	}
}

// handleDevContainerWebSocketProxy handles WebSocket upgrade requests to dev container ports
func (s *Server) handleDevContainerWebSocketProxy(w http.ResponseWriter, r *http.Request, targetHost string, port int, targetPath string) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket not supported", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to hijack connection: %s", err), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	targetAddr := fmt.Sprintf("%s:%d", targetHost, port)
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		log.Warn().Err(err).Str("target", targetAddr).Msg("Failed to connect to target for WebSocket proxy")
		return
	}
	defer targetConn.Close()

	targetURL := fmt.Sprintf("http://%s%s", targetAddr, targetPath)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, nil)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		return
	}

	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	if err := proxyReq.Write(targetConn); err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	targetReader := bufio.NewReader(targetConn)
	resp, err := http.ReadResponse(targetReader, proxyReq)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	resp.Write(clientConn)

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return
	}

	log.Debug().
		Str("target", targetAddr).
		Str("path", targetPath).
		Msg("WebSocket upgrade successful in dev container proxy")

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(targetConn, clientBuf)
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientConn, targetReader)
	}()

	wg.Wait()
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

	gpus = append(gpus, getNvidiaGPUs()...)

	if len(gpus) == 0 {
		gpus = append(gpus, getAMDGPUs()...)
	}

	return gpus
}

// getNvidiaGPUs queries NVIDIA GPUs using nvidia-smi
func getNvidiaGPUs() []GPUInfo {
	var gpus []GPUInfo

	if _, err := os.Stat("/usr/bin/nvidia-smi"); err != nil {
		return gpus
	}

	cmd := fmt.Sprintf("nvidia-smi --query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu --format=csv,noheader,nounits 2>/dev/null")

	out, err := execCommand("sh", "-c", cmd)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to query NVIDIA GPUs")
		return gpus
	}

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
			MemoryTotal: memTotal * 1024 * 1024,
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

	hasRocmSmi := false
	if _, err := os.Stat("/usr/local/bin/rocm-smi"); err == nil {
		hasRocmSmi = true
	} else if _, err := os.Stat("/opt/rocm/bin/rocm-smi"); err == nil {
		hasRocmSmi = true
	}

	if hasRocmSmi {
		cmd := "rocm-smi --showid --showtemp --showuse --showmeminfo vram --csv 2>/dev/null"
		out, err := execCommand("sh", "-c", cmd)
		if err != nil {
			log.Debug().Err(err).Msg("Failed to query AMD GPUs via rocm-smi")
		} else {
			lines := strings.Split(strings.TrimSpace(out), "\n")
			for i, line := range lines {
				if i == 0 || line == "" {
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

	if _, err := os.Stat("/dev/kfd"); err == nil {
		matches, _ := filepath.Glob("/dev/dri/renderD*")
		gpuCount := len(matches)
		if gpuCount == 0 {
			gpuCount = 1
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
	_ = ctx
	return string(out), err
}

// handleGetDevContainerBlkio returns cumulative blkio write/read bytes for a container.
func (s *Server) handleGetDevContainerBlkio(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["session_id"]
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	stats, err := s.devContainerManager.GetContainerBlkioStats(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDeleteGoldenCache removes the golden Docker cache for a project.
func (s *Server) handleGetGoldenBuildResult(w http.ResponseWriter, r *http.Request) {
	projectID := mux.Vars(r)["project_id"]
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	result := s.devContainerManager.GetGoldenBuildResult(projectID)
	if result == nil {
		http.Error(w, "no build result available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleDeleteGoldenCache(w http.ResponseWriter, r *http.Request) {
	projectID := mux.Vars(r)["project_id"]
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	if err := DeleteGolden(projectID); err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to delete golden cache")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// handleGetGoldenCopyProgress returns the current golden cache copy progress.
func (s *Server) handleGetGoldenCopyProgress(w http.ResponseWriter, r *http.Request) {
	projectID := mux.Vars(r)["project_id"]
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	progress := s.devContainerManager.GetGoldenCopyProgress(projectID)
	if progress == nil {
		http.Error(w, "no copy in progress", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(progress)
}
