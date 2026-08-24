package model

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupModelModalitiesQwen38Vision(t *testing.T) {
	for _, modelID := range []string{
		"qwen3.8-27b",
		"qwen3.8-27b-instruct",
		"provider/qwen3.8-27b",
	} {
		t.Run(modelID, func(t *testing.T) {
			profile, ok := LookupModelModalities(modelID)
			require.True(t, ok)
			assert.Equal(t, []types.Modality{types.ModalityText, types.ModalityImage}, profile.Input)
			assert.Equal(t, []types.Modality{types.ModalityText}, profile.Output)
			assert.Equal(t, "probed", profile.Source)
			assert.Equal(t, "2026-08-24", profile.VerifiedAt)
		})
	}
}

func TestLookupModelModalitiesUnknownModel(t *testing.T) {
	_, ok := LookupModelModalities("unknown-model")
	assert.False(t, ok)
}
