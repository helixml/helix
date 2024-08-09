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
	modelName, err = types.TransformModelName(modelName.String())
	if err != nil {
		return nil, err
	}
	model, ok := models[modelName]
	if !ok {
		modelNames := []string{}
		for modelName := range models {
			modelNames = append(modelNames, modelName.String())
		}
		return nil, fmt.Errorf("no model for model name %s (available models: %v)", modelName, modelNames)
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
	models[types.Model_Ollama_Mistral7b_v3] = NewOllamaGenericText(types.Model_Ollama_Mistral7b_v3.String(), MB*6440) // https://ollama.com/library/mistral:v0.3
	models[types.Model_Ollama_Mixtral] = NewOllamaGenericText(types.Model_Ollama_Mistral7b.String(), GB*24)
	models[types.Model_Ollama_CodeLlama] = NewOllamaGenericText(types.Model_Ollama_CodeLlama.String(), GB*24)

	// NousHermes2Pro
	models[types.Model_Ollama_NousHermes2Pro] = NewOllamaGenericText(types.Model_Ollama_NousHermes2Pro.String(), MB*6440)
	models[types.Model_Ollama_NousHermes2ThetaLlama3] = NewOllamaGenericText(types.Model_Ollama_NousHermes2ThetaLlama3.String(), MB*8792)

	// Llama3
	models[types.Model_Ollama_Llama3_8b] = NewOllamaGenericText(types.Model_Ollama_Llama3_8b.String(), MB*5349)
	models[types.Model_Ollama_Llama3_70b] = NewOllamaGenericText(types.Model_Ollama_Llama3_70b.String(), GB*39)

	models[types.Model_Ollama_Llama3_8b_fp16] = NewOllamaGenericText(types.Model_Ollama_Llama3_8b_fp16.String(), GB*16)
	models[types.Model_Ollama_Llama3_8b_q6_K] = NewOllamaGenericText(types.Model_Ollama_Llama3_8b_q6_K.String(), MB*6295)
	models[types.Model_Ollama_Llama3_8b_q8_0] = NewOllamaGenericText(types.Model_Ollama_Llama3_8b_q8_0.String(), MB*8107)

	models[types.Model_Ollama_Phi3] = NewOllamaGenericText(types.Model_Ollama_Phi3.String(), MB*2300)

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
