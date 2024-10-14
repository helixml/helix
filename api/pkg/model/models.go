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
				// know what they're doing).
				return modelName, nil
			}
		}
	case types.SessionTypeImage:
		return Model_Cog_SDXL, nil
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
	return models, nil
}

const (
	Model_Axolotl_Mistral7b string = "mistralai/Mistral-7B-Instruct-v0.1"
	Model_Cog_SDXL          string = "stabilityai/stable-diffusion-xl-base-1.0"

	// We only need constants for _some_ ollama models that are hardcoded in
	// various places (backward compat). Other ones can be added dynamically now.
	Model_Ollama_Llama3_8b              string = "llama3:instruct"
	Model_Ollama_Mixtral                string = "mixtral:instruct"
	Model_Ollama_NousHermes2ThetaLlama3 string = "adrienbrault/nous-hermes2theta-llama3-8b:q8_0"
	Model_Ollama_Llama3_70b             string = "llama3:70b"
	Model_Ollama_Phi3                   string = "phi3:instruct"
)

// See also types/models.go for model name constants
func GetDefaultOllamaModels() ([]*OllamaGenericText, error) {
	models := []*OllamaGenericText{
		// Latest models, Oct 2024 updates (all with 128k context)
		{
			Id:            "llama3.1:8b-instruct-q8_0", // https://ollama.com/library/llama3.1:8b-instruct-q8_0
			Name:          "Llama 3.1 8B",
			Memory:        GB * 15,
			ContextLength: 32768, // goes up to 128k, but then uses 35GB
			Description:   "Fast and good for everyday tasks, from Meta - 8bit quantized, 32K context",
			Hide:          false,
		},
		{
			Id:            "llama3.1:70b", // https://ollama.com/library/llama3.1:70b
			Name:          "Llama 3.1 70B",
			Memory:        MB * 1,
			ContextLength: 131072,
			Description:   "Smarter but slower, from Meta - 4bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "llama3.2:1b-instruct-q8_0", // https://ollama.com/library/llama3.2:1b-instruct-q8_0
			Name:          "Llama 3.2 1B",
			Memory:        MB * 1,
			ContextLength: 131072,
			Description:   "Tiny model, from Meta - 8bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "llama3.2:3b-instruct-q8_0", // https://ollama.com/library/llama3.2:3b-instruct-q8_0
			Name:          "Llama 3.2 3B",
			Memory:        MB * 1,
			ContextLength: 131072,
			Description:   "Small model, from Meta - 8bit quantized, 128K context",
			Hide:          false,
		},
		// Old llama3:instruct, leaving in here because the id is in lots of our examples
		{
			Id:            "llama3:instruct", // https://ollama.com/library/llama3:instruct
			Name:          "Llama 3 8B",
			Memory:        MB * 1,
			ContextLength: 8192,
			Description:   "Older model, from Meta - 8bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "phi3.5:3.8b-mini-instruct-q8_0", // https://ollama.com/library/phi3.5:3.8b-mini-instruct-q8_0
			Name:          "Phi 3.5 3.8B",
			Memory:        MB * 1,
			ContextLength: 131072,
			Description:   "Fast and good for everyday tasks, from Microsoft - 8bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "gemma2:2b-instruct-q8_0", // https://ollama.com/library/gemma2:2b-instruct-q8_0
			Name:          "Gemma 2 2B",
			Memory:        MB * 1,
			ContextLength: 8192,
			Description:   "Fast and good for everyday tasks, from Google - 8bit quantized, 8K context",
			Hide:          false,
		},
		{
			Id:            "gemma2:9b-instruct-q8_0", // https://ollama.com/library/gemma2:9b-instruct-q8_0
			Name:          "Gemma 2 9B",
			Memory:        MB * 1,
			ContextLength: 8192,
			Description:   "Fast and good for everyday tasks, from Google - 8bit quantized, 8K context",
			Hide:          false,
		},
		{
			Id:            "gemma2:27b-instruct-q8_0", // https://ollama.com/library/gemma2:27b-instruct-q8_0
			Name:          "Gemma 2 27B",
			Memory:        MB * 1,
			ContextLength: 8192,
			Description:   "Large model with enhanced capabilities, from Google - 8bit quantized, 8K context",
			Hide:          false,
		},
		{
			Id:            "qwen2.5:7b-instruct-q8_0", // https://ollama.com/library/qwen2.5:7b-instruct-q8_0
			Name:          "Qwen 2.5 7B",
			Memory:        MB * 1,
			ContextLength: 32768,
			Description:   "Fast and good for everyday tasks, from Alibaba - 8bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "qwen2.5:72b", // https://ollama.com/library/qwen2.5:72b
			Name:          "Qwen 2.5 72B",
			Memory:        MB * 1,
			ContextLength: 32768,
			Description:   "Large model with enhanced capabilities, from Alibaba - 4bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "hermes3:8b-llama3.1-q8_0", // https://ollama.com/library/hermes3:8b-llama3.1-q8_0
			Name:          "Hermes 3 8B Llama 3.1",
			Memory:        MB * 1,
			ContextLength: 131072,
			Description:   "Function calling and structured output, from Nous- 8bit quantized, 128K context",
			Hide:          false,
		},
		{
			Id:            "aya:8b-23-q8_0", // https://ollama.com/library/aya:8b-23-q8_0
			Name:          "Aya 8B",
			Memory:        MB * 1,
			ContextLength: 8192,
			Description:   "Small multi-lingual model from Cohere - 8bit quantized, 8K context",
			Hide:          false,
		},
		{
			Id:            "aya:35b", // https://ollama.com/library/aya:35b
			Name:          "Aya 35B",
			Memory:        MB * 1,
			ContextLength: 8192,
			Description:   "Large multi-lingual model from Cohere - 4bit quantized, 8K context",
			Hide:          false,
		},
		// Still baked into images because of use in qapair gen
		{
			Id:            "mixtral:instruct", // https://ollama.com/library/mixtral:instruct
			Name:          "Mixtral",
			Memory:        MB * 1,
			ContextLength: 32768,
			Description:   "Medium multi-lingual model, from Mistral - 4bit quantized, 32K context",
			Hide:          false,
		},

		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************
		// OLDER MODELS, NO LONGER BAKED INTO IMAGES
		// keeping just for backward compatibility (if anyone
		// specifies them manually in their runner configuration)
		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************
		// ****************************************************************************

		// XXX TODO These memory requirements are all wrong, need to fix by
		// running the models and looking at ollama ps (via the dashboard)
		{
			Id:            "mistral:7b-instruct", // https://ollama.com/library/mistral:7b-instruct
			Name:          "Mistral 7B v0.3",
			Memory:        MB * 4199,
			ContextLength: 32768,
			Hide:          true,
		},
		{
			Id:            "codellama:70b-instruct-q2_K", // https://ollama.com/library/codellama:70b-instruct-q2_K
			Name:          "CodeLlama 70B",
			Memory:        GB * 25,
			ContextLength: 2048,
			Hide:          true,
		},

		// NousHermes2Pro
		{
			Id:            "adrienbrault/nous-hermes2pro:Q5_K_S", // https://ollama.com/adrienbrault/nous-hermes2pro:Q5_K_S
			Name:          "Nous-Hermes 2 Pro",
			Memory:        GB * 5,
			ContextLength: 32768,
			Hide:          true,
		},
		{
			Id:            "adrienbrault/nous-hermes2theta-llama3-8b:q8_0", // https://ollama.com/adrienbrault/nous-hermes2theta-llama3-8b:q8_0
			Name:          "Nous-Hermes 2 Theta",
			Memory:        MB * 8107,
			ContextLength: 8192,
			Hide:          true,
		},

		{
			Id:            "llama3:70b", // https://ollama.com/library/llama3:70b
			Name:          "Llama 3 70B",
			Memory:        GB * 40,
			ContextLength: 8192,
			Description:   "Large model with enhanced capabilities",
			Hide:          true,
		},
		{
			Id:            "llama3:8b-instruct-fp16", // https://ollama.com/library/llama3:8b-instruct-fp16
			Name:          "Llama 3 8B FP16",
			Memory:        GB * 16,
			ContextLength: 8192,
			Description:   "Fast and good for everyday tasks",
			Hide:          true,
		},
		{
			Id:            "llama3:8b-instruct-q6_K", // https://ollama.com/library/llama3:8b-instruct-q6_K
			Name:          "Llama 3 8B Q6_K",
			Memory:        MB * 6295,
			ContextLength: 8192,
			Description:   "Fast and good for everyday tasks",
			Hide:          true,
		},
		{
			Id:            "llama3:8b-instruct-q8_0", // https://ollama.com/library/llama3:8b-instruct-q8_0
			Name:          "Llama 3 8B Q8_0",
			Memory:        MB * 8107,
			ContextLength: 4096,
			Description:   "Large model with enhanced capabilities",
			Hide:          true,
		},
		{
			Id:            "phi3:instruct", // https://ollama.com/library/phi3:instruct
			Name:          "Phi-3",
			Memory:        MB * 2300,
			ContextLength: 131072,
			Description:   "Fast and good for everyday tasks",
			Hide:          true,
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
