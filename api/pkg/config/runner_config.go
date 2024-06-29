package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type RunnerConfig struct {
	ApiHost  string `envconfig:"API_HOST" default:"http://localhost:8080"` // Control-plane API host
	ApiToken string `envconfig:"API_TOKEN"`                                // Control-plane API token

	GPUMemory string `envconfig:"GPU_MEMORY"` // Available GPU memory size for the runner (i.e. "24GB")

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
		WarmupModels []string      `envconfig:"RUNTIME_AXOLOTL_WARMUP_MODELS" default:"mistralai/Mistral-7B-Instruct-v0.1"`
		InstanceTTL  time.Duration `envconfig:"RUNTIME_AXOLOTL_INSTANCE_TTL" default:"10s"`
	}
	Ollama struct {
		Enabled      bool          `envconfig:"RUNTIME_OLLAMA_ENABLED" default:"true"`
		WarmupModels []string      `envconfig:"RUNTIME_OLLAMA_WARMUP_MODELS" default:"llama3:instruct,mixtral:instruct,codellama:70b-instruct-q2_K,adrienbrault/nous-hermes2theta-llama3-8b:q8_0,phi3:instruct"`
		InstanceTTL  time.Duration `envconfig:"RUNTIME_OLLAMA_INSTANCE_TTL" default:"10s"`
	}
}
