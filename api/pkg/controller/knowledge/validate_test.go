package knowledge

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		knowledge   *types.AssistantKnowledge
		expectError bool
	}{
		{
			name: "Valid cron schedule",
			knowledge: &types.AssistantKnowledge{
				RefreshSchedule: "0 0 * * *", // Every 24 hours
			},
			expectError: false,
		},
		{
			name: "Invalid cron schedule - too frequent",
			knowledge: &types.AssistantKnowledge{
				RefreshSchedule: "*/5 * * * *", // Every 5 minutes
			},
			expectError: true,
		},
		{
			name: "Invalid humanized schedule - too frequent",
			knowledge: &types.AssistantKnowledge{
				RefreshSchedule: "@every 5m",
			},
			expectError: true,
		},
		{
			name: "Valid humanized schedule",
			knowledge: &types.AssistantKnowledge{
				RefreshSchedule: "@every 15m",
			},
			expectError: false,
		},
		{
			name: "Invalid cron syntax",
			knowledge: &types.AssistantKnowledge{
				RefreshSchedule: "invalid cron",
			},
			expectError: true,
		},
		{
			name: "Empty schedule",
			knowledge: &types.AssistantKnowledge{
				RefreshSchedule: "",
			},
			expectError: false,
		},
		// Add more test cases for web source validation if needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.knowledge)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
