package model

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

func GetModel(modelName types.ModelName) (Model, error) {
	models, err := GetModels()
	if err != nil {
		return nil, err
	}
	model, ok := models[modelName]
	if !ok {
		return nil, fmt.Errorf("no model for model name %s", modelName)
	}
	return model, nil
}

// rather then keep processing model names from sessions into instances of the model struct
// (just so we can ask it GetMemoryRequirements())
// this gives us an in memory cache of model instances we can quickly lookup from
func GetModels() (map[types.ModelName]Model, error) {
	models := map[types.ModelName]Model{}
	models[types.Model_Axolotl_Mistral7b] = &Mistral7bInstruct01{}
	models[types.Model_Cog_SDXL] = &CogSDXL{}

	// Ollama
	models[types.Model_Ollama_Mistral7b] = NewOllamaGenericText(types.Model_Ollama_Mistral7b.String(), MB*6440)
	models[types.Model_Ollama_Mixtral] = NewOllamaGenericText(types.Model_Ollama_Mistral7b.String(), GB*24)
	models[types.Model_Ollama_DeepseekCoder] = NewOllamaGenericText(types.Model_Ollama_DeepseekCoder.String(), GB*24)
	models[types.Model_Ollama_NousHermes2Pro] = NewOllamaGenericText(types.Model_Ollama_NousHermes2Pro.String(), MB*6440)
	models[types.Model_Ollama_Qwen72b] = NewOllamaGenericText(types.Model_Ollama_Qwen72b.String(), GB*24)

	return models, nil
}

func GetLowestMemoryRequirement() (uint64, error) {
	models, err := GetModels()
	if err != nil {
		return 0, err
	}
	lowestMemoryRequirement := uint64(0)
	for _, model := range models {
		finetune := model.GetMemoryRequirements(types.SessionModeFinetune)
		if finetune > 0 && (lowestMemoryRequirement == 0 || finetune < lowestMemoryRequirement) {
			lowestMemoryRequirement = finetune
		}
		inference := model.GetMemoryRequirements(types.SessionModeInference)
		if inference > 0 && (lowestMemoryRequirement == 0 || inference < lowestMemoryRequirement) {
			lowestMemoryRequirement = inference
		}
	}
	return lowestMemoryRequirement, err
}
