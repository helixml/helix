package org

import (
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

// `helix org bots doctor`: config lint for a bot, plus a sandbox health check
// when given one of its sessions. Findings come from real failure modes; see
// the helix-bot-builder skill's troubleshooting reference.
func newBotsDoctorCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "doctor <bot-id> [session-id]",
		Short: "Lint a bot's config and (with a session) check its sandbox",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			orgID, err := c.resolveOrg(ctx, orgFlag)
			if err != nil {
				return err
			}
			b, err := c.getBot(ctx, orgID, args[0])
			if err != nil {
				return err
			}
			var probs, notes []string
			rt := b.str("code_agent_runtime")
			if rt == "" {
				rt = b.str("agent_runtime")
			}
			content := b.str("content")
			prof := b.profile()
			botTools := map[string]bool{}
			for _, t := range b.strs("tools") {
				botTools[t] = true
			}
			switch rt {
			case "zed_agent":
				probs = append(probs, "harness zed_agent loses AGENTS.md after every clear/new thread (open bug) — use opencode or deepseek_harness")
			case "goose_code":
				notes = append(notes, "goose_code makes extra side calls and has leaked browser MCP servers — prefer opencode or deepseek_harness")
			case "claude_code", "codex_cli":
				notes = append(notes, rt+" only works with vendor models (subscription or vendor key)")
			}
			if strings.TrimSpace(content) == "" {
				probs = append(probs, "empty prompt")
			} else if len(content) > 20000 {
				notes = append(notes, fmt.Sprintf("prompt is %d chars — move per-system detail into repo skills (.agents/skills)", len(content)))
			}
			if v, _ := b["restart_required"].(bool); v {
				notes = append(notes, "restart_required: the main session runs stale config (instances unaffected)")
			}
			if b.str("sandbox_status") == "failed" {
				probs = append(probs, "sandbox failed: "+b.str("sandbox_status_message"))
			}
			var served, dropped []string
			for _, t := range prof.Tools {
				if botTools[t] {
					served = append(served, t)
				} else {
					dropped = append(dropped, t)
				}
			}
			if len(dropped) > 0 {
				probs = append(probs, fmt.Sprintf("instance_profile.tools %v are not on the bot, so instances never get them", dropped))
			}
			hasMCP := func(n string) bool {
				for _, m := range prof.MCPServers {
					if m == n {
						return true
					}
				}
				return false
			}
			if strings.Contains(content, "get_secret") && !contains(served, "get_secret") {
				probs = append(probs, "prompt mentions get_secret but instances are not served it (add to instance_profile.tools and the bot's tools)")
			}
			if strings.Contains(strings.ToLower(content), "browser") && !hasMCP("chrome-devtools") {
				probs = append(probs, "prompt relies on the browser but instance_profile.mcp_servers lacks chrome-devtools")
			}
			if rt == "deepseek_harness" && hasMCP("helix-desktop") {
				notes = append(notes, "deepseek_harness fails session/new if any MCP server fails; helix-desktop fails on headless")
			}
			instRuntime := prof.SandboxRuntime
			if instRuntime == "" {
				instRuntime = b.str("effective_sandbox_runtime")
			}
			if instRuntime == "ubuntu-desktop" {
				notes = append(notes, "instances run ubuntu-desktop; headless-ubuntu starts faster unless a human must watch")
			}
			if ins, err := c.listInstances(ctx, orgID, args[0]); err == nil {
				idle := 0
				for _, i := range ins {
					if i.SandboxStatus == "" || i.SandboxStatus == "terminated_idle" || i.SandboxStatus == "stopped" {
						idle++
					}
				}
				if idle > 0 {
					notes = append(notes, fmt.Sprintf("%d idle/stopped instances (helix org instances delete %s --all --idle)", idle, args[0]))
				}
			}
			fmt.Printf("bot %s: harness=%s model=%s status=%s instance_runtime=%s instance_mcp=%v instance_tools=%v\n",
				args[0], rt, b.str("model"), b.str("status"), instRuntime, prof.MCPServers, served)

			if len(args) == 2 {
				sid := args[1]
				res, err := execInSession(ctx, orgID, sid, `test -f ~/.helix-setup-failed && echo SETUP_FAILED && cat ~/.helix-setup-failed
echo AGENTS=$(wc -c < ~/work/AGENTS.md 2>/dev/null || echo 0)
echo SKILLS=$(ls ~/.agents/skills 2>/dev/null | tr '\n' ' ')
echo CDM=$(ps -eo args | grep -c '^chrome-devtools-mcp')
echo CHROME=$(ps -eo args | grep -c '^/opt/google/chrome/chrome --')`, 30)
				if err != nil {
					probs = append(probs, err.Error())
				} else {
					out := res.Stdout
					fmt.Print(out)
					if strings.Contains(out, "SETUP_FAILED") {
						probs = append(probs, "workspace setup failed (helix session logs "+sid+")")
					}
					if strings.Contains(out, "AGENTS=0") {
						probs = append(probs, "AGENTS.md missing or empty in the sandbox")
					}
				}
				if last, err := c.lastInteraction(ctx, sid); err == nil && last != nil {
					switch last.State {
					case types.InteractionStateError:
						probs = append(probs, "last turn errored: "+last.Error)
					case types.InteractionStateWaiting:
						notes = append(notes, "last turn still running (helix session watch "+sid+")")
					}
				}
			}
			for _, p := range probs {
				fmt.Println("✗", p)
			}
			for _, n := range notes {
				fmt.Println("•", n)
			}
			if len(probs) == 0 {
				fmt.Println("✓ no blocking problems found")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
