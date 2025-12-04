package model

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// what to do next: there's tremendous value in supporting all ollama models out of the box, so switch model names to being dynamic rather than hardcoded, so it's super easy to add new ones to the UI
// then have a generic ollama model that can be instantiated anyhow
// we need to maintain a mapping from known model names to optimal memory usage, though

// TODO: having a separate Name type just so we can put the
// InferenceRuntime() method on it is silly, we should move the InferenceRuntime
// logic onto the Model struct maybe
type Name string

func NewModel(name string) Name {
	return Name(name)
}

func (m Name) String() string {
	return string(m)
}

func (m Name) InferenceRuntime() types.InferenceRuntime {
	// Check for VLLM models first
	vllmModels, err := GetDefaultVLLMModels()
	if err == nil {
		for _, model := range vllmModels {
			if m.String() == model.ID {
				return types.InferenceRuntimeVLLM
			}
		}
	}

	// Only ollama model names contain a colon.
	// TODO: add explicit API field for backend
	if strings.Contains(m.String(), ":") {
		return types.InferenceRuntimeOllama
	}

	// if m.String() == ModelCogSdxl {
	// 	return types.InferenceRuntimeCog
	// }

	diffusersModels, err := GetDefaultDiffusersModels()
	if err != nil {
		return types.InferenceRuntimeAxolotl
	}
	for _, model := range diffusersModels {
		if m.String() == model.ID {
			return types.InferenceRuntimeDiffusers
		}
	}

	// misnamed: axolotl runtime handles axolotl and cog/sd-scripts
	return types.InferenceRuntimeAxolotl
}

// ParseProviderFromModel extracts a provider prefix from a model name.
// Format: "provider/model" returns (provider, model)
// If no prefix, returns ("", modelName)
func ParseProviderFromModel(modelName string) (provider, model string) {
	if idx := strings.Index(modelName, "/"); idx > 0 {
		return modelName[:idx], modelName[idx+1:]
	}
	return "", modelName
}

// this will handle aliases and defaults
func ProcessModelName(
	provider string,
	modelName string,
	sessionType types.SessionType,
) (string, error) {
	switch sessionType {
	case types.SessionTypeText:
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
			return ModelOllamaLlama370b, nil
		case "helix-3.5":
			return ModelOllamaLlama38b, nil
		case "helix-mixtral":
			return ModelOllamaMixtral, nil
		case "helix-json":
			return ModelOllamaNoushermes2thetallama3, nil
		case "helix-small":
			return ModelOllamaPhi3, nil
		case "external_agent":
			// External agent requests should use this identifier
			// The actual model is configured within the external agent (Zed, etc.)
			return "external_agent", nil
		default:
			if modelName == "" {
				// default text model for non-finetune inference
				return ModelOllamaLlama38b, nil
			}
			// allow user-provided model name (e.g. assume API users
			// know what they're doing).
			return modelName, nil
		}
	case types.SessionTypeImage:
		if modelName == "" {
			// default image model for image inference
			return ModelDiffusersSdturbo, nil
		}
		// allow user-provided model name (e.g. assume API users
		// know what they're doing).
		return modelName, nil
	}

	// shouldn't get here
	return "", fmt.Errorf("don't know what model to provide for args %v %v", modelName, sessionType)
}

// rather then keep processing model names from sessions into instances of the model struct
// (just so we can ask it GetMemoryRequirements())
// this gives us an in memory cache of model instances we can quickly lookup from
func GetModels() (map[string]Model, error) {
	models := map[string]Model{}
	// models[ModelAxolotlMistral7b] = &Mistral7bInstruct01{}
	// models[ModelCogSdxl] = &CogSDXL{}
	ollamaModels, err := GetDefaultOllamaModels()
	if err != nil {
		return nil, err
	}
	for _, model := range ollamaModels {
		models[model.ID] = model
	}
	diffusersModels, err := GetDefaultDiffusersModels()
	if err != nil {
		return nil, err
	}
	for _, model := range diffusersModels {
		models[model.ID] = model
	}
	vllmModels, err := GetDefaultVLLMModels()
	if err != nil {
		return nil, err
	}
	for _, model := range vllmModels {
		models[model.ID] = model
	}
	return models, nil
}

const (
	ModelDiffusersSdturbo string = "stabilityai/sd-turbo"
	ModelDiffusersFluxdev string = "black-forest-labs/FLUX.1-dev"

	// We only need constants for _some_ ollama models that are hardcoded in
	// various places (backward compat). Other ones can be added dynamically now.
	ModelOllamaLlama38b               string = "llama3:instruct"
	ModelOllamaMixtral                string = "mixtral:instruct"
	ModelOllamaNoushermes2thetallama3 string = "adrienbrault/nous-hermes2theta-llama3-8b:q8_0"
	ModelOllamaLlama370b              string = "llama3:70b"
	ModelOllamaPhi3                   string = "phi3:instruct"
)

