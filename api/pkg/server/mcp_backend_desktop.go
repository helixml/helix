package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SandboxDialer dials into sandbox containers via RevDial.
// Extracted as an interface so tests can substitute a fake.
type SandboxDialer interface {
	Dial(ctx context.Context, key string) (net.Conn, error)
}

// DesktopMCPBackend implements MCPBackend by proxying MCP requests over
// RevDial to the desktop-bridge process (port 9878) inside the sandbox
// container. This replaces the old hardcoded localhost:9878 URL that only
// worked when Zed ran in the same container network.
type DesktopMCPBackend struct {
	store  store.Store
	dialer SandboxDialer
}

// NewDesktopMCPBackend creates a new Desktop MCP backend.
func NewDesktopMCPBackend(s store.Store, dialer SandboxDialer) *DesktopMCPBackend {
	return &DesktopMCPBackend{
		store:  s,
		dialer: dialer,
	}
}

// ServeHTTP implements MCPBackend.
// It expects a session_id query parameter, verifies ownership, dials the
// desktop container via RevDial, and forwards the HTTP request/response.
func (b *DesktopMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	ctx := r.Context()
	sessionID := r.URL.Query().Get("session_id")

	if sessionID == "" {
		http.Error(w, "session_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Verify session exists and user owns it
	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("desktop MCP: session not found")
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if session.Owner != user.ID {
		log.Warn().
			Str("session_id", sessionID).
			Str("user_id", user.ID).
			Str("owner", session.Owner).
			Msg("desktop MCP: user does not own session")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Dial the desktop container via RevDial
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	conn, err := b.dialer.Dial(ctx, runnerID)
	if err != nil {
		log.Error().Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("desktop MCP: sandbox not connected")
		http.Error(w, fmt.Sprintf("sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	// Build the request to forward over the tunnel.
	// RevDial tunnels to port 9876 (the desktop HTTP server), which mounts the
	// MCP handler at /mcp. Port 9878 is the standalone MCP server but is NOT
	// reachable through RevDial.
	targetURL := "http://localhost:9876/mcp"
	if r.URL.RawQuery != "" {
		// Strip our session_id param — desktop-bridge doesn't need it
		targetURL += "?" + stripParam(r.URL.RawQuery, "session_id")
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		log.Error().Err(err).Msg("desktop MCP: failed to create proxy request")
		http.Error(w, "failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy relevant headers
	for _, h := range []string{"Content-Type", "Accept"} {
		if v := r.Header.Get(h); v != "" {
			proxyReq.Header.Set(h, v)
		}
	}
	proxyReq.ContentLength = r.ContentLength

	// Write request to RevDial connection
	if err := proxyReq.Write(conn); err != nil {
		log.Error().Err(err).Msg("desktop MCP: failed to write request to RevDial")
		http.Error(w, "failed to forward request to desktop", http.StatusBadGateway)
		return
	}

	// Read response from RevDial connection
	resp, err := http.ReadResponse(bufio.NewReader(conn), proxyReq)
	if err != nil {
		log.Error().Err(err).Msg("desktop MCP: failed to read response from RevDial")
		http.Error(w, "failed to read response from desktop", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers and status
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// stripParam removes a query parameter by name from a raw query string.
func stripParam(rawQuery, param string) string {
	result := ""
	for _, part := range splitQuery(rawQuery) {
		if len(part) > len(param) && part[:len(param)+1] == param+"=" {
			continue
		}
		if result != "" {
			result += "&"
		}
		result += part
	}
	return result
}

// splitQuery splits a raw query string on '&'.
func splitQuery(raw string) []string {
	if raw == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '&' {
			parts = append(parts, raw[start:i])
			start = i + 1
		}
	}
	parts = append(parts, raw[start:])
	return parts
}
