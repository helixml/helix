//go:build !windows
// +build !windows

package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var (
	vllmCommander Commander = &RealCommander{}
	_             Runtime   = &VLLMRuntime{}
)

type VLLMRuntime struct {
	version            string
	cacheDir           string
	port               int
	startTimeout       time.Duration
	contextLength      int64
	model              string
	args               []string
	huggingFaceToken   string
	gpuIndex           int   // Primary GPU index for single-GPU models
	gpuIndices         []int // All GPU indices for multi-GPU models
	tensorParallelSize int   // Number of GPUs for tensor parallelism
	cmd                *exec.Cmd
	cancel             context.CancelFunc
	logBuffer          *system.ModelInstanceLogBuffer // Log buffer for this instance
	commandLine        string                         // The actual command line executed
	ready              bool                           // True when vLLM is ready to handle requests
	processTracker     *ProcessTracker                // Process tracker for monitoring
	slotID             *uuid.UUID                     // Associated slot ID
	originalCtx        context.Context                // Context passed to most recent Start() call
}

type VLLMRuntimeParams struct {
	CacheDir           *string                        // Where to store the models
	Port               *int                           // If nil, will be assigned a random port
	StartTimeout       *time.Duration                 // How long to wait for vLLM to start, if nil, will use default
	ContextLength      *int64                         // Optional: Context length to use for the model
	Model              *string                        // Optional: Model to use
	Args               []string                       // Optional: Additional arguments to pass to vLLM
	HuggingFaceToken   *string                        // Optional: Hugging Face token for model access
	GPUIndex           *int                           // Optional: Primary GPU index for single-GPU models (for multi-GPU scheduling)
	GPUIndices         []int                          // Optional: GPU indices for multi-GPU models (overrides GPUIndex)
	TensorParallelSize *int                           // Optional: Number of GPUs for tensor parallelism (default 1)
	LogBuffer          *system.ModelInstanceLogBuffer // Optional: Log buffer for capturing logs
}

func NewVLLMRuntime(_ context.Context, params VLLMRuntimeParams) (*VLLMRuntime, error) {
	defaultCacheDir := os.TempDir()
	if params.CacheDir == nil {
		params.CacheDir = &defaultCacheDir
	}

	log.Debug().Interface("startTimeout", params.StartTimeout).Msg("start timeout before default check")
	defaultStartTimeout := 24 * time.Hour
	if params.StartTimeout == nil {
		params.StartTimeout = &defaultStartTimeout
	}
	log.Debug().Dur("startTimeout", *params.StartTimeout).Msg("Using start timeout")
	if params.Port == nil {
		port, err := freeport.GetFreePort()
		if err != nil {
			return nil, fmt.Errorf("error getting free port: %s", err.Error())
		}
		params.Port = &port
		log.Debug().Int("port", *params.Port).Msg("Found free port")
	}

	// Determine context length
	var contextLength int64

	// If context length is provided, use it
	if params.ContextLength != nil {
		contextLength = *params.ContextLength
		log.Debug().Int64("context_length", contextLength).Msg("Using provided context length")
	}

	// Check for model parameter
	var model string
	if params.Model != nil {
		model = *params.Model
		log.Debug().Str("model", model).Msg("Using model")
	}

	// Extract HF token
	var hfToken string
	if params.HuggingFaceToken != nil {
		hfToken = *params.HuggingFaceToken
	}

	// Extract GPU configuration
	var gpuIndex int
	var gpuIndices []int
	tensorParallelSize := 1

	// Multi-GPU setup takes precedence over single-GPU
	if len(params.GPUIndices) > 0 {
		gpuIndices = params.GPUIndices
		gpuIndex = gpuIndices[0] // Use first GPU as primary
		tensorParallelSize = len(gpuIndices)
	} else if params.GPUIndex != nil {
		gpuIndex = *params.GPUIndex
		gpuIndices = []int{gpuIndex}
		tensorParallelSize = 1
	}

	// Override tensor parallel size if explicitly provided
	if params.TensorParallelSize != nil {
		tensorParallelSize = *params.TensorParallelSize
	}

	// Log args received
	log.Debug().
		Str("model", model).
		Int64("context_length", contextLength).
		Strs("args", params.Args).
		Bool("hf_token_provided", hfToken != "").
		Int("gpu_index", gpuIndex).
		Ints("gpu_indices", gpuIndices).
		Int("tensor_parallel_size", tensorParallelSize).
		Msg("NewVLLMRuntime received args")

	return &VLLMRuntime{
		version:            "unknown",
		cacheDir:           *params.CacheDir,
		port:               *params.Port,
		startTimeout:       *params.StartTimeout,
		contextLength:      contextLength,
		model:              model,
		args:               params.Args,
		huggingFaceToken:   hfToken,
		gpuIndex:           gpuIndex,
		gpuIndices:         gpuIndices,
		tensorParallelSize: tensorParallelSize,
		logBuffer:          params.LogBuffer,
	}, nil
}

