package config

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// DefaultCliURL is the control plane the CLI talks to when nothing else is
// configured.
const DefaultCliURL = "https://app.helix.ml"

type CliConfig struct {
	URL           string `envconfig:"HELIX_URL"`
	APIKey        string `envconfig:"HELIX_API_KEY"`
	TLSSkipVerify bool   `envconfig:"HELIX_TLS_SKIP_VERIFY" default:"false"`
}

// CliURL resolves the control plane URL: HELIX_URL, then HELIX_API_URL (the
// name a Helix sandbox exports — see hydra_executor.buildEnvVars), then
// defaultURL. Every `helix` subcommand resolves its URL through here so the
// sandbox convention works everywhere without per-command fallbacks.
func CliURL(defaultURL string) string {
	if url := os.Getenv("HELIX_URL"); url != "" {
		return url
	}
	if url := os.Getenv("HELIX_API_URL"); url != "" {
		return url
	}
	return defaultURL
}

// CliAPIKey resolves the API key: HELIX_API_KEY, then USER_API_TOKEN (the
// name a Helix sandbox exports). Empty when neither is set.
func CliAPIKey() string {
	if key := os.Getenv("HELIX_API_KEY"); key != "" {
		return key
	}
	return os.Getenv("USER_API_TOKEN")
}

// LoadCliConfig resolves the control plane URL and API key for the CLI.
//
// Two naming conventions are accepted. HELIX_URL / HELIX_API_KEY are what a
// user sets on their own machine. Inside a Helix sandbox the platform exports
// HELIX_API_URL / USER_API_TOKEN instead, so those are honoured when the
// user-facing names are unset: the `helix` CLI, and the agent skills that
// describe it, work in both places without setup.
func LoadCliConfig() (CliConfig, error) {
	_ = godotenv.Load()

	var cfg CliConfig
	if err := envconfig.Process("", &cfg); err != nil {
		return CliConfig{}, err
	}
	cfg.URL = CliURL(DefaultCliURL)
	cfg.APIKey = CliAPIKey()
	return cfg, nil
}
