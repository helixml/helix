package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type RunnerConfig struct {
	Models   Models
	Runtimes Runtimes
	CacheDir string `envconfig:"CACHE_DIR" default:"/root/.cache/huggingface"` // Used to download model weights. Ideally should be persistent
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
	V2Engine bool `envconfig:"RUNTIME_V2_ENGINE" default:"true"`
	Axolotl  struct {
		Enabled     bool          `envconfig:"RUNTIME_AXOLOTL_ENABLED" default:"true"`
		InstanceTTL time.Duration `envconfig:"RUNTIME_AXOLOTL_INSTANCE_TTL" default:"10s"`
	}
	Ollama OllamaRuntimeConfig
}

type OllamaRuntimeConfig struct {
	Enabled     bool          `envconfig:"RUNTIME_OLLAMA_ENABLED" default:"true"`
	InstanceTTL time.Duration `envconfig:"RUNTIME_OLLAMA_INSTANCE_TTL" default:"10s"`
}