func (v *VLLMRuntime) Start(ctx context.Context) error {
	log.Debug().
		Str("model", v.model).
		Int64("context_length", v.contextLength).
		Strs("args", v.args).
		Msg("Starting vLLM runtime with args")

	// Make sure the port is not already in use
	if isPortInUse(v.port) {
		return fmt.Errorf("port %d is already in use", v.port)
	}

	// Check if the cache dir exists, if not create it
	if _, err := os.Stat(v.cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(v.cacheDir, 0755); err != nil {
			return fmt.Errorf("error creating cache dir: %s", err.Error())
		}
	}
	// Check that the cache dir is writable
	if _, err := os.Stat(v.cacheDir); os.IsPermission(err) {
		return fmt.Errorf("cache dir is not writable: %s", v.cacheDir)
	}

	// Store context from most recent Start() call for runtime lifecycle operations
	// This ensures that long-running operations (like model downloads) are not
	// cancelled when the client request context times out
	v.originalCtx = ctx
	originalCtx := v.originalCtx

	// Prepare vLLM cmd context (a cancel context derived from original)
	// This child context is used for the command process itself and can be
	// cancelled independently without affecting the original context
	log.Debug().Msg("Preparing vLLM context")
	ctx, cancel := context.WithCancel(originalCtx)
	v.cancel = cancel
	var err error
	defer func() {
		// If there is an error at any point after this, cancel the context to cancel the cmd
		if err != nil {
			v.cancel()
		}
	}()

	// Start vLLM cmd - uses child context so it can be cancelled independently
	cmd, commandLine, err := startVLLMCmd(ctx, vllmCommander, v.port, v.cacheDir, v.contextLength, v.model, v.args, v.huggingFaceToken, v.gpuIndex, v.gpuIndices, v.tensorParallelSize, v.logBuffer)
	if err != nil {
		return fmt.Errorf("error building vLLM cmd: %w", err)
	}
	v.cmd = cmd
	v.commandLine = commandLine

	// Register the process with the tracker if available
	if v.processTracker != nil && v.slotID != nil && v.cmd != nil && v.cmd.Process != nil {
		v.processTracker.RegisterProcess(v.cmd.Process.Pid, *v.slotID, v.model, v.commandLine)
		log.Info().
			Int("pid", v.cmd.Process.Pid).
			Str("slot_id", v.slotID.String()).
			Str("model", v.model).
			Msg("PROCESS_TRACKER: Registered VLLM process")
	}

	// Wait for vLLM to be ready
	log.Debug().Str("url", v.URL()).Dur("timeout", v.startTimeout).Msg("Waiting for vLLM to start")
	err = v.waitUntilVLLMIsReady(ctx, v.startTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for vLLM to start: %s", err.Error())
	}

	// Mark as ready only after HTTP readiness check passes
	v.ready = true

	log.Info().
		Str("model", v.model).
		Strs("args", v.args).
		Int("pid", v.cmd.Process.Pid).
		Msg("vLLM has started successfully")

	// Set the version (if available)
	v.version = "vLLM"

	return nil
}

