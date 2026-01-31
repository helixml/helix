package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKoditMCPBackend_RedirectRewrite verifies that redirect Location headers
// pointing to internal Kodit URLs are rewritten to external client-facing URLs.
// This test would fail before the fix that added ModifyResponse to rewrite redirects.
func TestKoditMCPBackend_RedirectRewrite(t *testing.T) {
	// Create a mock Kodit server that returns a redirect with internal URL
	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate Kodit's behavior: redirect /mcp to /mcp/
		if r.URL.Path == "/mcp" {
			w.Header().Set("Location", "http://"+r.Host+"/mcp/")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		// For /mcp/, return success
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockKodit.Close()

	// Create backend pointing to mock Kodit
	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	// Create request simulating client accessing /api/v1/mcp/kodit
	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/mcp/kodit", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	user := &types.User{ID: "test-user"}

	backend.ServeHTTP(rec, req, user)

	// Should get a redirect
	require.Equal(t, http.StatusTemporaryRedirect, rec.Code)

	// The Location header should be rewritten to external URL, not internal Kodit URL
	location := rec.Header().Get("Location")
	assert.NotEmpty(t, location, "Location header should be set")
	assert.Contains(t, location, "localhost:8080", "Location should use client-facing host")
	assert.Contains(t, location, "/api/v1/mcp/kodit/", "Location should use external path")
	assert.NotContains(t, location, mockKodit.URL, "Location should NOT contain internal Kodit URL")
}

// TestKoditMCPBackend_TrailingSlashPreserved verifies that requests with trailing
// slashes are forwarded with trailing slashes to avoid redirect loops.
// Without this fix, /api/v1/mcp/kodit/ would be sent to /mcp (no slash),
// causing Kodit to redirect to /mcp/, which we'd rewrite back to /api/v1/mcp/kodit/,
// creating an infinite redirect loop.
func TestKoditMCPBackend_TrailingSlashPreserved(t *testing.T) {
	var receivedPath string

	// Create a mock Kodit server that records the path it receives
	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockKodit.Close()

	// Create backend pointing to mock Kodit
	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	// Create request WITH trailing slash
	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/mcp/kodit/", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	user := &types.User{ID: "test-user"}

	backend.ServeHTTP(rec, req, user)

	require.Equal(t, http.StatusOK, rec.Code)

	// The path sent to Kodit should have trailing slash
	assert.Equal(t, "/mcp/", receivedPath, "Path to Kodit should preserve trailing slash")
}

// TestKoditMCPBackend_SessionIDPassthrough verifies that the Mcp-Session-Id header
// from Kodit responses is passed through to the client. MCP clients need this
// header to maintain session state.
func TestKoditMCPBackend_SessionIDPassthrough(t *testing.T) {
	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Kodit sets session ID in response header
		w.Header().Set("Mcp-Session-Id", "session-abc123")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer mockKodit.Close()

	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/mcp/kodit/", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	require.Equal(t, http.StatusOK, rec.Code)

	// Session ID header must be passed through to client
	sessionID := rec.Header().Get("Mcp-Session-Id")
	assert.Equal(t, "session-abc123", sessionID, "Mcp-Session-Id header should be passed through")
}

// TestKoditMCPBackend_RequestBodyForwarded verifies that POST request bodies
// (JSON-RPC payloads) are correctly forwarded to Kodit.
func TestKoditMCPBackend_RequestBodyForwarded(t *testing.T) {
	var receivedBody string

	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer mockKodit.Close()

	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	jsonRPCPayload := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/mcp/kodit/", strings.NewReader(jsonRPCPayload))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, jsonRPCPayload, receivedBody, "Request body should be forwarded to Kodit")
}

// TestKoditMCPBackend_QueryParamsForwarded verifies that query parameters
// are forwarded to Kodit (e.g., for session ID in GET requests).
func TestKoditMCPBackend_QueryParamsForwarded(t *testing.T) {
	var receivedQuery string

	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer mockKodit.Close()

	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/mcp/kodit/?session_id=xyz789", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "session_id=xyz789", receivedQuery, "Query parameters should be forwarded")
}

// TestKoditMCPBackend_AcceptHeaderForwarded verifies that the Accept header
// is forwarded to Kodit. This is critical for SSE - Kodit needs to see
// Accept: text/event-stream to return streaming responses.
func TestKoditMCPBackend_AcceptHeaderForwarded(t *testing.T) {
	var receivedAccept string

	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockKodit.Close()

	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/mcp/kodit/", nil)
	req.Header.Set("Accept", "text/event-stream")
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", receivedAccept, "Accept header should be forwarded")
}

// TestKoditMCPBackend_ContentTypePassthrough verifies that response Content-Type
// is passed through from Kodit. SSE responses need text/event-stream preserved.
func TestKoditMCPBackend_ContentTypePassthrough(t *testing.T) {
	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {}\n\n"))
	}))
	defer mockKodit.Close()

	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/mcp/kodit/", nil)
	req.Header.Set("Accept", "text/event-stream")
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"),
		"Content-Type should be passed through for SSE")
}

// TestKoditMCPBackend_PathSuffixForwarded verifies that path suffixes after
// /api/v1/mcp/kodit/ are correctly forwarded to Kodit.
func TestKoditMCPBackend_PathSuffixForwarded(t *testing.T) {
	var receivedPath string

	mockKodit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer mockKodit.Close()

	backend := NewKoditMCPBackend(&config.Kodit{
		Enabled: true,
		BaseURL: mockKodit.URL,
	})

	// Request to /api/v1/mcp/kodit/sse with path suffix "sse"
	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/mcp/kodit/sse", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": "sse"})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/mcp/sse", receivedPath, "Path suffix should be forwarded as /mcp/{suffix}")
}
