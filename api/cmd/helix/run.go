package helix

import (
	"bytes"
	"encoding/json"
	"io"
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
	Type      string
	Prompt    string
}

func NewRunOptions() *RunOptions {
	return &RunOptions{
		RunnerUrl: getDefaultServeOptionString("RUNNER_URL", "http://localhost:8080"),
		Type:      "image",
		Prompt:    "a question mark floating in space",
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

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Type, "type", allOptions.Type,
		`Type of generative AI: image, text`,
	)

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Prompt, "prompt", allOptions.Prompt,
		`Prompt for the model`,
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
		ID:       "cli-user",
		Created:  time.Now(),
		Creator:  "user",
		Message:  options.Prompt,
		Finished: true,
	}
	interactionSystem := types.Interaction{
		ID:       "cli-system",
		Created:  time.Now(),
		Creator:  "system",
		Finished: false,
	}

	var modelName types.ModelName
	if options.Type == "image" {
		modelName = types.Model_SDXL
	} else if options.Type == "text" {
		modelName = types.Model_Mistral7b
	}

	id := system.GenerateUUID()
	session := types.Session{
		ID:           "cli-" + id,
		Name:         "cli",
		Created:      time.Now(),
		Updated:      time.Now(),
		Mode:         "inference",
		Type:         types.SessionType(options.Type),
		ModelName:    modelName,
		LoraDir:      "",
		Interactions: []types.Interaction{interaction, interactionSystem},
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

	for {
		resp, err := http.Get(options.RunnerUrl + "/api/v1/worker/state")
		if err != nil {
			return err
		}
		bd, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		rr := make(map[string]types.RunnerTaskResponse)
		err = json.Unmarshal(bd, &rr)
		if err != nil {
			return err
		}
		wtr, ok := rr["cli-"+id]
		if ok {
			log.Printf("Progress: %+v%%", wtr.Progress)
			if len(wtr.Files) > 0 {
				log.Printf("File has been written: %s", wtr.Files[0][len("/app/sd-scripts/./output_images/"):])
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