func (v *VLLMRuntime) URL() string {
	return fmt.Sprintf("http://localhost:%d", v.port)
}

// SetProcessTracker sets the process tracker for monitoring
func (v *VLLMRuntime) SetProcessTracker(tracker *ProcessTracker, slotID uuid.UUID) {
	v.processTracker = tracker
	v.slotID = &slotID
}

func (v *VLLMRuntime) Stop() error {
	defer v.cancel() // Cancel the context no matter what

	// Clear original context so future Start() calls can use a new context
	v.originalCtx = nil

	// Mark as not ready when stopping
	v.ready = false

	if v.cmd == nil {
		return nil
	}

	// Add detailed debug info with stack trace to help debug shutdown causes
	stackTrace := make([]byte, 4096)
	stackSize := runtime.Stack(stackTrace, true)
	contextInfo := "none"
	if v.cmd.ProcessState != nil {
		contextInfo = fmt.Sprintf("exit_code=%d, exited=%t", v.cmd.ProcessState.ExitCode(), v.cmd.ProcessState.Exited())
	}

	log.Info().
		Int("pid", v.cmd.Process.Pid).
		Str("model", v.model).
		Str("stack_trace", string(stackTrace[:stackSize])).
		Str("context_info", contextInfo).
		Msg("VLLM_STOP: Stopping vLLM runtime")

	if err := killProcessTree(v.cmd.Process.Pid); err != nil {
		log.Error().
			Err(err).
			Int("pid", v.cmd.Process.Pid).
			Str("model", v.model).
			Msg("VLLM_STOP: CRITICAL - Failed to stop vLLM model process, potential GPU memory leak!")
		return err
	}
	log.Info().
		Int("pid", v.cmd.Process.Pid).
		Str("model", v.model).
		Msg("VLLM_STOP: vLLM runtime stopped successfully")
	return nil
}

func (v *VLLMRuntime) PullModel(_ context.Context, modelName string, progressFunc func(progress PullProgress) error) error {
	// If no model name is provided, use the configured model
	if modelName == "" {
		modelName = v.model
	}

	// Validate model name
	if modelName == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	// vLLM doesn't have an explicit pull/download API like Ollama
	// Models are loaded on startup or when first requested
	log.Info().Msgf("vLLM will download model %s on first use", modelName)

	// We report initial progress as started
	err := progressFunc(PullProgress{
		Status:    "Model will be downloaded on first use",
		Completed: 0,
		Total:     1,
	})
	if err != nil {
		return err
	}

	// Report as completed
	return progressFunc(PullProgress{
		Status:    "Ready to use",
		Completed: 1,
		Total:     1,
	})
}

func (v *VLLMRuntime) ListModels(_ context.Context) ([]string, error) {
	return []string{}, nil // TODO: implement
}

