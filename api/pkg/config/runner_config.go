package config

import "github.com/kelseyhightower/envconfig"

type RunnerConfig struct {
	Models Models

	Runtimes Runtimes
}

func LoadRunnerConfig() (RunnerConfig, error) {
	var cfg RunnerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return RunnerConfig{}, err
	}
	return cfg, nil
}

type Models struct {
	Filter string `envconfig:"MODELS_FILTER" default:""`
}

type Runtimes struct {
	Axolotl struct {
		Enabled      bool     `envconfig:"RUNTIME_AXOLOTL_ENABLED" default:"true"`
		WarmupModels []string `envconfig:"RUNTIME_AXOLOTL_WARMUP_MODELS" default:"mistralai/Mistral-7B-Instruct-v0.1,stabilityai/stable-diffusion-xl-base-1.0"`
	}
	Ollama struct {
		Enabled      bool     `envconfig:"RUNTIME_OLLAMA_ENABLED" default:"true"`
		WarmupModels []string `envconfig:"RUNTIME_OLLAMA_WARMUP_MODELS" default:"mistral:7b-instruct"`
	}
}
