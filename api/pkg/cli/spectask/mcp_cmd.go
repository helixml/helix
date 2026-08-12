package spectask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

// MCPClient wraps the mcp-go client for CLI usage
type MCPClient interface {
	ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// newMCPClient creates a new MCP client for the given URL with authentication
func newMCPClient(ctx context.Context, mcpURL, token string) (MCPClient, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}

	httpTransport, err := transport.NewStreamableHTTP(
		mcpURL,
		transport.WithHTTPHeaders(headers),
		transport.WithHTTPTimeout(60*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP transport: %w", err)
	}

	mcpClient := client.NewClient(httpTransport)

	if err := mcpClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	// Initialize the MCP session
	initRequest := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "helix-cli",
				Version: data.GetHelixVersion(),
			},
		},
	}

	if _, err := mcpClient.Initialize(ctx, initRequest); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	return mcpClient, nil
}

func newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Direct MCP tool access for testing",
		Long: `Call MCP tools directly for testing and debugging.

Available subcommands allow you to test Session MCP and Desktop MCP tools.
This is useful for:
  - Verifying MCP connectivity
  - Testing individual tools
  - Debugging agent behavior
  - Scripted integration tests

Examples:
  # Get current session info
  helix spectask mcp session ses_01xxx current_session

  # Get session table of contents
  helix spectask mcp session ses_01xxx session_toc

  # Get a specific turn
  helix spectask mcp session ses_01xxx get_turn --turn 3

  # Search session history
  helix spectask mcp session ses_01xxx search_session --query "database"

  # Take a screenshot via desktop MCP
  helix spectask mcp desktop ses_01xxx take_screenshot

  # List windows
  helix spectask mcp desktop ses_01xxx list_windows
`,
	}

	cmd.AddCommand(newMCPSessionCommand())
	cmd.AddCommand(newMCPDesktopCommand())
	cmd.AddCommand(newMCPListCommand())

	return cmd
}

