package helix

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/runner"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/copydir"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type RunnerOptions struct {
	Runner  runner.RunnerOptions
	Janitor config.Janitor
	Server  runner.RunnerServerOptions
}

func NewRunnerOptions() *RunnerOptions {
	return &RunnerOptions{
		Runner: runner.RunnerOptions{
			ID:                           getDefaultServeOptionString("RUNNER_ID", ""),
			ApiHost:                      getDefaultServeOptionString("API_HOST", ""),
			ApiToken:                     getDefaultServeOptionString("API_TOKEN", ""),
			MemoryBytes:                  uint64(getDefaultServeOptionInt("MEMORY_BYTES", 0)),
			MemoryString:                 getDefaultServeOptionString("MEMORY_STRING", ""),
			GetTaskDelayMilliseconds:     getDefaultServeOptionInt("GET_TASK_DELAY_MILLISECONDS", 100),
			ReportStateDelaySeconds:      getDefaultServeOptionInt("REPORT_STATE_DELAY_SECONDS", 1),
			Labels:                       getDefaultServeOptionMap("LABELS", map[string]string{}),
			SchedulingDecisionBufferSize: getDefaultServeOptionInt("SCHEDULING_DECISION_BUFFER_SIZE", 100),
			JobHistoryBufferSize:         getDefaultServeOptionInt("JOB_HISTORY_BUFFER_SIZE", 100),
			MockRunner:                   getDefaultServeOptionBool("MOCK_RUNNER", false),
			MockRunnerError:              getDefaultServeOptionString("MOCK_RUNNER_ERROR", ""),
			MockRunnerDelay:              getDefaultServeOptionInt("MOCK_RUNNER_DELAY", 0),
			FilterModelName:              getDefaultServeOptionString("FILTER_MODEL_NAME", ""),
			FilterMode:                   getDefaultServeOptionString("FILTER_MODE", ""),
			AllowMultipleCopies:          getDefaultServeOptionBool("ALLOW_MULTIPLE_COPIES", false),
			MaxModelInstances:            getDefaultServeOptionInt("MAX_MODEL_INSTANCES", 0),
			CacheDir:                     getDefaultServeOptionString("CACHE_DIR", "/root/.cache/huggingface"), // TODO: change to maybe just /data
		},
		Janitor: config.Janitor{
			SentryDsnAPI: getDefaultServeOptionString("SENTRY_DSN_API", ""),
		},
		Server: runner.RunnerServerOptions{
			Host: getDefaultServeOptionString("SERVER_HOST", "0.0.0.0"),
			Port: getDefaultServeOptionInt("SERVER_PORT", 8080),
		},
	}
}

func newRunnerCmd() *cobra.Command {
	allOptions := NewRunnerOptions()

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
		&allOptions.Runner.ApiHost, "api-host", allOptions.Runner.ApiHost,
		`The base URL of the api - e.g. http://1.2.3.4:8080`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.ApiToken, "api-token", allOptions.Runner.ApiToken,
		`The auth token for this runner`,
	)

	runnerCmd.PersistentFlags().Uint64Var(
		&allOptions.Runner.MemoryBytes, "memory-bytes", allOptions.Runner.MemoryBytes,
		`The number of bytes of GPU memory available - e.g. 1073741824`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Runner.MemoryString, "memory", allOptions.Runner.MemoryString,
		`Short notation for the amount of GPU memory available - e.g. 1GB`,
	)

	runnerCmd.PersistentFlags().IntVar(
		&allOptions.Runner.GetTaskDelayMilliseconds, "get-task-delay-milliseconds", allOptions.Runner.GetTaskDelayMilliseconds,
		`How many milliseconds do we wait between running the control loop (which asks for the next global session)`,
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

	runnerCmd.PersistentFlags().BoolVar(
		&allOptions.Runner.AllowMultipleCopies, "allow-multiple-copies", allOptions.Runner.AllowMultipleCopies,
		`Should we allow multiple copies of the same model to run at the same time?`,
	)

	runnerCmd.PersistentFlags().IntVar(
		&allOptions.Runner.MaxModelInstances, "max-model-instances", allOptions.Runner.MaxModelInstances,
		`How many instances of a model can we run at the same time?`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Server.Host, "server-host", allOptions.Server.Host,
		`The host to bind the runner server to.`,
	)
	runnerCmd.PersistentFlags().IntVar(
		&allOptions.Server.Port, "server-port", allOptions.Server.Port,
		`The port to bind the runner server to.`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Janitor.SentryDsnAPI, "janitor-sentry-dsn", allOptions.Janitor.SentryDsnAPI,
		`The sentry DSN.`,
	)

	return runnerCmd
}

var ITX_A = &types.Interaction{
	ID:       "warmup-user",
	Created:  time.Now(),
	Creator:  "user",
	Message:  "a new runner is born",
	Finished: true,
}
var ITX_B = &types.Interaction{
	ID:       "warmup-system",
	Created:  time.Now(),
	Creator:  "system",
	Finished: false,
}

var WarmupSession_Model_Mistral7b = types.Session{
	ID:           types.WarmupTextSessionID,
	Name:         "warmup-text",
	Created:      time.Now(),
	Updated:      time.Now(),
	Mode:         "inference",
	Type:         types.SessionTypeText,
	ModelName:    model.Model_Axolotl_Mistral7b,
	LoraDir:      "",
	Interactions: []*types.Interaction{ITX_A, ITX_B},
	Owner:        "warmup-user",
	OwnerType:    "user",
}

