package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type CliConfig struct {
	URL           string `envconfig:"HELIX_URL" default:"https://app.helix.ml"`
	APIKey        string `envconfig:"HELIX_API_KEY"`
	TLSSkipVerify bool   `envconfig:"HELIX_TLS_SKIP_VERIFY" default:"false"`
}

func LoadCliConfig() (CliConfig, error) {
	_ = godotenv.Load()

	var cfg CliConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return CliConfig{}, err
	}
	return cfg, nil
}
