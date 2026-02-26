package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// ExposedPort represents a port exposed from a session's dev container
type ExposedPort struct {
	Port      int       `json:"port"`
	Protocol  string    `json:"protocol"` // "http" or "tcp"
	Name      string    `json:"name,omitempty"`
	URL       string    `json:"url"`
	Status    string    `json:"status"` // "active", "inactive"
	CreatedAt time.Time `json:"created_at"`
}

// ExposePortRequest is the request body for exposing a port
type ExposePortRequest struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"` // defaults to "http"
	Name     string `json:"name,omitempty"`
}

// ExposePortResponse is the response for exposing a port
type ExposePortResponse struct {
	SessionID     string   `json:"session_id"`
	Port          int      `json:"port"`
	Protocol      string   `json:"protocol"`
	Name          string   `json:"name,omitempty"`
	URLs          []string `json:"urls"`
	AllocatedPort int      `json:"allocated_port,omitempty"` // for random port mode
	Status        string   `json:"status"`
}

// ListExposedPortsResponse is the response for listing exposed ports
type ListExposedPortsResponse struct {
	SessionID    string        `json:"session_id"`
	ExposedPorts []ExposedPort `json:"exposed_ports"`
}

// PortListener represents an active port-based proxy listener
type PortListener struct {
	AllocatedPort int
	SessionID     string
	TargetPort    int
	Listener      net.Listener
	cancel        context.CancelFunc
}

// ExposedPortManager tracks exposed ports per session
type ExposedPortManager struct {
	mu             sync.RWMutex
	exposedPorts   map[string][]ExposedPort // sessionID -> ports
	baseURL        string                   // e.g., "https://helix.example.com"
	devSubdomain   string                   // e.g., "dev" for *.dev.helix.example.com
	randomPortBase int                      // starting port for random allocation
	randomPortMax  int                      // max port for random allocation
	allocatedPorts map[int]string           // port -> sessionID
	portListeners  map[int]*PortListener    // allocated port -> listener
	apiServer      *HelixAPIServer          // for RevDial access
}

// NewExposedPortManager creates a new exposed port manager
func NewExposedPortManager(baseURL, devSubdomain string) *ExposedPortManager {
	return &ExposedPortManager{
		exposedPorts:   make(map[string][]ExposedPort),
		baseURL:        baseURL,
		devSubdomain:   devSubdomain,
		randomPortBase: 30000,
		randomPortMax:  40000,
		allocatedPorts: make(map[int]string),
		portListeners:  make(map[int]*PortListener),
	}
}

// SetAPIServer sets the API server reference for RevDial access
func (m *ExposedPortManager) SetAPIServer(apiServer *HelixAPIServer) {
	m.apiServer = apiServer
}

