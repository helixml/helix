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
	version       string
	cacheDir      string
	port          int
	startTimeout  time.Duration
	contextLength int64
	model         string
	args          []string
	cmd           *exec.Cmd
	cancel        context.CancelFunc
}

type VLLMRuntimeParams struct {
	CacheDir      *string        // Where to store the models
	Port          *int           // If nil, will be assigned a random port
	StartTimeout  *time.Duration // How long to wait for vLLM to start, if nil, will use default
	ContextLength *int64         // Optional: Context length to use for the model
	Model         *string        // Optional: Model to use
	Args          []string       // Optional: Additional arguments to pass to vLLM
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

	// Log args received
	log.Debug().
		Str("model", model).
		Int64("context_length", contextLength).
		Strs("args", params.Args).
		Msg("NewVLLMRuntime received args")

	return &VLLMRuntime{
		version:       "unknown",
		cacheDir:      *params.CacheDir,
		port:          *params.Port,
		startTimeout:  *params.StartTimeout,
		contextLength: contextLength,
		model:         model,
		args:          params.Args,
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

	// Prepare vLLM cmd context (a cancel context)
	log.Debug().Msg("Preparing vLLM context")
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel
	var err error
	defer func() {
		// If there is an error at any point after this, cancel the context to cancel the cmd
		if err != nil {
			v.cancel()
		}
	}()

	// Start vLLM cmd
	cmd, err := startVLLMCmd(ctx, vllmCommander, v.port, v.cacheDir, v.contextLength, v.model, v.args)
	if err != nil {
		return fmt.Errorf("error building vLLM cmd: %w", err)
	}
	v.cmd = cmd

	// Wait for vLLM to be ready
	log.Debug().Str("url", v.URL()).Dur("timeout", v.startTimeout).Msg("Waiting for vLLM to start")
	err = v.waitUntilVLLMIsReady(ctx, v.startTimeout)
	if err != nil {
		return fmt.Errorf("error waiting for vLLM to start: %s", err.Error())
	}
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

func (v *VLLMRuntime) Stop() error {
	defer v.cancel() // Cancel the context no matter what

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
		Msg("Stopping vLLM runtime")

	if err := killProcessTree(v.cmd.Process.Pid); err != nil {
		log.Error().Msgf("error stopping vLLM model process: %s", err.Error())
		return err
	}
	log.Info().Msg("vLLM runtime stopped")
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

func (v *VLLMRuntime) ListModels(ctx context.Context) ([]string, error) {
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

func (v *VLLMRuntime) Status(_ context.Context) string {
	// vLLM doesn't have a built-in status endpoint like Ollama
	// For now, just return a simple status
	return "running"
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

func startVLLMCmd(ctx context.Context, commander Commander, port int, cacheDir string, contextLength int64, model string, customArgs []string) (*exec.Cmd, error) {
	// Find vLLM on the path
	vllmPath, err := commander.LookPath("python")
	if err != nil {
		return nil, fmt.Errorf("python not found in PATH")
	}
	log.Debug().Str("python_path", vllmPath).Msg("Found python")

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
		return nil, fmt.Errorf("model parameter is required for vLLM runtime")
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
			args = append(args, "--tensor-parallel-size", "1") // Default to 1 GPU
		}
	}

	// Add custom arguments
	args = append(args, customArgs...)

	log.Debug().
		Strs("args", args).
		Strs("custom_args", customArgs).
		Msg("Final vLLM command arguments")

	cmd := commander.CommandContext(ctx, vllmPath, args...)

	// Set only the specific environment variables needed
	// This is more secure than inheriting all parent environment variables
	env := []string{
		// System paths - often needed by Python to find libraries
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")),

		// Python configuration
		"PYTHONUNBUFFERED=1",

		// CUDA configuration
		"CUDA_VISIBLE_DEVICES=0", // Default to first GPU

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

		// Hugging Face authentication
		fmt.Sprintf("HF_TOKEN=%s", os.Getenv("HF_TOKEN")),
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

	log.Debug().Interface("env", cmd.Env).Str("cmd", fmt.Sprintf("%s %s", vllmPath, strings.Join(args, " "))).Msg("vLLM serve command")

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

	log.Debug().Msg("Starting vLLM serve")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("error starting vLLM model instance: %w", err)
	}

	go func() {
		for {
			if err := cmd.Wait(); err != nil {
				errMsg := string(stderrBuf.Bytes())
				log.Error().Err(err).Str("stderr", errMsg).Int("exit_code", cmd.ProcessState.ExitCode()).Msg("vLLM exited with error")

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

				// Set the same environment variables
				newCmd.Env = env
				newCmd.Stdout = os.Stdout

				// Set up stderr handling
				newStderrBuf := system.NewLimitedBuffer(1024 * 10)
				newStderrWriters := []io.Writer{os.Stderr, newStderrBuf}
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

	return cmd, nil
}
