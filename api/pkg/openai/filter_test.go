package openai

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedModel(t *testing.T) {
	assert.True(t, unsupportedModel("gpt-4o-transcribe"))
	assert.False(t, unsupportedModel("gpt-4o"))
}

func TestFilterUnsupportedModels(t *testing.T) {
	models := []types.OpenAIModel{
		{ID: "gpt-4o-transcribe"},
		{ID: "gpt-4o"},
	}

	filteredModels := filterUnsupportedModels(models)
	assert.Equal(t, 1, len(filteredModels))
	assert.Equal(t, "gpt-4o", filteredModels[0].ID)
}
