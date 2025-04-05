package openai

import (
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

var (
	unsupportedModels = []string{
		"transcribe",
		"audio",
		"tts",
		"realtime",
	}
)

func unsupportedModel(model string) bool {
	for _, unsupportedModel := range unsupportedModels {
		if strings.Contains(strings.ToLower(model), strings.ToLower(unsupportedModel)) {
			return true
		}
	}

	return false
}

func filterUnsupportedModels(models []types.OpenAIModel) []types.OpenAIModel {
	filteredModels := make([]types.OpenAIModel, 0)
	for _, model := range models {
		if unsupportedModel(model.ID) {
			continue
		}

		filteredModels = append(filteredModels, model)
	}

	return filteredModels
}
