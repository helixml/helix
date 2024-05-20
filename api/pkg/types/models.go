package types

import (
	"fmt"
	"strings"
)

// what to do next: there's tremendous value in supporting all ollama models out of the box, so switch model names to being dynamic rather than hardcoded, so it's super easy to add new ones to the UI
// then have a generic ollama model that can be instantiated anyhow
// we need to maintain a mapping from known model names to optimal memory usage, though

type ModelName string

const (
	Model_None              ModelName = ""
	Model_Axolotl_Mistral7b ModelName = "mistralai/Mistral-7B-Instruct-v0.1"
	Model_Cog_SDXL          ModelName = "stabilityai/stable-diffusion-xl-base-1.0"

	Model_Ollama_Mistral7b ModelName = "mistral:7b-instruct"
	Model_Ollama_Mixtral   ModelName = "mixtral:instruct"
	Model_Ollama_CodeLlama ModelName = "codellama:70b-instruct-q2_K"

	Model_Ollama_NousHermes2Pro         ModelName = "adrienbrault/nous-hermes2pro:Q5_K_S"
	Model_Ollama_NousHermes2ThetaLlama3 ModelName = "adrienbrault/nous-hermes2theta-llama3-8b:q8_0"

	Model_Ollama_Llama3_8b  ModelName = "llama3:instruct"
	Model_Ollama_Llama3_70b ModelName = "llama3:70b"

	Model_Ollama_Phi3 ModelName = "phi3:instruct"
)

func NewModel(name string) ModelName {
	return ModelName(name)
}

func (m ModelName) String() string {
	return string(m)
}

func (m ModelName) InferenceRuntime() InferenceRuntime {
	// only ollama model names contain a colon.
	// TODO: add explicit API field for backend
	if strings.Contains(m.String(), ":") {
		return InferenceRuntimeOllama
	}
	// misnamed: axolotl runtime handles axolotl and cog/sd-scripts
	return InferenceRuntimeAxolotl
}

func ValidateModelName(modelName string, acceptEmpty bool) (ModelName, error) {
	// All model names are valid for now.
	return ModelName(modelName), nil
}

// this will handle aliases and defaults
func ProcessModelName(
	modelName string,
	sessionMode SessionMode,
	sessionType SessionType,
	hasFinetune bool,
) (ModelName, error) {
	switch sessionType {
	case SessionTypeText:
		if sessionType == SessionTypeText && hasFinetune {
			// fine tuning doesn't work with ollama yet
			return Model_Axolotl_Mistral7b, nil
		}

		// switch based on user toggle
		switch modelName {
		case "helix-4":
			return Model_Ollama_Llama3_70b, nil
		case "helix-3.5":
			return Model_Ollama_Llama3_8b, nil
		case "helix-mixtral":
			return Model_Ollama_Mixtral, nil
		case "helix-json":
			return Model_Ollama_NousHermes2ThetaLlama3, nil
		case "helix-small":
			return Model_Ollama_Phi3, nil
		default:
			if modelName == "" {
				// default text model for non-finetune inference
				return Model_Ollama_Llama3_8b, nil
			} else {
				// allow user-provided model name (e.g. assume API users
				// know what they're doing)
				return ValidateModelName(modelName, false)
			}
		}
	case SessionTypeImage:
		return Model_Cog_SDXL, nil
	}

	// shouldn't get here
	return "", fmt.Errorf("don't know what model to provide for args %v %v %v", sessionMode, sessionType, hasFinetune)
}
