package browser

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestNewManagedLauncherConfinesWebRTCToProxy(t *testing.T) {
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Flags":{"headless":[]}}`))
	}))
	defer manager.Close()

	var cfg config.ServerConfig
	cfg.RAG.Crawler.LauncherEnabled = true
	cfg.RAG.Crawler.LauncherURL = manager.URL

	b, err := New(&cfg)
	require.NoError(t, err)

	require.Equal(t, "disable_non_proxied_udp", b.launcher.Get(webRTCIPHandlingPolicy))
	_, header := b.launcher.ClientHeader()
	require.Contains(t, header.Get("Rod-Launcher"), `"force-webrtc-ip-handling-policy":["disable_non_proxied_udp"]`)
}
