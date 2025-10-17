package runner

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Slot is the crazy mirror equivalent of scheduler.Slot
// You can think of it as the same thing as a Slot, but it's a bit fatter because it ecapsulates all
// the horrible logic involved with starting and destroying a ModelInstance.
// E.g. axolotl expects a session, whereas ollama expects an LLMInferenceRequest.
type Slot struct {
	ID                     uuid.UUID // Same as scheduler.Slot
	RunnerID               string    // Same as scheduler.Slot
	Model                  string    // The model assigned to this slot
	ModelMemoryRequirement uint64    // The memory requirement of the model (bytes)
	ContextLength          int64     // Optional context length override for the model
	IntendedRuntime        types.Runtime
	RuntimeArgs            map[string]any // Runtime-specific arguments
	MemoryEstimationMeta   map[string]any // Metadata about memory estimation for tooltips
	Active                 bool           // True if the slot is active
	Ready                  bool           // True if the slot is ready to be used
	activeRequests         int64          // Number of concurrent active requests (atomic)
	GPUIndex               *int           // Primary GPU for single-GPU models (nil for CPU-only)
	GPUIndices             []int          // All GPUs used for multi-GPU models
	TensorParallelSize     int            // Number of GPUs for tensor parallelism (1 = single GPU, 0 = CPU-only)
	CommandLine            string         // The actual command line executed for this slot
	Created                time.Time      // When the slot was created
	runnerOptions          *Options
	runningRuntime         Runtime
	apiServer              *HelixRunnerAPIServer // Reference to API server for system config
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
	ListModels(ctx context.Context) ([]string, error)
	Version() string
	Status(ctx context.Context) string // To hold general status information like ollama ps output
	Runtime() types.Runtime
	URL() string
	CommandLine() string // Returns the actual command line executed (if available)
}

type CreateSlotParams struct {
	RunnerOptions          *Options
	ID                     uuid.UUID
	Runtime                types.Runtime
	Model                  string
	ModelMemoryRequirement uint64
	ContextLength          int64                 // Optional context length override
	RuntimeArgs            map[string]any        // Runtime-specific arguments
	MemoryEstimationMeta   map[string]any        // Metadata about memory estimation for tooltips
	APIServer              *HelixRunnerAPIServer // Reference to API server for system config

	// GPU allocation from scheduler - authoritative allocation decision
	GPUIndex           *int  // Primary GPU for single-GPU models
	GPUIndices         []int // All GPUs used for multi-GPU models
	TensorParallelSize int   // Number of GPUs for tensor parallelism (1 = single GPU)

	// RuntimeArgs can include:
	// - "model": string - Override the model to use
	// - "args": []string - Additional command line arguments to pass to the runtime
	// - "args": map[string]interface{} - Key-value pairs to be converted to command line arguments
	//   For example: {"tensor-parallel-size": "2"} becomes ["--tensor-parallel-size", "2"]
}