// allocatePort finds an available port in the range and marks it as allocated
func (m *ExposedPortManager) allocatePort(sessionID string) (int, error) {
	for port := m.randomPortBase; port < m.randomPortMax; port++ {
		if _, exists := m.allocatedPorts[port]; !exists {
			// Try to bind to verify it's actually available
			listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				continue // Port in use by another process
			}
			listener.Close() // We'll re-bind when starting the proxy
			m.allocatedPorts[port] = sessionID
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", m.randomPortBase, m.randomPortMax)
}

// ExposePort registers a port exposure for a session
func (m *ExposedPortManager) ExposePort(sessionID string, req *ExposePortRequest) (*ExposePortResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	protocol := req.Protocol
	if protocol == "" {
		protocol = "http"
	}

	// Check if port already exposed
	for _, p := range m.exposedPorts[sessionID] {
		if p.Port == req.Port {
			return &ExposePortResponse{
				SessionID: sessionID,
				Port:      req.Port,
				Protocol:  protocol,
				Name:      p.Name,
				URLs:      []string{p.URL},
				Status:    p.Status,
			}, nil
		}
	}

	// Build URL based on configuration
	var urls []string
	var allocatedPort int

	// Option 1: Subdomain-based URL (if devSubdomain is configured)
	if m.devSubdomain != "" && m.baseURL != "" {
		// Parse base URL to get domain
		domain := strings.TrimPrefix(m.baseURL, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.Split(domain, ":")[0] // remove port if present

		// Format: p{port}-{session_id}.{dev_subdomain}.{domain}
		// e.g., p8080-ses-abc123.dev.helix.example.com
		subdomainURL := fmt.Sprintf("https://p%d-%s.%s.%s", req.Port, sessionID, m.devSubdomain, domain)
		urls = append(urls, subdomainURL)
	}

	// Option 2: Port-based URL (allocate a unique port)
	// This works without DNS wildcards - clients connect directly to the allocated port
	if m.apiServer != nil {
		port, err := m.allocatePort(sessionID)
		if err == nil {
			allocatedPort = port
			// Parse base URL to get host
			host := strings.TrimPrefix(m.baseURL, "https://")
			host = strings.TrimPrefix(host, "http://")
			host = strings.Split(host, ":")[0] // remove port if present

			// Determine scheme from baseURL
			scheme := "http"
			if strings.HasPrefix(m.baseURL, "https://") {
				scheme = "https"
			}

			portURL := fmt.Sprintf("%s://%s:%d", scheme, host, port)
			urls = append(urls, portURL)

			// Start the port listener (do this after releasing the lock)
			go m.startPortListener(sessionID, req.Port, port)
		} else {
			log.Warn().Err(err).Msg("Failed to allocate port for port-based proxy")
		}
	}

	// Option 3: Path-based URL (always available, but has limitations)
	// Format: {baseURL}/api/v1/sessions/{sessionID}/proxy/{port}/
	pathURL := fmt.Sprintf("%s/api/v1/sessions/%s/proxy/%d/", m.baseURL, sessionID, req.Port)
	urls = append(urls, pathURL)

	// Create exposed port record
	exposed := ExposedPort{
		Port:      req.Port,
		Protocol:  protocol,
		Name:      req.Name,
		URL:       urls[0], // primary URL
		Status:    "active",
		CreatedAt: time.Now(),
	}

	m.exposedPorts[sessionID] = append(m.exposedPorts[sessionID], exposed)

	return &ExposePortResponse{
		SessionID:     sessionID,
		Port:          req.Port,
		Protocol:      protocol,
		Name:          req.Name,
		URLs:          urls,
		AllocatedPort: allocatedPort,
		Status:        "active",
	}, nil
}

// UnexposePort removes a port exposure
func (m *ExposedPortManager) UnexposePort(sessionID string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ports := m.exposedPorts[sessionID]
	for i, p := range ports {
		if p.Port == port {
			m.exposedPorts[sessionID] = append(ports[:i], ports[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("port %d not exposed for session %s", port, sessionID)
}

// ListExposedPorts returns all exposed ports for a session
func (m *ExposedPortManager) ListExposedPorts(sessionID string) []ExposedPort {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.exposedPorts[sessionID]
}

// CleanupSession removes all exposed ports for a session
func (m *ExposedPortManager) CleanupSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop any running port listeners
	for port, listener := range m.portListeners {
		if listener.SessionID == sessionID {
			log.Info().
				Int("port", port).
				Str("session_id", sessionID).
				Msg("Stopping port listener for session cleanup")
			listener.cancel()
			listener.Listener.Close()
			delete(m.portListeners, port)
		}
	}

	// Free any allocated random ports
	for port, sid := range m.allocatedPorts {
		if sid == sessionID {
			delete(m.allocatedPorts, port)
		}
	}

	delete(m.exposedPorts, sessionID)
}

// startPortListener starts an HTTP server on the allocated port and proxies to the session
func (m *ExposedPortManager) startPortListener(sessionID string, targetPort, allocatedPort int) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create HTTP handler that proxies to the session
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		m.handlePortHTTPRequest(rw, r, sessionID, targetPort)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", allocatedPort),
		Handler: handler,
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Error().Err(err).
			Int("port", allocatedPort).
			Str("session_id", sessionID).
			Msg("Failed to start port listener")
		cancel()
		return
	}

	// Store the listener
	m.mu.Lock()
	m.portListeners[allocatedPort] = &PortListener{
		AllocatedPort: allocatedPort,
		SessionID:     sessionID,
		TargetPort:    targetPort,
		Listener:      listener,
		cancel:        cancel,
	}
	m.mu.Unlock()

	log.Info().
		Int("allocated_port", allocatedPort).
		Int("target_port", targetPort).
		Str("session_id", sessionID).
		Msg("Started port-based HTTP proxy listener")

	// Start HTTP server in goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Debug().Err(err).Int("port", allocatedPort).Msg("Port proxy server error")
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)
	log.Info().Int("port", allocatedPort).Msg("Port listener stopped")
}

// handlePortHTTPRequest proxies an HTTP request to the session via RevDial/Hydra
func (m *ExposedPortManager) handlePortHTTPRequest(rw http.ResponseWriter, r *http.Request, sessionID string, targetPort int) {
	if m.apiServer == nil {
		http.Error(rw, "API server not configured", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// Get the session to find the sandbox
	session, err := m.apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("session not found: %s", err), http.StatusNotFound)
		return
	}

	sandboxID := session.SandboxID
	if sandboxID == "" {
		http.Error(rw, "session has no sandbox", http.StatusServiceUnavailable)
		return
	}

	// Build Hydra proxy path
	hydraPath := fmt.Sprintf("/api/v1/dev-containers/%s/proxy/%d%s", sessionID, targetPort, r.URL.Path)
	if r.URL.RawQuery != "" {
		hydraPath += "?" + r.URL.RawQuery
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("sandbox_id", sandboxID).
		Int("port", targetPort).
		Str("path", hydraPath).
		Str("method", r.Method).
		Msg("Port proxy: forwarding request to Hydra")

	// Connect to Hydra via RevDial
	hydraClient := hydra.NewRevDialClient(m.apiServer.connman, "hydra-"+sandboxID)

	proxyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	hydraConn, err := m.apiServer.connman.Dial(proxyCtx, hydraClient.DeviceID())
	if err != nil {
		log.Warn().Err(err).Str("sandbox_id", sandboxID).Msg("Failed to dial Hydra for port proxy")
		http.Error(rw, fmt.Sprintf("failed to connect to sandbox: %s", err), http.StatusBadGateway)
		return
	}
	defer hydraConn.Close()

	// Check for WebSocket upgrade
	if r.Header.Get("Upgrade") == "websocket" {
		m.handleWebSocketProxy(rw, r, hydraConn, hydraPath)
		return
	}

	// Build HTTP request to Hydra
	proxyReq, err := http.NewRequestWithContext(proxyCtx, r.Method, "http://hydra"+hydraPath, r.Body)
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to create request: %s", err), http.StatusInternalServerError)
		return
	}

	// Copy headers
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

	// Set forwarding headers
	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)
	if r.TLS != nil {
		proxyReq.Header.Set("X-Forwarded-Proto", "https")
	} else {
		proxyReq.Header.Set("X-Forwarded-Proto", "http")
	}

	// Send request over RevDial connection
	if err := proxyReq.Write(hydraConn); err != nil {
		log.Warn().Err(err).Msg("Failed to write request to Hydra")
		http.Error(rw, fmt.Sprintf("failed to send request: %s", err), http.StatusBadGateway)
		return
	}

	// Read response
	resp, err := http.ReadResponse(bufio.NewReader(hydraConn), proxyReq)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to read response from Hydra")
		http.Error(rw, fmt.Sprintf("failed to read response: %s", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			rw.Header().Add(key, value)
		}
	}

	rw.WriteHeader(resp.StatusCode)

	// Stream response body
	if _, err := io.Copy(rw, resp.Body); err != nil {
		log.Debug().Err(err).Msg("Error streaming port proxy response")
	}
}

// handleWebSocketProxy handles WebSocket upgrade requests
func (m *ExposedPortManager) handleWebSocketProxy(rw http.ResponseWriter, r *http.Request, hydraConn net.Conn, hydraPath string) {
	// Hijack the client connection
	hijacker, ok := rw.(http.Hijacker)
	if !ok {
		http.Error(rw, "WebSocket not supported", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to hijack connection: %s", err), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Build WebSocket upgrade request for Hydra
	proxyReq, err := http.NewRequest(r.Method, "http://hydra"+hydraPath, nil)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		return
	}

	// Copy all headers including WebSocket upgrade headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Send upgrade request to Hydra
	if err := proxyReq.Write(hydraConn); err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// Read Hydra's response
	hydraReader := bufio.NewReader(hydraConn)
	resp, err := http.ReadResponse(hydraReader, proxyReq)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// Forward response to client
	resp.Write(clientConn)

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return // Not upgraded, done
	}

	log.Debug().Str("path", hydraPath).Msg("WebSocket upgrade successful, starting bidirectional copy")

	// Bidirectional copy for WebSocket
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(hydraConn, clientBuf)
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientConn, hydraReader)
	}()

	wg.Wait()
}

