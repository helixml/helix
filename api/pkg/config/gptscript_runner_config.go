package config

import "github.com/kelseyhightower/envconfig"

type GPTScriptRunnerConfig struct {
	OpenAIKey string `envconfig:"OPENAI_API_KEY" required:"true"`

	// Control-plane connection
	APIHost string `envconfig:"API_HOST" default:"localhost"`
	APIPort string `envconfig:"API_PORT" default:"80"`
}

func LoadGPTScriptRunnerConfig() (GPTScriptRunnerConfig, error) {
	var cfg GPTScriptRunnerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return GPTScriptRunnerConfig{}, err
	}
	return cfg, nil
}
