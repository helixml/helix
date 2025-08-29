//go:build !windows
// +build !windows

package runner

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"

	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

var (
	axolotlCommander Commander = &RealCommander{}
	_                Runtime   = &AxolotlRuntime{}
)

type AxolotlRuntime struct {
	version        string
	axolotlClient  *AxolotlClient
	port           int
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	startTimeout   time.Duration
	runnerOptions  *Options
	logBuffer      *system.ModelInstanceLogBuffer // Log buffer for this instance
	processTracker *ProcessTracker                // Process tracker for monitoring
	slotID         *uuid.UUID                     // Associated slot ID
}
type AxolotlRuntimeParams struct {
	Port          *int           // If nil, will be assigned a random port
	StartTimeout  *time.Duration // How long to wait for axolotl to start
	RunnerOptions *Options
	LogBuffer     *system.ModelInstanceLogBuffer // Optional: Log buffer for capturing logs
}

func NewAxolotlRuntime(_ context.Context, params AxolotlRuntimeParams) (*AxolotlRuntime, error) {
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
	if params.RunnerOptions == nil {
		return nil, fmt.Errorf("runner options are required")
	}
	return &AxolotlRuntime{
		port:          *params.Port,
		startTimeout:  *params.StartTimeout,
		runnerOptions: params.RunnerOptions,
		logBuffer:     params.LogBuffer,
	}, nil
}

func (d *AxolotlRuntime) Start(ctx context.Context) error {
	log.Debug().Msg("Starting Axolotl runtime")

	// Make sure the port is not already in use
	if isPortInUse(d.port) {
		return fmt.Errorf("port %d is already in use", d.port)
	}

	// Prepare axolotl cmd context (a cancel context)
	log.Debug().Msg("Preparing axolotl context")
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	var err error
	defer func() {
		// If there is an error at any point after this, cancel the context to cancel the cmd
		if err != nil {
			d.cancel()
		}
	}()

	// Start axolotl cmd
	cmd, err := startAxolotlCmd(ctx, axolotlCommander, d.port, d.logBuffer)
	if err != nil {
		return fmt.Errorf("error building axolotl cmd: %w", err)
	}
	d.cmd = cmd

	// Create Axolotl Client
	url, err := url.Parse(fmt.Sprintf("http://localhost:%d", d.port))
	if err != nil {
		return fmt.Errorf("error parsing axolotl url: %w", err)
	}
	d.axolotlClient, err = NewAxolotlClient(ctx, url.String())
	if err != nil {
		return fmt.Errorf("error creating axolotl client: %w", err)
	}

	// Wait for axolotl to be ready
	log.Debug().Str("url", url.String()).Dur("timeout", d.startTimeout).Msg("Waiting for axolotl to start")
	err = d.waitUntilReady(ctx)
	if err != nil {
		return fmt.Errorf("error waiting for axolotl to start: %s", err.Error())
	}
	log.Info().Msg("axolotl has started")

	// Set the version
	version, err := d.axolotlClient.Version(ctx)
	if err != nil {
		return fmt.Errorf("error getting axolotl info: %w", err)
	}
	d.version = version

	return nil
}

func (d *AxolotlRuntime) Stop() error {
	if d.cmd == nil {
		return nil
	}
	log.Info().Msg("Stopping axolotl runtime")
	if err := killProcessTree(d.cmd.Process.Pid); err != nil {
		log.Error().Msgf("error stopping axolotl model process: %s", err.Error())
		return err
	}
	d.cancel()
	log.Info().Msg("axolotl runtime stopped")

	return nil
}

func (d *AxolotlRuntime) URL() string {
	return fmt.Sprintf("http://localhost:%d", d.port)
}

// SetProcessTracker sets the process tracker for monitoring
func (d *AxolotlRuntime) SetProcessTracker(tracker *ProcessTracker, slotID uuid.UUID) {
	d.processTracker = tracker
	d.slotID = &slotID
}

func (d *AxolotlRuntime) Runtime() types.Runtime {
	return types.RuntimeAxolotl
}

func (d *AxolotlRuntime) PullModel(_ context.Context, model string, progress func(PullProgress) error) error {
	clientOptions := system.ClientOptions{
		Host:  d.runnerOptions.APIHost,
		Token: d.runnerOptions.APIToken,
	}
	fileHandler := NewFileHandler(d.runnerOptions.ID, clientOptions, func(response *types.RunnerTaskResponse) {
		log.Debug().Interface("response", response).Msg("File handler event")
	})

	// Extract the session ID from the model name
	_, sessionID, loraDir, err := parseHelixLoraModelName(model)
	if err != nil {
		return fmt.Errorf("error parsing model name: %w", err)
	}

	// Pull model from the control plane
	err = progress(PullProgress{
		Status:    "downloading",
		Completed: 0,
		Total:     100,
	})
	if err != nil {
		return fmt.Errorf("error reporting pull progress: %w", err)
	}

	downloadedLoraDir := buildLocalLoraDir(sessionID)
	log.Debug().
		Str("session_id", sessionID).
		Str("lora_dir", loraDir).
		Str("downloaded_lora_dir", downloadedLoraDir).
		Msg("downloading lora dir")
	err = fileHandler.downloadFolder(sessionID, loraDir, downloadedLoraDir)
	if err != nil {
		return fmt.Errorf("downloading LORA dir: %w", err)
	}
	err = progress(PullProgress{
		Status:    "downloaded",
		Completed: 100,
		Total:     100,
	})
	if err != nil {
		return fmt.Errorf("error reporting pull progress: %w", err)
	}

	return nil
}

