package server

import (
	"testing"
)

func TestValidateAndNormalizeDomain(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
		errorMsg    string
	}{
		// Valid cases
		{
			name:        "simple domain",
			input:       "example.com",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "subdomain",
			input:       "sub.example.com",
			expected:    "sub.example.com",
			expectError: false,
		},
		{
			name:        "uppercase normalized to lowercase",
			input:       "EXAMPLE.COM",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "mixed case normalized",
			input:       "Example.Com",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "domain with whitespace trimmed",
			input:       "  example.com  ",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "empty string allowed (clears domain)",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "co.uk domain",
			input:       "company.co.uk",
			expected:    "company.co.uk",
			expectError: false,
		},
		{
			name:        "domain with hyphen",
			input:       "my-company.com",
			expected:    "my-company.com",
			expectError: false,
		},
		{
			name:        "domain with numbers",
			input:       "company123.com",
			expected:    "company123.com",
			expectError: false,
		},

		// Invalid cases
		{
			name:        "starts with @",
			input:       "@example.com",
			expectError: true,
			errorMsg:    "should not start with @",
		},
		{
			name:        "email address",
			input:       "user@example.com",
			expectError: true,
			errorMsg:    "should not contain @",
		},
		{
			name:        "no TLD",
			input:       "example",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "starts with dot",
			input:       ".example.com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "ends with dot",
			input:       "example.com.",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "starts with hyphen",
			input:       "-example.com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "double dot",
			input:       "example..com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "special characters",
			input:       "example!.com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateAndNormalizeDomain(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// containsString checks if s contains substr (case-insensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
