package config

import (
	"github.com/rs/zerolog/log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type CliConfig struct {
	URL    string `envconfig:"HELIX_URL" default:"https://app.tryhelix.ai"`
	APIKey string `envconfig:"HELIX_API_KEY"`
}

func LoadCliConfig() (CliConfig, error) {
	err := godotenv.Load()
	if err != nil {
		log.Warn().Msg("Error loading .env file")
	}

	var cfg CliConfig
	err = envconfig.Process("", &cfg)
	if err != nil {
		return CliConfig{}, err
	}
	return cfg, nil
}
