package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Slot is the crazy mirror equivalent of scheduler.Slot
// You can think of it as the same thing as a Slot, but it's a bit fatter because it ecapsulates all
// the horrible logic involved with starting and destroying a ModelInstance.
// E.g. axolotl expects a session, whereas ollama expects an LLMInferenceRequest.
type Slot struct {
	ID              uuid.UUID // Same as scheduler.Slot
	RunnerID        string    // Same as scheduler.Slot
	Model           string    // The model assigned to this slot
	ContextLength   int64     // Optional context length override for the model
	IntendedRuntime types.Runtime
	RuntimeArgs     map[string]any // Runtime-specific arguments
	Active          bool           // True if the slot is active
	Ready           bool           // True if the slot is ready to be used
	runnerOptions   *Options
	runningRuntime  Runtime
}

type PullProgress struct {
	Status    string
	Completed int64
	Total     int64
}

type Runtime interface {
	Start(ctx context.Context) error
	Stop() error
	PullModel(ctx context.Context, model string, progress func(PullProgress) error) error
	Warm(ctx context.Context, model string) error
	Version() string
	Status(ctx context.Context) string // To hold general status information like ollama ps output
	Runtime() types.Runtime
	URL() string
}

type CreateSlotParams struct {
	RunnerOptions *Options
	ID            uuid.UUID
	Runtime       types.Runtime
	Model         string
	ContextLength int64          // Optional context length override
	RuntimeArgs   map[string]any // Runtime-specific arguments
	// RuntimeArgs can include:
	// - "model": string - Override the model to use
	// - "args": []string - Additional command line arguments to pass to the runtime
	// - "args": map[string]interface{} - Key-value pairs to be converted to command line arguments
	//   For example: {"tensor-parallel-size": "2"} becomes ["--tensor-parallel-size", "2"]
}

func NewEmptySlot(params CreateSlotParams) *Slot {
	return &Slot{
		ID:              params.ID,
		RunnerID:        params.RunnerOptions.ID,
		Model:           params.Model,
		ContextLength:   params.ContextLength,
		IntendedRuntime: params.Runtime,
		RuntimeArgs:     params.RuntimeArgs,
		Active:          false,
		Ready:           false,
		runnerOptions:   params.RunnerOptions,
		runningRuntime:  nil, // This is set during creation
	}
}

