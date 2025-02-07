package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"syscall"
	"time"

	"bufio"
	"strings"

	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var (
	diffusersCommander Commander = &RealCommander{}
)

type DiffusersRuntime struct {
	version         string
	DiffusersClient *DiffusersClient
	cacheDir        string
	port            int
	cmd             *exec.Cmd
	cancel          context.CancelFunc
	startTimeout    time.Duration
}

type DiffusersRuntimeParams struct {
	CacheDir     *string        // Where to store the models
	Port         *int           // If nil, will be assigned a random port
	StartTimeout *time.Duration // How long to wait for ollama to start
}

func NewDiffusersRuntime(_ context.Context, params DiffusersRuntimeParams) (*DiffusersRuntime, error) {
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
	return &DiffusersRuntime{
		cacheDir:     *params.CacheDir,
		port:         *params.Port,
		startTimeout: *params.StartTimeout,
	}, nil
}

func (d *DiffusersRuntime) Start(ctx context.Context) error {
	log.Debug().Msg("Starting Diffusers runtime")

	// Make sure the port is not already in use
	if isPortInUse(d.port) {
		return fmt.Errorf("port %d is already in use", d.port)
	}

	// Check if the cache dir exists, if not create it
	if _, err := os.Stat(d.cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(d.cacheDir, 0755); err != nil {
			return fmt.Errorf("error creating cache dir: %s", err.Error())
		}
	}
	// Check that the cache dir is writable
	if _, err := os.Stat(d.cacheDir); os.IsPermission(err) {
		return fmt.Errorf("cache dir is not writable: %s", d.cacheDir)
	}

	// Prepare ollama cmd context (a cancel context)
	log.Debug().Msg("Preparing Diffusers context")
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	var err error
	defer func() {
		// If there is an error at any point after this, cancel the context to cancel the cmd
		if err != nil {
			d.cancel()
		}
	}()

	// Start ollama cmd
	cmd, err := startDiffusersCmd(ctx, diffusersCommander, d.port, d.cacheDir)
	if err != nil {
		return fmt.Errorf("error building diffusers cmd: %w", err)
	}
	d.cmd = cmd

	// Create Diffusers Client
	url, err := url.Parse(fmt.Sprintf("http://localhost:%d", d.port))
	if err != nil {
		return fmt.Errorf("error parsing diffusers url: %w", err)
	}
	d.DiffusersClient, err = NewDiffusersClient(ctx, url.String())
	if err != nil {
		return fmt.Errorf("error creating diffusers client: %w", err)
	}

	// Wait for diffusers to be ready
	log.Debug().Str("url", url.String()).Dur("timeout", d.startTimeout).Msg("Waiting for diffusers to start")
	err = d.waitUntilDiffusersIsReady(ctx)
	if err != nil {
		return fmt.Errorf("error waiting for diffusers to start: %s", err.Error())
	}
	log.Info().Msg("diffusers has started")

	// Set the version
	version, err := d.DiffusersClient.Version(ctx)
	if err != nil {
		return fmt.Errorf("error getting diffusers info: %w", err)
	}
	d.version = version

	return nil
}

func (d *DiffusersRuntime) Stop() error {
	if d.cmd == nil {
		return nil
	}
	log.Info().Msg("Stopping Diffusers runtime")
	if err := killProcessTree(d.cmd.Process.Pid); err != nil {
		log.Error().Msgf("error stopping Diffusers model process: %s", err.Error())
		return err
	}
	d.cancel()
	log.Info().Msg("Diffusers runtime stopped")

	return nil
}

func (d *DiffusersRuntime) PullModel(ctx context.Context, modelName string, _ func(progress PullProgress) error) error {
	return d.DiffusersClient.Pull(ctx, modelName)
}

func (d *DiffusersRuntime) Warm(ctx context.Context, modelName string) error {
	return d.DiffusersClient.Warm(ctx, modelName)
}

func (d *DiffusersRuntime) URL() string {
	return fmt.Sprintf("http://localhost:%d", d.port)
}

func (d *DiffusersRuntime) Runtime() types.Runtime {
	return types.RuntimeDiffusers
}

func (d *DiffusersRuntime) Version() string {
	return d.version
}