var WarmupSession_Model_Ollama_Llama3_8b = types.Session{
	ID:           types.WarmupTextSessionID,
	Name:         "warmup-text",
	Created:      time.Now(),
	Updated:      time.Now(),
	Mode:         "inference",
	Type:         types.SessionTypeText,
	ModelName:    "llama3:instruct",
	LoraDir:      "",
	Interactions: []*types.Interaction{ITX_A, ITX_B},
	Owner:        "warmup-user",
	OwnerType:    "user",
}

var WarmupSession_Model_SDXL = types.Session{
	ID:           types.WarmupImageSessionID,
	Name:         "warmup-image",
	Created:      time.Now(),
	Updated:      time.Now(),
	Mode:         "inference",
	Type:         types.SessionTypeImage,
	ModelName:    model.Model_Cog_SDXL,
	LoraDir:      "",
	Interactions: []*types.Interaction{ITX_A, ITX_B},
	Owner:        "warmup-user",
	OwnerType:    "user",
}

func runnerCLI(cmd *cobra.Command, options *RunnerOptions) error {
	system.SetupLogging()

	if options.Runner.ApiToken == "" {
		return fmt.Errorf("api token is required")
	}

	_, err := model.TransformModelName(options.Runner.FilterModelName)
	if err != nil {
		return err
	}

	_, err = types.ValidateSessionMode(options.Runner.FilterMode, true)
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
	err = janitor.Initialize()
	if err != nil {
		return err
	}

	err = initializeModelsCache(options.Runner.Config)
	if err != nil {
		log.Error().Err(err).Msgf("failed to initialize models cache")
	}

	// we will append the instance ID onto these paths
	// because it's a model_instance that will spawn Python
	// processes that will then speak back to these routes
	options.Runner.TaskURL = fmt.Sprintf("http://localhost:%d%s", options.Server.Port, system.GetApiPath("/worker/task"))
	options.Runner.InitialSessionURL = fmt.Sprintf("http://localhost:%d%s", options.Server.Port, system.GetApiPath("/worker/initial_session"))

	// global state - expedient hack (TODO remove this when we switch cog away
	// from downloading lora weights via http from the filestore)
	model.API_HOST = options.Runner.ApiHost
	model.API_TOKEN = options.Runner.ApiToken

	useWarmupSessions := []types.Session{}
	if !options.Runner.MockRunner {
		// Axolotl runtime warmup
		if options.Runner.Config.Runtimes.Axolotl.Enabled {
			for _, modelName := range options.Runner.Config.Runtimes.Axolotl.WarmupModels {
				switch modelName {
				case model.Model_Axolotl_Mistral7b:
					log.Info().Msgf("Adding warmup session for model %s", modelName)
					useWarmupSessions = append(useWarmupSessions, WarmupSession_Model_Mistral7b)
				case model.Model_Cog_SDXL:
					log.Info().Msgf("Adding warmup session for model %s", modelName)
					useWarmupSessions = append(useWarmupSessions, WarmupSession_Model_SDXL)
				default:
					log.Error().Msgf("Unknown warmup model %s", modelName)
				}
			}
		}

		// Ollama runtime warmup
		if options.Runner.Config.Runtimes.Ollama.Enabled && !options.Runner.Config.Runtimes.V2Engine {
			for _, modelName := range options.Runner.Config.Runtimes.Ollama.WarmupModels {
				switch modelName {
				case model.Model_Ollama_Llama3_8b:
					log.Info().Msgf("Adding warmup session for model %s", modelName)
					useWarmupSessions = append(useWarmupSessions, WarmupSession_Model_Ollama_Llama3_8b)
				}
			}
		}
	}

	runnerController, err := runner.NewRunner(ctx, options.Runner, useWarmupSessions)
	if err != nil {
		return err
	}

	err = runnerController.Initialize(ctx)
	if err != nil {
		return err
	}

	go runnerController.Run()

	server, err := runner.NewRunnerServer(options.Server, runnerController)
	if err != nil {
		return err
	}

	log.Info().Msgf("Helix runner listening on %s:%d", options.Server.Host, options.Server.Port)

	go func() {
		err := server.ListenAndServe(ctx, cm)
		if err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	return nil
}

// inbuiltModelsDirectory directory inside the Docker image that can have
// a cache of models that are already downloaded during the build process.
// These files need to be copied into runner cache dir
const inbuiltModelsDirectory = "/workspace/ollama"

func initializeModelsCache(cfg *config.RunnerConfig) error {
	log.Info().Msgf("Copying baked models from %s into cache dir %s - this may take a while...", inbuiltModelsDirectory, cfg.CacheDir)
	_, err := os.Stat(inbuiltModelsDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			// If the directory doesn't exist, nothing to do
			return nil
		}
		return fmt.Errorf("error checking inbuilt models directory: %w", err)
	}

	// Check if the cache dir exists, if not create it
	if _, err := os.Stat(cfg.CacheDir); os.IsNotExist(err) {
		err = os.MkdirAll(cfg.CacheDir, 0755)
		if err != nil {
			return err
		}
	}

	return copydir.CopyDir(cfg.CacheDir, inbuiltModelsDirectory)

}
