//go:build !windows
// +build !windows

package runner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/ollama/ollama/api"
	"github.com/rs/zerolog/log"
)

var (
	ollamaCommander Commander = &RealCommander{}
	_               Runtime   = &OllamaRuntime{}
)

type OllamaRuntime struct {
	version        string
	cacheDir       string
	port           int
	startTimeout   time.Duration
	contextLength  int64
	model          string
	args           []string
	numParallel    int // Number of parallel requests
	ollamaClient   *api.Client
	cmd            *exec.Cmd
	stderrBuf      *system.LimitedBuffer // Buffer for capturing stderr output
	cancel         context.CancelFunc
	gpuIndex       int                            // Primary GPU index for single-GPU models
	gpuIndices     []int                          // All GPU indices for multi-GPU models
	logBuffer      *system.ModelInstanceLogBuffer // Log buffer for this instance
	processTracker *ProcessTracker                // Process tracker for monitoring
	slotID         *uuid.UUID                     // Associated slot ID

	// GPU allocation restart tracking
	restartAttempts   int                // Number of restart attempts due to GPU allocation issues
	monitoringStarted bool               // Track if monitoring goroutine is already running
	monitoringCancel  context.CancelFunc // Cancel function for monitoring goroutine
	originalCtx       context.Context    // Context passed to most recent Start() call
	started           bool               // Track if runtime is currently started

	// Crash callback - called when the Ollama process exits unexpectedly
	onCrash func(stderr string)
}

type Model struct {
	Name              string    `json:"model"`
	ModifiedAt        time.Time `json:"modified_at"`
	Size              int64     `json:"size"`
	Digest            string    `json:"digest"`
	ParentModel       string    `json:"parent_model"`
	Format            string    `json:"format"`
	Family            string    `json:"family"`
	Families          []string  `json:"families"`
	ParameterSize     string    `json:"parameter_size"`
	QuantizationLevel string    `json:"quantization_level"`
}

type OllamaRuntimeParams struct {
	CacheDir      *string                        // Where to store the models
	Port          *int                           // If nil, will be assigned a random port
	StartTimeout  *time.Duration                 // How long to wait for ollama to start, if nil, will use default
	ContextLength *int64                         // Optional: Context length to use for the model
	Model         *string                        // Optional: Model to use
	Args          []string                       // Optional: Additional arguments to pass to Ollama
	NumParallel   *int                           // Optional: Number of parallel requests (default 1)
	GPUIndex      *int                           // Optional: Primary GPU index for single-GPU models
	GPUIndices    []int                          // Optional: GPU indices for multi-GPU models (overrides GPUIndex)
	LogBuffer     *system.ModelInstanceLogBuffer // Optional: Log buffer for capturing logs
	OnCrash       func(stderr string)            // Optional: Callback when Ollama process crashes
}

func NewOllamaRuntime(_ context.Context, params OllamaRuntimeParams) (*OllamaRuntime, error) {
	defaultCacheDir := os.TempDir()
	if params.CacheDir == nil {
		params.CacheDir = &defaultCacheDir
	}

	defaultStartTimeout := 30 * time.Second
	if params.StartTimeout == nil {
		params.StartTimeout = &defaultStartTimeout
	}
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

	// Set model if provided
	var model string
	if params.Model != nil {
		model = *params.Model
		log.Debug().Str("model", model).Msg("Using model")
	}

	// Extract GPU configuration
	var gpuIndex int
	var gpuIndices []int

	// Multi-GPU setup takes precedence over single-GPU
	if len(params.GPUIndices) > 0 {
		gpuIndices = params.GPUIndices
		gpuIndex = gpuIndices[0] // Use first GPU as primary
	} else if params.GPUIndex != nil {
		gpuIndex = *params.GPUIndex
		gpuIndices = []int{gpuIndex}
	}

	// Set numParallel with default
	numParallel := 1
	if params.NumParallel != nil {
		numParallel = *params.NumParallel
		log.Info().
			Str("model", model).
			Int("num_parallel_from_params", numParallel).
			Msg("🔍 TRACING: NewOllamaRuntime received NumParallel from params")
	} else {
		log.Warn().
			Str("model", model).
			Int("num_parallel_default", numParallel).
			Msg("🔍 TRACING: NewOllamaRuntime using default NumParallel (params.NumParallel was nil)")
	}

	log.Debug().
		Str("model", model).
		Int64("context_length", contextLength).
		Int("num_parallel", numParallel).
		Strs("args", params.Args).
		Int("gpu_index", gpuIndex).
		Ints("gpu_indices", gpuIndices).
		Msg("NewOllamaRuntime configuration")

	return &OllamaRuntime{
		version:       "unknown",
		cacheDir:      *params.CacheDir,
		port:          *params.Port,
		startTimeout:  *params.StartTimeout,
		contextLength: contextLength,
		model:         model,
		args:          params.Args,
		numParallel:   numParallel,
		gpuIndex:      gpuIndex,
		gpuIndices:    gpuIndices,
		logBuffer:     params.LogBuffer,
		onCrash:       params.OnCrash,
	}, nil
}

