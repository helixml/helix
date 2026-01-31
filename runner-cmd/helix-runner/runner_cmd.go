package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/runner"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/copydir"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type RunnerOptions struct {
	Runner  runner.Options
	Janitor config.Janitor
}

func newRunnerOptions() *RunnerOptions {
	return &RunnerOptions{
		Runner: runner.Options{
			ID:                           getDefaultServeOptionString("RUNNER_ID", ""),
			APIHost:                      getDefaultServeOptionString("API_HOST", ""),
			APIToken:                     getDefaultServeOptionString("API_TOKEN", ""),
			MemoryBytes:                  uint64(getDefaultServeOptionInt("MEMORY_BYTES", 0)),
			MemoryString:                 getDefaultServeOptionString("MEMORY_STRING", ""),
			ReportStateDelaySeconds:      getDefaultServeOptionInt("REPORT_STATE_DELAY_SECONDS", 1),
			Labels:                       getDefaultServeOptionMap("LABELS", map[string]string{}),
			SchedulingDecisionBufferSize: getDefaultServeOptionInt("SCHEDULING_DECISION_BUFFER_SIZE", 100),
			JobHistoryBufferSize:         getDefaultServeOptionInt("JOB_HISTORY_BUFFER_SIZE", 100),
			MockRunner:                   getDefaultServeOptionBool("MOCK_RUNNER", false),
			MockRunnerError:              getDefaultServeOptionString("MOCK_RUNNER_ERROR", ""),
			MockRunnerDelay:              getDefaultServeOptionInt("MOCK_RUNNER_DELAY", 0),
			FilterModelName:              getDefaultServeOptionString("FILTER_MODEL_NAME", ""),
			FilterMode:                   getDefaultServeOptionString("FILTER_MODE", ""),
			// Default cache dir: /root/.cache/huggingface in Docker (persistent storage, models baked there)
			// Falls back to temp dir on non-Linux systems (macOS) where /root doesn't exist
			CacheDir:                     getDefaultServeOptionString("CACHE_DIR", getDefaultCacheDir()),
			WebServer: runner.WebServer{
				Host: getDefaultServeOptionString("SERVER_HOST", "127.0.0.1"),
				Port: getDefaultServeOptionInt("SERVER_PORT", 8080),
			},
			DevelopmentCPUOnly: getDefaultServeOptionBool("DEVELOPMENT_CPU_ONLY", false),
		},
		Janitor: config.Janitor{
			SentryDsnAPI: getDefaultServeOptionString("SENTRY_DSN_API", ""),
		},
	}
}

func newRunnerCmd() *cobra.Command {
	allOptions := newRunnerOptions()

	runnerCmd := &cobra.Command{
		Use:     "runner",
		Short:   "Start a helix runner.",
		Long:    "Start a helix runner.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {

			runnerConfig, err := config.LoadRunnerConfig()
			if err != nil {
				return fmt.Errorf("failed to load server config: %v", err)
			}

			allOptions.Runner.Config = &runnerConfig

			return runnerCLI(cmd, allOptions)
		},
	}

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.ID, "runner-id", allOptions.Runner.ID,
		`The ID of this runner to report to the api server when asking for jobs`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.APIHost, "api-host", allOptions.Runner.APIHost,
		`The base URL of the api - e.g. http://1.2.3.4:8080`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.APIToken, "api-token", allOptions.Runner.APIToken,
		`The auth token for this runner`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.WebServer.Host, "server-host", allOptions.Runner.WebServer.Host,
		`The host to bind the api server to.`,
	)

	runnerCmd.PersistentFlags().IntVar(
		&allOptions.Runner.WebServer.Port, "server-port", allOptions.Runner.WebServer.Port,
		`The port to bind the api server to.`,
	)

	runnerCmd.PersistentFlags().Uint64Var(
		&allOptions.Runner.MemoryBytes, "memory-bytes", allOptions.Runner.MemoryBytes,
		`The number of bytes of GPU memory available - e.g. 1073741824`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.MemoryString, "memory", allOptions.Runner.MemoryString,
		`Short notation for the amount of GPU memory available - e.g. 1GB`,
	)

	runnerCmd.PersistentFlags().StringToStringVar(
		&allOptions.Runner.Labels, "label", allOptions.Runner.Labels,
		`Labels to attach to this runner`,
	)

	runnerCmd.PersistentFlags().IntVar(
		&allOptions.Runner.SchedulingDecisionBufferSize, "scheduling-decision-buffer-size", allOptions.Runner.SchedulingDecisionBufferSize,
		`How many scheduling decisions to buffer before we start dropping them.`,
	)

	runnerCmd.PersistentFlags().IntVar(
		&allOptions.Runner.JobHistoryBufferSize, "job-history-buffer-size", allOptions.Runner.JobHistoryBufferSize,
		`How many jobs do we keep in the history buffer for the runner.`,
	)

	runnerCmd.PersistentFlags().BoolVar(
		&allOptions.Runner.MockRunner, "mock-runner", allOptions.Runner.MockRunner,
		`Are we running a mock runner?`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.MockRunnerError, "mock-runner-error", allOptions.Runner.MockRunnerError,
		`If defined, the runner will always throw this error for all jobs.`,
	)

	runnerCmd.PersistentFlags().IntVar(
		&allOptions.Runner.MockRunnerDelay, "mock-runner-delay", allOptions.Runner.MockRunnerDelay,
		`How many seconds to delay the mock runner process.`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.FilterModelName, "filter-model-name", allOptions.Runner.FilterModelName,
		`Only run jobs of this model name`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.FilterMode, "filter-mode", allOptions.Runner.FilterMode,
		`Only run jobs of this mode`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Janitor.SentryDsnAPI, "janitor-sentry-dsn", allOptions.Janitor.SentryDsnAPI,
		`The sentry DSN.`,
	)

	return runnerCmd
}