func (d *AxolotlRuntime) ListModels(_ context.Context) ([]string, error) {
	return []string{}, nil // TODO: implement
}

func (d *AxolotlRuntime) Warm(ctx context.Context, model string) error {
	// Extract the session ID from the model name
	_, sessionID, _, err := parseHelixLoraModelName(model)
	if err != nil {
		return fmt.Errorf("error parsing model name: %w", err)
	}

	downloadedLoraDir := buildLocalLoraDir(sessionID)
	client, err := CreateOpenaiClient(ctx, fmt.Sprintf("%s/v1", d.URL()))
	if err != nil {
		return fmt.Errorf("error creating openai client: %w", err)
	}

	_, err = client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: downloadedLoraDir,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "Say the ONLY word 'warm'.",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("creating chat completion: %w", err)
	}

	return nil
}

func (d *AxolotlRuntime) Version() string {
	return d.version
}

func startAxolotlCmd(ctx context.Context, commander Commander, port int, logBuffer *system.ModelInstanceLogBuffer) (*exec.Cmd, error) {
	log.Trace().Msg("Preparing Axolotl command")
	cmd := commander.CommandContext(
		ctx,
		"uvicorn", "axolotl_finetune_server:app",
		"--host", "0.0.0.0",
		"--port", strconv.Itoa(port),
	)

	// Set the working directory to the runner dir (which makes relative path stuff easier)
	cmd.Dir = "runner"

	cmd.Env = append(cmd.Env,
		// AXOLOTL RESTORATION NOTE:
		// When axolotl is re-enabled, you'll need to:
		// 1. Install miniconda in base-images/Dockerfile.runner (see git history)
		// 2. Uncomment axolotl installation in base-images/Dockerfile.runner
		// 3. Change base image FROM to winglian/axolotl image
		// 4. Enable this PYTHONPATH:
		// "PYTHONPATH=/workspace/axolotl/src:/root/miniconda3/envs/py3.11/lib/python3.11/site-packages"
		//
		// Currently using clean Ubuntu 24.04 + Python 3.12 base, no miniconda:
		"PYTHONPATH=/workspace/axolotl/src:/usr/lib/python3.12/site-packages",
		// Add the APP_FOLDER environment variable which is required by the old code
		fmt.Sprintf("APP_FOLDER=%s", path.Clean(path.Join("..", "..", "axolotl"))),
		// Set python to be unbuffered so we get logs in real time
		"PYTHONUNBUFFERED=1",
		// Set the log level, which is a name, but must be uppercased
		fmt.Sprintf("LOG_LEVEL=%s", strings.ToUpper(os.Getenv("LOG_LEVEL"))),
	)
	log.Trace().Interface("env", cmd.Env).Msg("axolotl serve command")

	// Prepare stdout and stderr
	log.Trace().Msg("Preparing stdout and stderr")
	cmd.Stdout = os.Stdout
	// this buffer is so we can keep the last 10kb of stderr so if
	// there is an error we can send it to the api
	stderrBuf := system.NewLimitedBuffer(1024 * 10)
	stderrWriters := []io.Writer{os.Stderr, stderrBuf}

	// If we have a log buffer for this instance, add it to the writers
	if logBuffer != nil {
		stderrWriters = append(stderrWriters, logBuffer)
	}
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

	log.Trace().Msg("Starting axolotl")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("error starting axolotl: %w", err)
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			errMsg := string(stderrBuf.Bytes())
			log.Error().Err(err).Str("stderr", errMsg).Int("exit_code", cmd.ProcessState.ExitCode()).Msg("axolotl exited with error")

			return
		}
	}()

	return cmd, nil
}

func (d *AxolotlRuntime) Status(_ context.Context) string {
	if d.version == "" {
		return "not ready"
	}
	return "ready"
}

func (d *AxolotlRuntime) CommandLine() string {
	// Axolotl doesn't expose the command line in a structured way
	// Return a placeholder for now
	return "axolotl serve (command line not captured)"
}

func (d *AxolotlRuntime) waitUntilReady(ctx context.Context) error {
	startCtx, cancel := context.WithTimeout(ctx, d.startTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-startCtx.Done():
			return startCtx.Err()
		case <-ticker.C:
			err := d.axolotlClient.Healthz(ctx)
			if err != nil {
				continue
			}
			return nil
		}
	}
}