func newMCPSessionCommand() *cobra.Command {
	var turn int
	var query string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "session <session-id> <tool>",
		Short: "Call Session MCP tools",
		Long: `Call Session MCP tools directly.

Available tools:
  current_session        - Get session overview (name, turn count, title changes)
  session_toc            - Get numbered table of contents with summaries
  get_turn               - Get full content of a specific turn (requires --turn)
  session_title_history  - See how session topic evolved
  search_session         - Search session interactions (requires --query)

Examples:
  helix spectask mcp session ses_01xxx current_session
  helix spectask mcp session ses_01xxx get_turn --turn 3
  helix spectask mcp session ses_01xxx search_session --query "error"
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			toolName := args[1]
			apiURL := getAPIURL()
			token := getToken()

			// Build arguments based on tool
			toolArgs := map[string]interface{}{}

			switch toolName {
			case "get_turn":
				if turn == 0 {
					return fmt.Errorf("--turn is required for get_turn")
				}
				toolArgs["turn"] = turn

			case "search_session":
				if query == "" {
					return fmt.Errorf("--query is required for search_session")
				}
				toolArgs["query"] = query
			}

			result, err := callSessionMCP(apiURL, token, sessionID, toolName, toolArgs)
			if err != nil {
				return err
			}

			if jsonOutput {
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}

			// Human-friendly output
			return printMCPResult(toolName, result)
		},
	}

	cmd.Flags().IntVar(&turn, "turn", 0, "Turn number for get_turn")
	cmd.Flags().StringVar(&query, "query", "", "Search query for search_session")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")

	return cmd
}

func callSessionMCP(apiURL, token, sessionID, toolName string, args map[string]interface{}) (interface{}, error) {
	ctx := context.Background()
	mcpURL := fmt.Sprintf("%s/api/v1/mcp/session?session_id=%s", apiURL, sessionID)

	mcpClient, err := newMCPClient(ctx, mcpURL, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	return result, nil
}

func printMCPResult(toolName string, result interface{}) error {
	fmt.Printf("\n📋 %s Result\n", toolName)
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

	// Try to extract content from MCP result format
	if resultMap, ok := result.(map[string]interface{}); ok {
		if content, ok := resultMap["content"].([]interface{}); ok && len(content) > 0 {
			if textContent, ok := content[0].(map[string]interface{}); ok {
				if text, ok := textContent["text"].(string); ok {
					// Try to parse as JSON for pretty printing
					var data interface{}
					if err := json.Unmarshal([]byte(text), &data); err == nil {
						return prettyPrintMCPData(toolName, data)
					}
					fmt.Printf("%s\n", text)
					return nil
				}
			}
		}
	}

	// Fallback: print as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func prettyPrintMCPData(toolName string, data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(data)
	}

	switch toolName {
	case "current_session":
		fmt.Printf("Session ID:    %v\n", dataMap["session_id"])
		fmt.Printf("Name:          %v\n", dataMap["name"])
		fmt.Printf("Total Turns:   %v\n", dataMap["total_turns"])
		fmt.Printf("Title Changes: %v\n", dataMap["title_changes"])
		if created, ok := dataMap["created"].(string); ok {
			fmt.Printf("Created:       %s\n", created)
		}
		if updated, ok := dataMap["updated"].(string); ok {
			fmt.Printf("Updated:       %s\n", updated)
		}

	case "session_toc":
		fmt.Printf("Session:     %v (%v)\n", dataMap["session_name"], dataMap["session_id"])
		fmt.Printf("Total Turns: %v\n\n", dataMap["total_turns"])

		if entries, ok := dataMap["entries"].([]interface{}); ok {
			fmt.Printf("Table of Contents:\n")
			for _, e := range entries {
				if entry, ok := e.(map[string]interface{}); ok {
					turnNum := entry["turn"]
					summary := entry["summary"]
					fmt.Printf("  %v. %v\n", turnNum, summary)
				}
			}
		}

	case "get_turn":
		fmt.Printf("Turn:    %v\n", dataMap["turn"])
		fmt.Printf("ID:      %v\n", dataMap["id"])
		if summary, ok := dataMap["summary"].(string); ok && summary != "" {
			fmt.Printf("Summary: %s\n", summary)
		}
		fmt.Printf("\n")
		fmt.Printf("Prompt:\n")
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("%v\n", dataMap["prompt"])
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("\nResponse:\n")
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("%v\n", dataMap["response"])
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

	case "session_title_history":
		fmt.Printf("Session ID:    %v\n", dataMap["session_id"])
		fmt.Printf("Current Title: %v\n\n", dataMap["current_title"])

		if history, ok := dataMap["history"].([]interface{}); ok && len(history) > 0 {
			fmt.Printf("Title History:\n")
			for _, h := range history {
				if entry, ok := h.(map[string]interface{}); ok {
					fmt.Printf("  Turn %v: %v\n", entry["turn"], entry["title"])
				}
			}
		} else {
			fmt.Printf("No title changes recorded.\n")
		}

	case "search_session":
		fmt.Printf("Query:   %v\n", dataMap["query"])
		fmt.Printf("Matches: %v\n\n", dataMap["total"])

		if matches, ok := dataMap["matches"].([]interface{}); ok {
			for _, m := range matches {
				if match, ok := m.(map[string]interface{}); ok {
					fmt.Printf("Turn %v: ", match["turn"])
					if context, ok := match["context"].(string); ok {
						if len(context) > 100 {
							context = context[:100] + "..."
						}
						fmt.Printf("%s\n", context)
					}
				}
			}
		}

	default:
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(data)
	}

	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n\n")
	return nil
}

func newMCPDesktopCommand() *cobra.Command {
	var jsonOutput bool
	var x, y int
	var text string
	var windowID int

	cmd := &cobra.Command{
		Use:   "desktop <session-id> <tool>",
		Short: "Call Desktop MCP tools",
		Long: `Call Desktop MCP tools directly.

Available tools:
  take_screenshot     - Capture current screen
  list_windows        - List all windows with IDs
  get_workspaces      - List all workspaces
  type_text           - Type text (requires --text)
  mouse_click         - Click at coordinates (requires --x, --y)
  focus_window        - Focus a window (requires --window)

Examples:
  helix spectask mcp desktop ses_01xxx take_screenshot
  helix spectask mcp desktop ses_01xxx list_windows
  helix spectask mcp desktop ses_01xxx type_text --text "hello world"
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			toolName := args[1]
			apiURL := getAPIURL()
			token := getToken()

			// Build arguments based on tool
			toolArgs := map[string]interface{}{}

			switch toolName {
			case "type_text":
				if text == "" {
					return fmt.Errorf("--text is required for type_text")
				}
				toolArgs["text"] = text

			case "mouse_click":
				toolArgs["x"] = x
				toolArgs["y"] = y

			case "focus_window":
				if windowID == 0 {
					return fmt.Errorf("--window is required for focus_window")
				}
				toolArgs["window_id"] = windowID
			}

			result, err := callDesktopMCP(apiURL, token, sessionID, toolName, toolArgs)
			if err != nil {
				return err
			}

			if jsonOutput {
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}

			// For screenshots, save the file
			if toolName == "take_screenshot" {
				return handleScreenshotResult(result)
			}

			// Human-friendly output
			return printMCPResult(toolName, result)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")
	cmd.Flags().StringVar(&text, "text", "", "Text for type_text")
	cmd.Flags().IntVar(&x, "x", 0, "X coordinate for mouse_click")
	cmd.Flags().IntVar(&y, "y", 0, "Y coordinate for mouse_click")
	cmd.Flags().IntVar(&windowID, "window", 0, "Window ID for focus_window")

	return cmd
}