func runnerCLI(cmd *cobra.Command, options *RunnerOptions) error {
	fmt.Fprintf(os.Stderr, "DEBUG: runnerCLI() called\n")
	fmt.Fprintf(os.Stderr, "DEBUG: Starting logging setup\n")
	system.SetupLogging()
	fmt.Fprintf(os.Stderr, "DEBUG: Logging setup complete\n")

	if options.Runner.APIToken == "" {
		return fmt.Errorf("api token is required")
	}

	_, err := types.ValidateSessionMode(options.Runner.FilterMode, true)
	if err != nil {
		return err
	}

	// Cleanup manager ensures that resources are freed before exiting:
	cm := system.NewCleanupManager()
	defer cm.Cleanup(cmd.Context())
	ctx := cmd.Context()

	// Context ensures main goroutine waits until killed with ctrl+c:
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	janitor := janitor.NewJanitor(options.Janitor)
	fmt.Fprintf(os.Stderr, "DEBUG: About to initialize janitor\n")
	err = janitor.Initialize()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "DEBUG: Janitor initialized\n")

	fmt.Fprintf(os.Stderr, "DEBUG: About to initialize models cache\n")
	err = initializeModelsCache(options.Runner.Config)
	if err != nil {
		log.Error().Err(err).Msgf("failed to initialize models cache")
	}
	fmt.Fprintf(os.Stderr, "DEBUG: Models cache initialized\n")

	fmt.Fprintf(os.Stderr, "DEBUG: About to create runner controller\n")
	runnerController, err := runner.NewRunner(ctx, options.Runner)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "DEBUG: Runner controller created successfully\n")

	go runnerController.Run(ctx)

	<-ctx.Done()
	return nil
}

// inbuiltModelsDirectory directory inside the Docker image that can have
// a cache of models that are already downloaded during the build process.
// These files need to be copied into runner cache dir
var bakedModelDirectories = []string{"/workspace/ollama", "/workspace/diffusers"}

func initializeModelsCache(cfg *config.RunnerConfig) error {
	log.Info().Msgf("Copying baked models from %v into container cache dir %s - this may take a while the first time...", bakedModelDirectories, cfg.CacheDir)

	for _, dir := range bakedModelDirectories {
		// If the directory doesn't exist, nothing to do
		_, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				log.Debug().Msgf("Baked models directory %s does not exist", dir)
				continue
			}
			return fmt.Errorf("error checking inbuilt models directory: %w", err)
		}

		// Check if the cache dir exists, if not create it
		if _, err := os.Stat(cfg.CacheDir); os.IsNotExist(err) {
			err = os.MkdirAll(cfg.CacheDir, 0755)
			if err != nil {
				return fmt.Errorf("error creating cache dir: %w", err)
			}
		}

		// Copy the directory from the Docker image into the cache dir
		log.Debug().Msgf("Copying %s into container dir %s", dir, cfg.CacheDir)
		err = copydir.CopyDir(cfg.CacheDir, dir)
		if err != nil {
			return fmt.Errorf("error copying inbuilt models directory: %w", err)
		}
	}

	return nil
}

// getDefaultCacheDir returns the appropriate cache directory based on the platform.
// On Linux (Docker), we use /root/.cache/huggingface which is often bind-mounted
// as persistent storage and matches where initializeModelsCache copies models.
// On other platforms (macOS), we use a temp directory to avoid "read-only file system" errors.
func getDefaultCacheDir() string {
	// Check if /root exists and is writable (Linux/Docker)
	if info, err := os.Stat("/root"); err == nil && info.IsDir() {
		return "/root/.cache/huggingface"
	}
	// Fall back to temp directory (macOS, etc.)
	return os.TempDir()
}

// Utility functions for environment variable handling
func getDefaultServeOptionString(envName string, defaultValue string) string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return envValue
	}
	return defaultValue
}

func getDefaultServeOptionBool(envName string, defaultValue bool) bool {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return true
	}
	return defaultValue
}

func getDefaultServeOptionInt(envName string, defaultValue int) int {
	envValue := os.Getenv(envName)
	if envValue != "" {
		i, err := strconv.Atoi(envValue)
		if err == nil {
			return i
		}
	}
	return defaultValue
}

// comma separated key=value pairs e.g. LABELS="name=apples,height=10"
func getDefaultServeOptionMap(envName string, defaultValue map[string]string) map[string]string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		parts := strings.Split(envValue, ",")
		data := make(map[string]string)
		for _, part := range parts {
			kv := strings.Split(part, "=")
			if len(kv) == 2 {
				data[kv[0]] = kv[1]
			} else {
				log.Warn().Msgf("invalid key=value pair: %s", part)
			}
		}
		return data
	}
	return defaultValue
}