// If there is an error at any point during creation, we call Stop to kill the runtime. Otherwise it
// can just sit there taking up GPU and doing nothing.
func (s *Slot) Create(ctx context.Context) (err error) {
	log.Info().
		Str("model", s.Model).
		Interface("runtime", s.IntendedRuntime).
		Str("slot_id", s.ID.String()).
		Msg("Starting to create slot")

	// Need to be very careful to shutdown the runtime if there is an error!
	// Safest to do this in a defer so that it always checks.
	defer func() {
		if err != nil {
			if s.runningRuntime != nil {
				log.Warn().
					Str("model", s.Model).
					Interface("runtime", s.IntendedRuntime).
					Str("slot_id", s.ID.String()).
					Err(err).
					Msg("error creating slot, stopping runtime")
				stopErr := s.runningRuntime.Stop()
				if stopErr != nil {
					log.Error().
						Err(stopErr).
						Str("model", s.Model).
						Str("slot_id", s.ID.String()).
						Interface("runtime", s.IntendedRuntime).
						Msg("error stopping runtime, possible memory leak")
				}
			} else {
				log.Warn().
					Str("model", s.Model).
					Interface("runtime", s.IntendedRuntime).
					Str("slot_id", s.ID.String()).
					Err(err).
					Msg("error creating slot, but no runtime to stop")
			}
		}
	}()

	switch s.IntendedRuntime {
	case types.RuntimeOllama:
		runtimeParams := OllamaRuntimeParams{
			CacheDir: &s.runnerOptions.CacheDir,
		}

		// Only set ContextLength if it's non-zero
		if s.ContextLength > 0 {
			runtimeParams.ContextLength = &s.ContextLength
		}

		// Process runtime args for Ollama
		modelStr := s.Model // Default model from slot
		if s.RuntimeArgs != nil {
			// Override model if specified in runtime args
			if modelVal, ok := s.RuntimeArgs["model"].(string); ok && modelVal != "" {
				modelStr = modelVal
			}

			// Handle Ollama command line arguments
			if args, ok := s.RuntimeArgs["args"].([]string); ok && len(args) > 0 {
				runtimeParams.Args = args
			} else if argsMap, ok := s.RuntimeArgs["args"].(map[string]interface{}); ok && len(argsMap) > 0 {
				// If args are provided as a map, convert to array
				args := []string{}
				for k, v := range argsMap {
					if !strings.HasPrefix(k, "--") {
						k = "--" + k
					}
					args = append(args, k, fmt.Sprintf("%v", v))
				}
				runtimeParams.Args = args
			}
		}

		// Set the model parameter
		runtimeParams.Model = &modelStr

		s.runningRuntime, err = NewOllamaRuntime(ctx, runtimeParams)
		if err != nil {
			return
		}
	case types.RuntimeDiffusers:
		s.runningRuntime, err = NewDiffusersRuntime(ctx, DiffusersRuntimeParams{
			CacheDir: &s.runnerOptions.CacheDir,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	case types.RuntimeAxolotl:
		s.runningRuntime, err = NewAxolotlRuntime(ctx, AxolotlRuntimeParams{
			RunnerOptions: s.runnerOptions,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	case types.RuntimeVLLM:
		runtimeParams := VLLMRuntimeParams{
			CacheDir: &s.runnerOptions.CacheDir,
		}

		// Only set ContextLength if it's non-zero
		if s.ContextLength > 0 {
			runtimeParams.ContextLength = &s.ContextLength
		}

		// Process runtime args for vLLM
		modelStr := s.Model // Default model from slot
		if s.RuntimeArgs != nil {
			log.Debug().
				Str("slot_id", s.ID.String()).
				Str("model", s.Model).
				Interface("runtime_args", s.RuntimeArgs).
				Msg("🐟 Processing RuntimeArgs for VLLM in Slot.Create")

			// Override model if specified in runtime args
			if modelVal, ok := s.RuntimeArgs["model"].(string); ok && modelVal != "" {
				modelStr = modelVal
			}

			// Handle vLLM command line arguments
			if args, ok := s.RuntimeArgs["args"].([]string); ok && len(args) > 0 {
				log.Debug().
					Strs("args", args).
					Str("model", modelStr).
					Msg("🐟 Using string array arguments for vLLM")
				runtimeParams.Args = args
			} else if argsIface, ok := s.RuntimeArgs["args"].([]interface{}); ok && len(argsIface) > 0 {
				// Convert []interface{} to []string (common after JSON deserialization)
				args := make([]string, len(argsIface))
				for i, v := range argsIface {
					args[i] = fmt.Sprintf("%v", v)
				}

				log.Debug().
					Strs("converted_args", args).
					Str("model", modelStr).
					Msg("🐟 Converted []interface{} to []string for vLLM args")
				runtimeParams.Args = args
			} else if argsMap, ok := s.RuntimeArgs["args"].(map[string]interface{}); ok && len(argsMap) > 0 {
				// Convert map to array of args in the format ["--key1", "value1", "--key2", "value2"]
				args := []string{}
				for k, v := range argsMap {
					if !strings.HasPrefix(k, "--") {
						k = "--" + k
					}
					args = append(args, k, fmt.Sprintf("%v", v))
				}
				log.Debug().
					Interface("args_map", argsMap).
					Strs("converted_args", args).
					Str("model", modelStr).
					Msg("🐟 Using map arguments for vLLM")
				runtimeParams.Args = args
			} else if argsVal, ok := s.RuntimeArgs["args"]; ok {
				// Log when we can't parse the args
				log.Warn().
					Interface("args_value", argsVal).
					Str("args_type", fmt.Sprintf("%T", argsVal)).
					Str("model", modelStr).
					Msg("🐟 Could not parse args of unexpected type")
			}
		}

		// Check if we have a model
		if modelStr == "" {
			err = fmt.Errorf("model must be specified for vLLM runtime")
			return
		}

		// Set the model parameter
		runtimeParams.Model = &modelStr

		log.Debug().
			Str("model", modelStr).
			Interface("runtime_params", runtimeParams).
			Strs("args", runtimeParams.Args).
			Msg("🐟 Creating vLLM runtime with args")

		s.runningRuntime, err = NewVLLMRuntime(ctx, runtimeParams)
		if err != nil {
			log.Error().
				Err(err).
				Str("model", modelStr).
				Interface("runtime_params", runtimeParams).
				Msg("Failed to create vLLM runtime")
			return
		}
	default:
		err = fmt.Errorf("unknown runtime: %s", s.IntendedRuntime)
		return
	}

	// Start the runtime
	log.Debug().
		Str("model", s.Model).
		Interface("runtime", s.IntendedRuntime).
		Str("slot_id", s.ID.String()).
		Msg("Starting runtime for slot")

	err = s.runningRuntime.Start(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Str("model", s.Model).
			Interface("runtime", s.IntendedRuntime).
			Str("slot_id", s.ID.String()).
			Msg("Failed to start runtime for slot")
		return
	}

	// Create OpenAI Client
	openAIClient, err := CreateOpenaiClient(ctx, fmt.Sprintf("%s/v1", s.runningRuntime.URL()))
	if err != nil {
		log.Error().
			Err(err).
			Str("model", s.Model).
			Interface("runtime", s.IntendedRuntime).
			Str("slot_id", s.ID.String()).
			Msg("Failed to create OpenAI client for slot")
		return
	}

	// For VLLM runtime, skip model verification since it starts with the specified model
	if s.IntendedRuntime == types.RuntimeVLLM {
		log.Info().
			Str("model", s.Model).
			Str("slot_id", s.ID.String()).
			Msg("skipping model verification for VLLM runtime")
		s.Active = true
		// Warm up the model
		log.Debug().
			Str("model", s.Model).
			Str("slot_id", s.ID.String()).
			Msg("Warming up vLLM model")
		err = s.runningRuntime.Warm(ctx, s.Model)
		if err != nil {
			log.Error().
				Err(err).
				Str("model", s.Model).
				Str("slot_id", s.ID.String()).
				Msg("Failed to warm up vLLM model")
			return
		}
		s.Active = false
		s.Ready = true
		log.Info().
			Str("model", s.Model).
			Str("slot_id", s.ID.String()).
			Msg("vLLM model is ready")
		return
	}

	// Check that the model is available in this runtime
	models, err := openAIClient.ListModels(ctx)
	if err != nil {
		return
	}
	found := false
	modelList := make([]string, 0, len(models.Models))
	for _, m := range models.Models {
		modelList = append(modelList, m.ID)
		if m.ID == s.Model {
			found = true
			break
		}
	}
	if !found {
		// TODO(phil): I disabled model pulling for now because it's more work. But it is there if
		// we need it
		err = fmt.Errorf("model %s not found, available models: %s", s.Model, strings.Join(modelList, ", "))
		return
	}

	s.Active = true
	// Warm up the model
	err = s.runningRuntime.Warm(ctx, s.Model)
	if err != nil {
		return
	}
	s.Active = false
	s.Ready = true
	return
}

func (s *Slot) Delete() error {
	if s.runningRuntime != nil {
		return s.runningRuntime.Stop()
	}
	return nil
}

func (s *Slot) Version() string {
	if s.runningRuntime != nil {
		return s.runningRuntime.Version()
	}
	return "unknown"
}

func (s *Slot) Runtime() types.Runtime {
	if s.runningRuntime != nil {
		return s.runningRuntime.Runtime()
	}
	return types.Runtime("unknown")
}

func (s *Slot) Status(ctx context.Context) string {
	if s.runningRuntime != nil {
		return s.runningRuntime.Status(ctx)
	}
	return "unknown"
}

func (s *Slot) URL() string {
	if s.runningRuntime != nil {
		return s.runningRuntime.URL()
	}
	return ""
}
