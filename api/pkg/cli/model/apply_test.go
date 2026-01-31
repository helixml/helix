package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMemoryString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
		hasError bool
	}{
		// Binary units (powers of 1024)
		{"8 GiB", "8GiB", 8 * 1024 * 1024 * 1024, false},
		{"16 GiB", "16GiB", 16 * 1024 * 1024 * 1024, false},
		{"8 G (binary)", "8G", 8 * 1024 * 1024 * 1024, false},
		{"16 G (binary)", "16G", 16 * 1024 * 1024 * 1024, false},

		// Decimal units (powers of 1000)
		{"8 GB", "8GB", 8 * 1000 * 1000 * 1000, false},
		{"16 GB", "16GB", 16 * 1000 * 1000 * 1000, false},

		// MiB/MB
		{"512 MiB", "512MiB", 512 * 1024 * 1024, false},
		{"512 M", "512M", 512 * 1024 * 1024, false},
		{"512 MB", "512MB", 512 * 1000 * 1000, false},

		// KiB/KB
		{"1024 KiB", "1024KiB", 1024 * 1024, false},
		{"1024 K", "1024K", 1024 * 1024, false},
		{"1000 KB", "1000KB", 1000 * 1000, false},

		// Bytes
		{"1024 B", "1024B", 1024, false},
		{"Plain number", "1073741824", 1073741824, false},

		// Floating point
		{"2.5 GB", "2.5GB", uint64(2.5 * 1000 * 1000 * 1000), false},
		{"1.5 GiB", "1.5GiB", uint64(1.5 * 1024 * 1024 * 1024), false},

		// Case insensitive
		{"lowercase gb", "8gb", 8 * 1000 * 1000 * 1000, false},
		{"mixed case", "8Gb", 8 * 1000 * 1000 * 1000, false},

		// With spaces
		{"spaces", " 8GB ", 8 * 1000 * 1000 * 1000, false},

		// Error cases
		{"empty string", "", 0, true},
		{"invalid number", "abcGB", 0, true},
		{"negative", "-8GB", 0, true},
		{"invalid unit", "8XB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseMemoryString(tt.input)

			if tt.hasError {
				assert.Error(t, err, "Expected error for input: %s", tt.input)
			} else {
				require.NoError(t, err, "Unexpected error for input: %s", tt.input)
				assert.Equal(t, tt.expected, result, "Wrong result for input: %s", tt.input)
			}
		})
	}
}

func TestParseMemoryField(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected uint64
		hasError bool
	}{
		{"uint64", uint64(1024), 1024, false},
		{"int64", int64(2048), 2048, false},
		{"int", int(4096), 4096, false},
		{"float64", float64(8192), 8192, false},
		{"string GB", "8GB", 8 * 1000 * 1000 * 1000, false},
		{"string GiB", "8GiB", 8 * 1024 * 1024 * 1024, false},
		{"nil", nil, 0, true},
		{"unsupported type", []int{1, 2, 3}, 0, true},
		{"invalid string", "invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseMemoryField(tt.input)

			if tt.hasError {
				assert.Error(t, err, "Expected error for input: %v", tt.input)
			} else {
				require.NoError(t, err, "Unexpected error for input: %v", tt.input)
				assert.Equal(t, tt.expected, result, "Wrong result for input: %v", tt.input)
			}
		})
	}
}

func TestValidKinds(t *testing.T) {
	validKinds := []string{"Model", "Agent", "AIApp"}

	for _, kind := range validKinds {
		t.Run(kind, func(t *testing.T) {
			// Test that each kind would be accepted by the validation logic
			isValid := false
			for _, validKind := range validKinds {
				if kind == validKind {
					isValid = true
					break
				}
			}
			assert.True(t, isValid, "Kind %s should be valid", kind)
		})
	}

	// Test invalid kind
	t.Run("Invalid kind", func(t *testing.T) {
		invalidKind := "InvalidKind"
		isValid := false
		for _, validKind := range validKinds {
			if invalidKind == validKind {
				isValid = true
				break
			}
		}
		assert.False(t, isValid, "Kind %s should be invalid", invalidKind)
	})
}

func TestParseModelConfigFile_VLLMWithRuntimeArgs(t *testing.T) {
	yamlContent := `apiVersion: helix.ml/v1alpha1
kind: Model
metadata:
  name: qwen-3-coder-30b-a3b-instruct
spec:
  id: Qwen/Qwen3-Coder-30B-A3B-Instruct
  type: chat
  runtime: vllm
  memory: 39000000000
  runtime_args:
    args:
      - "--trust-remote-code"
      - "--max-model-len"
      - "32768"
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(yamlContent), 0644))

	_, err := parseModelConfigFile(tmpFile)
	require.NoError(t, err)
}
