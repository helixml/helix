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
	cmd           *exec.Cmd
	cancel        context.CancelFunc
}

type VLLMRuntimeParams struct {
	CacheDir      *string        // Where to store the models
	Port          *int           // If nil, will be assigned a random port
	StartTimeout  *time.Duration // How long to wait for vLLM to start, if nil, will use default
	ContextLength *int64         // Optional: Context length to use for the model
}

func NewVLLMRuntime(_ context.Context, params VLLMRuntimeParams) (*VLLMRuntime, error) {
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

	return &VLLMRuntime{
		version:       "unknown",
		cacheDir:      *params.CacheDir,
		port:          *params.Port,
		startTimeout:  *params.StartTimeout,
		contextLength: contextLength,
	}, nil
}

func (v *VLLMRuntime) Start(ctx context.Context) error {
	log.Debug().Msg("Starting vLLM runtime")

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
	cmd, err := startVLLMCmd(ctx, vllmCommander, v.port, v.cacheDir, v.contextLength)
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
	log.Info().Msg("vLLM has started")

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
	log.Info().Int("pid", v.cmd.Process.Pid).Msg("Stopping vLLM runtime")
	if err := killProcessTree(v.cmd.Process.Pid); err != nil {
		log.Error().Msgf("error stopping vLLM model process: %s", err.Error())
		return err
	}
	log.Info().Msg("vLLM runtime stopped")
	return nil
}

func (v *VLLMRuntime) PullModel(_ context.Context, modelName string, progressFunc func(progress PullProgress) error) error {
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

func (v *VLLMRuntime) Warm(ctx context.Context, model string) error {
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
		return fmt.Errorf("error creating warm-up request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending warm-up request: %w", err)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error warming up model, status: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

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

	for {
		select {
		case <-startCtx.Done():
			return startCtx.Err()
		case <-ticker.C:
			// Try to connect to the vLLM server's health endpoint
			url := fmt.Sprintf("%s/v1/models", v.URL())
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				continue
			}

			client := &http.Client{Timeout: 1 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()

			if resp.StatusCode < 400 {
				return nil
			}
		}
	}
}

func startVLLMCmd(ctx context.Context, commander Commander, port int, cacheDir string, contextLength int64) (*exec.Cmd, error) {
	// Find vLLM on the path
	vllmPath, err := commander.LookPath("python")
	if err != nil {
		return nil, fmt.Errorf("python not found in PATH")
	}
	log.Debug().Str("python_path", vllmPath).Msg("Found python")

	// Prepare vLLM serve command
	log.Debug().Msg("Preparing vLLM serve command")

	// Common arguments for vLLM
	args := []string{
		"-m", "vllm.entrypoints.openai.api_server",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
		"--model", "HuggingFaceH4/zephyr-7b-beta", // Default model, can be overridden at runtime
		"--tensor-parallel-size", "1", // Default to 1 GPU
	}

	// Add context length if specified
	if contextLength > 0 {
		args = append(args, "--max-model-len", fmt.Sprintf("%d", contextLength))
	}

	cmd := commander.CommandContext(ctx, vllmPath, args...)

	// Build environment variables
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"HTTP_PROXY=" + os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"),
		"PYTHONUNBUFFERED=1",
		"CUDA_VISIBLE_DEVICES=0",         // Default to first GPU
		"TRANSFORMERS_CACHE=" + cacheDir, // Where to store the models
		"HF_HOME=" + cacheDir,
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
		if err := cmd.Wait(); err != nil {
			errMsg := string(stderrBuf.Bytes())
			log.Error().Err(err).Str("stderr", errMsg).Int("exit_code", cmd.ProcessState.ExitCode()).Msg("vLLM exited with error")
			return
		}
	}()

	return cmd, nil
}
