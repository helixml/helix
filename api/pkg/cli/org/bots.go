package org

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

func newBotsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bots",
		Short:   "List and manage helix-org bots",
		Aliases: []string{"bot"},
	}
	cmd.AddCommand(newBotsListCmd())
	cmd.AddCommand(newBotsGetCmd())
	cmd.AddCommand(newBotsStartCmd())
	cmd.AddCommand(newBotsStopCmd())
	cmd.AddCommand(newBotsRestartCmd())
	cmd.AddCommand(newBotsChatCmd())
	return cmd
}

func newBotsListCmd() *cobra.Command {
	var orgFlag string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bots in an organization",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			var bots []orgapi.BotDTO
			if err := c.doJSON(cmd.Context(), http.MethodGet, "/orgs/"+orgID+"/bots", nil, &bots, 30*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(bots)
			}
			fmt.Printf("%-28s %-24s %-10s %s\n", "ID", "NAME", "STATUS", "KIND")
			for _, b := range bots {
				kind := b.Kind
				if kind == "" {
					kind = "agent"
				}
				status := b.AgentStatus
				if status == "" {
					status = "-"
				}
				fmt.Printf("%-28s %-24s %-10s %s\n", b.ID, truncate(b.Name, 24), status, kind)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newBotsGetCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "get <bot-id>",
		Short: "Get one bot (detail + project/session ids)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			var detail orgapi.BotDetailDTO
			if err := c.doJSON(cmd.Context(), http.MethodGet, "/orgs/"+orgID+"/bots/"+args[0], nil, &detail, 30*time.Second); err != nil {
				return err
			}
			return printJSON(detail)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

func newBotsStartCmd() *cobra.Command {
	return botActionCmd("start", "Start (activate) a bot's agent desktop", http.MethodPost, "activate")
}
func newBotsStopCmd() *cobra.Command {
	return botActionCmd("stop", "Stop a bot's agent desktop", http.MethodPost, "stop-agent")
}
func newBotsRestartCmd() *cobra.Command {
	return botActionCmd("restart", "Restart a bot's agent (fresh session)", http.MethodPost, "restart-agent")
}

func botActionCmd(use, short, method, suffix string) *cobra.Command {
	var orgFlag string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   use + " <bot-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/orgs/%s/bots/%s/%s", orgID, args[0], suffix)
			// activate/restart → BotActivateDTO (202); stop → 204.
			if suffix == "stop-agent" {
				if err := c.doJSON(cmd.Context(), method, path, nil, nil, 120*time.Second); err != nil {
					return err
				}
				fmt.Printf("%s %s ok\n", use, args[0])
				return nil
			}
			var res orgapi.BotActivateDTO
			if err := c.doJSON(cmd.Context(), method, path, nil, &res, 120*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(res)
			}
			fmt.Printf("%s %s: activation_id=%s project_id=%s session_id=%s\n",
				use, args[0], res.ActivationID, res.ProjectID, res.SessionID)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newBotsChatCmd() *cobra.Command {
	var (
		orgFlag   string
		timeout   int
		noStart   bool
		waitPoll  int
		sessionID string
	)
	cmd := &cobra.Command{
		Use:   "chat <bot-id> [message...]",
		Short: "Send a message to a bot's exploratory chat session",
		Long: `Chat with a helix-org bot via its project exploratory session.

Resolves the bot's project, starts the agent if needed (unless --no-start),
finds or waits for the exploratory session, then POSTs the message to
/sessions/chat and prints the assistant reply.

Examples:
  helix org bots chat chief-of-staff "List bots and what repos b-mason has"
  helix org bots chat b-mason --org unmanned-org "status?"
  echo "hello" | helix org bots chat chief-of-staff
`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			botID := args[0]
			msg := strings.TrimSpace(strings.Join(args[1:], " "))
			if msg == "" {
				bts, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				msg = strings.TrimSpace(string(bts))
			}
			if msg == "" {
				return fmt.Errorf("message required (args or stdin)")
			}

			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			orgID, err := c.resolveOrg(ctx, orgFlag)
			if err != nil {
				return err
			}

			var detail orgapi.BotDetailDTO
			if err := c.doJSON(ctx, http.MethodGet, "/orgs/"+orgID+"/bots/"+botID, nil, &detail, 30*time.Second); err != nil {
				return err
			}
			projectID := detail.ProjectID
			if projectID == "" {
				return fmt.Errorf("bot %s has no project yet — try: helix org bots start %s", botID, botID)
			}

			if !noStart && detail.Bot.AgentStatus != "running" {
				fmt.Fprintf(os.Stderr, "starting bot %s…\n", botID)
				_ = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/orgs/%s/bots/%s/activate", orgID, botID), nil, nil, 120*time.Second)
			}

			sid := sessionID
			if sid == "" {
				var err error
				sid, err = waitExploratorySession(ctx, c, projectID, time.Duration(waitPoll)*time.Second)
				if err != nil {
					return err
				}
			}
			fmt.Fprintf(os.Stderr, "session %s → %s\n", sid, botID)

			reply, err := sendSessionChat(ctx, sid, msg, time.Duration(timeout)*time.Second)
			if err != nil {
				return err
			}
			fmt.Println(reply)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Chat request timeout seconds")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "Do not activate the bot if stopped")
	cmd.Flags().IntVar(&waitPoll, "wait", 90, "Seconds to wait for exploratory session after start")
	cmd.Flags().StringVar(&sessionID, "session", "", "Use this session id (skip exploratory lookup)")
	return cmd
}

func waitExploratorySession(ctx context.Context, c *httpClient, projectID string, maxWait time.Duration) (string, error) {
	deadline := time.Now().Add(maxWait)
	if maxWait <= 0 {
		deadline = time.Now().Add(90 * time.Second)
	}
	var lastErr error
	for time.Now().Before(deadline) {
		var sess types.Session
		err := c.doJSON(ctx, http.MethodGet, "/projects/"+projectID+"/exploratory-session", nil, &sess, 15*time.Second)
		if err == nil && sess.ID != "" {
			return sess.ID, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	var sess types.Session
	if err := c.doJSON(ctx, http.MethodPost, "/projects/"+projectID+"/exploratory-session", map[string]any{}, &sess, 60*time.Second); err == nil && sess.ID != "" {
		return sess.ID, nil
	}
	if lastErr != nil {
		return "", fmt.Errorf("no exploratory session for project %s: %w", projectID, lastErr)
	}
	return "", fmt.Errorf("no exploratory session for project %s after waiting", projectID)
}

func sendSessionChat(ctx context.Context, sessionID, message string, timeout time.Duration) (string, error) {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := apiClient.ChatSession(reqCtx, &types.SessionChatRequest{
		SessionID: sessionID,
		Type:      types.SessionTypeText,
		Stream:    false,
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					ContentType: types.MessageContentTypeText,
					Parts:       []any{message},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return extractAssistantText([]byte(resp)), nil
}

// extractAssistantText pulls human-readable content from either an OpenAI-style
// chat.completion JSON or a plain Session / SSE blob.
func extractAssistantText(raw []byte) string {
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return string(raw)
	}
	if choices, ok := asMap["choices"].([]any); ok && len(choices) > 0 {
		if ch, ok := choices[0].(map[string]any); ok {
			if msg, ok := ch["message"].(map[string]any); ok {
				if content, ok := msg["content"].(string); ok {
					return content
				}
			}
			if text, ok := ch["text"].(string); ok {
				return text
			}
		}
	}
	if s, ok := asMap["response"].(string); ok {
		return s
	}
	if s, ok := asMap["content"].(string); ok {
		return s
	}
	b, _ := json.MarshalIndent(asMap, "", "  ")
	return string(b)
}
