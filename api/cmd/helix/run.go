package helix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

type RunOptions struct {
	ApiUrl      string
	ApiKey      string
	Type        string
	Prompt      string
	ActiveTools string
	SessionId   string
}

func NewRunOptions() *RunOptions {
	return &RunOptions{
		ApiUrl: getDefaultServeOptionString("HELIX_API_URL", "https://app.tryhelix.ai"),
		ApiKey: getDefaultServeOptionString("HELIX_API_KEY", ""),
		// e.g. export HELIX_ACTIVE_TOOLS=tool_01hsdm1n7ftba0s0vtejrjf0k2,tool_01hsdmasz3sp16qep1v2mm7enm,tool_01hsdmj3ntya23dmd2qdr1eyhn
		ActiveTools: getDefaultServeOptionString("HELIX_ACTIVE_TOOLS", ""),
		Type:        "text",
		Prompt:      "what laptops are available to buy?",
		SessionId:   "", // means new session
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

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.Prompt, "session", allOptions.SessionId,
		`If specified, add to existing session`,
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

	// can't update these on an existing session
	if options.SessionId == "" {
		writer.WriteField("mode", "inference")
		writer.WriteField("type", options.Type)
	}
	writer.WriteField("active_tools", options.ActiveTools)

	writer.Close()

	var req *http.Request
	var err error
	if options.SessionId == "" {
		// Create a new POST request with the multipart content type and body
		req, err = http.NewRequest("POST", options.ApiUrl+"/api/v1/sessions", &buffer)
	} else {
		// Update an existing session
		req, err = http.NewRequest("PUT", options.ApiUrl+"/api/v1/sessions/"+options.SessionId, &buffer)
	}
	if err != nil {
		fmt.Println("Error creating request:", err)
		return err
	}

	// Set the bearer token API key
	req.Header.Set("Authorization", "Bearer "+options.ApiKey)

	// fmt.Println("Using API key", options.ApiKey)

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

	// fmt.Println("Response:", string(responseBody))

	var session types.Session
	err = json.Unmarshal(responseBody, &session)
	if err != nil {
		fmt.Printf("Error parsing response body %s: %s\n", string(responseBody), err)
		return err
	}

	// fmt.Println("Session:", session)

	spinner, err := createSpinner("Running inference", "✅")
	if err != nil {
		fmt.Printf("failed to make spinner from config struct: %v\n", err)
		os.Exit(1)
	}
	spinner.Start()

	var latestSession *types.Interaction
	for {
		// Poll the session status
		session, err := getSessionStatus(options.ApiUrl, options.ApiKey, session.ID)
		if err != nil {
			fmt.Println("Error getting session status:", err)
			break
		}

		// Print the session status
		if len(session.Interactions) > 0 {
			i := session.Interactions[len(session.Interactions)-1]
			// fmt.Printf("Last interaction: %+v\n", i)
			if i.Finished {
				latestSession = i
				break
			}
		}

		// Update the spinner

		// Sleep for 1 second
		time.Sleep(1 * time.Second)
	}

	spinner.Stop()
	fmt.Println("")
	fmt.Println(latestSession.Message)

	return nil
}

// Function to get the session status
func getSessionStatus(apiUrl, apiKey, sessionID string) (*types.Session, error) {
	// Create the GET request
	req, err := http.NewRequest("GET", apiUrl+"/api/v1/sessions/"+sessionID, nil)
	if err != nil {
		return nil, err
	}

	// Set the bearer token API key
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse the response body
	var session types.Session
	err = json.Unmarshal(responseBody, &session)
	if err != nil {
		return nil, err
	}

	return &session, nil
}

func createSpinner(message string, emoji string) (*yacspin.Spinner, error) {
	// build the configuration, each field is documented
	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[69],
		Suffix:            " ", // puts a least one space between the animating spinner and the Message
		Message:           message,
		SuffixAutoColon:   true,
		ColorAll:          false,
		Colors:            []string{"fgMagenta"},
		StopCharacter:     emoji,
		StopColors:        []string{"fgGreen"},
		StopMessage:       message,
		StopFailCharacter: "✗",
		StopFailColors:    []string{"fgRed"},
		StopFailMessage:   "failed",
	}

	s, err := yacspin.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to make spinner from struct: %w", err)
	}

	stopOnSignal(s)
	return s, nil
}

func stopOnSignal(spinner *yacspin.Spinner) {
	// ensure we stop the spinner before exiting, otherwise cursor will remain
	// hidden and terminal will require a `reset`
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh

		spinner.StopFailMessage("interrupted")

		// ignoring error intentionally
		_ = spinner.StopFail()

		os.Exit(0)
	}()
}