func (v *VLLMRuntime) Warm(ctx context.Context, model string) error {
	// If no model is provided, use the configured model
	if model == "" {
		model = v.model
	}

	// Validate model
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	log.Info().
		Str("model", model).
		Str("url", v.URL()).
		Msg("Warming up vLLM model")

	// Send a simple OpenAI-compatible request to warm up the model
	url := fmt.Sprintf("%s/v1/chat/completions", v.URL())

	// Create a simple request body
	requestBody := `{
		"model": "` + model + `",
		"messages": [
			{"role": "user", "content": "Say the word 'warm'."}
		],
		"max_tokens": 5
	}`

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(requestBody))
	if err != nil {
		log.Error().
			Err(err).
			Str("model", model).
			Str("url", url).
			Msg("Error creating warm-up request")
		return fmt.Errorf("error creating warm-up request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set a timeout for the request (30 seconds should be plenty for a simple warm-up)
	warmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req = req.WithContext(warmCtx)

	// Send the request
	startTime := time.Now()

	log.Debug().
		Str("model", model).
		Str("url", url).
		Msg("Sending warm-up request to vLLM")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Check if it's a context timeout
		if warmCtx.Err() != nil {
			log.Error().
				Err(err).
				Dur("elapsed", time.Since(startTime)).
				Str("model", model).
				Str("url", url).
				Msg("Timeout during vLLM warm-up request")
			return fmt.Errorf("timeout during warm-up request: %w", err)
		}

		log.Error().
			Err(err).
			Dur("elapsed", time.Since(startTime)).
			Str("model", model).
			Str("url", url).
			Msg("Error sending warm-up request")
		return fmt.Errorf("error sending warm-up request: %w", err)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		log.Error().
			Int("status", resp.StatusCode).
			Str("response", string(bodyBytes)).
			Dur("elapsed", time.Since(startTime)).
			Str("model", model).
			Str("url", url).
			Msg("Error warming up model")
		return fmt.Errorf("error warming up model, status: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	log.Info().
		Int("status", resp.StatusCode).
		Str("response", string(bodyBytes)).
		Dur("elapsed", time.Since(startTime)).
		Str("model", model).
		Msg("Successfully warmed up vLLM model")

	return nil
}

func (v *VLLMRuntime) Runtime() types.Runtime {
	return types.RuntimeVLLM
}

func (v *VLLMRuntime) Version() string {
	return v.version
}

func (v *VLLMRuntime) Status(ctx context.Context) string {
	if !v.ready {
		return "starting"
	}

	// Double-check that vLLM is still responding to requests
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("%s/v1/models", v.URL())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "error"
	}

	resp, err := client.Do(req)
	if err != nil {
		return "error"
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "error"
	}

	return "running"
}

func (v *VLLMRuntime) CommandLine() string {
	return v.commandLine
}

func (v *VLLMRuntime) waitUntilVLLMIsReady(ctx context.Context, startTimeout time.Duration) error {
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	lastLogTime := startTime
	attemptCount := 0

	client := &http.Client{Timeout: 1 * time.Second}

	log.Info().
		Dur("timeout", startTimeout).
		Str("model", v.model).
		Str("url", v.URL()).
		Msg("Starting vLLM ready check with timeout")

	for {
		select {
		case <-startCtx.Done():
			elapsed := time.Since(startTime)
			// Enhanced error logging with stack trace
			stackTrace := make([]byte, 4096)
			stackSize := runtime.Stack(stackTrace, true)

			parentCtxErr := "none"
			if ctx.Err() != nil {
				parentCtxErr = ctx.Err().Error()
			}

			log.Error().
				Dur("elapsed", elapsed).
				Str("model", v.model).
				Str("error", startCtx.Err().Error()).
				Str("parent_context_error", parentCtxErr).
				Str("stack_trace", string(stackTrace[:stackSize])).
				Msg("vLLM ready check timed out or context canceled")
			return startCtx.Err()
		case <-ticker.C:
			attemptCount++
			elapsed := time.Since(startTime)

			// Log status every 5 seconds
			if time.Since(lastLogTime) > 5*time.Second {
				timeLeft := startTimeout - elapsed
				log.Debug().
					Dur("elapsed", elapsed).
					Dur("time_left", timeLeft).
					Int("attempt", attemptCount).
					Str("model", v.model).
					Msg("Waiting for vLLM to be ready")
				lastLogTime = time.Now()
			}

			// Try to connect to the vLLM server's health endpoint
			url := fmt.Sprintf("%s/v1/models", v.URL())
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				if time.Since(lastLogTime) > 5*time.Second {
					log.Debug().Err(err).Str("url", url).Msg("Error creating request to check vLLM readiness")
				}
				continue
			}

			resp, err := client.Do(req)
			if err != nil {
				if time.Since(lastLogTime) > 5*time.Second {
					log.Debug().Err(err).Str("url", url).Msg("Error connecting to vLLM server")
				}
				continue
			}

			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode < 400 {
				log.Info().
					Dur("elapsed", elapsed).
					Int("status", resp.StatusCode).
					Str("model", v.model).
					Str("response", string(bodyBytes)).
					Msg("vLLM server is ready")
				return nil
			} else {
				if time.Since(lastLogTime) > 5*time.Second {
					log.Debug().
						Int("status", resp.StatusCode).
						Str("response", string(bodyBytes)).
						Msg("vLLM server returned error status")
				}
			}
		}
	}
}

