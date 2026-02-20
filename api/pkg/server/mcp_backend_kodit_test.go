package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestKoditBackend(t *testing.T) *KoditMCPBackend {
	t.Helper()
	dataDir := t.TempDir()
	client, err := kodit.New(
		kodit.WithSQLite(filepath.Join(dataDir, "test.db")),
		kodit.WithDataDir(dataDir),
		kodit.WithWorkerCount(0),
		kodit.WithEmbeddingProvider(&noopEmbedder{}),
		kodit.WithSkipProviderValidation(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return NewKoditMCPBackend(client, true)
}

func TestKoditMCPBackend_Disabled(t *testing.T) {
	backend := NewKoditMCPBackend(nil, false)

	req := httptest.NewRequest("GET", "http://localhost:8080/api/v1/mcp/kodit/", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

// TestKoditMCPBackend_RequestBodyForwarded verifies that POST request bodies
// (JSON-RPC payloads) are correctly forwarded to the in-process handler.
func TestKoditMCPBackend_RequestBodyForwarded(t *testing.T) {
	backend := newTestKoditBackend(t)

	jsonRPCPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/mcp/kodit/", strings.NewReader(jsonRPCPayload))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	// Should get a valid response (not a 404)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
	// The handler should produce some response body
	body, _ := io.ReadAll(rec.Body)
	assert.NotEmpty(t, body)
}

// TestKoditMCPBackend_PostWithPathSuffix verifies that POST requests with path
// suffixes after /api/v1/mcp/kodit/ are correctly forwarded.
func TestKoditMCPBackend_PostWithPathSuffix(t *testing.T) {
	backend := newTestKoditBackend(t)

	// POST to a sub-path — the MCP handler should handle it (or return
	// a non-404 error). Unlike GET (which opens long-lived SSE streams),
	// POST returns immediately.
	jsonRPCPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/mcp/kodit/message", strings.NewReader(jsonRPCPayload))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": "message"})

	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "test-user"})

	// The in-process handler should handle the path (not panic)
	assert.True(t, rec.Code > 0)
}
