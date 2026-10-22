package config

import (
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"
)

func TestSandboxMaxDevContainersDefault(t *testing.T) {
	const key = "HELIX_SANDBOX_MAX_DEV_CONTAINERS"
	value, exists := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if exists {
			require.NoError(t, os.Setenv(key, value))
		}
	})

	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))
	require.Equal(t, -1, cfg.SandboxMaxDevContainers)
}

func TestSandboxMaxDevContainersPositiveOverride(t *testing.T) {
	t.Setenv("HELIX_SANDBOX_MAX_DEV_CONTAINERS", "12")
	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))
	require.Equal(t, 12, cfg.SandboxMaxDevContainers)
}