// getEffectiveToken returns the provided token if not empty, otherwise falls back to environment variable
func getEffectiveToken(providedToken string) string {
	if providedToken != "" {
		return providedToken
	}
	return os.Getenv("HF_TOKEN")
}

// formatGPUIndices converts a slice of GPU indices to a comma-separated string for CUDA_VISIBLE_DEVICES
func formatGPUIndices(gpuIndices []int) string {
	if len(gpuIndices) == 0 {
		return "0" // Default to GPU 0
	}

	var indices []string
	for _, idx := range gpuIndices {
		indices = append(indices, fmt.Sprintf("%d", idx))
	}
	return strings.Join(indices, ",")
}

func startVLLMCmd(ctx context.Context, commander Commander, port int, cacheDir string, contextLength int64, model string, customArgs []string, hfToken string, _ int, gpuIndices []int, tensorParallelSize int, logBuffer *system.ModelInstanceLogBuffer) (*exec.Cmd, string, error) {
	// Use clean vLLM virtualenv Python - fail if not found (no fallback to avoid confusion)
	vllmPath := "/workspace/vllm/venv/bin/python"
	if _, err := os.Stat(vllmPath); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("vLLM virtualenv not found at %s - Docker build may have failed or vLLM installation incomplete", vllmPath)
	}
	log.Debug().Str("python_path", vllmPath).Msg("Using clean vLLM virtualenv Python 3.12 - completely isolated from system packages")

	// Prepare vLLM serve command
	log.Debug().
		Str("model", model).
		Int64("context_length", contextLength).
		Strs("custom_args", customArgs).
		Msg("Preparing vLLM serve command with custom args")

	// First prepare a map of custom arguments for quick checking
	customArgsMap := make(map[string]bool)
	for i, arg := range customArgs {
		if i < len(customArgs)-1 && strings.HasPrefix(arg, "--") {
			customArgsMap[arg] = true
		}
	}

	// Build base arguments
	args := []string{
		"-m", "vllm.entrypoints.openai.api_server",
	}

	// Only add default arguments if they're not overridden in customArgs
	if !customArgsMap["--host"] {
		args = append(args, "--host", "127.0.0.1")
	}

	if !customArgsMap["--port"] {
		args = append(args, "--port", fmt.Sprintf("%d", port))
	}

	// Only add model argument if provided and not already in custom args
	if !customArgsMap["--model"] && model != "" {
		args = append(args, "--model", model)
	} else if !customArgsMap["--model"] && model == "" {
		return nil, "", fmt.Errorf("model parameter is required for vLLM runtime")
	}

	if !customArgsMap["--max-model-len"] && contextLength > 0 {
		args = append(args, "--max-model-len", fmt.Sprintf("%d", contextLength))
	}

	// If not in custom args, add device flag for CPU-only mode
	if !customArgsMap["--device"] {
		if os.Getenv("DEVELOPMENT_CPU_ONLY") == "true" || os.Getenv("VLLM_DEVICE") == "cpu" {
			log.Debug().Msg("Adding --device=cpu command line flag for CPU-only mode")
			args = append(args, "--device", "cpu")

			// Also need to disable async output processing which isn't supported on CPU
			if !customArgsMap["--disable-async-output-proc"] {
				log.Debug().Msg("Adding --disable-async-output-proc for CPU compatibility")
				args = append(args, "--disable-async-output-proc")
			}

			// Set explicit worker class for CPU mode
			if !customArgsMap["--worker-cls"] {
				log.Debug().Msg("Setting explicit worker class for CPU mode")
				args = append(args, "--worker-cls", "vllm.worker.cpu_worker.CPUWorker")
			}

			// Set tensor parallel size to 1 for CPU
			if !customArgsMap["--tensor-parallel-size"] {
				args = append(args, "--tensor-parallel-size", "1")
			}
		} else if !customArgsMap["--tensor-parallel-size"] {
			// Use dynamic tensor parallel size for GPU mode
			args = append(args, "--tensor-parallel-size", fmt.Sprintf("%d", tensorParallelSize))
		}
	}

	// GPU memory utilization is now handled by the scheduler via RuntimeArgs
	// The scheduler calculates the appropriate ratio and passes it via substituted RuntimeArgs
	// No need to add a template placeholder here as it won't be substituted at runtime level

	// Add custom arguments
	args = append(args, customArgs...)

	log.Debug().
		Strs("args", args).
		Strs("custom_args", customArgs).
		Msg("Final vLLM command arguments")

	cmd := commander.CommandContext(ctx, vllmPath, args...)

	// Set the working directory to /vllm
	cmd.Dir = "/vllm"
	log.Debug().Str("workdir", cmd.Dir).Msg("Set vLLM working directory")

	// Set only the specific environment variables needed
	// This is more secure than inheriting all parent environment variables
	env := []string{
		// vLLM is installed in clean virtualenv - no PYTHONPATH needed since venv handles it
		// Using clean Python 3.12 venv completely isolated from system packages
		//
		// AXOLOTL RESTORATION NOTE:
		// When axolotl is re-enabled, you'll need to:
		// 1. Install miniconda in base-images/Dockerfile.runner (see git history)
		// 2. Add back: "PYTHONPATH=/root/miniconda3/envs/py3.11/lib/python3.11/site-packages"
		// 3. Change vllmPath above to use miniconda python as needed
		// 4. Update base image FROM to winglian/axolotl image
		// System paths - often needed by Python to find libraries
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")),

		// Python configuration
		"PYTHONUNBUFFERED=1",

		// CUDA configuration - use selected GPU(s) for proper multi-GPU scheduling
		fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", formatGPUIndices(gpuIndices)),

		// Cache directories
		fmt.Sprintf("TRANSFORMERS_CACHE=%s", cacheDir),
		fmt.Sprintf("HF_HOME=%s", cacheDir),

		// Proxy settings if needed
		fmt.Sprintf("HTTP_PROXY=%s", os.Getenv("HTTP_PROXY")),
		fmt.Sprintf("HTTPS_PROXY=%s", os.Getenv("HTTPS_PROXY")),
		fmt.Sprintf("NO_PROXY=%s", os.Getenv("NO_PROXY")),

		// set this when on EasyJet flights or high security airgapped deployments
		// "HF_HUB_OFFLINE=1",
		// TODO: figure out how to do offline vLLM properly..

		// Hugging Face authentication - prefer provided token over environment
		fmt.Sprintf("HF_TOKEN=%s", getEffectiveToken(hfToken)),
	}

	// Check for CPU-only mode
	if os.Getenv("DEVELOPMENT_CPU_ONLY") == "true" {
		log.Debug().Msg("CPU-only mode detected via DEVELOPMENT_CPU_ONLY, setting VLLM_DEVICE=cpu")
		env = append(env, "VLLM_DEVICE=cpu")
		env = append(env, "VLLM_LOGGING_LEVEL=DEBUG")
	} else {
		// Pass through existing vLLM config if set
		if vllmDevice := os.Getenv("VLLM_DEVICE"); vllmDevice != "" {
			log.Debug().Str("VLLM_DEVICE", vllmDevice).Msg("Using VLLM_DEVICE from environment")
			env = append(env, fmt.Sprintf("VLLM_DEVICE=%s", vllmDevice))
		}
		if vllmLoggingLevel := os.Getenv("VLLM_LOGGING_LEVEL"); vllmLoggingLevel != "" {
			log.Debug().Str("VLLM_LOGGING_LEVEL", vllmLoggingLevel).Msg("Using VLLM_LOGGING_LEVEL from environment")
			env = append(env, fmt.Sprintf("VLLM_LOGGING_LEVEL=%s", vllmLoggingLevel))
		}
	}

	cmd.Env = env

	// Construct the full command line for display
	commandLine := fmt.Sprintf("%s %s", vllmPath, strings.Join(args, " "))
	log.Debug().Interface("env", cmd.Env).Str("cmd", commandLine).Msg("vLLM serve command")

	// Prepare stdout and stderr
	log.Debug().Msg("Preparing stdout and stderr")
	cmd.Stdout = os.Stdout

	// Set up stderr capture with enhanced logging
	var stderrBuf *system.LimitedBuffer
	var stderrWriters []io.Writer

	// Always include the legacy limited buffer for backward compatibility
	stderrBuf = system.NewLimitedBuffer(1024 * 10)
	stderrWriters = []io.Writer{os.Stderr, stderrBuf}

	// If we have a log buffer for this instance, add it to the writers
	if logBuffer != nil {
		stderrWriters = append(stderrWriters, logBuffer)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, "", err
	}
	go func() {
		_, err := io.Copy(io.MultiWriter(stderrWriters...), stderrPipe)
		if err != nil {
			log.Error().Msgf("Error copying stderr: %v", err)
		}
	}()

	log.Debug().Msg("Starting vLLM serve")
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("error starting vLLM model instance: %w", err)
	}

	go func() {
		for {
			if err := cmd.Wait(); err != nil {
				errMsg := string(stderrBuf.Bytes())
				log.Error().Err(err).Str("stderr", errMsg).Int("exit_code", cmd.ProcessState.ExitCode()).Msg("vLLM exited with error")

				// Update log buffer status if available
				if logBuffer != nil {
					logBuffer.SetStatus("errored")
				}

				// Don't restart if context is canceled
				if ctx.Err() != nil {
					// Enhanced logging to capture why the context was canceled
					contextErr := ctx.Err()
					stackTrace := make([]byte, 4096)
					stackSize := runtime.Stack(stackTrace, true)
					log.Info().
						Err(contextErr).
						Str("model", model).
						Str("stack_trace", string(stackTrace[:stackSize])).
						Msg("Not restarting vLLM because context was canceled")
					return
				}

				// Restart the process
				log.Info().Str("model", model).Int("port", port).Msg("Restarting vLLM process after unexpected exit")
				newCmd := commander.CommandContext(ctx, vllmPath, args...)

				// Set the same working directory for the restarted command
				newCmd.Dir = "/vllm"
				log.Debug().Str("workdir", newCmd.Dir).Msg("Set vLLM working directory for restarted process")

				// Set the same environment variables
				newCmd.Env = env
				newCmd.Stdout = os.Stdout

				// Set up stderr handling
				newStderrBuf := system.NewLimitedBuffer(1024 * 10)
				newStderrWriters := []io.Writer{os.Stderr, newStderrBuf}

				// If we have a log buffer for this instance, add it to the writers
				if logBuffer != nil {
					newStderrWriters = append(newStderrWriters, logBuffer)
				}

				newStderrPipe, err := newCmd.StderrPipe()
				if err != nil {
					log.Error().Err(err).Msg("Failed to create stderr pipe for restarted vLLM")
					return
				}

				go func() {
					_, err := io.Copy(io.MultiWriter(newStderrWriters...), newStderrPipe)
					if err != nil {
						log.Error().Msgf("Error copying stderr for restarted vLLM: %v", err)
					}
				}()

				// Start the new process
				log.Info().Msg("Starting restarted vLLM process")
				if err := newCmd.Start(); err != nil {
					log.Error().Err(err).Msg("Failed to start restarted vLLM process")
					return
				}

				// Update command for next iteration
				cmd = newCmd
				stderrBuf = newStderrBuf

				// Continue the loop to wait on the new process
				continue
			}

			// If process exited cleanly (no error), don't restart
			log.Info().Msg("vLLM process exited cleanly")
			return
		}
	}()

	return cmd, commandLine, nil
}
