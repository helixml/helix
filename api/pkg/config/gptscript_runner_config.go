package config

import (
	"os"

	"github.com/kelseyhightower/envconfig"
)

type GPTScriptRunnerConfig struct {
	OpenAIKey string `envconfig:"OPENAI_API_KEY" required:"true"`

	// Control-plane connection
	APIHost  string `envconfig:"API_HOST" default:"http://localhost:80"`
	APIToken string `envconfig:"API_TOKEN" required:"true"`
	RunnerID string `envconfig:"RUNNER_ID"`

	Concurrency int `envconfig:"CONCURRENCY" default:"20"`
	// Exit after executing this many tasks. Useful when
	// GPTScript is run as a one-off task.
	MaxTasks int `envconfig:"MAX_TASKS" default:"1"`
}

// ZedAgentRunnerConfig represents the configuration for the Zed agent runner
type ZedAgentRunnerConfig struct {
	// Control-plane connection
	APIHost  string `envconfig:"API_HOST" default:"http://localhost:80"`
	APIToken string `envconfig:"API_TOKEN" required:"true"`
	RunnerID string `envconfig:"RUNNER_ID"`

	// Runner settings
	Concurrency int `envconfig:"CONCURRENCY" default:"5"`
	MaxTasks    int `envconfig:"MAX_TASKS" default:"1"`

	// RDP server settings
	RDPStartPort int    `envconfig:"RDP_START_PORT" default:"3389"`
	RDPUser      string `envconfig:"RDP_USER" default:"zed"`
	RDPPassword  string `envconfig:"RDP_PASSWORD" default:"zed123"`

	// Zed editor settings
	ZedBinary  string `envconfig:"ZED_BINARY" default:"zed"`
	DisplayNum int    `envconfig:"DISPLAY_NUM" default:"1"`
	VNCPort    int    `envconfig:"VNC_PORT" default:"5901"`

	// Session management
	SessionTimeout int    `envconfig:"SESSION_TIMEOUT" default:"3600"` // seconds
	MaxSessions    int    `envconfig:"MAX_SESSIONS" default:"10"`
	WorkspaceDir   string `envconfig:"WORKSPACE_DIR" default:"/tmp/zed-workspaces"`
}

func LoadGPTScriptRunnerConfig() (GPTScriptRunnerConfig, error) {
	var cfg GPTScriptRunnerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return GPTScriptRunnerConfig{}, err
	}

	// If runner ID is not provided, use the hostname
	if cfg.RunnerID == "" {
		cfg.RunnerID, _ = os.Hostname()
	}

	return cfg, nil
}

// LoadZedAgentRunnerConfig loads the configuration for the Zed agent runner
func LoadZedAgentRunnerConfig() (ZedAgentRunnerConfig, error) {
	var cfg ZedAgentRunnerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return ZedAgentRunnerConfig{}, err
	}

	// If runner ID is not provided, use the hostname
	if cfg.RunnerID == "" {
		cfg.RunnerID, _ = os.Hostname()
	}

	return cfg, nil
}
