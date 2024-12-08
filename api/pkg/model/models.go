package model

import (
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

func GetModel(modelName string) (Model, error) {
	models, err := GetModels()
	if err != nil {
		return nil, err
	}
	modelName, err = TransformModelName(modelName)
	if err != nil {
		return nil, err
	}
	model, ok := models[modelName]
	if !ok {
		modelNames := []string{}
		for modelName := range models {
			modelNames = append(modelNames, modelName)
		}
		return nil, fmt.Errorf("no model for model name %s (available models: %v)", modelName, modelNames)
	}
	return model, nil
}

// what to do next: there's tremendous value in supporting all ollama models out of the box, so switch model names to being dynamic rather than hardcoded, so it's super easy to add new ones to the UI
// then have a generic ollama model that can be instantiated anyhow
// we need to maintain a mapping from known model names to optimal memory usage, though

// TODO: having a separate ModelName type just so we can put the
// InferenceRuntime() method on it is silly, we should move the InferenceRuntime
// logic onto the Model struct maybe
type ModelName string

func NewModel(name string) ModelName {
	return ModelName(name)
}

func (m ModelName) String() string {
	return string(m)
}

func (m ModelName) InferenceRuntime() types.InferenceRuntime {
	// only ollama model names contain a colon.
	// TODO: add explicit API field for backend
	if strings.Contains(m.String(), ":") {
		return types.InferenceRuntimeOllama
	}
	if m.String() == Model_Cog_SDXL {
		return types.InferenceRuntimeCog
	}
	diffusersModels, err := GetDefaultDiffusersModels()
	if err != nil {
		return types.InferenceRuntimeAxolotl
	}
	for _, model := range diffusersModels {
		if m.String() == model.Id {
			return types.InferenceRuntimeDiffusers
		}
	}

	// misnamed: axolotl runtime handles axolotl and cog/sd-scripts
	return types.InferenceRuntimeAxolotl
}

func TransformModelName(modelName string) (string, error) {
	// All other model names are valid for now.
	return modelName, nil
}

// this will handle aliases and defaults
func ProcessModelName(
	provider string,
	modelName string,
	sessionMode types.SessionMode,
	sessionType types.SessionType,
	hasFinetune bool,
	ragEnabled bool,
) (string, error) {
	switch sessionType {
	case types.SessionTypeText:
		if sessionType == types.SessionTypeText && !ragEnabled && (sessionMode == types.SessionModeFinetune || hasFinetune) {
			// fine tuning doesn't work with ollama yet
			return Model_Axolotl_Mistral7b, nil
		}

		switch provider {
		case "helix":
			// Check and validate
		default:
			// Any other provider just return directly as we don't
			// care about the model specifics (memory, etc)
			return modelName, nil
		}

		// switch based on user toggle
		// TODO: we plan to retire the helix-* model names, but we are keeping for now for backwards compatibility
		switch modelName {
		case "helix-4":
		case "helix-3.5":
		case "helix-mixtral":
		case "helix-json":
		case "helix-small":
			return Model_Ollama_Llama31_8b, nil
		default:
			if modelName == "" {
				// default text model for non-finetune inference
				return Model_Ollama_Llama31_8b, nil

			} else {
				// allow user-provided model name (e.g. assume API users
				// know what they're doing).
				return modelName, nil
			}
		}
	case types.SessionTypeImage:
		if modelName == "" {
			// default image model for image inference
			return Model_Diffusers_SDTurbo, nil
		}
		// allow user-provided model name (e.g. assume API users
		// know what they're doing).
		return modelName, nil
	}

	// shouldn't get here
	return "", fmt.Errorf("don't know what model to provide for args %v %v %v", sessionMode, sessionType, hasFinetune)
}

// rather then keep processing model names from sessions into instances of the model struct
// (just so we can ask it GetMemoryRequirements())
// this gives us an in memory cache of model instances we can quickly lookup from
func GetModels() (map[string]Model, error) {
	models := map[string]Model{}
	models[Model_Axolotl_Mistral7b] = &Mistral7bInstruct01{}
	models[Model_Cog_SDXL] = &CogSDXL{}
	ollamaModels, err := GetDefaultOllamaModels()
	if err != nil {
		return nil, err
	}
	for _, model := range ollamaModels {
		models[model.Id] = model
	}
	diffusersModels, err := GetDefaultDiffusersModels()
	if err != nil {
		return nil, err
	}
	for _, model := range diffusersModels {
		models[model.Id] = model
	}
	return models, nil
}

const (
	Model_Axolotl_Mistral7b string = "mistralai/Mistral-7B-Instruct-v0.1"
	Model_Cog_SDXL          string = "stabilityai/stable-diffusion-xl-base-1.0"
	Model_Diffusers_SD35    string = "stabilityai/stable-diffusion-3.5-medium"
	Model_Diffusers_SDTurbo string = "stabilityai/sd-turbo"
	Model_Diffusers_FluxDev string = "black-forest-labs/FLUX.1-dev"

	// We only need constants for _some_ ollama models that are hardcoded in
	// various places (backward compat). Other ones can be added dynamically now.
	Model_Ollama_Llama31_8b string = "llama3.1:8b-instruct-q8_0"
)

func GetDefaultDiffusersModels() ([]*DiffusersGenericImage, error) {
	return []*DiffusersGenericImage{
		{
			Id:          Model_Diffusers_FluxDev,
			Name:        "FLUX.1-dev",
			Memory:      GB * 39,
			Description: "High quality image model, from Black Forest Labs",
			Hide:        false,
		},
	}, nil
}

// See also types/models.go for model name constants
func GetDefaultOllamaModels() ([]*OllamaGenericText, error) {
	models := []*OllamaGenericText{
		// Latest models, Dec 2024 updates
		{
			Id:            "llama3.1:8b-instruct-q8_0", // https://ollama.com/library/llama3.1:8b-instruct-q8_0
			Name:          "Llama 3.1 8B",
			Memory:        GB * 15,
			ContextLength: 32768, // goes up to 128k, but then uses 35GB
			Description:   "Fast and good for everyday tasks, from Meta - 8bit quantized, 32K context",
			Hide:          false,
		},
		{
			Id:            "llama3.3:70b-instruct-q4_K_M", // https://ollama.com/library/llama3.1:70b-instruct-q4_K_M
			Name:          "Llama 3.3 70B",
			Memory:        GB * 48,
			ContextLength: 16384,
			Description:   "Smarter but slower, from Meta - 4bit quantized, 16K context",
			Hide:          false,
		},
		{
			Id:            "llama3.2:1b-instruct-q8_0", // https://ollama.com/library/llama3.2:1b-instruct-q8_0
			Name:          "Llama 3.2 1B",
			Memory:        GB * 15,
			ContextLength: 131072,
			Description:   "Tiny model, from Meta - 8bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "llama3.2:3b-instruct-q8_0", // https://ollama.com/library/llama3.2:3b-instruct-q8_0
			Name:          "Llama 3.2 3B",
			Memory:        GB * 26,
			ContextLength: 131072,
			Description:   "Small model, from Meta - 8bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "phi3.5:3.8b-mini-instruct-q8_0", // https://ollama.com/library/phi3.5:3.8b-mini-instruct-q8_0
			Name:          "Phi 3.5 3.8B",
			Memory:        GB * 35,
			ContextLength: 65536,
			Description:   "Fast and good for everyday tasks, from Microsoft - 8bit quantized, 64K context",
			Hide:          false,
		},
		{
			Id:            "qwen2.5:7b-instruct-q8_0", // https://ollama.com/library/qwen2.5:7b-instruct-q8_0
			Name:          "Qwen 2.5 7B",
			Memory:        GB * 12,
			ContextLength: 32768,
			Description:   "Fast and good for everyday tasks, from Alibaba - 8bit quantized, 32K context",
			Hide:          false,
		},
		{
			Id:            "aya:8b-23-q8_0", // https://ollama.com/library/aya:8b-23-q8_0
			Name:          "Aya 8B",
			Memory:        GB * 11,
			ContextLength: 8192,
			Description:   "Small multi-lingual model from Cohere - 8bit quantized, 8K context",
			Hide:          false,
		},
		{
			Id:            "aya:35b-23-q4_0", // https://ollama.com/library/aya:35b-23-q4_0
			Name:          "Aya 35B",
			Memory:        GB * 32,
			ContextLength: 8192,
			Description:   "Large multi-lingual model from Cohere - 4bit quantized, 8K context",
			Hide:          false,
		},
	}

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
