package browser

import (
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestEnsureNoProxyForBrowserHosts(t *testing.T) {
	tests := []struct {
		name            string
		httpProxy       string
		httpsProxy      string
		existingNoProxy string
		chromeURL       string
		launcherURL     string
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
			name:            "proxy configured, adds chrome host",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "localhost,127.0.0.1",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			expectedNoProxy: "localhost,127.0.0.1,chrome",
			expectChange:    true,
		},
		{
			name:            "proxy configured, chrome already in NO_PROXY",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "localhost,127.0.0.1,chrome",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			expectedNoProxy: "localhost,127.0.0.1,chrome",
			expectChange:    false,
		},
		{
			name:            "proxy configured, different chrome and launcher hosts",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "localhost",
			chromeURL:       "http://my-chrome:9222",
			launcherURL:     "http://my-launcher:7317",
			expectedNoProxy: "localhost,my-chrome,my-launcher",
			expectChange:    true,
		},
		{
			name:            "HTTPS_PROXY only",
			httpsProxy:      "http://proxy:8080",
			existingNoProxy: "",
			chromeURL:       "http://chrome:9222",
			launcherURL:     "http://chrome:7317",
			expectedNoProxy: "chrome",
			expectChange:    true,
		},
		{
			name:            "IP-based chrome URL",
			httpProxy:       "http://proxy:8080",
			existingNoProxy: "localhost",
			chromeURL:       "http://192.168.1.10:9222",
			launcherURL:     "http://192.168.1.10:7317",
			expectedNoProxy: "localhost,192.168.1.10",
			expectChange:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env vars that EnsureNoProxyForBrowserHosts reads/writes.
			// We can't use t.Setenv because we need to handle Unsetenv for lowercase variants.
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

			cfg := &config.ServerConfig{}
			cfg.RAG.Crawler.ChromeURL = tt.chromeURL
			cfg.RAG.Crawler.LauncherURL = tt.launcherURL

			EnsureNoProxyForBrowserHosts(cfg)

			result := os.Getenv("NO_PROXY")
			assert.Equal(t, tt.expectedNoProxy, result)

			// Verify both cases are set when changed
			if tt.expectChange {
				assert.Equal(t, os.Getenv("NO_PROXY"), os.Getenv("no_proxy"))
			}
		})
	}
}
