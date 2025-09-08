package config

import (
	"fmt"
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
	RDPPassword  string `envconfig:"RDP_PASSWORD"` // Optional - used for initial setup only

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

	// Security validation for RDP_PASSWORD
	if cfg.RDPPassword == "" {
		return ExternalAgentRunnerConfig{}, fmt.Errorf("RDP_PASSWORD is required for secure container initialization")
	}

	// Check for common insecure default passwords
	// insecurePasswords := []string{
	// 	"password", "123456", "admin", "zed", "rdp", "guest", "user",
	// 	"initial", "default", "temp", "test", "demo", "changeme",
	// 	"initial-secure-rdp-password", "CHANGE_ME_TO_UNIQUE_SECURE_PASSWORD",
	// 	"YOUR_SECURE_INITIAL_RDP_PASSWORD_HERE", "your-unique-initial-rdp-password",
	// 	"dev-insecure-change-me-in-production", // Development default
	// }

	// for _, insecure := range insecurePasswords {
	// 	if cfg.RDPPassword == insecure {
	// 		if insecure == "dev-insecure-change-me-in-production" {
	// 			// Allow development default but warn
	// 			log.Warn().
	// 				Str("password", insecure).
	// 				Msg("⚠️ SECURITY WARNING: Using insecure development default RDP password! Generate a secure password with: openssl rand -base64 32")
	// 			break
	// 		}
	// 		return ExternalAgentRunnerConfig{}, fmt.Errorf("RDP_PASSWORD cannot use insecure default value: %s", insecure)
	// 	}
	// }

	// Ensure minimum password complexity
	if len(cfg.RDPPassword) < 8 {
		return ExternalAgentRunnerConfig{}, fmt.Errorf("RDP_PASSWORD must be at least 8 characters long")
	}

	return cfg, nil
}
