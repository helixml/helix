package config

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"
)

func TestSecretIntakeConfigPreservesEnvironmentName(t *testing.T) {
	t.Setenv("HELIX_SECRET_INTAKE_ENABLED", "true")
	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))
	require.True(t, cfg.ConnectPortal.SecretIntakeEnabled)
}
