package openai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetContextValues(t *testing.T) {
	t.Run("sets values on fresh context", func(t *testing.T) {
		ctx := context.Background()
		vals := &ContextValues{
			OwnerID:       "owner-123",
			SessionID:     "session-456",
			InteractionID: "interaction-789",
			ProjectID:     "project-abc",
			SpecTaskID:    "task-def",
		}

		ctx = SetContextValues(ctx, vals)

		got, ok := GetContextValues(ctx)
		require.True(t, ok)
		assert.Equal(t, "owner-123", got.OwnerID)
		assert.Equal(t, "session-456", got.SessionID)
		assert.Equal(t, "interaction-789", got.InteractionID)
		assert.Equal(t, "project-abc", got.ProjectID)
		assert.Equal(t, "task-def", got.SpecTaskID)
		assert.Nil(t, got.OriginalRequest)
	})

	t.Run("preserves OriginalRequest from existing context", func(t *testing.T) {
		ctx := context.Background()
		originalRequest := []byte(`{"model": "gpt-4"}`)

		existingVals := &ContextValues{
			OwnerID:         "old-owner",
			SessionID:       "old-session",
			OriginalRequest: originalRequest,
		}
		ctx = SetContextValues(ctx, existingVals)

		newVals := &ContextValues{
			OwnerID:   "new-owner",
			SessionID: "new-session",
			ProjectID: "new-project",
		}
		ctx = SetContextValues(ctx, newVals)

		got, ok := GetContextValues(ctx)
		require.True(t, ok)
		assert.Equal(t, "new-owner", got.OwnerID)
		assert.Equal(t, "new-session", got.SessionID)
		assert.Equal(t, "new-project", got.ProjectID)
		assert.Equal(t, originalRequest, got.OriginalRequest)
	})

	t.Run("returns false for nil context", func(t *testing.T) {
		got, ok := GetContextValues(nil)
		assert.False(t, ok)
		assert.Nil(t, got)
	})

	t.Run("returns false for context without values", func(t *testing.T) {
		ctx := context.Background()
		got, ok := GetContextValues(ctx)
		assert.False(t, ok)
		assert.Nil(t, got)
	})
}
