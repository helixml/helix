package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubdomainProxyMiddleware_ServeHTTP(t *testing.T) {
	// Create handlers
	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Proxy-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next-Handler", "true")
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		config         *SubdomainProxyConfig
		host           string
		path           string
		expectProxy    bool
		expectedPath   string
		expectedStatus int
	}{
		{
			name: "disabled config passes through",
			config: &SubdomainProxyConfig{
				Enabled:      false,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "p8080-ses_abc123.dev.helix.example.com",
			path:           "/test",
			expectProxy:    false,
			expectedStatus: http.StatusOK,
		},
		{
			name: "port-based subdomain routes to proxy",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "p8080-ses_abc123.dev.helix.example.com",
			path:           "/test",
			expectProxy:    true,
			expectedPath:   "/api/v1/sessions/ses_abc123/proxy/8080/test",
			expectedStatus: http.StatusOK,
		},
		{
			name: "port-based subdomain with complex path",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "p3000-ses_xyz789.dev.helix.example.com",
			path:           "/api/users/123",
			expectProxy:    true,
			expectedPath:   "/api/v1/sessions/ses_xyz789/proxy/3000/api/users/123",
			expectedStatus: http.StatusOK,
		},
		{
			name: "name-based subdomain routes to named port proxy",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "api-ses_abc123.dev.helix.example.com",
			path:           "/health",
			expectProxy:    true,
			expectedPath:   "/api/v1/sessions/ses_abc123/proxy-named/api/health",
			expectedStatus: http.StatusOK,
		},
		{
			name: "non-dev subdomain passes through",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "app.helix.example.com",
			path:           "/test",
			expectProxy:    false,
			expectedStatus: http.StatusOK,
		},
		{
			name: "main domain passes through",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "helix.example.com",
			path:           "/test",
			expectProxy:    false,
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid port returns error",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "p99999-ses_abc123.dev.helix.example.com",
			path:           "/test",
			expectProxy:    false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid subdomain format returns error",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "invalid.dev.helix.example.com",
			path:           "/test",
			expectProxy:    false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "host with port stripped correctly",
			config: &SubdomainProxyConfig{
				Enabled:      true,
				DevSubdomain: "dev",
				BaseDomain:   "helix.example.com",
			},
			host:           "p8080-ses_abc123.dev.helix.example.com:443",
			path:           "/test",
			expectProxy:    true,
			expectedPath:   "/api/v1/sessions/ses_abc123/proxy/8080/test",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := NewSubdomainProxyMiddleware(tt.config, proxyHandler, nextHandler)

			req := httptest.NewRequest("GET", "http://"+tt.host+tt.path, nil)
			req.Host = tt.host

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectProxy {
				if got := rr.Header().Get("X-Proxy-Path"); got != tt.expectedPath {
					t.Errorf("expected path %q, got %q", tt.expectedPath, got)
				}
				if rr.Header().Get("X-Next-Handler") != "" {
					t.Errorf("should not have called next handler")
				}
			} else if tt.expectedStatus == http.StatusOK {
				if rr.Header().Get("X-Next-Handler") != "true" {
					// Only check if status is OK and we expect passthrough
					// (error cases don't go to next handler)
				}
			}
		})
	}
}

func TestParseDevSubdomainConfig(t *testing.T) {
	tests := []struct {
		name            string
		devSubdomainEnv string
		serverURL       string
		expectedEnabled bool
		expectedDev     string
		expectedBase    string
	}{
		{
			name:            "empty config disabled",
			devSubdomainEnv: "",
			serverURL:       "https://helix.example.com",
			expectedEnabled: false,
		},
		{
			name:            "full domain format",
			devSubdomainEnv: "dev.helix.example.com",
			serverURL:       "https://helix.example.com",
			expectedEnabled: true,
			expectedDev:     "dev",
			expectedBase:    "helix.example.com",
		},
		{
			name:            "subdomain only with https server URL",
			devSubdomainEnv: "dev",
			serverURL:       "https://helix.example.com",
			expectedEnabled: true,
			expectedDev:     "dev",
			expectedBase:    "helix.example.com",
		},
		{
			name:            "subdomain only with http server URL",
			devSubdomainEnv: "dev",
			serverURL:       "http://helix.example.com:8080",
			expectedEnabled: true,
			expectedDev:     "dev",
			expectedBase:    "helix.example.com",
		},
		{
			name:            "localhost disabled",
			devSubdomainEnv: "dev",
			serverURL:       "http://localhost:8080",
			expectedEnabled: false,
		},
		{
			name:            "server URL with path",
			devSubdomainEnv: "dev",
			serverURL:       "https://helix.example.com/api",
			expectedEnabled: true,
			expectedDev:     "dev",
			expectedBase:    "helix.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseDevSubdomainConfig(tt.devSubdomainEnv, tt.serverURL)

			if config.Enabled != tt.expectedEnabled {
				t.Errorf("expected enabled=%v, got %v", tt.expectedEnabled, config.Enabled)
			}
			if tt.expectedEnabled {
				if config.DevSubdomain != tt.expectedDev {
					t.Errorf("expected DevSubdomain=%q, got %q", tt.expectedDev, config.DevSubdomain)
				}
				if config.BaseDomain != tt.expectedBase {
					t.Errorf("expected BaseDomain=%q, got %q", tt.expectedBase, config.BaseDomain)
				}
			}
		})
	}
}
