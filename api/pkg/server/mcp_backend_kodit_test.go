package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestKoditMCPBackend_Disabled(t *testing.T) {
	backend := NewKoditMCPBackend(nil, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/mcp/kodit/", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": ""})
	backend.ServeHTTP(rec, req, &types.User{ID: "u"})
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestKoditMCPBackend_Routing(t *testing.T) {
	// Fake handler echoes the rewritten path so we can verify routing.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	backend := &KoditMCPBackend{handler: handler, enabled: true}

	tests := []struct {
		name, url, pathVar, wantPath string
	}{
		{"root with slash", "/api/v1/mcp/kodit/", "", "/mcp/"},
		{"with suffix", "/api/v1/mcp/kodit/message", "message", "/mcp/message"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.url, strings.NewReader(`{}`))
			req = mux.SetURLVars(req, map[string]string{"server": "kodit", "path": tt.pathVar})
			rec := httptest.NewRecorder()
			backend.ServeHTTP(rec, req, &types.User{ID: "u"})
			assert.Equal(t, http.StatusOK, rec.Code)
			body, _ := io.ReadAll(rec.Body)
			assert.Equal(t, tt.wantPath, string(body))
		})
	}
}
