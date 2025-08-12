//go:build !windows
// +build !windows

package runner

import (
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
	version       string
	cacheDir      string
	port          int
	startTimeout  time.Duration
	contextLength int64
	model         string
	args          []string
	ollamaClient  *api.Client
	cmd           *exec.Cmd
	cancel        context.CancelFunc
	gpuIndex      int   // Primary GPU index for single-GPU models
	gpuIndices    []int // All GPU indices for multi-GPU models

	// GPU allocation restart tracking
	restartAttempts   int                // Number of restart attempts due to GPU allocation issues
	monitoringStarted bool               // Track if monitoring goroutine is already running
	monitoringCancel  context.CancelFunc // Cancel function for monitoring goroutine
	originalCtx       context.Context    // Context passed to most recent Start() call
	started           bool               // Track if runtime is currently started
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
	CacheDir      *string        // Where to store the models
	Port          *int           // If nil, will be assigned a random port
	StartTimeout  *time.Duration // How long to wait for ollama to start, if nil, will use default
	ContextLength *int64         // Optional: Context length to use for the model
	Model         *string        // Optional: Model to use
	Args          []string       // Optional: Additional arguments to pass to Ollama
	GPUIndex      *int           // Optional: Primary GPU index for single-GPU models
	GPUIndices    []int          // Optional: GPU indices for multi-GPU models (overrides GPUIndex)
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
	var gpuIndex int = 0
	var gpuIndices []int

	// Multi-GPU setup takes precedence over single-GPU
	if len(params.GPUIndices) > 0 {
		gpuIndices = params.GPUIndices
		gpuIndex = gpuIndices[0] // Use first GPU as primary
	} else if params.GPUIndex != nil {
		gpuIndex = *params.GPUIndex
		gpuIndices = []int{gpuIndex}
	}

	// Log GPU configuration
	log.Debug().
		Str("model", model).
		Int64("context_length", contextLength).
		Strs("args", params.Args).
		Int("gpu_index", gpuIndex).
		Ints("gpu_indices", gpuIndices).
		Msg("NewOllamaRuntime GPU configuration")

	return &OllamaRuntime{
		version:       "unknown",
		cacheDir:      *params.CacheDir,
		port:          *params.Port,
		startTimeout:  *params.StartTimeout,
		contextLength: contextLength,
		model:         model,
		args:          params.Args,
		gpuIndex:      gpuIndex,
		gpuIndices:    gpuIndices,
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
	cmd, err := startOllamaCmd(ctx, ollamaCommander, i.port, i.cacheDir, i.contextLength, i.gpuIndex, i.gpuIndices)
	if err != nil {
		return fmt.Errorf("error building ollama cmd: %w", err)
	}
	i.cmd = cmd

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

func startOllamaCmd(ctx context.Context, commander Commander, port int, cacheDir string, contextLength int64, gpuIndex int, gpuIndices []int) (*exec.Cmd, error) {
	// Find ollama on the path
	ollamaPath, err := commander.LookPath("ollama")
	if err != nil {
		return nil, fmt.Errorf("ollama not found in PATH")
	}
	log.Debug().Str("ollama_path", ollamaPath).Msg("Found ollama")

	// Prepare ollama serve command
	log.Debug().Msg("Preparing ollama serve command")
	cmd := commander.CommandContext(ctx, ollamaPath, "serve")
	ollamaHost := fmt.Sprintf("127.0.0.1:%d", port)

	// Build environment variables
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"HTTP_PROXY=" + os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"),
		"OLLAMA_KEEP_ALIVE=-1",
		"OLLAMA_MAX_LOADED_MODELS=1",
		"OLLAMA_NUM_PARALLEL=1",
		"OLLAMA_FLASH_ATTENTION=1",
		"OLLAMA_KV_CACHE_TYPE=q8_0",
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

	// Prepare stdout and stderr
	log.Debug().Msg("Preparing stdout and stderr")
	cmd.Stdout = os.Stdout
	// this buffer is so we can keep the last 10kb of stderr so if
	// there is an error we can send it to the api
	stderrBuf := system.NewLimitedBuffer(1024 * 10)
	stderrWriters := []io.Writer{os.Stderr, stderrBuf}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		_, err := io.Copy(io.MultiWriter(stderrWriters...), stderrPipe)
		if err != nil {
			log.Error().Msgf("Error copying stderr: %v", err)
		}
	}()

	log.Debug().Msg("Starting ollama serve")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("error starting Ollama model instance: %w", err)
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			errMsg := string(stderrBuf.Bytes())
			log.Error().Err(err).Str("stderr", errMsg).Int("exit_code", cmd.ProcessState.ExitCode()).Msg("Ollama exited with error")

			return
		}
	}()

	return cmd, nil
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
