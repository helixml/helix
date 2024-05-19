package config

import "github.com/kelseyhightower/envconfig"

type GPTScriptRunnerConfig struct {
	OpenAIKey string `envconfig:"OPENAI_API_KEY" required:"true"`

	// Control-plane connection
	APIHost  string `envconfig:"API_HOST" default:"http://localhost:80"`
	APIToken string `envconfig:"API_TOKEN" required:"true"`

	Concurrency int `envconfig:"CONCURRENCY" default:"20"`
	// Exit after executing this many tasks. Useful when
	// GPTScript is run as a one-off task.
	MaxTasks int `envconfig:"MAX_TASKS" default:"1"`
}

func LoadGPTScriptRunnerConfig() (GPTScriptRunnerConfig, error) {
	var cfg GPTScriptRunnerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return GPTScriptRunnerConfig{}, err
	}
	return cfg, nil
}
