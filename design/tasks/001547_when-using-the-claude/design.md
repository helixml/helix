# Design: Claude Agent UX Parity in Zed

## What `claude-agent-acp` Actually Is

`@zed-industries/claude-agent-acp` (github.com/zed-industries/claude-agent-acp) is an **ACP adapter for the Claude Agent SDK** — not the Claude Code CLI. It:

- Uses `ANTHROPIC_API_KEY` or `~/.claude/.credentials.json` OAuth for auth
- Dynamically queries available models from the SDK and reports them to the ACP client (Zed renders these as the model selector)
- Advertises session modes including `bypassPermissions` — but **only if `!IS_ROOT || !!process.env.IS_SANDBOX`**
- Reads a **managed settings file** at startup: `/etc/claude-code/managed-settings.json` (Linux), which can inject `env`, `model`, and `permissions.defaultMode`

## Root Causes

### 1. Bypass permissions hidden on root

`src/acp-agent.ts`:
```typescript
const ALLOW_BYPASS = !IS_ROOT || !!process.env.IS_SANDBOX;
// bypassPermissions mode only added to availableModes if ALLOW_BYPASS is true
```

Helix containers can run as root. The fix is to set `IS_SANDBOX=1` in the process environment.

### 2. `agent_servers.claude.env` is the wrong injection point

The daemon currently writes env vars via `agent_servers.claude.env` in Zed settings. This fights with user preferences (overwritten every 30s) and isn't the mechanism `claude-agent-acp` expects. The package has a dedicated managed settings path for this purpose.

## Solution: Use the Managed Settings File

Instead of writing `agent_servers.claude.env` in Zed settings, the daemon should write `/etc/claude-code/managed-settings.json`. This file is read by `claude-agent-acp` at process startup (before the ACP session begins) via `loadManagedSettings()` → `applyEnvironmentSettings()`.

**`/etc/claude-code/managed-settings.json` for subscription mode:**
```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.anthropic.com",
    "IS_SANDBOX": "1"
  }
}
```

**For API key mode:**
```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://helix-proxy/v1",
    "ANTHROPIC_API_KEY": "<user-api-key>",
    "IS_SANDBOX": "1"
  },
  "model": "claude-sonnet-4-6"
}
```

With this approach, the daemon stops writing `agent_servers.claude` entirely. Zed uses its built-in Claude agent defaults, which means:
- User model/mode changes in the UI persist (daemon doesn't touch Zed settings)
- `IS_SANDBOX=1` makes bypass permissions appear regardless of root status
- `ANTHROPIC_BASE_URL` override still prevents the container's Hydra-injected URL from leaking in

The credentials file (`~/.claude/.credentials.json`) is still written by the daemon as before.

## Key Files

| File | Change |
|---|---|
| `helix/api/cmd/settings-sync-daemon/main.go` | Replace `generateAgentServerConfig` (which returns `agent_servers.claude`) with a function that writes `/etc/claude-code/managed-settings.json` |
| `helix/api/cmd/settings-sync-daemon/main.go` | Stop writing `agent_servers` key to Zed settings entirely for claude_code runtime |

## Notes for Future Agents

- `claude-agent-acp` is NOT Claude Code CLI. It's the Claude Agent SDK wrapped in ACP. Source: github.com/zed-industries/claude-agent-acp.
- The managed settings file on Linux is `/etc/claude-code/managed-settings.json`. Writing here requires root or appropriate permissions in the container — the daemon likely has this.
- `IS_SANDBOX=1` is the correct way to enable bypass permissions when running as root.
- `ANTHROPIC_BASE_URL` must be overridden because Hydra injects a container-local proxy URL into all container environments. The managed settings `env` block overrides process env before the SDK initializes.
- Models are reported dynamically by the SDK — the model selector appears automatically once auth works. No need to populate `favorite_models` in Zed settings.
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` does NOT exist in `claude-agent-acp`. Earlier spec was wrong about this.