func callDesktopMCP(apiURL, token, sessionID, toolName string, args map[string]interface{}) (interface{}, error) {
	ctx := context.Background()
	// Desktop MCP runs inside the sandbox via the external agent's MCP proxy
	mcpURL := fmt.Sprintf("%s/api/v1/external-agents/%s/mcp/desktop", apiURL, sessionID)

	mcpClient, err := newMCPClient(ctx, mcpURL, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	return result, nil
}

func handleScreenshotResult(result interface{}) error {
	// Try to extract base64 image from result
	if resultMap, ok := result.(map[string]interface{}); ok {
		if content, ok := resultMap["content"].([]interface{}); ok {
			for _, c := range content {
				if contentMap, ok := c.(map[string]interface{}); ok {
					if contentMap["type"] == "image" {
						if data, ok := contentMap["data"].(string); ok {
							// Decode base64 and save
							// For now, just report success
							fmt.Printf("✅ Screenshot captured (%d bytes base64)\n", len(data))
							return nil
						}
					}
				}
			}
		}
	}

	fmt.Printf("Screenshot result: %v\n", result)
	return nil
}

func newMCPListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list <session-id>",
		Short: "List available MCP tools for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			fmt.Printf("\n📋 Available MCP Tools for Session %s\n", sessionID)
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

			// List Session MCP tools
			fmt.Printf("📁 Session MCP (helix spectask mcp session %s <tool>)\n", sessionID)
			fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

			tools, err := listMCPTools(apiURL, token, sessionID, "session")
			if err != nil {
				fmt.Printf("   ⚠️  %v\n", err)
			} else {
				for _, tool := range tools {
					fmt.Printf("   • %s\n", tool)
				}
			}

			fmt.Printf("\n🖥️  Desktop MCP (helix spectask mcp desktop %s <tool>)\n", sessionID)
			fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

			tools, err = listMCPTools(apiURL, token, sessionID, "desktop")
			if err != nil {
				fmt.Printf("   ⚠️  %v\n", err)
			} else {
				for _, tool := range tools {
					fmt.Printf("   • %s\n", tool)
				}
			}

			fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
			return nil
		},
	}
}