func (i *OllamaRuntime) Start(ctx context.Context) error {
	log.Debug().Msg("Starting Ollama runtime")

	// Prevent multiple Start() calls without Stop()
	if i.started {
		return fmt.Errorf("runtime is already started, call Stop() first")
	}

	// Make sure the port is not already in use
	if isPortInUse(i.port) {
		return fmt.Errorf("port %d is already in use", i.port)
	}

	// Check if the cache dir exists, if not create it
	if _, err := os.Stat(i.cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(i.cacheDir, 0755); err != nil {
			return fmt.Errorf("error creating cache dir: %s", err.Error())
		}
	}
	// Check that the cache dir is writable
	if _, err := os.Stat(i.cacheDir); os.IsPermission(err) {
		return fmt.Errorf("cache dir is not writable: %s", i.cacheDir)
	}

	// Store context from most recent Start() call for monitoring and restarts
	i.originalCtx = ctx
	originalCtx := i.originalCtx

	// Prepare ollama cmd context (a cancel context)
	log.Debug().Msg("Preparing ollama context")
	ctx, cancel := context.WithCancel(ctx)
	i.cancel = cancel
	var err error
	defer func() {
		// If there is an error at any point after this, cancel the context to cancel the cmd
		if err != nil {
			i.cancel()
		}
	}()

	// Start ollama cmd
	cmd, stderrBuf, err := startOllamaCmd(ctx, ollamaCommander, i.port, i.cacheDir, i.contextLength, i.numParallel, i.gpuIndex, i.gpuIndices, i.logBuffer, i.onCrash)
	if err != nil {
		return fmt.Errorf("error building ollama cmd: %w", err)
	}
	i.cmd = cmd
	i.stderrBuf = stderrBuf

	// Register the process with the tracker if available
	if i.processTracker != nil && i.slotID != nil && i.cmd != nil && i.cmd.Process != nil {
		i.processTracker.RegisterProcess(i.cmd.Process.Pid, *i.slotID, i.model, fmt.Sprintf("ollama serve (port %d)", i.port))
		log.Info().
			Int("pid", i.cmd.Process.Pid).
			Str("slot_id", i.slotID.String()).
			Str("model", i.model).
			Msg("PROCESS_TRACKER: Registered Ollama process")
	}

	// NOTE: We monitor for crashes via stderr parsing in startOllamaCmd(), not via cmd.Wait()
	// This is because Ollama's subprocess (llama runner) can crash while the main process stays alive

	// Create ollama client
	url, err := url.Parse(fmt.Sprintf("http://localhost:%d", i.port))
	if err != nil {
		return fmt.Errorf("error parsing ollama url: %w", err)
	}
	log.Debug().Str("url", url.String()).Msg("Creating Ollama client")
	ollamaClient := api.NewClient(url, http.DefaultClient)
	i.ollamaClient = ollamaClient

	// Wait for ollama to be ready
	log.Debug().Str("url", url.String()).Dur("timeout", i.startTimeout).Msg("Waiting for Ollama to start")
	err = i.waitUntilOllamaIsReady(ctx, i.startTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for Ollama to start: %s", err.Error())
	}
	log.Info().Msg("Ollama has started")

	// Set the version
	version, err := i.ollamaClient.Version(ctx)
	if err != nil {
		return fmt.Errorf("error getting ollama info: %w", err)
	}
	i.version = version

	// Start internal GPU allocation monitoring with original context
	// (not the child context that gets canceled by Stop)
	// Only start monitoring once to prevent multiple goroutines
	if !i.monitoringStarted {
		i.startInternalGPUMonitoring(originalCtx)
		i.monitoringStarted = true
	}

	// Mark runtime as started
	i.started = true

	return nil
}

