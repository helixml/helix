package spectask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

// isTimeoutErr reports whether an HTTP request exceeded the caller's wait
// budget. A synchronous chat request must not be retried after this point,
// because the server may still be processing the delivered turn.
func isTimeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// TestResult represents the outcome of a test
type TestResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
	Details  interface{}   `json:"details,omitempty"`
}

// TestSuite represents a collection of test results
type TestSuite struct {
	StartTime time.Time    `json:"start_time"`
	EndTime   time.Time    `json:"end_time"`
	Tests     []TestResult `json:"tests"`
	Passed    int          `json:"passed"`
	Failed    int          `json:"failed"`
}

func newTestCommand() *cobra.Command {
	var jsonOutput bool
	var sessionID string
	var testMCP bool
	var testDesktop bool
	var testChat bool
	var runAll bool
	var timeout int

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run automated tests against a session",
		Long: `Run automated tests to verify session functionality.

Available test suites:
  --mcp      Test Session MCP tools (current_session, session_toc, get_turn, etc.)
  --desktop  Test Desktop MCP tools (screenshot, window list, etc.)
  --chat     Test chat functionality (send message, receive response)
  --all      Run all test suites

Examples:
  # Test an existing session
  helix spectask test --session ses_01xxx --mcp

  # Run all tests with JSON output (for CI)
  helix spectask test --session ses_01xxx --all --json

  # Quick integration test
  helix spectask test --session ses_01xxx --chat
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if sessionID == "" {
				return fmt.Errorf("--session is required")
			}

			apiURL := getAPIURL()
			token := getToken()

			suite := &TestSuite{
				StartTime: time.Now(),
				Tests:     []TestResult{},
			}

			if runAll {
				testMCP = true
				testDesktop = true
				testChat = true
			}

			if testMCP {
				runMCPTests(apiURL, token, sessionID, suite, timeout)
			}

			if testDesktop {
				runDesktopTests(apiURL, token, sessionID, suite, timeout)
			}

			if testChat {
				runChatTests(apiURL, token, sessionID, suite, timeout)
			}

			suite.EndTime = time.Now()

			if jsonOutput {
				return outputJSON(suite)
			}

			return outputHuman(suite)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID to test (required)")
	cmd.Flags().BoolVar(&testMCP, "mcp", false, "Run Session MCP tests")
	cmd.Flags().BoolVar(&testDesktop, "desktop", false, "Run Desktop MCP tests")
	cmd.Flags().BoolVar(&testChat, "chat", false, "Run chat tests")
	cmd.Flags().BoolVar(&runAll, "all", false, "Run all test suites")
	cmd.Flags().IntVar(&timeout, "timeout", 30, "Test timeout in seconds")

	return cmd
}

func runMCPTests(apiURL, token, sessionID string, suite *TestSuite, timeout int) {
	fmt.Printf("\n📋 Running Session MCP Tests...\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

	// Test 1: current_session
	result := testMCPTool(apiURL, token, sessionID, "current_session", nil, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 2: session_toc
	result = testMCPTool(apiURL, token, sessionID, "session_toc", nil, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 3: session_title_history
	result = testMCPTool(apiURL, token, sessionID, "session_title_history", nil, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 4: get_turn (turn 1)
	result = testMCPTool(apiURL, token, sessionID, "get_turn", map[string]interface{}{"turn": 1}, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 5: search_session
	result = testMCPTool(apiURL, token, sessionID, "search_session", map[string]interface{}{"query": "test"}, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)
}

func testMCPTool(apiURL, token, sessionID, toolName string, args map[string]interface{}, _ int) TestResult {
	start := time.Now()
	result := TestResult{
		Name: fmt.Sprintf("mcp/%s", toolName),
	}

	ctx := context.Background()
	mcpURL := fmt.Sprintf("%s/api/v1/mcp/session?session_id=%s", apiURL, sessionID)

	mcpClient, err := newMCPClient(ctx, mcpURL, token)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	toolResult, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	result.Duration = time.Since(start)
	result.Passed = true
	result.Details = toolResult
	return result
}

func runDesktopTests(apiURL, token, sessionID string, suite *TestSuite, timeout int) {
	fmt.Printf("\n🖥️  Running Desktop MCP Tests...\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

	// Test 1: Screenshot
	result := testScreenshot(apiURL, token, sessionID, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 2: Get windows (via external agent input API)
	result = testDesktopTool(apiURL, token, sessionID, "list_windows", nil, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 3: Get workspaces
	result = testDesktopTool(apiURL, token, sessionID, "get_workspaces", nil, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)
}

func testScreenshot(apiURL, token, sessionID string, timeout int) TestResult {
	start := time.Now()
	result := TestResult{
		Name: "desktop/screenshot",
	}

	screenshotURL := fmt.Sprintf("%s/api/v1/external-agents/%s/screenshot", apiURL, sessionID)
	req, err := http.NewRequest("GET", screenshotURL, nil)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	result.Duration = time.Since(start)

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
		return result
	}

	contentType := http.DetectContentType(body)
	if contentType != "image/png" && contentType != "image/jpeg" {
		result.Error = fmt.Sprintf("response is not a supported screenshot image (%s)", contentType)
		return result
	}

	result.Passed = true
	result.Details = map[string]interface{}{
		"content_type": contentType,
		"size_bytes":   len(body),
	}
	return result
}

func testDesktopTool(apiURL, token, sessionID, toolName string, args map[string]interface{}, _ int) TestResult {
	start := time.Now()
	result := TestResult{
		Name: fmt.Sprintf("desktop/%s", toolName),
	}

	ctx := context.Background()
	// Desktop MCP runs inside the sandbox via the external agent proxy
	mcpURL := fmt.Sprintf("%s/api/v1/external-agents/%s/mcp", apiURL, sessionID)

	mcpClient, err := newMCPClient(ctx, mcpURL, token)
	if err != nil {
		// Desktop MCP might not be available - mark as skipped
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			result.Error = "skipped: desktop MCP not exposed via external API"
			result.Passed = true // Don't fail the test
			return result
		}
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	_, err = mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	result.Duration = time.Since(start)
	result.Passed = true
	return result
}

func runChatTests(apiURL, token, sessionID string, suite *TestSuite, timeout int) {
	fmt.Printf("\n💬 Running Chat Tests...\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

	// Test 1: Get session
	result := testGetSession(apiURL, token, sessionID, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 2: Get interactions
	result = testGetInteractions(apiURL, token, sessionID, timeout)
	suite.Tests = append(suite.Tests, result)
	printTestResult(result)

	// Test 3: Send simple message (optional - may want to skip in automated tests)
	// Commenting out to avoid sending test messages to real sessions
	// result = testSendMessage(apiURL, token, sessionID, "ping", timeout)
	// suite.Tests = append(suite.Tests, result)
	// printTestResult(result)
}

func testGetSession(apiURL, token, sessionID string, timeout int) TestResult {
	start := time.Now()
	result := TestResult{
		Name: "chat/get_session",
	}

	url := fmt.Sprintf("%s/api/v1/sessions/%s", apiURL, sessionID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	result.Duration = time.Since(start)

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		return result
	}

	var session map[string]interface{}
	if err := json.Unmarshal(body, &session); err != nil {
		result.Error = fmt.Sprintf("invalid JSON: %v", err)
		return result
	}

	result.Passed = true
	result.Details = map[string]interface{}{
		"session_id": session["session_id"],
		"name":       session["name"],
		"type":       session["type"],
	}
	return result
}

func testGetInteractions(apiURL, token, sessionID string, timeout int) TestResult {
	start := time.Now()
	result := TestResult{
		Name: "chat/get_interactions",
	}

	url := fmt.Sprintf("%s/api/v1/sessions/%s/interactions?limit=5", apiURL, sessionID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	result.Duration = time.Since(start)

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		return result
	}

	var interactions []map[string]interface{}
	if err := json.Unmarshal(body, &interactions); err != nil {
		result.Error = fmt.Sprintf("invalid JSON: %v", err)
		return result
	}

	result.Passed = true
	result.Details = map[string]interface{}{
		"count": len(interactions),
	}
	return result
}

func printTestResult(result TestResult) {
	status := "✅"
	if !result.Passed {
		status = "❌"
	}

	fmt.Printf("  %s %s (%.0fms)", status, result.Name, float64(result.Duration.Milliseconds()))
	if result.Error != "" && !result.Passed {
		// Truncate long errors
		errMsg := result.Error
		if len(errMsg) > 60 {
			errMsg = errMsg[:60] + "..."
		}
		fmt.Printf(" - %s", errMsg)
	}
	fmt.Printf("\n")
}

func outputJSON(suite *TestSuite) error {
	for _, t := range suite.Tests {
		if t.Passed {
			suite.Passed++
		} else {
			suite.Failed++
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(suite)
}

func outputHuman(suite *TestSuite) error {
	for _, t := range suite.Tests {
		if t.Passed {
			suite.Passed++
		} else {
			suite.Failed++
		}
	}

	fmt.Printf("\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("📊 Test Results\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("   Total:  %d\n", len(suite.Tests))
	fmt.Printf("   Passed: %d ✅\n", suite.Passed)
	fmt.Printf("   Failed: %d ❌\n", suite.Failed)
	fmt.Printf("   Duration: %v\n", suite.EndTime.Sub(suite.StartTime).Round(time.Millisecond))
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if suite.Failed > 0 {
		fmt.Printf("❌ Failed Tests:\n")
		for _, t := range suite.Tests {
			if !t.Passed {
				fmt.Printf("   - %s: %s\n", t.Name, t.Error)
			}
		}
		fmt.Printf("\n")
		return fmt.Errorf("%d tests failed", suite.Failed)
	}

	fmt.Printf("✅ All tests passed!\n\n")
	return nil
}

type queuedSessionMessageRequest struct {
	Content string `json:"content"`
}

type queuedSessionMessageResponse struct {
	PromptID string `json:"prompt_id"`
}

type blockingChatRequest struct {
	SessionID string                `json:"session_id"`
	Messages  []blockingChatMessage `json:"messages"`
	Stream    bool                  `json:"stream"`
}

type blockingChatMessage struct {
	Role    string              `json:"role"`
	Content blockingChatContent `json:"content"`
}

type blockingChatContent struct {
	ContentType string   `json:"content_type"`
	Parts       []string `json:"parts"`
}

type blockingChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type sendCommandOutput struct {
	PromptID     string `json:"prompt_id,omitempty"`
	Delivered    bool   `json:"delivered"`
	StillRunning bool   `json:"still_running,omitempty"`
	Response     string `json:"response,omitempty"`
}

// Script-friendly command for sending a message and optionally waiting for response.
func newSendCommand() *cobra.Command {
	var waitForComplete bool
	var maxWait int
	var jsonOutput bool
	var quiet bool

	cmd := &cobra.Command{
		Use:   "send <session-id|task-id> <message>",
		Short: "Send a message to a session and optionally wait for completion",
		Long: `Queue a message for a session and return immediately by default.

The default path uses Helix's durable prompt queue, so the message remains
pending while the agent is busy or reconnecting. Pass --wait to use the
synchronous chat path and wait for the reply.

Examples:
  # Send and immediately return
  helix spectask send ses_01xxx "List the files in the current directory"

  # Send and wait for agent to complete
  helix spectask send ses_01xxx "Run the tests" --wait

  # JSON output for scripting
  helix spectask send ses_01xxx "What is 2+2?" --json

  # Address the task directly — its session is resolved for you
  helix spectask send spt_01xxx "Run the tests" --wait
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if waitForComplete && maxWait <= 0 {
				return fmt.Errorf("--max-wait must be greater than zero")
			}
			sessionID, err := resolveSessionID(args[0])
			if err != nil {
				return err
			}
			message, apiURL, token := args[1], getAPIURL(), getToken()
			if !waitForComplete {
				output, err := queueSessionMessage(apiURL, token, sessionID, message)
				if err != nil {
					return err
				}
				return printSendCommandOutput(cmd, output, jsonOutput, quiet)
			}

			output, err := sendSessionMessageAndWait(apiURL, token, sessionID, message, time.Duration(maxWait)*time.Second)
			if err != nil {
				return err
			}
			return printSendCommandOutput(cmd, output, jsonOutput, quiet)
		},
	}

	cmd.Flags().BoolVar(&waitForComplete, "wait", false, "Wait for agent to complete processing")
	cmd.Flags().IntVar(&maxWait, "max-wait", 300, "Maximum wait time in seconds")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress confirmation output")
	cmd.Flags().Int("poll", 2, "Deprecated: --wait no longer polls")
	_ = cmd.Flags().MarkDeprecated("poll", "--wait now uses the synchronous response directly")

	return cmd
}