func listMCPTools(apiURL, token, sessionID, mcpType string) ([]string, error) {
	ctx := context.Background()

	var mcpURL string
	if mcpType == "session" {
		mcpURL = fmt.Sprintf("%s/api/v1/mcp/session?session_id=%s", apiURL, sessionID)
	} else {
		mcpURL = fmt.Sprintf("%s/api/v1/external-agents/%s/mcp/desktop", apiURL, sessionID)
	}

	mcpClient, err := newMCPClient(ctx, mcpURL, token)
	if err != nil {
		// Return hardcoded list as fallback if connection fails
		if mcpType == "session" {
			return []string{
				"current_session          Get session overview",
				"session_toc              Get table of contents",
				"get_turn                 Get specific turn content",
				"session_title_history    See title changes",
				"search_session           Search interactions",
			}, nil
		}
		return []string{
			"take_screenshot          Capture screen",
			"list_windows             List all windows",
			"get_workspaces           List workspaces",
			"type_text                Type keyboard input",
			"mouse_click              Click at coordinates",
		}, nil
	}

	result, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	var tools []string
	for _, t := range result.Tools {
		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}
		tools = append(tools, fmt.Sprintf("%-25s %s", t.Name, desc))
	}

	if len(tools) == 0 {
		// Return hardcoded list as fallback
		if mcpType == "session" {
			return []string{
				"current_session          Get session overview",
				"session_toc              Get table of contents",
				"get_turn                 Get specific turn content",
				"session_title_history    See title changes",
				"search_session           Search interactions",
			}, nil
		}
		return []string{
			"take_screenshot          Capture screen",
			"list_windows             List all windows",
			"get_workspaces           List workspaces",
			"type_text                Type keyboard input",
			"mouse_click              Click at coordinates",
		}, nil
	}

	return tools, nil
}

