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
	for modelName, model := range ollamaModels {
		models[modelName] = model
	}
	return models, nil
}

const (
	Model_None              string = ""
	Model_Axolotl_Mistral7b string = "mistralai/Mistral-7B-Instruct-v0.1"
	Model_Cog_SDXL          string = "stabilityai/stable-diffusion-xl-base-1.0"

	// Oct 2024 updates
	Model_Ollama_Llama31_8b_q8_0     string = "llama3.1:8b-instruct-q8_0"
	Model_Ollama_Llama31_70b         string = "llama3.1:70b"
	Model_Ollama_Llama32_1b_q8_0     string = "llama3.2:1b-instruct-q8_0"
	Model_Ollama_Llama32_3b_q8_0     string = "llama3.2:3b-instruct-q8_0"
	Model_Ollama_Llama3_8b           string = "llama3:instruct"
	Model_Ollama_Phi35_38b_mini_q8_0 string = "phi3.5:3.8b-mini-instruct-q8_0"
	Model_Ollama_Gemma2_2b_q8_0      string = "gemma2:2b-instruct-q8_0"
	Model_Ollama_Gemma2_9b_q8_0      string = "gemma2:9b-instruct-q8_0"
	Model_Ollama_Gemma2_27b_q8_0     string = "gemma2:27b-instruct-q8_0"
	Model_Ollama_Qwen25_7b_q8_0      string = "qwen2.5:7b-instruct-q8_0"
	Model_Ollama_Qwen25_72b          string = "qwen2.5:72b"
	Model_Ollama_Hermes3_8b_Llama31  string = "hermes3:8b-llama3.1-q8_0"
	Model_Ollama_Aya_8b_q8_0         string = "aya:8b-23-q8_0"
	Model_Ollama_Aya_35b             string = "aya:35b"

	// Older models
	Model_Ollama_Mistral7b    string = "mistral:7b-instruct"
	Model_Ollama_Mistral7b_v3 string = "mistral:v0.3"
	Model_Ollama_Mixtral      string = "mixtral:instruct"
	Model_Ollama_CodeLlama    string = "codellama:70b-instruct-q2_K"

	Model_Ollama_NousHermes2Pro         string = "adrienbrault/nous-hermes2pro:Q5_K_S"
	Model_Ollama_NousHermes2ThetaLlama3 string = "adrienbrault/nous-hermes2theta-llama3-8b:q8_0"

	Model_Ollama_Llama3_70b string = "llama3:70b"

	Model_Ollama_Llama3_8b_fp16 string = "llama3:8b-instruct-fp16"
	Model_Ollama_Llama3_8b_q6_K string = "llama3:8b-instruct-q6_K"
	Model_Ollama_Llama3_8b_q8_0 string = "llama3:8b-instruct-q8_0"

	Model_Ollama_Phi3 string = "phi3:instruct"
)

