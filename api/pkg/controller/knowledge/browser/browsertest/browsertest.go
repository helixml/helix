// Package browsertest starts a local headless Chrome for tests that need a
// real browser. Tests are skipped when no Chrome binary is available.
package browsertest

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-rod/rod/lib/launcher"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
)

// chromeBin returns a local Chrome binary: $HELIX_TEST_CHROME_BIN, one on
// PATH, or a Playwright-installed Chromium. It never downloads one.
func chromeBin() string {
	if bin := os.Getenv("HELIX_TEST_CHROME_BIN"); bin != "" {
		return bin
	}
	if bin, ok := launcher.LookPath(); ok {
		return bin
	}
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".cache", "ms-playwright", "chromium-*", "chrome-linux*", "chrome"))
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return ""
}

// New launches a headless Chrome and returns a Browser connected to it.
func New(t *testing.T) *browser.Browser {
	t.Helper()
	bin := chromeBin()
	if bin == "" {
		t.Skip("no local Chrome binary (set HELIX_TEST_CHROME_BIN)")
	}
	l := launcher.New().Bin(bin).Headless(true).Set("no-sandbox")
	wsURL, err := l.Launch()
	if err != nil {
		t.Skipf("launch Chrome %s: %v", bin, err)
	}
	t.Cleanup(l.Kill)

	u, err := url.Parse(wsURL)
	if err != nil {
		t.Fatalf("parse devtools URL: %v", err)
	}
	cfg := &config.ServerConfig{}
	cfg.RAG.Crawler.ChromeURL = "http://" + u.Host
	b, err := browser.New(cfg)
	if err != nil {
		t.Fatalf("connect to Chrome: %v", err)
	}
	t.Cleanup(b.Close)
	return b
}
