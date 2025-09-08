package config

import (
	"os"
	"testing"
)

func TestLoadExternalAgentRunnerConfig_RDPPasswordValidation(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid secure password",
			password:    "MySecureRDPPassword123!",
			expectError: false,
		},
		{
			name:        "empty password",
			password:    "",
			expectError: true,
			errorMsg:    "RDP_PASSWORD is required",
		},
		{
			name:        "too short password",
			password:    "short",
			expectError: true,
			errorMsg:    "must be at least 8 characters",
		},
		{
			name:        "insecure default - password",
			password:    "password",
			expectError: true,
			errorMsg:    "cannot use insecure default value",
		},
		{
			name:        "insecure default - admin",
			password:    "admin",
			expectError: true,
			errorMsg:    "cannot use insecure default value",
		},
		{
			name:        "insecure default - example placeholder",
			password:    "YOUR_SECURE_INITIAL_RDP_PASSWORD_HERE",
			expectError: true,
			errorMsg:    "cannot use insecure default value",
		},
		{
			name:        "insecure default - change me",
			password:    "CHANGE_ME_TO_UNIQUE_SECURE_PASSWORD",
			expectError: true,
			errorMsg:    "cannot use insecure default value",
		},
		{
			name:        "development default password",
			password:    "dev-insecure-change-me-in-production",
			expectError: true,
			errorMsg:    "is using development default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set required environment variables
			os.Setenv("API_TOKEN", "test-token")
			os.Setenv("RDP_PASSWORD", tt.password)
			defer func() {
				os.Unsetenv("API_TOKEN")
				os.Unsetenv("RDP_PASSWORD")
			}()

			cfg, err := LoadExternalAgentRunnerConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none for password: %s", tt.password)
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %s", err.Error())
					return
				}
				if cfg.RDPPassword != tt.password {
					t.Errorf("Expected password %s, got %s", tt.password, cfg.RDPPassword)
				}
			}
		})
	}
}

func TestLoadExternalAgentRunnerConfig_DefaultValues(t *testing.T) {
	// Set required environment variables
	os.Setenv("API_TOKEN", "test-token")
	os.Setenv("RDP_PASSWORD", "ValidSecurePassword123")
	defer func() {
		os.Unsetenv("API_TOKEN")
		os.Unsetenv("RDP_PASSWORD")
	}()

	cfg, err := LoadExternalAgentRunnerConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}

	// Test default values
	if cfg.Concurrency != 5 {
		t.Errorf("Expected default Concurrency 5, got %d", cfg.Concurrency)
	}
	if cfg.MaxTasks != 0 {
		t.Errorf("Expected default MaxTasks 0, got %d", cfg.MaxTasks)
	}
	if cfg.RDPStartPort != 3389 {
		t.Errorf("Expected default RDPStartPort 3389, got %d", cfg.RDPStartPort)
	}
	if cfg.RDPUser != "zed" {
		t.Errorf("Expected default RDPUser 'zed', got %s", cfg.RDPUser)
	}
	if cfg.ZedBinary != "zed" {
		t.Errorf("Expected default ZedBinary 'zed', got %s", cfg.ZedBinary)
	}
	if cfg.DisplayNum != 1 {
		t.Errorf("Expected default DisplayNum 1, got %d", cfg.DisplayNum)
	}
	if cfg.SessionTimeout != 3600 {
		t.Errorf("Expected default SessionTimeout 3600, got %d", cfg.SessionTimeout)
	}
}

func TestLoadExternalAgentRunnerConfig_RunnerIDGeneration(t *testing.T) {
	// Set required environment variables without RUNNER_ID
	os.Setenv("API_TOKEN", "test-token")
	os.Setenv("RDP_PASSWORD", "ValidSecurePassword123")
	defer func() {
		os.Unsetenv("API_TOKEN")
		os.Unsetenv("RDP_PASSWORD")
	}()

	cfg, err := LoadExternalAgentRunnerConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}

	// Should auto-generate runner ID from hostname
	if cfg.RunnerID == "" {
		t.Error("Expected RunnerID to be auto-generated from hostname, got empty string")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) <= len(s) && (substr == "" || findSubstring(s, substr) >= 0)
}

func findSubstring(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