// See also types/models.go for model name constants
func GetDefaultOllamaModels() (map[string]Model, error) {
	models := map[string]Model{}

	// Latest models, Oct 2024 updates (all with 128k context)
	models[Model_Ollama_Llama31_8b_q8_0] = &OllamaGenericText{
		id:            "llama3.1:8b-instruct-q8_0", // https://ollama.com/library/llama3.1:8b-instruct-q8_0
		name:          "Llama 3.1 8B Q8_0",
		memory:        MB * 8107, // 8.5GiB in MiB
		contextLength: 131072,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Llama31_70b] = &OllamaGenericText{
		id:            "llama3.1:70b", // https://ollama.com/library/llama3.1:70b
		name:          "Llama 3.1 70B",
		memory:        GB * 39,
		contextLength: 131072,
		description:   "Large model with enhanced capabilities",
	}
	models[Model_Ollama_Llama32_1b_q8_0] = &OllamaGenericText{
		id:            "llama3.2:1b-instruct-q8_0", // https://ollama.com/library/llama3.2:1b-instruct-q8_0
		name:          "Llama 3.2 1B Q8_0",
		memory:        MB * 1240,
		contextLength: 131072,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Llama32_3b_q8_0] = &OllamaGenericText{
		id:            "llama3.2:3b-instruct-q8_0", // https://ollama.com/library/llama3.2:3b-instruct-q8_0
		name:          "Llama 3.2 3B Q8_0",
		memory:        MB * 2048,
		contextLength: 131072,
		description:   "Fast and good for everyday tasks",
	}
	// Old llama3:instruct, leaving in here because the id is in lots of our examples
	models[Model_Ollama_Llama3_8b] = &OllamaGenericText{
		id:            "llama3:instruct", // https://ollama.com/library/llama3:instruct
		name:          "Llama 3 8B",
		memory:        MB * 4483,
		contextLength: 8192,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Phi35_38b_mini_q8_0] = &OllamaGenericText{
		id:            "phi3.5:3.8b-mini-instruct-q8_0", // https://ollama.com/library/phi3.5:3.8b-mini-instruct-q8_0
		name:          "Phi-3.5 3.8B Mini Q8_0",
		memory:        MB * 2098,
		contextLength: 131072,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Gemma2_2b_q8_0] = &OllamaGenericText{
		id:            "gemma2:2b-instruct-q8_0", // https://ollama.com/library/gemma2:2b-instruct-q8_0
		name:          "Gemma 2 2B Q8_0",
		memory:        MB * 1526,
		contextLength: 8192,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Gemma2_9b_q8_0] = &OllamaGenericText{
		id:            "gemma2:9b-instruct-q8_0", // https://ollama.com/library/gemma2:9b-instruct-q8_0
		name:          "Gemma 2 9B Q8_0",
		memory:        MB * 9216,
		contextLength: 8192,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Gemma2_27b_q8_0] = &OllamaGenericText{
		id:            "gemma2:27b-instruct-q8_0", // https://ollama.com/library/gemma2:27b-instruct-q8_0
		name:          "Gemma 2 27B Q8_0",
		memory:        GB * 27,
		contextLength: 131072,
		description:   "Large model with enhanced capabilities",
	}
	models[Model_Ollama_Qwen25_7b_q8_0] = &OllamaGenericText{
		id:            "qwen2.5:7b-instruct-q8_0", // https://ollama.com/library/qwen2.5:7b-instruct-q8_0
		name:          "Qwen 2.5 7B Q8_0",
		memory:        MB * 7168,
		contextLength: 131072,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Qwen25_72b] = &OllamaGenericText{
		id:            "qwen2.5:72b", // https://ollama.com/library/qwen2.5:72b
		name:          "Qwen 2.5 72B",
		memory:        GB * 40,
		contextLength: 131072,
		description:   "Large model with enhanced capabilities",
	}
	models[Model_Ollama_Hermes3_8b_Llama31] = &OllamaGenericText{
		id:            "hermes3:8b-llama3.1-q8_0", // https://ollama.com/library/hermes3:8b-llama3.1-q8_0
		name:          "Hermes 3 8B Llama 3.1 Q8_0",
		memory:        MB * 8192,
		contextLength: 131072,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Aya_8b_q8_0] = &OllamaGenericText{
		id:            "aya:8b-23-q8_0", // https://ollama.com/library/aya:8b-23-q8_0
		name:          "Aya 8B Q8_0",
		memory:        MB * 8192,
		contextLength: 131072,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Aya_35b] = &OllamaGenericText{
		id:            "aya:35b", // https://ollama.com/library/aya:35b
		name:          "Aya 35B",
		memory:        GB * 35,
		contextLength: 131072,
		description:   "Large model with enhanced capabilities",
	}
	// Still baked into images because of use in qapair gen
	models[Model_Ollama_Mixtral] = &OllamaGenericText{
		id:            "mixtral:instruct", // https://ollama.com/library/mixtral:instruct
		name:          "Mixtral",
		memory:        GB * 24,
		contextLength: 32768,
	}

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
	models[Model_Ollama_Mistral7b] = &OllamaGenericText{
		id:            "mistral:7b-instruct", // https://ollama.com/library/mistral:7b-instruct
		name:          "Mistral 7B v0.3",
		memory:        MB * 6440,
		contextLength: 32768,
	}
	models[Model_Ollama_Mistral7b_v3] = &OllamaGenericText{
		id:            "mistral:v0.3", // https://ollama.com/library/mistral:v0.3
		name:          "Mistral 7B v0.3",
		memory:        MB * 6440,
		contextLength: 32768,
	}
	models[Model_Ollama_CodeLlama] = &OllamaGenericText{
		id:            "codellama:70b-instruct-q2_K", // https://ollama.com/library/codellama:70b-instruct-q2_K
		name:          "CodeLlama 70B",
		memory:        GB * 24,
		contextLength: 16384,
	}

	// NousHermes2Pro
	models[Model_Ollama_NousHermes2Pro] = &OllamaGenericText{
		id:            "adrienbrault/nous-hermes2pro:Q5_K_S", // https://ollama.com/adrienbrault/nous-hermes2pro:Q5_K_S
		name:          "Nous-Hermes 2 Pro",
		memory:        MB * 6440,
		contextLength: 8192,
	}
	models[Model_Ollama_NousHermes2ThetaLlama3] = &OllamaGenericText{
		id:            "adrienbrault/nous-hermes2theta-llama3-8b:q8_0", // https://ollama.com/adrienbrault/nous-hermes2theta-llama3-8b
		name:          "Nous-Hermes 2 Theta",
		memory:        MB * 8792,
		contextLength: 8192,
	}

	models[Model_Ollama_Llama3_70b] = &OllamaGenericText{
		id:            "llama3:70b", // https://ollama.com/library/llama3:70b
		name:          "Llama 3 70B",
		memory:        GB * 39,
		contextLength: 4096,
		description:   "Large model with enhanced capabilities",
	}
	models[Model_Ollama_Llama3_8b_fp16] = &OllamaGenericText{
		id:            "llama3:8b-instruct-fp16", // https://ollama.com/library/llama3:8b-instruct-fp16
		name:          "Llama 3 8B FP16",
		memory:        GB * 16,
		contextLength: 4096,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Llama3_8b_q6_K] = &OllamaGenericText{
		id:            "llama3:8b-instruct-q6_K", // https://ollama.com/library/llama3:8b-instruct-q6_K
		name:          "Llama 3 8B Q6_K",
		memory:        MB * 6295,
		contextLength: 4096,
		description:   "Fast and good for everyday tasks",
	}
	models[Model_Ollama_Llama3_8b_q8_0] = &OllamaGenericText{
		id:            "llama3:8b-instruct-q8_0", // https://ollama.com/library/llama3:8b-instruct-q8_0
		name:          "Llama 3 8B Q8_0",
		memory:        MB * 8107,
		contextLength: 4096,
		description:   "Large model with enhanced capabilities",
	}
	models[Model_Ollama_Phi3] = &OllamaGenericText{
		id:            "phi3:instruct", // https://ollama.com/library/phi3:instruct
		name:          "Phi-3",
		memory:        MB * 2300,
		contextLength: 2048,
		description:   "Fast and good for everyday tasks",
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