func startDiffusersCmd(ctx context.Context, commander Commander, port int, cacheDir string) (*exec.Cmd, error) {
	// Find uv on the path
	uvPath, err := commander.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("uv not found in PATH")
	}
	log.Trace().Str("uv_path", uvPath).Msg("Found uv")

	log.Trace().Msg("Preparing Diffusers command")
	cmd := exec.CommandContext(
		ctx,
		"uv", "run", "--frozen", "--no-dev", // Don't install dev dependencies
		"uvicorn", "main:app",
		"--host", "0.0.0.0",
		"--port", strconv.Itoa(port),
	)

	// Set the working directory to the runner dir (which makes relative path stuff easier)
	cmd.Dir = "/workspace/helix/runner/helix-diffusers"

	cmd.Env = append(cmd.Env,
		fmt.Sprintf("CACHE_DIR=%s", path.Join(cacheDir, "hub")), // Mimic the diffusers library's default cache dir
		// Add the HF_TOKEN environment variable which is required by the diffusers library
		fmt.Sprintf("HF_TOKEN=%s", os.Getenv("HF_TOKEN")),
		// Set python to be unbuffered so we get logs in real time
		"PYTHONUNBUFFERED=1",
	)
	log.Trace().Interface("env", cmd.Env).Msg("Diffusers serve command")

	// Prepare stdout and stderr
	log.Trace().Msg("Preparing stdout and stderr")
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

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	log.Trace().Msg("Starting Diffusers")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("error starting Diffusers: %w", err)
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			errMsg := string(stderrBuf.Bytes())
			log.Error().Err(err).Str("stderr", errMsg).Int("exit_code", cmd.ProcessState.ExitCode()).Msg("Diffusers exited with error")

			return
		}
	}()

	return cmd, nil
}

func (d *DiffusersRuntime) waitUntilDiffusersIsReady(ctx context.Context) error {
	startCtx, cancel := context.WithTimeout(ctx, d.startTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-startCtx.Done():
			return startCtx.Err()
		case <-ticker.C:
			err := d.DiffusersClient.Healthz(ctx)
			if err != nil {
				continue
			}
			return nil
		}
	}
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type DiffusersClient struct {
	client HTTPDoer
	url    string
}

func NewDiffusersClient(_ context.Context, url string) (*DiffusersClient, error) {
	return &DiffusersClient{
		client: http.DefaultClient,
		url:    url,
	}, nil
}

func (d *DiffusersClient) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", d.url+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers healthz returned status %d", resp.StatusCode)
	}
	return nil
}

type DiffusersVersionResponse struct {
	Version string `json:"version"`
}

func (d *DiffusersClient) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", d.url+"/version", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("diffusers version returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}
	var versionResp DiffusersVersionResponse
	if err := json.Unmarshal(body, &versionResp); err != nil {
		return "", fmt.Errorf("error unmarshalling response: %w", err)
	}
	return versionResp.Version, nil
}

type DiffusersPullRequest struct {
	Model string `json:"model"`
}

func (d *DiffusersClient) Pull(ctx context.Context, modelName string) error {
	pullReq := DiffusersPullRequest{
		Model: modelName,
	}
	body, err := json.Marshal(pullReq)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.url+"/pull", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers pull returned status %d", resp.StatusCode)
	}
	return nil
}

type DiffusersWarmRequest struct {
	Model string `json:"model"`
}

func (d *DiffusersClient) Warm(ctx context.Context, modelName string) error {
	warmReq := DiffusersWarmRequest{
		Model: modelName,
	}
	body, err := json.Marshal(warmReq)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", d.url+"/warm", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers warm returned status %d", resp.StatusCode)
	}
	return nil
}

type GenerateStreamingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size"`
	N      int    `json:"n"`
}

func (d *DiffusersClient) GenerateStreaming(ctx context.Context, prompt string, callback func(types.HelixImageGenerationUpdate) error) error {
	// Create request body
	body, err := json.Marshal(GenerateStreamingRequest{
		Prompt: prompt,
	})
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", d.url+"/v1/images/generations/stream", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("diffusers generate streaming returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines or non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		log.Debug().Str("line", line).Msg("Received line")
		// Extract the JSON payload after "data: "
		jsonData := strings.TrimPrefix(line, "data: ")

		// Skip keep-alive messages
		if jsonData == "" || jsonData == ":keep-alive" {
			continue
		}

		// Parse the JSON update
		var update types.HelixImageGenerationUpdate
		if err := json.Unmarshal([]byte(jsonData), &update); err != nil {
			return fmt.Errorf("error parsing response: %w", err)
		}

		// Call the callback with the update
		if err := callback(update); err != nil {
			return fmt.Errorf("callback error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	return nil
}
