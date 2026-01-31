package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

var (
	agentID   string
	model     string
	timeout   int
	stream    bool
	verbose   bool
	sessionID string
	agentType string
)

var rootCmd = &cobra.Command{
	Use:     "chat",
	Short:   "Chat with Helix agents",
	Aliases: []string{"c"},
	Long: `Send messages to Helix agents including external agents like Zed.

Examples:
  # Chat with a specific agent
  helix chat --agent=a1b2c3 "hi"
  
  # Continue an existing session
  helix chat --agent=a1b2c3 --session=ses_123 "what did we discuss?"
  
  # Use a specific model
  helix chat --agent=a1b2c3 --model=claude-3-5-sonnet "explain this code"
  
  # Enable verbose output to see debug info
  helix chat -v --agent=a1b2c3 "help me debug"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		message := args[0]

		if agentID == "" {
			return fmt.Errorf("agent ID is required (use --agent flag)")
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(timeout)*time.Second)
		defer cancel()

		// Create API client using environment variables (HELIX_URL, HELIX_API_KEY)
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		if verbose {
			fmt.Printf("🚀 Starting chat session...\n")
			fmt.Printf("🤖 Agent ID: %s\n", agentID)
			fmt.Printf("💬 Message: %s\n", message)
			if model != "" {
				fmt.Printf("🧠 Model: %s\n", model)
			}
			if sessionID != "" {
				fmt.Printf("💾 Session ID: %s\n", sessionID)
			}
			fmt.Printf("⏱️  Timeout: %ds\n", timeout)
			fmt.Printf("🌊 Stream: %t\n\n", stream)
		}

		// Create the session chat request
		chatReq := &types.SessionChatRequest{
			AppID:     agentID, // Agent ID goes in AppID field
			SessionID: sessionID,
			Messages: []*types.Message{
				{
					Role: "user", // Use string literal for role
					Content: types.MessageContent{
						ContentType: types.MessageContentTypeText,
						Parts:       []any{message}, // Simple text message
					},
				},
			},
			Stream: stream,
			Type:   types.SessionTypeText,
		}

		// Set agent type if specified via flag
		if agentType != "" {
			chatReq.AgentType = agentType

			// For external agents, provide empty config (wolf_executor handles paths)
			if agentType == "zed_external" {
				chatReq.ExternalAgentConfig = &types.ExternalAgentConfig{}
			}
		}

		// Set model if provided
		if model != "" {
			chatReq.Model = model
		}

		if verbose {
			fmt.Printf("📤 Sending request to /api/v1/sessions/chat...\n")
		}

		// Make the API request using the client method
		response, err := apiClient.ChatSession(ctx, chatReq)
		if err != nil {
			return fmt.Errorf("❌ Chat request failed: %w", err)
		}

		if verbose {
			fmt.Printf("✅ Request completed successfully!\n")
			fmt.Printf("📥 Response received:\n\n")
		}

		// Print the response
		fmt.Printf("🤖 Agent Response:\n")
		fmt.Printf("%s\n", response)

		return nil
	},
}

func init() {
	rootCmd.Flags().StringVarP(&agentID, "agent", "a", "", "Agent ID to chat with (required)")
	rootCmd.Flags().StringVar(&model, "model", "", "Model to use (optional, uses agent default)")
	rootCmd.Flags().StringVar(&sessionID, "session", "", "Session ID to continue existing conversation")
	rootCmd.Flags().StringVar(&agentType, "agent-type", "", "Agent type override (e.g., 'zed_external')")
	rootCmd.Flags().IntVar(&timeout, "timeout", 120, "Timeout in seconds")
	rootCmd.Flags().BoolVar(&stream, "stream", false, "Enable streaming response")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	rootCmd.MarkFlagRequired("agent")
}

func New() *cobra.Command {
	return rootCmd
}