// exposeSessionPort handles POST /api/v1/sessions/{id}/expose
// @Summary Expose a port from the session's dev container
// @Description Makes a port from the session's dev container accessible via a public URL
// @Tags sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param request body ExposePortRequest true "Port to expose"
// @Success 200 {object} ExposePortResponse
// @Failure 400 {string} string "Bad request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Session not found"
// @Router /api/v1/sessions/{id}/expose [post]
func (apiServer *HelixAPIServer) exposeSessionPort(rw http.ResponseWriter, r *http.Request) (*ExposePortResponse, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	sessionID := vars["id"]

	// Get the session to verify ownership
	ctx := r.Context()
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session not found: %s", err))
	}

	err = apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Parse request
	var req ExposePortRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("invalid request: %s", err))
	}

	// Validate port
	if req.Port < 1 || req.Port > 65535 {
		return nil, system.NewHTTPError400("port must be between 1 and 65535")
	}

	// Register the exposed port
	resp, err := apiServer.exposedPortManager.ExposePort(sessionID, &req)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to expose port: %s", err))
	}

	log.Info().
		Str("session_id", sessionID).
		Int("port", req.Port).
		Strs("urls", resp.URLs).
		Msg("Exposed port for session")

	return resp, nil
}

// unexposeSessionPort handles DELETE /api/v1/sessions/{id}/expose/{port}
// @Summary Unexpose a port from the session's dev container
// @Description Removes public access to a previously exposed port
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param port path int true "Port number"
// @Success 200 {object} map[string]string
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Session or port not found"
// @Router /api/v1/sessions/{id}/expose/{port} [delete]
func (apiServer *HelixAPIServer) unexposeSessionPort(rw http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	sessionID := vars["id"]
	portStr := vars["port"]

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("invalid port: %s", portStr))
	}

	// Get the session to verify ownership
	ctx := r.Context()
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session not found: %s", err))
	}

	// Verify user has access to this session
	err = apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Remove the exposed port
	if err := apiServer.exposedPortManager.UnexposePort(sessionID, port); err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}

	log.Info().
		Str("session_id", sessionID).
		Int("port", port).
		Msg("Unexposed port for session")

	return map[string]string{"status": "removed"}, nil
}

