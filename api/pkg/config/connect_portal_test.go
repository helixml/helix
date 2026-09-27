package config

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"
)

func TestConnectPortalConfigPreservesEnvironmentNames(t *testing.T) {
	t.Setenv("HELIX_PORTAL_MOCK_ENABLED", "true")
	t.Setenv("HELIX_SECRET_INTAKE_ENABLED", "true")
	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))
	require.True(t, cfg.ConnectPortal.MockEnabled)
	require.True(t, cfg.ConnectPortal.SecretIntakeEnabled)
}
