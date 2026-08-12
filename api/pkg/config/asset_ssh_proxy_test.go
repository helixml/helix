package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInferAssetSSHProxyAddressUsesSandboxAPIHost(t *testing.T) {
	address, err := inferAssetSSHProxyAddress("http://api:8080", "https://helix.example.com")
	require.NoError(t, err)
	require.Equal(t, "api:2224", address)
}

func TestInferAssetSSHProxyAddressFallsBackToServerHost(t *testing.T) {
	address, err := inferAssetSSHProxyAddress("", "https://helix.example.com:8080")
	require.NoError(t, err)
	require.Equal(t, "helix.example.com:2224", address)
}

func TestInferAssetSSHProxyAddressRejectsMissingHostname(t *testing.T) {
	_, err := inferAssetSSHProxyAddress("://bad", "https://helix.example.com")
	require.ErrorContains(t, err, "parse SANDBOX_API_URL")
}
