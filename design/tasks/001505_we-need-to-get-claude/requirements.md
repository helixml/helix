# Requirements: Claude Code Integration — Make It Actually Work

## Context

Claude Code is one of the code agent runtimes available in Helix desktop sessions. Users configure it via the Helix UI (choosing `claude_code` runtime), and it runs inside Zed via the ACP (Agent Client Protocol). There are two credential modes:

1. **API key mode**: An Anthropic API key is configured via an inference provider in the system (either a global/system-level `ProviderEndpoint` or a user/org-level one). Requests route through Helix's Anthropic proxy, which resolves the appropriate provider endpoint and its API key.
2. **Subscription mode**: User connects their Claude subscription via OAuth, Claude Code talks directly to Anthropic

The recurring problem: **Claude Code keeps asking for permission on every tool use** (file edits, bash commands, reads), despite the system having multiple layers designed to prevent this. This has regressed at least 3 times (PRs #1629, #1637, #1778), each time due to a different subtle misconfiguration.

## User Stories

### US-1: Configure a Claude Code agent with an inference provider
As a user with an Anthropic inference provider configured (global or user-level), I want to create a coding agent that uses Claude Code, so I can use Claude as my autonomous coding agent without a subscription.

**Acceptance Criteria:**
- User selects "Claude Code" runtime and "API Key" credential mode in the UI
- User picks an Anthropic provider + Claude model (e.g., `claude-sonnet-4-20250514`)
- Agent is created and session starts without errors
- Claude Code receives `ANTHROPIC_BASE_URL` pointing at the Helix proxy; the proxy resolves the API key from the matching `ProviderEndpoint` (global, user, or org-level)
- Requests flow through Helix's `/v1/messages` proxy endpoint correctly

### US-2: Claude Code runs autonomously without permission prompts
As a user running a Claude Code session, I want the agent to execute tool calls (file edits, bash commands, file reads) without stopping to ask for permission, because it's running in a sandboxed container where permission prompts just block autonomous operation.

**Acceptance Criteria:**
- Claude Code's `~/.claude/settings.json` contains `"defaultMode": "bypassPermissions"` and `"skipDangerousModePermissionPrompt": true`
- Zed settings contain `tool_permissions.default = "allow"`
- The ACP agent_servers config includes `"default_mode": "bypassPermissions"` (not `"default"` — that was the last regression)
- Container has `IS_SANDBOX=1` env var so the ACP allows bypassPermissions mode
- No tool call prompts appear during normal agent operation — verified by running a real task

### US-3: End-to-end test for Claude Code permission bypass
As a developer, I want an automated test that verifies Claude Code doesn't prompt for permissions, so we stop regressing on this every few weeks.

**Acceptance Criteria:**
- Test starts a Claude Code session (API key mode)
- Sends a task that requires file creation, editing, and bash execution
- Verifies the agent completes the task without any permission prompt UI appearing
- Test fails if any of the bypass mechanisms are misconfigured
- Can run in CI or manually via the spectask CLI

## Non-Functional Requirements

- **No new dependencies**: All permission bypass mechanisms already exist. The work is about hardening configuration, adding tests, and documenting the layered bypass system.
- **Backward compatibility**: Subscription mode must continue working alongside API key mode.
- **Regression resistance**: The layered bypass system should be documented and tested so that changes to any one layer are caught.

## Known Bypass Layers (Must All Be Correct)

| Layer | Where | What | Last Broke |
|-------|-------|------|------------|
| 1. Claude Code settings.json | `~/.claude/settings.json` (written by `helix-workspace-setup.sh`) | `defaultMode: bypassPermissions`, `allow: [Bash, Read, Edit]` | PR #1629 |
| 2. Zed tool_permissions | `settings.json` (written by settings-sync-daemon) | `tool_permissions.default = "allow"` | PR #1637 |
| 3. ACP default_mode | agent_servers config in Zed settings (settings-sync-daemon) | `default_mode: "bypassPermissions"` | PR #1778 (was `"default"` not `"default_mode"`) |
| 4. IS_SANDBOX env var | Container env (set in `devcontainer.go`) | `IS_SANDBOX=1` | PR #1637 |

All four layers must be correctly configured or Claude Code will prompt for permissions.