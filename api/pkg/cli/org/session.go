package org

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

// NewSessionCmd returns `helix session`: talk to and debug any agent session
// by id — bot main sessions, bot instances, chats. Sandbox-backed commands
// (exec, put, get, logs, screenshot) find the sandbox from the session.
func NewSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session",
		Aliases: []string{"sessions", "ses"},
		Short:   "Talk to and debug a session: send, turns, watch, exec, put, logs",
		Long: `Work with one agent session by id (bot main sessions and bot instances alike).

Examples:
  helix session send ses_01xxx "What documents do you need?" --attach licence.pdf
  helix session turns ses_01xxx --last 5          # tool calls, timings, final text per turn
  helix session watch ses_01xxx                    # follow the running turn
  helix session put ses_01xxx ./licence.pdf        # → ~/work/incoming/licence.pdf
  helix session exec ses_01xxx -- ls ~/work
  helix session logs ses_01xxx                     # processes, setup, agent and browser logs
`,
	}
	cmd.AddCommand(newSessionSendCmd(), newSessionTurnsCmd(), newSessionWatchCmd(), newSessionExecCmd(),
		newSessionPutCmd(), newSessionGetCmd(), newSessionLogsCmd(), newSessionScreenshotCmd())
	return silenceUsage(cmd)
}

func printTurnFooter(r *turnResult, tools bool) {
	fmt.Fprintf(os.Stderr, "[%.1fs session=%s]\n", r.Seconds, r.SessionID)
	if tools && r.Interaction != nil {
		fmt.Fprintf(os.Stderr, "[%d tool calls] %s\n", len(r.ToolCalls), toolSummary(r.ToolCalls))
	}
}

