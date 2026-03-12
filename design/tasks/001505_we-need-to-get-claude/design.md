# Design: Claude Code Integration — Hardening Permission Bypass & API Key Mode

## Problem Statement

Claude Code in Helix desktop sessions keeps regressing to asking permission for every tool use. This has happened 3+ times (PRs #1629, #1637, #1778), each caused by a different subtle misconfiguration in the 4-layer permission bypass stack. The root cause is that the bypass system is fragile, undocumented as a unit, and has zero automated tests.

Additionally, the API key mode (where users provide their own Anthropic key rather than a Claude subscription) needs to be verified end-to-end since most testing has focused on subscription mode.

## Architecture Overview

Claude Code runs inside Zed via the ACP (Agent Client Protocol). The flow is:

```
User → Helix UI (configure agent) → Helix API (build config) → Settings-sync-daemon (write Zed settings) → Zed → Claude Code ACP
```

For API key mode specifically:
```
Claude Code → ANTHROPIC_BASE_URL (Helix proxy) → /v1/messages → Anthropic API (with user's API key)
```

### The 4-Layer Permission Bypass Stack

All four must be correctly configured or Claude Code prompts for every action:

```
Layer 1: ~/.claude/settings.json
  Written by: helix-workspace-setup.sh (at container start, before Zed launches)
  Contains: {"permissions":{"allow":["Bash","Read","Edit"],"defaultMode":"bypassPermissions"},"skipDangerousModePermissionPrompt":true}
  Why: Claude Code's own internal permission system

Layer 2: Zed tool_permissions
  Written by: settings-sync-daemon (syncFromHelix function, every poll cycle)
  Contains: agent.tool_permissions.default = "allow"
  Why: Zed's tool permission system — gates ACP tool calls before they reach Claude Code

Layer 3: ACP agent_servers default_mode
  Written by: settings-sync-daemon (generateAgentServerConfig function)
  Contains: agent_servers.claude.default_mode = "bypassPermissions"
  Why: Tells the Claude Code ACP wrapper to start in bypass mode

Layer 4: IS_SANDBOX env var
  Set by: devcontainer.go (buildEnv function)
  Contains: IS_SANDBOX=1
  Why: Claude Code ACP checks (!IS_ROOT || IS_SANDBOX) — containers run as root, so without IS_SANDBOX it blocks bypassPermissions
```

## Key Design Decisions

### Decision 1: Add a startup validation check in settings-sync-daemon

**Rationale:** Rather than only discovering bypass regressions when a user notices prompts, the settings-sync-daemon should validate all bypass layers on startup and log warnings if any are misconfigured.

**Approach:** After writing settings, read them back and verify:
- `tool_permissions.default` is `"allow"`
- `agent_servers.claude.default_mode` is `"bypassPermissions"` (not `"default"`, not missing)
- Log a clear `WARN` if any layer is wrong

### Decision 2: Add an E2E smoke test for Claude Code

**Rationale:** The only way to prevent repeated regressions is to test the actual behavior. A test that starts a session, sends a task, and checks for prompt-free completion catches any layer failure.

**Approach:** Use the existing `spectask` CLI infrastructure:
- `spectask start` with a Claude Code agent (API key mode)
- `spectask send` a simple task: "Create a file called /tmp/test.txt with the contents 'hello world', then run 'cat /tmp/test.txt'"
- `spectask stream` to watch output
- Verify task completes without timeout (permission prompts cause the agent to hang waiting for user input)
- This can be a shell script in `tests/` or a Go test

### Decision 3: Document the bypass stack in code comments

**Rationale:** Each previous regression happened because someone changed one layer without understanding the full stack. A single authoritative comment block in `settings-sync-daemon/main.go` listing all 4 layers prevents this.

**Approach:** Add a `// PERMISSION BYPASS STACK` comment block near the top of `generateAgentServerConfig()` explaining all 4 layers and linking to this design doc. Also add a section to `CLAUDE.md` developer guide.

### Decision 4: No changes to the Zed fork

**Rationale:** The Zed side (`tool_permissions.default = "allow"`) already works correctly. The problems have always been on the Helix side (wrong field names, missing settings, wrong env vars). No Zed changes needed.

## API Key Mode — What to Verify

The API key flow for Claude Code:

1. **UI**: User selects `claude_code` runtime + `api_key` credential type + Anthropic provider + Claude model
2. **API** (`buildCodeAgentConfigFromAssistant`): Builds `CodeAgentConfig` with `baseURL = helixURL` (no `/v1` suffix — Claude Code SDK appends `/v1/messages` itself)
3. **Settings-sync-daemon** (`generateAgentServerConfig`): Sets `ANTHROPIC_BASE_URL` and `ANTHROPIC_API_KEY` env vars in agent_servers config
4. **Helix proxy** (`/v1/messages`): Forwards to Anthropic with the user's API key, tracks usage

Key gotcha already handled: `baseURL` must NOT have `/v1` suffix because the Anthropic SDK appends `/v1/messages` to `ANTHROPIC_BASE_URL`.

## Codebase Patterns Discovered

- **Settings-sync-daemon** (`api/cmd/settings-sync-daemon/main.go`) is the central config writer. It polls the Helix API every 30s and writes Zed's `settings.json`. All agent_servers config goes through `generateAgentServerConfig()`.
- **helix-workspace-setup.sh** (`desktop/shared/`) runs once at container start, before Zed. It sets up `~/.claude/settings.json` with bypass permissions. It symlinks `~/.claude` to persistent storage for cross-session persistence.
- **devcontainer.go** (`api/pkg/hydra/`) builds the container environment. `IS_SANDBOX=1` is hardcoded there.
- **Field name sensitivity**: The ACP config uses `default_mode` (underscore), not `default` (which is a JSON keyword in some contexts). PR #1778's entire fix was changing `"default"` to `"default_mode"`.
- **Claude Code credential memoization**: Claude Code's credential reader is memoized. If it reads before `~/.claude/.credentials.json` exists, it caches null permanently. The settings-sync-daemon gates on file existence before emitting `agent_servers` config to prevent this.

## Risks

- **Claude Code npm updates**: Claude Code is installed via `npm install -g @anthropic-ai/claude-code@latest` in the Dockerfile. A new version could change permission semantics or config format. Consider pinning the version.
- **Zed rebase**: When rebasing the Zed fork, `tool_permissions` handling could change upstream. The porting guide should flag this.
- **ACP protocol changes**: The `default_mode` field in agent_servers config is part of the ACP protocol between Zed and Claude Code. Changes here require coordinated updates.