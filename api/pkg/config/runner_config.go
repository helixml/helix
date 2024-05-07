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
	Axolotl struct {
		Enabled      bool          `envconfig:"RUNTIME_AXOLOTL_ENABLED" default:"true"`
		WarmupModels []string      `envconfig:"RUNTIME_AXOLOTL_WARMUP_MODELS" default:"mistralai/Mistral-7B-Instruct-v0.1,stabilityai/stable-diffusion-xl-base-1.0"`
		InstanceTTL  time.Duration `envconfig:"RUNTIME_AXOLOTL_INSTANCE_TTL" default:"60s"`
	}
	Ollama struct {
		Enabled      bool     `envconfig:"RUNTIME_OLLAMA_ENABLED" default:"true"`
		WarmupModels []string `envconfig:"RUNTIME_OLLAMA_WARMUP_MODELS" default:"llama3:instruct,mixtral:instruct,codellama:70b-instruct-q2_K,adrienbrault/nous-hermes2pro:Q5_K_S"`
		// Ollama instance can be kept for much longer as it automatically unloads
		// the model from memory when it's not used
		InstanceTTL time.Duration `envconfig:"RUNTIME_OLLAMA_INSTANCE_TTL" default:"60s"`
	}
}
