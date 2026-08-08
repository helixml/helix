package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreviewURLHTTPSConfig(t *testing.T) {
	original, wasSet := os.LookupEnv("PREVIEW_URL_HTTPS")
	t.Cleanup(func() {
		if wasSet {
			require.NoError(t, os.Setenv("PREVIEW_URL_HTTPS", original))
			return
		}
		require.NoError(t, os.Unsetenv("PREVIEW_URL_HTTPS"))
	})

	require.NoError(t, os.Unsetenv("PREVIEW_URL_HTTPS"))
	cfg, err := LoadServerConfig()
	require.NoError(t, err)
	require.True(t, cfg.WebServer.PreviewURLHTTPS)

	require.NoError(t, os.Setenv("PREVIEW_URL_HTTPS", "false"))
	cfg, err = LoadServerConfig()
	require.NoError(t, err)
	require.False(t, cfg.WebServer.PreviewURLHTTPS)
}