// listExposedPorts handles GET /api/v1/sessions/{id}/expose
// @Summary List exposed ports for a session
// @Description Returns all ports currently exposed from the session's dev container
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} ListExposedPortsResponse
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Session not found"
// @Router /api/v1/sessions/{id}/expose [get]
func (apiServer *HelixAPIServer) listExposedPorts(rw http.ResponseWriter, r *http.Request) (*ListExposedPortsResponse, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	sessionID := vars["id"]

	// Get the session to verify ownership
	ctx := r.Context()
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session not found: %s", err))
	}

	// Verify user has access to this session
	err = apiServer.authorizeUserToSession(ctx, user, session, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	ports := apiServer.exposedPortManager.ListExposedPorts(sessionID)

	return &ListExposedPortsResponse{
		SessionID:    sessionID,
		ExposedPorts: ports,
	}, nil
}

// proxyToSessionPort handles requests to /api/v1/sessions/{id}/proxy/{port}/*
// This proxies HTTP requests to the session's dev container via Hydra
func (apiServer *HelixAPIServer) proxyToSessionPort(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	portStr := vars["port"]

	port, err := strconv.Atoi(portStr)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid port: %s", portStr), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get the session to find which sandbox it's running on
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("session not found: %s", err), http.StatusNotFound)
		return
	}

	// Get the Hydra client for this session's sandbox
	sandboxID := session.SandboxID
	if sandboxID == "" {
		http.Error(rw, "session has no sandbox", http.StatusServiceUnavailable)
		return
	}

	// Build the path to forward to Hydra
	// Strip the API prefix to get the relative path
	proxyPrefix := fmt.Sprintf("/api/v1/sessions/%s/proxy/%s", sessionID, portStr)
	targetPath := strings.TrimPrefix(r.URL.Path, proxyPrefix)
	if targetPath == "" {
		targetPath = "/"
	}

	// Build Hydra proxy URL
	hydraPath := fmt.Sprintf("/api/v1/dev-containers/%s/proxy/%d%s", sessionID, port, targetPath)
	if r.URL.RawQuery != "" {
		hydraPath += "?" + r.URL.RawQuery
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("sandbox_id", sandboxID).
		Int("port", port).
		Str("hydra_path", hydraPath).
		Str("method", r.Method).
		Msg("Proxying request to session port via Hydra")

	// Create Hydra client via RevDial
	hydraClient := hydra.NewRevDialClient(apiServer.connman, "hydra-"+sandboxID)

	// Make request to Hydra's proxy endpoint
	// This uses the RevDial connection to reach Hydra inside the sandbox
	proxyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Get a RevDial connection to Hydra
	conn, err := apiServer.connman.Dial(proxyCtx, hydraClient.DeviceID())
	if err != nil {
		log.Warn().Err(err).
			Str("session_id", sessionID).
			Str("sandbox_id", sandboxID).
			Msg("Failed to dial Hydra via RevDial")
		http.Error(rw, fmt.Sprintf("failed to connect to sandbox: %s", err), http.StatusBadGateway)
		return
	}
	defer conn.Close()

	// Build HTTP request to send over the RevDial connection
	proxyReq, err := http.NewRequestWithContext(proxyCtx, r.Method, "http://hydra"+hydraPath, r.Body)
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to create proxy request: %s", err), http.StatusInternalServerError)
		return
	}

	// Copy headers
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

	// Set forwarding headers
	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)
	if r.TLS != nil {
		proxyReq.Header.Set("X-Forwarded-Proto", "https")
	} else {
		proxyReq.Header.Set("X-Forwarded-Proto", "http")
	}

	// Write request to RevDial connection
	if err := proxyReq.Write(conn); err != nil {
		log.Warn().Err(err).Msg("Failed to write request to Hydra")
		http.Error(rw, fmt.Sprintf("failed to send request: %s", err), http.StatusBadGateway)
		return
	}

	// Read response from RevDial connection
	resp, err := http.ReadResponse(bufio.NewReader(conn), proxyReq)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to read response from Hydra")
		http.Error(rw, fmt.Sprintf("failed to read response: %s", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			rw.Header().Add(key, value)
		}
	}

	// Write status code
	rw.WriteHeader(resp.StatusCode)

	// Stream response body
	if _, err := io.Copy(rw, resp.Body); err != nil {
		log.Debug().Err(err).Msg("Error streaming proxy response")
	}
}

// initExposedPortManager initializes the exposed port manager
func (apiServer *HelixAPIServer) initExposedPortManager() {
	baseURL := apiServer.Cfg.WebServer.URL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Dev subdomain for subdomain-based virtual hosting
	// Format: DEV_SUBDOMAIN=dev -> *.dev.helix.example.com
	// Or full domain: DEV_SUBDOMAIN=dev.helix.example.com
	devSubdomain := apiServer.Cfg.WebServer.DevSubdomain

	apiServer.exposedPortManager = NewExposedPortManager(baseURL, devSubdomain)
	apiServer.exposedPortManager.SetAPIServer(apiServer)

	log.Info().
		Str("base_url", baseURL).
		Str("dev_subdomain", devSubdomain).
		Bool("port_based_proxy", true).
		Msg("Initialized exposed port manager with port-based proxy support")
}
