package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Agents live on the isolated sandbox bridge, where only helix-api.internal
// resolves; the control plane's own host would be advertised to a client that
// can never reach it.
func TestAssetSSHProxyAddressDefaultsToSandboxBridge(t *testing.T) {
	t.Setenv("SANDBOX_API_URL", "http://api:8080")
	t.Setenv("SERVER_URL", "https://helix.example.com")
	t.Setenv("ASSET_SSH_PROXY_ADDRESS", "")
	cfg, err := LoadServerConfig()
	require.NoError(t, err)
	require.Equal(t, "helix-api.internal:2224", cfg.WebServer.AssetSSHProxyAddress)
}

func TestAssetSSHProxyAddressHonoursOverride(t *testing.T) {
	t.Setenv("ASSET_SSH_PROXY_ADDRESS", "ssh.example.com:2224")
	cfg, err := LoadServerConfig()
	require.NoError(t, err)
	require.Equal(t, "ssh.example.com:2224", cfg.WebServer.AssetSSHProxyAddress)
}