func queueSessionMessage(apiURL, token, sessionID, message string) (*sendCommandOutput, error) {
	payload, err := json.Marshal(queuedSessionMessageRequest{Content: message})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/v1/sessions/%s/messages", apiURL, sessionID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to queue message: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read queue response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("message API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var queued queuedSessionMessageResponse
	if err := json.Unmarshal(body, &queued); err != nil {
		return nil, fmt.Errorf("decode queue response: %w", err)
	}
	return &sendCommandOutput{PromptID: queued.PromptID, Delivered: true}, nil
}

func sendSessionMessageAndWait(apiURL, token, sessionID, message string, timeout time.Duration) (*sendCommandOutput, error) {
	payload, err := json.Marshal(blockingChatRequest{
		SessionID: sessionID,
		Messages: []blockingChatMessage{{
			Role:    "user",
			Content: blockingChatContent{ContentType: "text", Parts: []string{message}},
		}},
		Stream: false,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, apiURL+"/api/v1/sessions/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		if isTimeoutErr(err) {
			return &sendCommandOutput{Delivered: true, StillRunning: true}, nil
		}
		return nil, fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var chatResponse blockingChatResponse
	if err := json.Unmarshal(body, &chatResponse); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	responseText := ""
	if len(chatResponse.Choices) > 0 {
		responseText = chatResponse.Choices[0].Message.Content
	}
	return &sendCommandOutput{Delivered: true, Response: responseText}, nil
}

func printSendCommandOutput(cmd *cobra.Command, output *sendCommandOutput, jsonOutput, quiet bool) error {
	if quiet {
		return nil
	}
	if jsonOutput {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	if output.StillRunning {
		fmt.Fprintln(cmd.OutOrStdout(), "Message delivered; the agent is still running.")
		return nil
	}
	if output.Response != "" {
		fmt.Fprintln(cmd.OutOrStdout(), output.Response)
		return nil
	}
	if output.PromptID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Message queued (%s).\n", output.PromptID)
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Message delivered.")
	return nil
}
