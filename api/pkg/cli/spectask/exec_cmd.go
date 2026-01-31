package spectask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ExecRequest represents the request body for the exec endpoint
type ExecRequest struct {
	Command    []string          `json:"command"`
	Background bool              `json:"background,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}

// ExecResponse represents the response from the exec endpoint
type ExecResponse struct {
	Output   string `json:"output,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Error    string `json:"error,omitempty"`
}

func newExecCommand() *cobra.Command {
	var (
		background bool
		env        []string
		timeout    int
	)

	cmd := &cobra.Command{
		Use:   "exec <session-id> <command> [args...]",
		Short: "Execute a command inside a session container",
		Long: `Execute a command inside a running session container.

This command allows you to run arbitrary commands in the desktop container
associated with a Helix session. Useful for:
  - Running scripts or tools
  - Debugging container state
  - Setting up test environments
  - Installing dependencies

Examples:
  # Run a simple command
  helix spectask exec ses_01xxx ls -la /home/retro/work

  # Run a background process
  helix spectask exec ses_01xxx --background python3 server.py

  # Run with environment variables
  helix spectask exec ses_01xxx --env FOO=bar --env BAZ=qux printenv

  # Run a shell command
  helix spectask exec ses_01xxx bash -c "echo hello && pwd"
`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			command := args[1:]

			apiURL := getAPIURL()
			token := getToken()

			if apiURL == "" || token == "" {
				return fmt.Errorf("HELIX_URL and HELIX_API_KEY environment variables must be set")
			}

			// Parse environment variables
			envMap := make(map[string]string)
			for _, e := range env {
				parts := strings.SplitN(e, "=", 2)
				if len(parts) == 2 {
					envMap[parts[0]] = parts[1]
				} else {
					return fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", e)
				}
			}

			result, err := execInSession(apiURL, token, sessionID, command, background, envMap, timeout)
			if err != nil {
				return err
			}

			if result.Error != "" {
				fmt.Printf("Error: %s\n", result.Error)
				if result.ExitCode != 0 {
					return fmt.Errorf("command exited with code %d", result.ExitCode)
				}
				return fmt.Errorf("command failed")
			}

			if result.Output != "" {
				fmt.Print(result.Output)
				// Ensure output ends with newline
				if !strings.HasSuffix(result.Output, "\n") {
					fmt.Println()
				}
			}

			if background {
				fmt.Println("Command started in background")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&background, "background", false, "Run command in background (don't wait for output)")
	cmd.Flags().StringArrayVar(&env, "env", nil, "Environment variables (KEY=VALUE format, can be repeated)")
	cmd.Flags().IntVar(&timeout, "timeout", 30, "Timeout in seconds (0 for no timeout)")

	return cmd
}

func execInSession(apiURL, token, sessionID string, command []string, background bool, env map[string]string, timeoutSecs int) (*ExecResponse, error) {
	execURL := fmt.Sprintf("%s/api/v1/external-agents/%s/exec", apiURL, sessionID)

	payload := ExecRequest{
		Command:    command,
		Background: background,
	}
	if len(env) > 0 {
		payload.Env = env
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", execURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	timeout := time.Duration(timeoutSecs) * time.Second
	if timeoutSecs == 0 {
		timeout = 0 // No timeout
	}
	client := &http.Client{Timeout: timeout}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("exec API returned %d: %s", resp.StatusCode, string(body))
	}

	var result ExecResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// If we can't parse as JSON, treat the body as plain output
		result.Output = string(body)
	}

	return &result, nil
}
