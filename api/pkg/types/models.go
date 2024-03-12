package types

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

type ModelName string

const (
	Model_None              ModelName = ""
	Model_Axolotl_Mistral7b ModelName = "mistralai/Mistral-7B-Instruct-v0.1"
	Model_Cog_SDXL          ModelName = "stabilityai/stable-diffusion-xl-base-1.0"

	Model_Ollama_Mistral7b ModelName = "mistral:7b-instruct"
	Model_Ollama_Mixtral   ModelName = "mixtral:instruct"
	Model_Ollama_Gemma7b   ModelName = "gemma:7b-instruct" // 7030MiB
)

func (m ModelName) String() string {
	return string(m)
}

func (m ModelName) InferenceRuntime() InferenceRuntime {
	switch m {
	case // Axolotl/Cog
		Model_Axolotl_Mistral7b,
		Model_Cog_SDXL:
		return InferenceRuntimeAxolotl
	case // Ollama
		Model_Ollama_Mistral7b,
		Model_Ollama_Mixtral,
		Model_Ollama_Gemma7b:
		return InferenceRuntimeOllama
	// TODO: vllm
	default:
		return InferenceRuntimeAxolotl
	}
}

func ValidateModelName(modelName string, acceptEmpty bool) (ModelName, error) {
	switch ModelName(modelName) {
	case Model_Axolotl_Mistral7b:
		return Model_Axolotl_Mistral7b, nil
	case Model_Ollama_Mistral7b:
		return Model_Ollama_Mistral7b, nil
	case Model_Ollama_Mixtral:
		return Model_Ollama_Mixtral, nil
	case Model_Ollama_Gemma7b:
		return Model_Ollama_Gemma7b, nil
	case Model_Cog_SDXL:
		return Model_Cog_SDXL, nil
	default:
		if acceptEmpty && modelName == string(Model_None) {
			return Model_None, nil
		} else {
			log.Error().Msgf("invalid model name requested: %s", modelName)
			return Model_None, fmt.Errorf("invalid model name: %s", modelName)
		}
	}
}
