package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsureNoProxyForInternalHosts(t *testing.T) {
	tests := []struct {
		name            string
		httpProxy       string
		httpsProxy      string
		existingNoProxy string
		chromeURL       string
		launcherURL     string
		searxngURL      string
		tikaURL         string
		expectedNoProxy string
		expectChange    bool
	}{
		{
			name:            "no proxy configured, does nothing",
			httpProxy:       "",
			httpsProxy:      "",
			existingNoProxy: "",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			expectedNoProxy: "",
			expectChange:    false,
		},
		{
			name:            "proxy configured, adds all internal hosts",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "localhost,127.0.0.1",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			searxngURL:      "http://searxng:8080",
			tikaURL:         "http://tika:9998",
			expectChange:    true,
		},
		{
			name:            "proxy configured, hosts already in NO_PROXY",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "localhost,127.0.0.1,chrome,searxng,tika",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			searxngURL:      "http://searxng:8080",
			tikaURL:         "http://tika:9998",
			expectedNoProxy: "localhost,127.0.0.1,chrome,searxng,tika",
			expectChange:    false,
		},
		{
			name:            "HTTPS_PROXY only",
			httpsProxy:      "http://proxy:8080",
			existingNoProxy: "",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			expectChange:    true,
		},
		{
			name:            "IP-based URLs",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "localhost",
			chromeURL:       "http://192.168.1.10:9222",
			launcherURL:     "http://192.168.1.10:7317",
			expectedNoProxy: "localhost,192.168.1.10",
			expectChange:    true,
		},
		{
			name:            "skips filesystem paths",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			expectChange:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env vars
			origHTTP := os.Getenv("HTTP_PROXY")
			origHTTPS := os.Getenv("HTTPS_PROXY")
			origNoProxy := os.Getenv("NO_PROXY")
			origNoProxyLower := os.Getenv("no_proxy")
			defer func() {
				os.Setenv("HTTP_PROXY", origHTTP)
				os.Setenv("HTTPS_PROXY", origHTTPS)
				os.Setenv("NO_PROXY", origNoProxy)
				os.Setenv("no_proxy", origNoProxyLower)
				os.Unsetenv("http_proxy")
				os.Unsetenv("https_proxy")
			}()

			// Set test env
			os.Setenv("HTTP_PROXY", tt.httpProxy)
			os.Setenv("HTTPS_PROXY", tt.httpsProxy)
			os.Setenv("NO_PROXY", tt.existingNoProxy)
			os.Setenv("no_proxy", "")
			os.Unsetenv("http_proxy")
			os.Unsetenv("https_proxy")

			cfg := &ServerConfig{}
			cfg.RAG.Crawler.ChromeURL = tt.chromeURL
			cfg.RAG.Crawler.LauncherURL = tt.launcherURL
			if tt.searxngURL != "" {
				cfg.Search.SearXNGBaseURL = tt.searxngURL
			}
			if tt.tikaURL != "" {
				cfg.TextExtractor.Tika.URL = tt.tikaURL
			}
			// Simulate production: FRONTEND_URL is a filesystem path
			cfg.WebServer.FrontendURL = "/www"

			cfg.EnsureNoProxyForInternalHosts()

			result := os.Getenv("NO_PROXY")

			if tt.expectedNoProxy != "" {
				assert.Equal(t, tt.expectedNoProxy, result)
			}

			if tt.expectChange {
				// Verify both cases are set
				assert.Equal(t, os.Getenv("NO_PROXY"), os.Getenv("no_proxy"))
				// Verify something was actually added (result differs from original)
				assert.NotEqual(t, tt.existingNoProxy, result)
			}

			// Verify filesystem paths are never added
			assert.NotContains(t, result, "/www")
		})
	}
}