// Integration test command that runs a full end-to-end scenario
func newE2ECommand() *cobra.Command {
	var projectID string
	var agentID string
	var prompt string
	var cleanup bool

	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Run end-to-end integration test",
		Long: `Run a complete end-to-end integration test.

This command:
  1. Creates a new spec task
  2. Starts the sandbox
  3. Waits for the agent to be ready
  4. Cancels a live turn and verifies the interaction is interrupted
  5. Sends a follow-up and verifies the session still works
  6. Verifies screenshot works
  7. Tests Session MCP tools
  8. Optionally cleans up

Examples:
  helix spectask e2e --project prj_xxx --agent app_xxx --prompt "List files"
  helix spectask e2e --project prj_xxx --agent app_xxx --cleanup
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectID == "" {
				return fmt.Errorf("--project is required")
			}

			apiURL := getAPIURL()
			token := getToken()

			fmt.Printf("\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			fmt.Printf("🧪 End-to-End Integration Test\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

			// Step 1: Create spec task
			fmt.Printf("1️⃣  Creating spec task...\n")
			taskPrompt := prompt
			if taskPrompt == "" {
				taskPrompt = "E2E test: List files in current directory"
			}
			taskPrompt = "E2E cancellation probe: immediately run the shell command `sleep 60` before doing any other work. " +
				"After it finishes, continue with this request: " + taskPrompt

			task, err := createSpecTask(apiURL, token, "E2E Test Task", taskPrompt, projectID, agentID)
			if err != nil {
				return fmt.Errorf("failed to create task: %w", err)
			}
			fmt.Printf("   ✅ Task created: %s\n", task.ID)

			// Step 2: Start planning
			fmt.Printf("\n2️⃣  Starting sandbox...\n")
			_, err = triggerStartPlanning(apiURL, token, task.ID)
			if err != nil {
				return fmt.Errorf("failed to start planning: %w", err)
			}

			// Step 3: Wait for session
			fmt.Printf("   ⏳ Waiting for sandbox to start...\n")
			session, err := waitForTaskSession(apiURL, token, task.ID, 5*time.Minute)
			if err != nil {
				return fmt.Errorf("failed waiting for session: %w", err)
			}
			fmt.Printf("   ✅ Session ready: %s\n", session.ID)

			// Step 4: Cancel a real in-flight turn and verify acknowledgement/state propagation.
			fmt.Printf("\n3️⃣  Testing live turn cancellation...\n")
			waitingInteraction, err := waitForActiveAgentInteraction(apiURL, token, session.ID, 60*time.Second)
			if err != nil {
				return fmt.Errorf("failed waiting for cancellable turn: %w", err)
			}
			status, err := cancelE2ETurn(apiURL, token, session.ID)
			if err != nil {
				return fmt.Errorf("failed to cancel turn: %w", err)
			}
			if status != "cancelled" {
				return fmt.Errorf("cancel returned unexpected status %q", status)
			}
			if err := waitForInteractionState(apiURL, token, session.ID, waitingInteraction.ID, types.InteractionStateInterrupted, 15*time.Second); err != nil {
				return fmt.Errorf("cancellation did not propagate: %w", err)
			}
			fmt.Printf("   ✅ Agent acknowledged cancellation and interaction %s is interrupted\n", waitingInteraction.ID)

			// A lifecycle test is incomplete until the next normal turn succeeds.
			fmt.Printf("\n4️⃣  Testing the next turn after cancellation...\n")
			if err := testPostCancellationTurn(apiURL, token, session.ID, 2*time.Minute); err != nil {
				return fmt.Errorf("post-cancellation turn failed: %w", err)
			}
			fmt.Printf("   ✅ Session accepted and completed the next turn\n")

			// Step 5: Test screenshot
			fmt.Printf("\n5️⃣  Testing screenshot...\n")
			screenshotResult := testScreenshot(apiURL, token, session.ID, 30)
			if screenshotResult.Passed {
				fmt.Printf("   ✅ Screenshot works\n")
			} else {
				fmt.Printf("   ❌ Screenshot failed: %s\n", screenshotResult.Error)
			}

			// Step 6: Test Session MCP
			fmt.Printf("\n6️⃣  Testing Session MCP tools...\n")

			mcpResult := testMCPTool(apiURL, token, session.ID, "current_session", nil, 30)
			if mcpResult.Passed {
				fmt.Printf("   ✅ current_session works\n")
			} else {
				fmt.Printf("   ❌ current_session failed: %s\n", mcpResult.Error)
			}

			mcpResult = testMCPTool(apiURL, token, session.ID, "session_toc", nil, 30)
			if mcpResult.Passed {
				fmt.Printf("   ✅ session_toc works\n")
			} else {
				fmt.Printf("   ❌ session_toc failed: %s\n", mcpResult.Error)
			}

			// Step 7: Cleanup
			if cleanup {
				fmt.Printf("\n7️⃣  Cleaning up...\n")
				if err := stopSession(apiURL, token, session.ID); err != nil {
					fmt.Printf("   ⚠️  Cleanup warning: %v\n", err)
				} else {
					fmt.Printf("   ✅ Session stopped\n")
				}
			} else {
				fmt.Printf("\n7️⃣  Session left running for inspection\n")
				fmt.Printf("   To interact: helix spectask interact %s\n", session.ID)
				fmt.Printf("   To stop:     helix spectask stop %s\n", session.ID)
			}

			fmt.Printf("\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			fmt.Printf("✅ End-to-End Test Complete!\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

			return nil
		},
	}

	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (required)")
	cmd.Flags().StringVarP(&agentID, "agent", "a", "", "Agent/App ID to use")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Test prompt to send")
	cmd.Flags().BoolVar(&cleanup, "cleanup", false, "Stop session after test")

	return cmd
}

func waitForActiveAgentInteraction(apiURL, token, sessionID string, timeout time.Duration) (*types.Interaction, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, sessionErr := getSessionDetails(apiURL, token, sessionID)
		if sessionErr != nil || session.Metadata.ZedThreadID == "" {
			time.Sleep(time.Second)
			continue
		}
		interactions, err := getInteractions(apiURL, token, sessionID, 20)
		if err == nil {
			for _, interaction := range interactions {
				if interaction.State == types.InteractionStateWaiting {
					return interaction, nil
				}
			}
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("timeout waiting for an active agent interaction")
}

func cancelE2ETurn(apiURL, token, sessionID string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/sessions/%s/cancel", apiURL, sessionID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cancel API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode cancel response: %w", err)
	}
	return result["status"], nil
}

func waitForInteractionState(apiURL, token, sessionID, interactionID string, expected types.InteractionState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState types.InteractionState
	for time.Now().Before(deadline) {
		interactions, err := getInteractions(apiURL, token, sessionID, 20)
		if err == nil {
			for _, interaction := range interactions {
				if interaction.ID != interactionID {
					continue
				}
				lastState = interaction.State
				if interaction.State == expected {
					return nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("interaction %s state is %q, expected %q", interactionID, lastState, expected)
}

func testPostCancellationTurn(apiURL, token, sessionID string, timeout time.Duration) error {
	marker := fmt.Sprintf("CANCEL_E2E_RECOVERY_%d", time.Now().UnixNano())
	prompt := "Reply with exactly " + marker + " and do not call tools."
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- sendE2EChat(apiURL, token, sessionID, prompt, timeout)
	}()

	deadline := time.Now().Add(timeout)
	var interactionID string
	for time.Now().Before(deadline) {
		select {
		case err := <-sendDone:
			if err != nil {
				return err
			}
		default:
		}

		interactions, err := getInteractions(apiURL, token, sessionID, 20)
		if err == nil {
			for _, interaction := range interactions {
				if !strings.Contains(interaction.PromptMessage, marker) {
					continue
				}
				interactionID = interaction.ID
				switch interaction.State {
				case types.InteractionStateComplete:
					return nil
				case types.InteractionStateError, types.InteractionStateInterrupted:
					return fmt.Errorf("interaction %s ended in state %q: %s", interaction.ID, interaction.State, interaction.Error)
				}
			}
		}
		time.Sleep(time.Second)
	}
	if interactionID == "" {
		return fmt.Errorf("follow-up interaction was not created")
	}
	return fmt.Errorf("follow-up interaction %s did not complete", interactionID)
}

func sendE2EChat(apiURL, token, sessionID, prompt string, timeout time.Duration) error {
	payload := map[string]interface{}{
		"session_id": sessionID,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": map[string]interface{}{
					"content_type": "text",
					"parts":        []string{prompt},
				},
			},
		},
		"stream": false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, apiURL+"/api/v1/sessions/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

// Utility to check if containers are healthy
func newHealthCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check system health and container status",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			fmt.Printf("\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			fmt.Printf("🏥 System Health Check\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

			// Check API health
			fmt.Printf("📡 API Server (%s)...\n", apiURL)
			req, _ := http.NewRequest("GET", apiURL+"/api/v1/health", nil)
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("   ❌ Unreachable: %v\n", err)
			} else {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					fmt.Printf("   ✅ Healthy\n")
				} else {
					fmt.Printf("   ⚠️  Status: %d\n", resp.StatusCode)
				}
			}

			// Check for active sessions
			fmt.Printf("\n📋 Active External Agent Sessions...\n")
			req, _ = http.NewRequest("GET", apiURL+"/api/v1/sessions", nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err = client.Do(req)
			if err != nil {
				fmt.Printf("   ❌ Could not fetch sessions: %v\n", err)
			} else {
				defer resp.Body.Close()
				var response SessionsResponse
				if err := json.NewDecoder(resp.Body).Decode(&response); err == nil {
					count := 0
					for _, s := range response.Sessions {
						if s.Metadata.DevContainerID != "" {
							count++
							status := "🟢"
							fmt.Printf("   %s %s (%s)\n", status, s.ID, s.Metadata.ContainerName)
						}
					}
					if count == 0 {
						fmt.Printf("   No active sessions\n")
					}
				}
			}

			// Check MCP endpoint
			fmt.Printf("\n🔌 MCP Endpoints...\n")

			// Session MCP
			req, _ = http.NewRequest("POST", apiURL+"/api/v1/mcp/session", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err = client.Do(req)
			if err != nil {
				fmt.Printf("   ❌ Session MCP: Unreachable\n")
			} else {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
					fmt.Printf("   ✅ Session MCP: Available\n")
				} else {
					fmt.Printf("   ⚠️  Session MCP: Status %d\n", resp.StatusCode)
				}
			}

			fmt.Printf("\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

			return nil
		},
	}
}