func (i *OllamaRuntime) URL() string {
	return fmt.Sprintf("http://localhost:%d", i.port)
}

// SetProcessTracker sets the process tracker for monitoring
func (i *OllamaRuntime) SetProcessTracker(tracker *ProcessTracker, slotID uuid.UUID) {
	i.processTracker = tracker
	i.slotID = &slotID
}

func (i *OllamaRuntime) Stop() error {
	// Stop monitoring first
	if i.monitoringCancel != nil {
		i.monitoringCancel()
	}

	// Clear original context so future Start() calls can use a new context
	i.originalCtx = nil

	// Mark runtime as stopped
	i.started = false

	// Then stop the Ollama process
	return i.stopOllamaProcessOnly()
}

// stopOllamaProcessOnly stops only the Ollama process, not the monitoring.
// Used internally during restarts where we want monitoring to continue.
func (i *OllamaRuntime) stopOllamaProcessOnly() error {
	defer i.cancel() // Cancel the context no matter what

	if i.cmd == nil {
		return nil
	}
	log.Info().Int("pid", i.cmd.Process.Pid).Msg("Stopping Ollama runtime")
	if err := killProcessTree(i.cmd.Process.Pid); err != nil {
		log.Error().Msgf("error stopping Ollama model process: %s", err.Error())
		return err
	}

	// Mark as stopped so internal restarts can call Start() again
	i.started = false

	log.Info().Msg("Ollama runtime stopped")
	return nil
}

func (i *OllamaRuntime) PullModel(ctx context.Context, modelName string, pullProgressFunc func(progress PullProgress) error) error {
	if i.ollamaClient == nil {
		return fmt.Errorf("ollama client not initialized")
	}

	// If no model name is provided, use the configured model
	if modelName == "" && i.model != "" {
		modelName = i.model
	}

	// Validate model name
	if modelName == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	log.Info().Msgf("Pulling model: %s", modelName)
	err := i.ollamaClient.Pull(ctx, &api.PullRequest{
		Model: modelName,
	}, func(progress api.ProgressResponse) error {
		return pullProgressFunc(PullProgress{
			Status:    progress.Status,
			Completed: progress.Completed,
			Total:     progress.Total,
		})
	})
	if err != nil {
		return fmt.Errorf("error pulling model: %w", err)
	}
	log.Info().Msgf("Finished pulling model: %s", modelName)
	return nil
}

func (i *OllamaRuntime) ListModels(ctx context.Context) ([]string, error) {
	models, err := i.ollamaClient.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing models: %w", err)
	}
	var resp []string
	for _, model := range models.Models {
		resp = append(resp, model.Name)
	}
	return resp, nil
}

func (i *OllamaRuntime) Warm(ctx context.Context, model string) error {
	// If no model is provided, use the configured model
	if model == "" && i.model != "" {
		model = i.model
	}

	// Validate model
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	err := i.ollamaClient.Chat(ctx, &api.ChatRequest{
		Model: model,
		Messages: []api.Message{
			{
				Role:    "user",
				Content: "Say the word 'warm'.",
			},
		},
	}, func(_ api.ChatResponse) error {
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "does not support chat") {
			_, err = i.ollamaClient.Embeddings(ctx, &api.EmbeddingRequest{
				Model:  model,
				Prompt: "Hello, world!",
			})
		}
	}
	return err
}

func (i *OllamaRuntime) Runtime() types.Runtime {
	return types.RuntimeOllama
}

