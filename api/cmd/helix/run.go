package helix

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

type RunOptions struct {
	RunnerUrl string
}

func NewRunOptions() *RunOptions {
	return &RunOptions{
		RunnerUrl: getDefaultServeOptionString("RUNNER_URL", "http://localhost:8080"),
	}
}

func newRunCmd() *cobra.Command {
	allOptions := NewRunOptions()

	runnerCmd := &cobra.Command{
		Use:     "run",
		Short:   "Run a task directly on a helix runner.",
		Long:    "Run a task directly on a helix runner.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCLI(cmd, allOptions)
		},
	}

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.RunnerUrl, "api-host", allOptions.RunnerUrl,
		`The base URL of the runner`,
	)

	return runnerCmd
}

func runCLI(cmd *cobra.Command, options *RunOptions) error {
	system.SetupLogging()

	// Cleanup manager ensures that resources are freed before exiting:
	cm := system.NewCleanupManager()
	defer cm.Cleanup(cmd.Context())
	ctx := cmd.Context()

	// Context ensures main goroutine waits until killed with ctrl+c:
	_, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	interaction := types.Interaction{
		ID:       "cli-intx",
		Created:  time.Now(),
		Creator:  "user",
		Runner:   "",
		Message:  "a unicorn riding a horse",
		Progress: 0,
		Files:    []string{},
		Finished: true,
		Metadata: map[string]string{},
		Error:    "",
	}

	id := system.GenerateUUID()
	session := types.Session{
		ID:           "cli-" + id,
		Name:         "cli",
		Created:      time.Now(),
		Updated:      time.Now(),
		Mode:         "inference",
		Type:         "image",
		ModelName:    "stabilityai/stable-diffusion-xl-base-1.0",
		FinetuneFile: "",
		Interactions: []types.Interaction{interaction},
		Owner:        "cli-user",
		OwnerType:    "user",
	}

	bs, err := json.Marshal(session)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", options.RunnerUrl+"/api/v1/worker/session", bytes.NewBuffer(bs))
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Printf("Response: %+v", resp)

	// TODO: poll /worker/state, updating the CLI with the result

	return nil
}
