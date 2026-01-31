package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttemptFixJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with triple backticks at start",
			input:    "```\n{\"key\": \"value\"}",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with ```json wrapper",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: "\n{\"key\": \"value\"}\n",
		},
		{
			name:     "JSON with ```json and trailing double backticks",
			input:    "```json\n{\"key\": \"value\"}\n``",
			expected: "\n{\"key\": \"value\"}\n",
		},
		{
			name:     "JSON with ```json and single trailing backtick",
			input:    "```json\n{\"key\": \"value\"}\n`",
			expected: "\n{\"key\": \"value\"}\n",
		},
		{
			name:     "JSON with ```json and extra content after closing",
			input:    "```json\n{\"key\": \"value\"}\n```\nSome extra text",
			expected: "\n{\"key\": \"value\"}\n",
		},
		{
			name:     "JSON with trailing backticks only (no ```json)",
			input:    "{\"key\": \"value\"}``",
			expected: `{"key": "value"}`,
		},
		{
			name:     "real failing case - petId with 2 trailing backticks",
			input:    "```json\n{\n  \"petId\": \"99944\"\n}\n``",
			expected: "\n{\n  \"petId\": \"99944\"\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AttemptFixJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