func GetDefaultDiffusersModels() ([]*DiffusersGenericImage, error) {
	return []*DiffusersGenericImage{
		// SD Turbo is useful for development because it's low RAM, but it doesn't produce very good images
		// {
		// 	ID:          ModelDiffusersSdturbo,
		// 	Name:        "SD Turbo",
		// 	Memory:      GB * 10,
		// 	Description: "High quality image model, from Stability AI",
		// 	Hide:        false,
		// },

		// Stopped downloading for some reason
		// {
		// 	ID:          ModelDiffusersFluxdev,
		// 	Name:        "FLUX.1-dev",
		// 	Memory:      GB * 39,
		// 	Description: "High quality image model, from Black Forest Labs",
		// 	Hide:        false,
		// },
	}, nil
}

// See also types/models.go for model name constants
func GetDefaultOllamaModels() ([]*OllamaGenericText, error) {
	models := []*OllamaGenericText{
		// Latest models, July 2025 updates
		{
			ID:            "gpt-oss:20b", // https://ollama.com/library/gpt-oss:20b
			Name:          "GPT-OSS 20B",
			Memory:        0,      // Ollama models use GGUF estimation, not database values
			ContextLength: 131072, // Assuming 128K context window (standard for recent models)
			Description:   "Open-weight reasoning model with agentic capabilities, from OpenAI - optimized for consumer hardware, ~20B parameters",
			Hide:          false,
			Prewarm:       true,
			Concurrency:   1, // too big for concurrency 2
		},
		{
			ID:            "gpt-oss:120b", // https://ollama.com/library/gpt-oss:120b
			Name:          "GPT-OSS 120B",
			Memory:        0,      // Ollama models use GGUF estimation, not database values
			ContextLength: 131072, // Assuming 128K context window (standard for recent models)
			Description:   "Large open-weight reasoning model with advanced capabilities, from OpenAI - production-grade, ~120B parameters",
			Hide:          false,
			Prewarm:       false, // Don't prewarm due to high memory requirements
			Concurrency:   1,     // too big for concurrency 2
		},
		{
			ID:            "qwen3:8b", // https://ollama.com/library/qwen3:8b
			Name:          "Qwen3 8B",
			Memory:        0,     // Ollama models use GGUF estimation, not database values
			ContextLength: 40960, // 40K context window
			Concurrency:   0,     // Use runtime default (DefaultOllamaParallelSequences)
			Description:   "Latest generation Qwen model with enhanced reasoning, from Alibaba - 4bit quantized, 40K context",
			Hide:          false,
			Prewarm:       true,
		},
		{
			ID:            "qwen2.5vl:32b", // https://ollama.com/library/qwen2.5vl:32b
			Name:          "Qwen2.5-VL 32B",
			Memory:        0,      // Ollama models use GGUF estimation, not database values
			ContextLength: 131072, // 125K context window
			Description:   "Flagship vision-language model with document parsing and visual agent capabilities, from Alibaba - 4bit quantized, 125K context",
			Hide:          false,
		},
		{
			ID:            "qwen3:32b", // https://ollama.com/library/qwen3:32b
			Name:          "Qwen3 32B",
			Memory:        0,     // Ollama models use GGUF estimation, not database values
			ContextLength: 40960, // 40K context window
			Description:   "Large Qwen3 model with superior reasoning and agent capabilities, from Alibaba - 4bit quantized, 40K context",
			Hide:          false,
		},
		{
			ID:            "qwen3:30b", // https://ollama.com/library/qwen3:30b
			Name:          "Qwen3 30B MoE",
			Memory:        0,      // Ollama models use GGUF estimation, not database values
			ContextLength: 262144, // 256K context window
			Description:   "Mixture-of-experts model with enhanced reasoning capabilities, from Alibaba - 4bit quantized, 256K context",
			Hide:          false,
		},
		// Old llama3:instruct and ph3:instruct, leaving in here because the id
		// is in lots of our examples and tests
		{
			ID:            "llama3:instruct", // https://ollama.com/library/llama3:instruct
			Name:          "Llama 3 8B",
			Memory:        0, // Ollama models use GGUF estimation, not database values
			ContextLength: 8192,
			Description:   "Older model, from Meta - 4bit quantized, 8K context",
			Hide:          true,
		},
		{
			ID:            "phi3:instruct", // https://ollama.com/library/phi3:instruct
			Name:          "Phi-3",
			Memory:        0, // Ollama models use GGUF estimation, not database values
			ContextLength: 131072,
			Description:   "Fast and good for everyday tasks",
			Hide:          true,
		},
		{
			ID:            "llama3:70b", // https://ollama.com/library/llama3:70b
			Name:          "Llama 3 70B",
			Memory:        0, // Ollama models use GGUF estimation, not database values
			ContextLength: 8192,
			Description:   "Large model with enhanced capabilities",
			Hide:          true,
		},
		{
			ID:            "gemma2:2b-instruct-q8_0", // https://ollama.com/library/gemma2:2b-instruct-q8_0
			Name:          "Gemma 2 2B",
			Memory:        0, // Ollama models use GGUF estimation, not database values
			ContextLength: 8192,
			Description:   "Fast and good for everyday tasks, from Google - 8bit quantized, 8K context",
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

// First, add VLLMGenericText struct with all required interface methods
type VLLMGenericText struct {
	ID            string
	Name          string
	Memory        uint64
	ContextLength int64
	Description   string
	Args          []string
	Hide          bool
	Prewarm       bool
}

func (o *VLLMGenericText) GetMemoryRequirements(_ types.SessionMode) uint64 {
	// For now, we don't differentiate between inference and fine-tuning
	// since VLLM is only used for inference
	return o.Memory
}

func (o *VLLMGenericText) GetContextLength() int64 {
	return o.ContextLength
}

func (o *VLLMGenericText) GetConcurrency() int {
	return 0 // Default to 0 (use runtime default)
}

func (o *VLLMGenericText) GetType() types.SessionType {
	return types.SessionTypeText
}

func (o *VLLMGenericText) GetID() string {
	return o.ID
}

func (o *VLLMGenericText) ModelName() Name {
	return NewModel(o.ID)
}

// Implementation of GetCommand - not used directly as VLLM is run via Runtime
func (o *VLLMGenericText) GetCommand(_ context.Context, _ types.SessionFilter, _ types.RunnerProcessConfig) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented: VLLM models run through vLLM runtime")
}

// Implementation of GetTextStreams - not used directly as VLLM is run via Runtime
func (o *VLLMGenericText) GetTextStreams(_ types.SessionMode, _ WorkerEventHandler) (*TextStream, *TextStream, error) {
	return nil, nil, fmt.Errorf("not implemented: VLLM models run through vLLM runtime")
}

// Implementation of PrepareFiles - not used directly as VLLM is run via Runtime
func (o *VLLMGenericText) PrepareFiles(_ *types.Session, _ bool, _ SessionFileManager) (*types.Session, error) {
	return nil, fmt.Errorf("not implemented: VLLM models run through vLLM runtime")
}

// TODO: probably noop
func (o *VLLMGenericText) GetTask(session *types.Session, _ SessionFileManager) (*types.RunnerTask, error) {
	task, err := getGenericTask(session)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (o *VLLMGenericText) GetDescription() string {
	return o.Description
}

func (o *VLLMGenericText) GetHumanReadableName() string {
	return o.Name
}

func (o *VLLMGenericText) GetHidden() bool {
	return o.Hide
}

// Add GetDefaultVLLMModels function
func GetDefaultVLLMModels() ([]*VLLMGenericText, error) {
	return []*VLLMGenericText{
		{
			ID:            "Qwen/Qwen2.5-VL-3B-Instruct",
			Name:          "Qwen 2.5 VL 3B",
			Memory:        GB * 23,
			ContextLength: 32768,
			Description:   "Smaller multi-modal vision-language model, from Alibaba",
			Args: []string{
				"--trust-remote-code",
				"--max-model-len", "32768",

				"--limit-mm-per-prompt", "{\"image\":10}",
			},
			Hide:    false,
			Prewarm: false,
		},
		{
			ID:            "Qwen/Qwen2.5-VL-7B-Instruct",
			Name:          "Qwen 2.5 VL 7B",
			Memory:        GB * 39,
			ContextLength: 32768,
			Description:   "Multi-modal vision-language model, from Alibaba",
			Args: []string{
				"--trust-remote-code",
				"--max-model-len", "32768",

				"--limit-mm-per-prompt", "{\"image\":10}",
			},
			Hide:    false,
			Prewarm: true,
		},
		{
			ID:            "MrLight/dse-qwen2-2b-mrl-v1",
			Name:          "DSE Qwen2 2B",
			Memory:        GB * 10,
			ContextLength: 8192,
			Description:   "Small embedding model for RAG, from MrLight",
			Args: []string{
				"--task", "embed",
				"--max-model-len", "8192",
				"--trust-remote-code",
				"--chat-template", "examples/template_dse_qwen2_vl.jinja",
			},
			Hide:    false,
			Prewarm: true,
		},
	}, nil
}

// GetVLLMArgsForModel returns the VLLM-specific arguments for a given model
func GetVLLMArgsForModel(modelName string) ([]string, error) {
	vllmModels, err := GetDefaultVLLMModels()
	if err != nil {
		return nil, err
	}

	for _, model := range vllmModels {
		if model.ID == modelName {
			log.Debug().
				Str("model", modelName).
				Strs("args", model.Args).
				Msg("Found VLLM args for model")
			return model.Args, nil
		}
	}

	// If model not found, return an empty args list
	log.Debug().
		Str("model", modelName).
		Msg("No VLLM args found for model")
	return []string{}, nil
}
