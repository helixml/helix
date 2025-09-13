package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_stripDateFromModelName(t *testing.T) {
	tests := []struct {
		name      string // description of this test case
		modelName string
		want      string
	}{
		// Sonnet models with dates
		{
			name:      "claude-sonnet-4 with dash date",
			modelName: "claude-sonnet-4-20250514",
			want:      "claude-sonnet-4",
		},
		{
			name:      "claude-sonnet-4 with @ date",
			modelName: "claude-sonnet-4@20250514",
			want:      "claude-sonnet-4",
		},
		{
			name:      "claude-sonnet-4 with thinking suffix and dash date",
			modelName: "claude-sonnet-4-20250514-thinking",
			want:      "claude-sonnet-4-20250514-thinking", // Should not strip since "thinking" is not a date
		},
		{
			name:      "claude-sonnet-4 with thinking suffix and @ date",
			modelName: "claude-sonnet-4@20250514-thinking",
			want:      "claude-sonnet-4@20250514-thinking", // Should not strip since "thinking" is not a date
		},

		// Opus models with dates
		{
			name:      "claude-opus-4 with dash date",
			modelName: "claude-opus-4-20250514",
			want:      "claude-opus-4",
		},
		{
			name:      "claude-opus-4 with @ date",
			modelName: "claude-opus-4@20250514",
			want:      "claude-opus-4",
		},
		{
			name:      "claude-opus-4-1 with dash date",
			modelName: "claude-opus-4-1-20250805",
			want:      "claude-opus-4-1",
		},
		{
			name:      "claude-opus-4-1 with @ date",
			modelName: "claude-opus-4-1@20250805",
			want:      "claude-opus-4-1",
		},
		{
			name:      "claude-opus-4-1 with thinking suffix and dash date",
			modelName: "claude-opus-4-1-20250805-thinking",
			want:      "claude-opus-4-1-20250805-thinking", // Should not strip since "thinking" is not a date
		},
		{
			name:      "claude-opus-4-1 with thinking suffix and @ date",
			modelName: "claude-opus-4-1@20250805-thinking",
			want:      "claude-opus-4-1@20250805-thinking", // Should not strip since "thinking" is not a date
		},

		// Haiku models with dates
		{
			name:      "claude-3-5-haiku with dash date",
			modelName: "claude-3-5-haiku-20241022",
			want:      "claude-3-5-haiku",
		},
		{
			name:      "claude-3-5-haiku with @ date",
			modelName: "claude-3-5-haiku@20241022",
			want:      "claude-3-5-haiku",
		},
		{
			name:      "claude-3-haiku with dash date",
			modelName: "claude-3-haiku-20240307",
			want:      "claude-3-haiku",
		},
		{
			name:      "claude-3-haiku with @ date",
			modelName: "claude-3-haiku@20240307",
			want:      "claude-3-haiku",
		},

		// Models without dates (should remain unchanged)
		{
			name:      "claude-sonnet-4 without date",
			modelName: "claude-sonnet-4",
			want:      "claude-sonnet-4",
		},
		{
			name:      "claude-opus-4 without date",
			modelName: "claude-opus-4",
			want:      "claude-opus-4",
		},
		{
			name:      "claude-opus-4-1 without date",
			modelName: "claude-opus-4-1",
			want:      "claude-opus-4-1",
		},
		{
			name:      "claude-3-5-haiku without date",
			modelName: "claude-3-5-haiku",
			want:      "claude-3-5-haiku",
		},
		{
			name:      "claude-3-haiku without date",
			modelName: "claude-3-haiku",
			want:      "claude-3-haiku",
		},

		// Edge cases
		{
			name:      "empty string",
			modelName: "",
			want:      "",
		},
		{
			name:      "single word",
			modelName: "claude",
			want:      "claude",
		},
		{
			name:      "model with non-date suffix",
			modelName: "claude-sonnet-4-beta",
			want:      "claude-sonnet-4-beta",
		},
		{
			name:      "model with short numeric suffix",
			modelName: "claude-sonnet-4-123",
			want:      "claude-sonnet-4-123", // Should not strip since it's not 8 digits
		},
		{
			name:      "model with long numeric suffix",
			modelName: "claude-sonnet-4-123456789",
			want:      "claude-sonnet-4-123456789", // Should not strip since it's not 8 digits
		},
		{
			name:      "model with mixed alphanumeric suffix",
			modelName: "claude-sonnet-4-2025a0514",
			want:      "claude-sonnet-4-2025a0514", // Should not strip since it contains non-numeric characters
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDateFromModelName(tt.modelName)
			assert.Equal(t, tt.want, got, "stripDateFromModelName() = %v, want %v", got, tt.want)
		})
	}
}
