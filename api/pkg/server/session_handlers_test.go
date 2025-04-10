package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
)

func TestLimitInteractions(t *testing.T) {
	// Helper function to create test interactions
	createTestInteractions := func() []*types.Interaction {
		interactions := []*types.Interaction{
			{
				ID:      "1",
				Message: "A",
			},
			{
				ID:      "2",
				Message: "B",
			},
			{
				ID:      "3",
				Message: "C",
			},
			{
				ID:      "4",
				Message: "D",
			},
			{
				ID:      "5",
				Message: "E",
			},
			{
				ID:      "6",
				Message: "F",
			},
		}
		return interactions
	}

	// Case when we have less interactions than the limit
	t.Run("LessThanLimit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 10)
		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
		assert.Equal(t, "A", result[0].Message)
		assert.Equal(t, "E", result[4].Message)
	})

	t.Run("Exact limit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 6)
		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
		assert.Equal(t, "A", result[0].Message)
		assert.Equal(t, "E", result[4].Message)
	})

	// More messages than the limit
	t.Run("MoreThanLimit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 3)
		assert.Equal(t, 3, len(result), "Should have all but the last interaction")
		assert.Equal(t, "C", result[0].Message)
		assert.Equal(t, "E", result[2].Message)
	})

	t.Run("ZeroLimit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 0)
		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
		assert.Equal(t, "A", result[0].Message)
		assert.Equal(t, "E", result[4].Message)
	})
}