func (i *OllamaRuntime) Version() string {
	return i.version
}

func (i *OllamaRuntime) Status(ctx context.Context) string {
	ps, err := i.ollamaClient.ListRunning(ctx)
	if err != nil {
		return fmt.Sprintf("error getting ollama status: %s", err.Error())
	}
	buf := bytes.NewBufferString("")
	for _, m := range ps.Models {
		sizeCPU := m.Size - m.SizeVRAM
		cpuPercent := math.Round(float64(sizeCPU) / float64(m.Size) * 100)
		gpuRAM := float64(m.SizeVRAM) / float64(model.GB)
		procStr := fmt.Sprintf("%s %d%%/%d%% CPU/GPU (%.2fGB GPU RAM)", m.Name, int(cpuPercent), int(100-cpuPercent), gpuRAM)
		buf.WriteString(fmt.Sprintf(" %s", procStr))
		buf.WriteString("\n")
	}
	return buf.String()
}

func (i *OllamaRuntime) CommandLine() string {
	// Ollama doesn't expose the command line in a structured way
	// Return a placeholder for now
	return "ollama serve (command line not captured)"
}

// CheckGPUAllocation checks if the model is fully allocated to GPU.
// Returns true if fully allocated (CPU% == 0), false otherwise.
func (i *OllamaRuntime) CheckGPUAllocation(ctx context.Context) (bool, error) {
	if i.ollamaClient == nil {
		return false, fmt.Errorf("ollama client not initialized")
	}

	ps, err := i.ollamaClient.ListRunning(ctx)
	if err != nil {
		return false, fmt.Errorf("error getting ollama status: %w", err)
	}

	for _, m := range ps.Models {
		sizeCPU := m.Size - m.SizeVRAM
		cpuPercent := math.Round(float64(sizeCPU) / float64(m.Size) * 100)

		// If any model has CPU allocation > 0%, it's not fully allocated to GPU
		if cpuPercent > 0 {
			log.Warn().
				Str("model", m.Name).
				Int("cpu_percent", int(cpuPercent)).
				Int("gpu_percent", int(100-cpuPercent)).
				Msg("Model not fully allocated to GPU")
			return false, nil
		}
	}

	return true, nil
}

// RestartIfNotFullyAllocated checks GPU allocation and restarts the runtime if needed.
// Returns true if restart was performed, false otherwise.
// Stops attempting restarts after 3 tries to prevent infinite loops.
func (i *OllamaRuntime) RestartIfNotFullyAllocated(ctx context.Context) (bool, error) {
	fullyAllocated, err := i.CheckGPUAllocation(ctx)
	if err != nil {
		return false, fmt.Errorf("error checking GPU allocation: %w", err)
	}

	if fullyAllocated {
		// Everything is fine, reset restart counter and no restart needed
		if i.restartAttempts > 0 {
			log.Info().
				Int("previous_attempts", i.restartAttempts).
				Msg("GPU allocation successful, resetting restart counter")
			i.restartAttempts = 0
		}
		return false, nil
	}

	// Check if we've exceeded the maximum restart attempts
	const maxRestartAttempts = 3
	if i.restartAttempts >= maxRestartAttempts {
		log.Warn().
			Int("restart_attempts", i.restartAttempts).
			Int("max_attempts", maxRestartAttempts).
			Msg("Ollama model not fully allocated to GPU after maximum restart attempts - giving up")
		return false, nil
	}

	i.restartAttempts++
	log.Info().
		Int("attempt", i.restartAttempts).
		Int("max_attempts", maxRestartAttempts).
		Msg("Model not fully allocated to GPU, restarting Ollama runtime")

	// Stop only the Ollama process (keep monitoring alive)
	if err := i.stopOllamaProcessOnly(); err != nil {
		log.Error().Err(err).Msg("Error stopping Ollama runtime during restart")
		// Continue with restart attempt even if stop failed
	}

	// Give a brief pause to allow GPU memory to be deallocated
	time.Sleep(2 * time.Second)

	// Start the runtime again with the original context (maintains caller's cancellation contract)
	// Check if we were stopped externally during the restart process
	if i.originalCtx == nil {
		log.Info().Msg("External stop detected during restart, aborting restart")
		return false, nil
	}

	if err := i.Start(i.originalCtx); err != nil {
		return true, fmt.Errorf("error restarting Ollama runtime: %w", err)
	}

	log.Info().
		Int("attempt", i.restartAttempts).
		Msg("Ollama runtime restarted successfully")
	return true, nil
}

