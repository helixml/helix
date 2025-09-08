package config

import (
	"os"

	"github.com/kelseyhightower/envconfig"
)

// ExternalAgentRunnerConfig represents the configuration for the external agent runner
type ExternalAgentRunnerConfig struct {
	// Control-plane connection
	APIHost  string `envconfig:"API_HOST" default:"http://localhost:80"`
	APIToken string `envconfig:"API_TOKEN" required:"true"`
	RunnerID string `envconfig:"RUNNER_ID"`

	// Runner settings
	Concurrency int `envconfig:"CONCURRENCY" default:"5"`
	MaxTasks    int `envconfig:"MAX_TASKS" default:"0"` // 0 means unlimited

	// RDP server settings
	RDPStartPort int    `envconfig:"RDP_START_PORT" default:"3389"`
	RDPUser      string `envconfig:"RDP_USER" default:"zed"`
	RDPPassword  string `envconfig:"RDP_PASSWORD" required:"true"`

	// Zed editor settings
	ZedBinary  string `envconfig:"ZED_BINARY" default:"zed"`
	DisplayNum int    `envconfig:"DISPLAY_NUM" default:"1"`

	// Session management
	SessionTimeout int    `envconfig:"SESSION_TIMEOUT" default:"3600"` // seconds
	MaxSessions    int    `envconfig:"MAX_SESSIONS" default:"10"`
	WorkspaceDir   string `envconfig:"WORKSPACE_DIR" default:"/tmp/zed-workspaces"`
}

// LoadExternalAgentRunnerConfig loads the configuration for the external agent runner
func LoadExternalAgentRunnerConfig() (ExternalAgentRunnerConfig, error) {
	var cfg ExternalAgentRunnerConfig
	err := envconfig.Process("", &cfg)
	if err != nil {
		return ExternalAgentRunnerConfig{}, err
	}

	// If runner ID is not provided, use the hostname
	if cfg.RunnerID == "" {
		cfg.RunnerID, _ = os.Hostname()
	}

	return cfg, nil
}