func NewEmptySlot(params CreateSlotParams) *Slot {
	return &Slot{
		ID:                     params.ID,
		RunnerID:               params.RunnerOptions.ID,
		Model:                  params.Model,
		ModelMemoryRequirement: params.ModelMemoryRequirement,
		ContextLength:          params.ContextLength,
		IntendedRuntime:        params.Runtime,
		RuntimeArgs:            params.RuntimeArgs,
		MemoryEstimationMeta:   params.MemoryEstimationMeta,
		Active:                 false,
		Ready:                  false,
		GPUIndex:               params.GPUIndex,
		GPUIndices:             params.GPUIndices,
		TensorParallelSize:     params.TensorParallelSize,
		Created:                time.Now(),
		runnerOptions:          params.RunnerOptions,
		apiServer:              params.APIServer,
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

	// Create log buffer for this slot (for all runtime types)
	var logBuffer *system.ModelInstanceLogBuffer
	if s.apiServer != nil {
		logBuffer = s.apiServer.logManager.CreateBuffer(s.ID.String(), s.Model)
	}

	switch s.IntendedRuntime {
	case types.RuntimeOllama:
		runtimeParams := OllamaRuntimeParams{
			CacheDir:  &s.runnerOptions.CacheDir,
			LogBuffer: logBuffer,
		}

		// Only set ContextLength if it's non-zero
		if s.ContextLength > 0 {
			runtimeParams.ContextLength = &s.ContextLength
		}

		// Use GPU allocation from scheduler (authoritative decision)
		// The scheduler has already made the optimal GPU allocation decision
		if s.GPUIndex != nil {
			runtimeParams.GPUIndex = s.GPUIndex
			s.TensorParallelSize = 1
			log.Info().
				Str("slot_id", s.ID.String()).
				Str("model", s.Model).
				Int("selected_gpu", *s.GPUIndex).
				Uint64("model_memory_requirement", s.ModelMemoryRequirement).
				Msg("Using single GPU allocation from scheduler for Ollama model")
		} else if len(s.GPUIndices) > 0 {
			runtimeParams.GPUIndices = s.GPUIndices
			s.TensorParallelSize = len(s.GPUIndices)
			log.Info().
				Str("slot_id", s.ID.String()).
				Str("model", s.Model).
				Ints("selected_gpus", s.GPUIndices).
				Int("tensor_parallel_size", s.TensorParallelSize).
				Uint64("model_memory_requirement", s.ModelMemoryRequirement).
				Msg("Using multi-GPU allocation from scheduler for Ollama model")
		} else {
			// No specific allocation - let Ollama use all available GPUs
			s.TensorParallelSize = 0 // Indicates auto mode
			log.Info().
				Str("slot_id", s.ID.String()).
				Str("model", s.Model).
				Uint64("model_memory_requirement", s.ModelMemoryRequirement).
				Msg("No specific GPU allocation from scheduler, using all available GPUs for Ollama model")
		}

		// Extract num_parallel from RuntimeArgs if provided by scheduler
		log.Info().
			Str("slot_id", s.ID.String()).
			Str("model", s.Model).
			Interface("runtime_args", s.RuntimeArgs).
			Msg("🔍 TRACING: RuntimeArgs received in slot creation")

		if s.RuntimeArgs != nil {
			if numParallelVal, ok := s.RuntimeArgs["num_parallel"].(int); ok && numParallelVal > 0 {
				runtimeParams.NumParallel = &numParallelVal
				log.Info().
					Str("slot_id", s.ID.String()).
					Str("model", s.Model).
					Int("num_parallel", numParallelVal).
					Msg("🔍 TRACING: Successfully extracted num_parallel from RuntimeArgs for Ollama model")
			} else {
				// DEBUG: Check what type the value actually is
				if val, exists := s.RuntimeArgs["num_parallel"]; exists {
					log.Warn().
						Str("slot_id", s.ID.String()).
						Str("model", s.Model).
						Interface("num_parallel_raw_value", val).
						Str("actual_type", fmt.Sprintf("%T", val)).
						Msg("🔍 TRACING: Type assertion failed - checking actual type")

					// Try different type assertions
					if float64Val, ok := val.(float64); ok {
						numParallelInt := int(float64Val)
						runtimeParams.NumParallel = &numParallelInt
						log.Info().
							Str("slot_id", s.ID.String()).
							Str("model", s.Model).
							Float64("num_parallel_float64", float64Val).
							Int("num_parallel_converted", numParallelInt).
							Msg("🔍 TRACING: Successfully extracted num_parallel as float64 and converted to int")
					} else if stringVal, ok := val.(string); ok {
						if numParallelInt, err := strconv.Atoi(stringVal); err == nil && numParallelInt > 0 {
							runtimeParams.NumParallel = &numParallelInt
							log.Info().
								Str("slot_id", s.ID.String()).
								Str("model", s.Model).
								Str("num_parallel_string", stringVal).
								Int("num_parallel_converted", numParallelInt).
								Msg("🔍 TRACING: Successfully extracted num_parallel as string and converted to int")
						} else {
							log.Error().
								Str("slot_id", s.ID.String()).
								Str("model", s.Model).
								Str("num_parallel_string", stringVal).
								Err(err).
								Msg("🔍 TRACING: Failed to convert num_parallel string to int")
						}
					} else {
						log.Error().
							Str("slot_id", s.ID.String()).
							Str("model", s.Model).
							Interface("num_parallel_value", val).
							Str("actual_type", fmt.Sprintf("%T", val)).
							Msg("🔍 TRACING: Unknown type for num_parallel - cannot convert")
					}
				} else {
					log.Warn().
						Str("slot_id", s.ID.String()).
						Str("model", s.Model).
						Interface("runtime_args", s.RuntimeArgs).
						Msg("🔍 TRACING: num_parallel key does not exist in RuntimeArgs")
				}
			}
		} else {
			log.Warn().
				Str("slot_id", s.ID.String()).
				Str("model", s.Model).
				Msg("🔍 TRACING: RuntimeArgs is nil - no num_parallel available")
		}

		log.Debug().
			Str("model", s.Model).
			Interface("runtime_params", runtimeParams).
			Interface("gpu_index", s.GPUIndex).
			Ints("gpu_indices", s.GPUIndices).
			Int("tensor_parallel_size", s.TensorParallelSize).
			Msg("Creating Ollama runtime with scheduler GPU allocation")

		log.Info().
			Str("slot_id", s.ID.String()).
			Str("model", s.Model).
			Interface("num_parallel_ptr", runtimeParams.NumParallel).
			Int("num_parallel_value", func() int {
				if runtimeParams.NumParallel != nil {
					return *runtimeParams.NumParallel
				}
				return 0
			}()).
			Msg("🔍 TRACING: Final runtimeParams.NumParallel being passed to NewOllamaRuntime")

		// Set up crash callback to handle unexpected Ollama crashes (e.g., CUDA errors)
		runtimeParams.OnCrash = func(stderr string) {
			log.Error().
				Str("slot_id", s.ID.String()).
				Str("model", s.Model).
				Str("stderr_preview", func() string {
					// Limit stderr to last 500 characters for logging
					if len(stderr) > 500 {
						return "..." + stderr[len(stderr)-500:]
					}
					return stderr
				}()).
				Msg("Ollama process crashed, cleaning up slot")

			// Clean up this slot since the runtime crashed
			// This will be called from the cmd.Wait() goroutine, so we need to do this asynchronously
			go func() {
				if err := s.Delete(); err != nil {
					log.Error().
						Err(err).
						Str("slot_id", s.ID.String()).
						Str("model", s.Model).
						Msg("Failed to delete slot after Ollama crash")
				} else {
					log.Info().
						Str("slot_id", s.ID.String()).
						Str("model", s.Model).
						Msg("Successfully cleaned up slot after Ollama crash")

					// Remove the slot from the server's slot map
					if s.apiServer != nil && s.apiServer.slots != nil {
						s.apiServer.slots.Delete(s.ID)
						log.Info().
							Str("slot_id", s.ID.String()).
							Msg("Removed crashed slot from server slot map")
					}
				}
			}()
		}

		s.runningRuntime, err = NewOllamaRuntime(ctx, runtimeParams)
		if err != nil {
			log.Error().
				Err(err).
				Str("model", s.Model).
				Interface("runtime_params", runtimeParams).
				Msg("Failed to create Ollama runtime")
			return
		}

		// Set up process tracking for Ollama runtime
		if ollamaRuntime, ok := s.runningRuntime.(*OllamaRuntime); ok && s.apiServer != nil {
			ollamaRuntime.SetProcessTracker(s.apiServer.processTracker, s.ID)
			log.Debug().
				Str("slot_id", s.ID.String()).
				Msg("PROCESS_TRACKER: Set up process tracking for Ollama runtime")
		}
	case types.RuntimeDiffusers:
		// Get effective HF token from API server (with fallback to env)
		// Same token resolution hierarchy as VLLM (see VLLM case for details)
		var hfToken *string
		if s.apiServer != nil {
			effectiveToken := s.apiServer.GetEffectiveHuggingFaceToken()
			if effectiveToken != "" {
				hfToken = &effectiveToken
			}
		}

		s.runningRuntime, err = NewDiffusersRuntime(ctx, DiffusersRuntimeParams{
			CacheDir:         &s.runnerOptions.CacheDir,
			HuggingFaceToken: hfToken,
			LogBuffer:        logBuffer,
		})
		if err != nil {
			return
		}
	case types.RuntimeAxolotl:
		s.runningRuntime, err = NewAxolotlRuntime(ctx, AxolotlRuntimeParams{
			RunnerOptions: s.runnerOptions,
			LogBuffer:     logBuffer,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	case types.RuntimeVLLM:
		// Get effective HF token from API server (with fallback to env)
		// Current: Uses global system token
		// Future: Will resolve in priority order:
		//   1. Model-specific token (from RuntimeArgs)
		//   2. User-specific token (from session context)
		//   3. Organization-specific token (from user's org)
		//   4. Global system token (current implementation)
		//   5. Environment variable (backward compatibility)
		var hfToken *string
		if s.apiServer != nil {
			effectiveToken := s.apiServer.GetEffectiveHuggingFaceToken()
			if effectiveToken != "" {
				hfToken = &effectiveToken
			}
		}

		// Log buffer was already created above for all runtime types

		runtimeParams := VLLMRuntimeParams{
			CacheDir:         &s.runnerOptions.CacheDir,
			HuggingFaceToken: hfToken,
			LogBuffer:        logBuffer,
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
				Msg("Processing RuntimeArgs for VLLM in Slot.Create")

			// Override model if specified in runtime args
			if modelVal, ok := s.RuntimeArgs["model"].(string); ok && modelVal != "" {
				modelStr = modelVal
			}

			// Handle vLLM command line arguments
			// Support both flattened format (runtime_args as direct array) and nested format (runtime_args.args)
			var parsedArgs []string

			// Check if runtime_args contains a direct "args" key (nested format)
			if args, ok := s.RuntimeArgs["args"].([]string); ok && len(args) > 0 {
				parsedArgs = args
				log.Debug().
					Strs("nested_args", parsedArgs).
					Str("model", modelStr).
					Msg("Using nested runtime_args.args (string array) for vLLM")
			} else if argsIface, ok := s.RuntimeArgs["args"].([]interface{}); ok && len(argsIface) > 0 {
				// Convert []interface{} to []string (common after JSON deserialization)
				parsedArgs = make([]string, len(argsIface))
				for i, v := range argsIface {
					parsedArgs[i] = fmt.Sprintf("%v", v)
				}
				log.Debug().
					Strs("nested_converted_args", parsedArgs).
					Str("model", modelStr).
					Msg("Converted nested []interface{} to []string for vLLM args")
			} else if argsMap, ok := s.RuntimeArgs["args"].(map[string]interface{}); ok && len(argsMap) > 0 {
				// Convert map to array of args in the format ["--key1", "value1", "--key2", "value2"]
				parsedArgs = []string{}
				for k, v := range argsMap {
					if !strings.HasPrefix(k, "--") {
						k = "--" + k
					}
					parsedArgs = append(parsedArgs, k, fmt.Sprintf("%v", v))
				}
				log.Debug().
					Interface("args_map", argsMap).
					Strs("nested_converted_args", parsedArgs).
					Str("model", modelStr).
					Msg("Using nested map arguments for vLLM")
			} else if argsVal, ok := s.RuntimeArgs["args"]; ok {
				// Log when we can't parse the nested args
				log.Warn().
					Interface("nested_args_value", argsVal).
					Str("args_type", fmt.Sprintf("%T", argsVal)).
					Str("model", modelStr).
					Msg("Could not parse nested args of unexpected type")
			}

			if len(parsedArgs) > 0 {
				runtimeParams.Args = parsedArgs
			}
		}

		// Check if we have a model
		if modelStr == "" {
			err = fmt.Errorf("model must be specified for vLLM runtime")
			return
		}

		// Set the model parameter
		runtimeParams.Model = &modelStr

		// Use GPU allocation from scheduler (authoritative decision)
		// The scheduler has already made the optimal GPU allocation decision
		if s.GPUIndex != nil {
			runtimeParams.GPUIndex = s.GPUIndex
			s.TensorParallelSize = 1
			log.Info().
				Str("slot_id", s.ID.String()).
				Str("model", modelStr).
				Int("selected_gpu", *s.GPUIndex).
				Uint64("model_memory_requirement", s.ModelMemoryRequirement).
				Msg("Using single GPU allocation from scheduler for VLLM model")
		} else if len(s.GPUIndices) > 0 {
			runtimeParams.GPUIndices = s.GPUIndices
			runtimeParams.TensorParallelSize = &s.TensorParallelSize
			log.Info().
				Str("slot_id", s.ID.String()).
				Str("model", modelStr).
				Ints("selected_gpus", s.GPUIndices).
				Int("tensor_parallel_size", s.TensorParallelSize).
				Uint64("model_memory_requirement", s.ModelMemoryRequirement).
				Msg("Using multi-GPU allocation from scheduler for VLLM model")
		} else {
			// Fallback to default GPU 0 if no allocation provided
			defaultGPU := 0
			runtimeParams.GPUIndex = &defaultGPU
			s.GPUIndex = &defaultGPU
			s.TensorParallelSize = 1
			log.Warn().
				Str("slot_id", s.ID.String()).
				Str("model", modelStr).
				Uint64("model_memory_requirement", s.ModelMemoryRequirement).
				Msg("No GPU allocation from scheduler, using default GPU 0 for VLLM model")
		}

		log.Debug().
			Str("model", modelStr).
			Interface("runtime_params", runtimeParams).
			Strs("args", runtimeParams.Args).
			Interface("gpu_index", s.GPUIndex).
			Ints("gpu_indices", s.GPUIndices).
			Int("tensor_parallel_size", s.TensorParallelSize).
			Msg("Creating vLLM runtime with scheduler GPU allocation")

		s.runningRuntime, err = NewVLLMRuntime(ctx, runtimeParams)
		if err != nil {
			log.Error().
				Err(err).
				Str("model", modelStr).
				Interface("runtime_params", runtimeParams).
				Msg("Failed to create vLLM runtime")
			return
		}

		// Set up process tracking for VLLM runtime
		if vllmRuntime, ok := s.runningRuntime.(*VLLMRuntime); ok && s.apiServer != nil {
			vllmRuntime.SetProcessTracker(s.apiServer.processTracker, s.ID)
			log.Debug().
				Str("slot_id", s.ID.String()).
				Msg("PROCESS_TRACKER: Set up process tracking for VLLM runtime")
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

	// Capture the command line after successful start
	s.CommandLine = s.runningRuntime.CommandLine()
	if s.CommandLine != "" {
		log.Debug().
			Str("model", s.Model).
			Str("slot_id", s.ID.String()).
			Str("command_line", s.CommandLine).
			Msg("Captured runtime command line")
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
	// Clean up log buffer
	if s.apiServer != nil {
		s.apiServer.logManager.RemoveBuffer(s.ID.String())
	}

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
