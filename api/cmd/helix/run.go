package helix

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/spf13/cobra"
)

type RunOptions struct {
	ApiUrl      string
	ApiKey      string
	Type        string
	Prompt      string
	ActiveTools string
	SessionId   string
}

// How we test helix - we use dagger!
// Dagger module to call into it...

func NewRunOptions() *RunOptions {
	return &RunOptions{
		ApiUrl: getDefaultServeOptionString("HELIX_API_URL", "https://app.tryhelix.ai"),
		ApiKey: getDefaultServeOptionString("HELIX_API_KEY", ""),
		// e.g. export HELIX_ACTIVE_TOOLS=tool_01hsdm1n7ftba0s0vtejrjf0k2,tool_01hsdmasz3sp16qep1v2mm7enm,tool_01hsdmj3ntya23dmd2qdr1eyhn
		ActiveTools: getDefaultServeOptionString("HELIX_ACTIVE_TOOLS", ""),
		Type:        "text",
		Prompt:      "what laptops are available to buy?",
	}
}

func newRunCmd() *cobra.Command {
	allOptions := NewRunOptions()

	runnerCmd := &cobra.Command{
		Use:     "run",
		Short:   "Run a session in helix.",
		Long:    "Run a text or image job on a remote helix API.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCLI(cmd, allOptions)
		},
	}

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.ApiUrl, "api-host", allOptions.ApiUrl,
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

	var buffer bytes.Buffer
	// Create a multipart writer for the buffer
	writer := multipart.NewWriter(&buffer)

	writer.WriteField("input", options.Prompt)
	writer.WriteField("mode", "inference")
	writer.WriteField("type", options.Type)
	writer.WriteField("active_tools", options.ActiveTools)

	writer.Close()

	// Create a new POST request with the multipart content type and body
	req, err := http.NewRequest("POST", options.ApiUrl+"/api/v1/sessions", &buffer)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return err
	}

	// Set the bearer token API key
	req.Header.Set("Authorization", "Bearer "+options.ApiKey)

	if err != nil {
		fmt.Println("Error creating request:", err)
		return err
	}
	// Set the Content-Type header to the writer's form data content type
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return err
	}
	defer resp.Body.Close()

	// Read the response (optional)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return err
	}

	fmt.Println("Response:", string(responseBody))

	return nil
}
