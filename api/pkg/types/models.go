package types

import (
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

	Model_Ollama_Mistral7b      ModelName = "mistral:7b-instruct"
	Model_Ollama_Mixtral        ModelName = "mixtral:instruct"
	Model_Ollama_CodeLlama      ModelName = "codellama:70b-instruct"
	Model_Ollama_NousHermes2Pro ModelName = "adrienbrault/nous-hermes2pro:Q5_K_S"
	Model_Ollama_Qwen72b        ModelName = "qwen:72b-chat"
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