func newSessionSendCmd() *cobra.Command {
	var (
		attach  []string
		raw     bool
		tools   bool
		timeout int
		key     string
	)
	cmd := &cobra.Command{
		Use:   "send <session-id|-> [message...]",
		Short: "Send one turn and print the reply the user sees",
		Long: `Send one blocking turn through /api/v1/sessions/chat and print the final message
(every text entry after the last tool call). --raw prints the whole turn blob that the
API returns (thinking, tool calls, page snapshots) — what a gateway integrator receives.

--key sends as an app API key, the way an end customer's backend does. With --key and
session "-" the gateway creates a new bot instance and prints its session id.
Attachments go inline as base64 data: URLs and land in the agent's ~/work/incoming/.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sid := args[0]
			if sid == "-" {
				sid = ""
			}
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
			if sid == "" && key == "" {
				return fmt.Errorf(`session "-" needs --key (an app key starts a new bot instance)`)
			}
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			r, err := c.sendTurn(cmd.Context(), sid, msg, attach, key, time.Duration(timeout)*time.Second)
			if err != nil {
				return err
			}
			if raw {
				fmt.Println(r.Raw)
			} else {
				fmt.Println(r.Reply)
			}
			printTurnFooter(r, tools)
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&attach, "attach", "a", nil, "File to attach (repeatable)")
	cmd.Flags().BoolVar(&raw, "raw", false, "Print the whole turn blob instead of the final message")
	cmd.Flags().BoolVar(&tools, "tools", false, "Print a tool-call summary of the turn (stderr)")
	cmd.Flags().IntVar(&timeout, "timeout", 900, "Seconds to wait for the turn")
	cmd.Flags().StringVar(&key, "key", "", "Send with this app API key (gateway), instead of $HELIX_API_KEY")
	return cmd
}

func newSessionTurnsCmd() *cobra.Command {
	var (
		last    int
		full    bool
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "turns <session-id>",
		Short: "Show turns: prompt, state, duration, every tool call, final text",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			xs, err := c.interactions(cmd.Context(), args[0], last, "desc")
			if err != nil {
				return err
			}
			for l, r := 0, len(xs)-1; l < r; l, r = l+1, r-1 {
				xs[l], xs[r] = xs[r], xs[l]
			}
			if jsonOut {
				out := make([]map[string]any, 0, len(xs))
				for _, i := range xs {
					out = append(out, map[string]any{
						"id": i.ID, "state": i.State, "error": i.Error, "prompt": i.PromptMessage,
						"created": i.Created, "completed": i.Completed, "reply": finalText(i),
						"tool_calls": toolCalls(i),
					})
				}
				return printJSON(out)
			}
			for _, i := range xs {
				printTurn(i, full)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&last, "last", 20, "Number of most recent turns")
	cmd.Flags().BoolVar(&full, "full", false, "Do not truncate; keep thinking")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func printTurn(i *types.Interaction, full bool) {
	dur := "…"
	if !i.Completed.IsZero() && !i.Created.IsZero() && i.Completed.After(i.Created) {
		dur = fmt.Sprintf("%.1fs", i.Completed.Sub(i.Created).Seconds())
	}
	calls := toolCalls(i)
	head := fmt.Sprintf("── %s %s state=%s %s tools=%d", i.ID, i.Created.Format("2006-01-02 15:04:05"), i.State, dur, len(calls))
	if i.Error != "" {
		head += " ERROR=" + i.Error
	}
	fmt.Println(head)
	fmt.Println("  👤 " + indent(clip(i.PromptMessage, 400, full)))
	for _, e := range entriesOf(i) {
		printEntry(e, full)
	}
}

func printEntry(e entry, full bool) {
	switch e.Type {
	case "tool_call":
		body := e.Content
		if n := strings.Index(body, "\n"); n >= 0 {
			body = body[n+1:]
		}
		body = strings.Join(strings.Fields(body), " ")
		fmt.Printf("  🔧 %s [%s] %s\n", e.ToolName, e.ToolStatus, clip(body, 160, full))
	case "text":
		t := e.Content
		if !full {
			t = stripThinking(t)
		}
		if strings.TrimSpace(t) != "" {
			fmt.Println("  💬 " + indent(clip(t, 600, full)))
		}
	}
}

func clip(s string, n int, full bool) string {
	if full {
		return s
	}
	return truncate(s, n)
}

func indent(s string) string { return strings.ReplaceAll(s, "\n", "\n     ") }

func newSessionWatchCmd() *cobra.Command {
	var (
		interval float64
		follow   bool
	)
	cmd := &cobra.Command{
		Use:   "watch <session-id>",
		Short: "Follow the current turn live (tool calls and text as they land)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			seen := map[string]bool{}
			for {
				i, err := c.lastInteraction(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if i != nil {
					for _, e := range entriesOf(i) {
						k := fmt.Sprintf("%s/%s/%s/%d", i.ID, e.MessageID, e.ToolStatus, len(e.Content))
						if !seen[k] {
							seen[k] = true
							printEntry(e, false)
						}
					}
					if !follow && (i.State == types.InteractionStateComplete || i.State == types.InteractionStateError) {
						fmt.Printf("── turn %s %s %s\n", i.ID, i.State, i.Error)
						return nil
					}
				}
				select {
				case <-cmd.Context().Done():
					return nil
				case <-time.After(time.Duration(interval * float64(time.Second))):
				}
			}
		},
	}
	cmd.Flags().Float64Var(&interval, "interval", 3, "Poll interval seconds")
	cmd.Flags().BoolVar(&follow, "follow", false, "Keep following across turns")
	return cmd
}

func sessionOrg(cmd *cobra.Command, orgFlag string) (string, error) {
	c, err := newHTTPClient()
	if err != nil {
		return "", err
	}
	return c.resolveOrg(cmd.Context(), orgFlag)
}

func newSessionExecCmd() *cobra.Command {
	var (
		orgFlag string
		timeout int
	)
	cmd := &cobra.Command{
		Use:   "exec <session-id> -- <command...>",
		Short: "Run a shell command in the session's sandbox (as root)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID, err := sessionOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			res, err := execInSession(cmd.Context(), orgID, args[0], strings.Join(args[1:], " "), timeout)
			if err != nil {
				return err
			}
			fmt.Print(res.Stdout)
			fmt.Fprint(os.Stderr, res.Stderr)
			if res.ExitCode != nil && *res.ExitCode != 0 {
				os.Exit(*res.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().IntVar(&timeout, "timeout", 60, "Command timeout seconds")
	return cmd
}

func newSessionPutCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "put <session-id> <local-file> [remote-path]",
		Short: "Copy a file into the session's sandbox (default ~/work/incoming/<name>)",
		Long: `Copy a local file into the agent's sandbox — the reliable way to hand documents to a bot
instance (filestore and artifact links are not reachable with an instance's restricted key).
Files land root-owned, mode 0644: readable by the agent, not modifiable.`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID, err := sessionOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[1])
			if err != nil {
				return err
			}
			remote := "/home/retro/work/incoming/" + filepath.Base(args[1])
			if len(args) == 3 {
				remote = args[2]
			}
			sb, err := sandboxForSession(cmd.Context(), orgID, args[0], 2*time.Minute)
			if err != nil {
				return err
			}
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			if err := apiClient.WriteSandboxFile(cmd.Context(), orgID, sb.ID, remote, data, 0); err != nil {
				return err
			}
			fmt.Printf("wrote %s (%d bytes)\n", remote, len(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

func newSessionGetCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "get <session-id> <remote-path> [local-file]",
		Short: "Copy a file out of the session's sandbox (stdout if no local file)",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID, err := sessionOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			sb, err := sandboxForSession(cmd.Context(), orgID, args[0], 0)
			if err != nil {
				return err
			}
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			data, err := apiClient.ReadSandboxFile(cmd.Context(), orgID, sb.ID, args[1])
			if err != nil {
				return err
			}
			if len(args) == 3 {
				return os.WriteFile(args[2], data, 0o644)
			}
			_, err = os.Stdout.Write(data)
			return err
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

const sessionLogsScript = `
n=%d
sec() { echo; echo "===== $1"; }
sec "processes"; ps -eo pid,etime,rss,args --sort=start_time | grep -E 'opencode|dsh|goose|qwen|claude|codex|zed|chrome-devtools-mcp|chrome --' | grep -v -E 'grep|crashpad|--type=' | cut -c1-170
sec "browser MCP servers (one per live agent thread; growing across clears = leak)"; ps -eo args | grep -c '^chrome-devtools-mcp'
sec "workspace"; ls -la ~/work ~/work/incoming 2>/dev/null | head -40; test -f ~/.helix-setup-failed && { echo "SETUP FAILED:"; cat ~/.helix-setup-failed; }
sec "AGENTS.md (head)"; head -c 600 ~/work/AGENTS.md 2>/dev/null; echo
sec "skills"; ls ~/.agents/skills 2>/dev/null; env | grep -E '^HELIX_SKILLS='
sec "setup log"; tail -n $n ~/.helix-setup.log 2>/dev/null
sec "Zed.log errors"; grep -iE 'error|failed|panic|denied|timeout' ~/.local/share/zed/logs/Zed.log 2>/dev/null | tail -n $n | cut -c1-300
sec "opencode log"; tail -n $n ~/work/.opencode-state/opencode/log/*.log 2>/dev/null | cut -c1-300
sec "chrome"; tail -n $n /tmp/helix-chrome.log 2>/dev/null | cut -c1-300
`

func newSessionLogsCmd() *cobra.Command {
	var (
		orgFlag string
		lines   int
	)
	cmd := &cobra.Command{
		Use:   "logs <session-id>",
		Short: "Diagnostic bundle from the sandbox: processes, workspace, skills, setup/agent/browser logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID, err := sessionOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			res, err := execInSession(cmd.Context(), orgID, args[0], fmt.Sprintf(sessionLogsScript, lines), 60)
			if err != nil {
				return err
			}
			fmt.Print(res.Stdout, res.Stderr)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().IntVar(&lines, "lines", 30, "Lines per log")
	return cmd
}

func newSessionScreenshotCmd() *cobra.Command {
	var (
		orgFlag string
		out     string
	)
	cmd := &cobra.Command{
		Use:   "screenshot <session-id>",
		Short: "Save a JPEG of the session's desktop (desktop runtimes only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID, err := sessionOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			sb, err := sandboxForSession(cmd.Context(), orgID, args[0], 0)
			if err != nil {
				return err
			}
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			if out == "" {
				out = args[0] + ".jpg"
			}
			img, err := apiClient.GetSandboxScreenshot(cmd.Context(), orgID, sb.ID, 0)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, img, 0o644); err != nil {
				return err
			}
			fmt.Println("wrote", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVarP(&out, "output", "o", "", "Output file (default <session>.jpg)")
	return cmd
}