func (i *OllamaRuntime) waitUntilOllamaIsReady(ctx context.Context, startTimeout time.Duration) error {
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-startCtx.Done():
			return startCtx.Err()
		case <-ticker.C:
			err := i.ollamaClient.Heartbeat(ctx)
			if err != nil {
				continue
			}
			return nil
		}
	}
}

func startOllamaCmd(ctx context.Context, commander Commander, port int, cacheDir string, contextLength int64, numParallel int, _ int, gpuIndices []int, logBuffer *system.ModelInstanceLogBuffer, crashCallback func(stderr string)) (*exec.Cmd, *system.LimitedBuffer, error) {
	// Find ollama on the path
	ollamaPath, err := commander.LookPath("ollama")
	if err != nil {
		return nil, nil, fmt.Errorf("ollama not found in PATH")
	}
	log.Debug().Str("ollama_path", ollamaPath).Msg("Found ollama")

	// Prepare ollama serve command
	log.Debug().Msg("Preparing ollama serve command")
	cmd := commander.CommandContext(ctx, ollamaPath, "serve")
	ollamaHost := fmt.Sprintf("127.0.0.1:%d", port)

	// Build environment variables
	log.Info().
		Int("num_parallel_being_set", numParallel).
		Str("env_var_value", fmt.Sprintf("OLLAMA_NUM_PARALLEL=%d", numParallel)).
		Msg("🔍 TRACING: Setting OLLAMA_NUM_PARALLEL environment variable in startOllamaCmd")

	// "OLLAMA_KV_CACHE_TYPE=" + memory.DefaultKVCacheType, # Disable as per https://github.com/ollama/ollama/issues/11671#issuecomment-3156327782
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"HTTP_PROXY=" + os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"),
		"OLLAMA_KEEP_ALIVE=-1",
		"OLLAMA_MAX_LOADED_MODELS=1",
		fmt.Sprintf("OLLAMA_NUM_PARALLEL=%d", numParallel),
		"OLLAMA_FLASH_ATTENTION=1",
		"OLLAMA_HOST=" + ollamaHost, // Bind on localhost with random port
		"OLLAMA_MODELS=" + cacheDir, // Where to store the models
	}

	// Add GPU configuration for multi-GPU scheduling
	if len(gpuIndices) > 0 {
		cudaDevices := formatGPUIndicesForOllama(gpuIndices)
		env = append(env, "CUDA_VISIBLE_DEVICES="+cudaDevices)
		log.Info().
			Ints("gpu_indices", gpuIndices).
			Str("cuda_visible_devices", cudaDevices).
			Msg("Configuring Ollama with selected GPUs")
	} else {
		// CPU-only mode or development mode
		if os.Getenv("DEVELOPMENT_CPU_ONLY") == "true" {
			env = append(env, "CUDA_VISIBLE_DEVICES=-1")
			log.Info().Msg("Configuring Ollama for CPU-only mode")
		} else {
			// Default to all available GPUs if no specific selection
			log.Debug().Msg("Ollama will use all available GPUs (no CUDA_VISIBLE_DEVICES set)")
		}
	}

	// Add context length configuration if provided
	if contextLength > 0 {
		env = append(env, fmt.Sprintf("OLLAMA_CONTEXT_LENGTH=%d", contextLength))
		log.Debug().Int64("context_length", contextLength).Msg("Setting Ollama context length")
	}

	cmd.Env = env
	log.Debug().Interface("env", cmd.Env).Msg("Ollama serve command")

	// Extra logging to trace OLLAMA_NUM_PARALLEL specifically
	for _, envVar := range env {
		if len(envVar) > 18 && envVar[:18] == "OLLAMA_NUM_PARALLEL" {
			log.Info().
				Str("env_var", envVar).
				Msg("🔍 TRACING: OLLAMA_NUM_PARALLEL environment variable set for ollama serve")
			break
		}
	}

	// Prepare stdout and stderr
	log.Debug().Msg("Preparing stdout and stderr")
	cmd.Stdout = os.Stdout
	// this buffer is so we can keep the last 10kb of stderr so if
	// there is an error we can send it to the api
	stderrBuf := system.NewLimitedBuffer(1024 * 10)
	// Only write to buffer and logBuffer, not to os.Stderr to reduce log pollution
	stderrWriters := []io.Writer{stderrBuf, os.Stderr}

	// If we have a log buffer for this instance, add it to the writers
	if logBuffer != nil {
		stderrWriters = append(stderrWriters, logBuffer)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	// Create a tee reader to both write to buffers AND scan for crashes
	stderrReader := io.TeeReader(stderrPipe, io.MultiWriter(stderrWriters...))

	go func() {
		scanner := bufio.NewScanner(stderrReader)

		for scanner.Scan() {
			line := scanner.Text()

			// Check for llama runner crash indicator
			// Example: "llama runner terminated" error="exit status 2"
			if strings.Contains(line, "llama runner terminated") &&
				strings.Contains(line, "exit status") {
				log.Error().
					Str("stderr_line", line).
					Msg("Detected llama runner subprocess crash in Ollama stderr")

				// Get accumulated stderr for context
				errMsg := string(stderrBuf.Bytes())

				log.Warn().
					Str("stderr_preview", func() string {
						if len(errMsg) > 500 {
							return "..." + errMsg[len(errMsg)-500:]
						}
						return errMsg
					}()).
					Msg("Ollama subprocess crashed, triggering cleanup")

				// Call the crash callback if configured
				if crashCallback != nil {
					crashCallback(errMsg)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			log.Error().Err(err).Msg("Error scanning Ollama stderr")
		}
	}()

	log.Debug().Msg("Starting ollama serve")
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("error starting Ollama model instance: %w", err)
	}

	return cmd, stderrBuf, nil
}

// startInternalGPUMonitoring starts a goroutine that periodically checks GPU allocation
// and restarts the runtime if models are not fully allocated to GPU.
func (i *OllamaRuntime) startInternalGPUMonitoring(ctx context.Context) {
	// Create monitoring-specific context that can be canceled independently
	monitoringCtx, monitoringCancel := context.WithCancel(ctx)
	i.monitoringCancel = monitoringCancel

	go func() {
		ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
		defer ticker.Stop()
		defer func() {
			// Reset monitoring flag when goroutine exits so monitoring can be restarted
			i.monitoringStarted = false
			i.monitoringCancel = nil
		}()

		log.Debug().Msg("Starting internal GPU allocation monitoring for Ollama runtime")

		for {
			select {
			case <-monitoringCtx.Done():
				log.Debug().Msg("GPU allocation monitoring stopped")
				return
			case <-ticker.C:
				// Use original context for checking, not the monitoring context
				i.checkAndRestartIfNeeded(ctx)
			}
		}
	}()
}

// checkAndRestartIfNeeded checks GPU allocation and restarts if needed
func (i *OllamaRuntime) checkAndRestartIfNeeded(ctx context.Context) {
	restarted, err := i.RestartIfNotFullyAllocated(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Msg("Error checking/restarting Ollama runtime for GPU allocation")
		return
	}

	if restarted {
		log.Info().Msg("Ollama runtime was restarted due to improper GPU allocation")
	}
}

// formatGPUIndicesForOllama converts a slice of GPU indices to a comma-separated string for CUDA_VISIBLE_DEVICES
func formatGPUIndicesForOllama(gpuIndices []int) string {
	if len(gpuIndices) == 0 {
		return "0" // Default to GPU 0
	}

	var indices []string
	for _, idx := range gpuIndices {
		indices = append(indices, fmt.Sprintf("%d", idx))
	}
	return strings.Join(indices, ",")
}
