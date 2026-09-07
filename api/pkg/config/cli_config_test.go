package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadCliConfig_UserEnvironment(t *testing.T) {
	t.Setenv("HELIX_URL", "https://helix.example.com")
	t.Setenv("HELIX_API_KEY", "hl-user")
	t.Setenv("HELIX_API_URL", "http://api:8080")
	t.Setenv("USER_API_TOKEN", "sandbox-token")

	cfg, err := LoadCliConfig()
	require.NoError(t, err)
	require.Equal(t, "https://helix.example.com", cfg.URL)
	require.Equal(t, "hl-user", cfg.APIKey)
}

func TestLoadCliConfig_SandboxEnvironment(t *testing.T) {
	t.Setenv("HELIX_URL", "")
	t.Setenv("HELIX_API_KEY", "")
	t.Setenv("HELIX_API_URL", "http://api:8080")
	t.Setenv("USER_API_TOKEN", "sandbox-token")

	cfg, err := LoadCliConfig()
	require.NoError(t, err)
	require.Equal(t, "http://api:8080", cfg.URL)
	require.Equal(t, "sandbox-token", cfg.APIKey)
}

func TestLoadCliConfig_DefaultURL(t *testing.T) {
	t.Setenv("HELIX_URL", "")
	t.Setenv("HELIX_API_KEY", "hl-user")
	t.Setenv("HELIX_API_URL", "")
	t.Setenv("USER_API_TOKEN", "")

	cfg, err := LoadCliConfig()
	require.NoError(t, err)
	require.Equal(t, "https://app.helix.ml", cfg.URL)
}

func TestCliURL_PrefersUserOverSandboxOverDefault(t *testing.T) {
	t.Setenv("HELIX_URL", "")
	t.Setenv("HELIX_API_URL", "")
	require.Equal(t, "http://localhost:8080", CliURL("http://localhost:8080"))

	t.Setenv("HELIX_API_URL", "http://api:8080")
	require.Equal(t, "http://api:8080", CliURL("http://localhost:8080"))

	t.Setenv("HELIX_URL", "https://helix.example.com")
	require.Equal(t, "https://helix.example.com", CliURL("http://localhost:8080"))
}
