# Design: Claude Agent UX Parity in Zed

## What `claude-agent-acp` Actually Is

`@zed-industries/claude-agent-acp` (github.com/zed-industries/claude-agent-acp) is an **ACP adapter for the Claude Agent SDK** — not the Claude Code CLI. It:

- Uses `ANTHROPIC_API_KEY` or `~/.claude/.credentials.json` OAuth for auth
- Dynamically queries available models from the SDK and reports them to Zed (which renders them as the model selector)
- Advertises session modes including `bypassPermissions`
- Reads a **managed settings file** at startup: `/etc/claude-code/managed-settings.json` (Linux), which can inject `env`, `model`, and `permissions.defaultMode` before the SDK initialises

## Current Daemon Behaviour

The daemon writes to `agent_servers.claude` in Zed's `settings.json`:

```json
{
  "agent_servers": {
    "claude": {
      "default_mode": "bypassPermissions",
      "env": {
        "ANTHROPIC_BASE_URL": "https://api.anthropic.com",
        "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
        "DISABLE_TELEMETRY": "1"
      }
    }
  }
}
```

This is the wrong injection point — it fights with user preferences (rewritten every 30s) and may interfere with how `claude-agent-acp` initialises. The exact mechanism suppressing the model selector and bypass-permissions toggle needs to be confirmed during implementation (likely `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` or the `ANTHROPIC_BASE_URL` override affecting SDK model fetching, but this is unverified).

## Proposed Solution

Use the managed settings file instead of `agent_servers.claude.env`. `claude-agent-acp` reads `/etc/claude-code/managed-settings.json` at startup via `loadManagedSettings()` → `applyEnvironmentSettings()` before the ACP session begins.

**`/etc/claude-code/managed-settings.json` for subscription mode:**
```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.anthropic.com"
  },
  "permissions": {
    "defaultMode": "bypassPermissions"
  }
}
```

**For API key mode:**
```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://helix-proxy/v1",
    "ANTHROPIC_API_KEY": "<user-api-key>"
  },
  "model": "claude-sonnet-4-6",
  "permissions": {
    "defaultMode": "bypassPermissions"
  }
}
```

Stop writing `agent_servers.claude` to Zed settings entirely. Zed uses its built-in Claude agent defaults — user model/mode changes persist across daemon cycles.

The credentials file (`~/.claude/.credentials.json`) continues to be written by the daemon as before.

## Key Files

| File | Change |
|---|---|
| `helix/api/cmd/settings-sync-daemon/main.go` | Write `/etc/claude-code/managed-settings.json` instead of returning `agent_servers.claude` config |
| `helix/api/cmd/settings-sync-daemon/main.go` | Stop writing `agent_servers` key to Zed settings for the `claude_code` runtime |

## Notes for Future Agents

- `claude-agent-acp` is NOT Claude Code CLI. It is the Claude Agent SDK wrapped in ACP. Source: github.com/zed-industries/claude-agent-acp.
- The managed settings file on Linux is `/etc/claude-code/managed-settings.json`.
- `ANTHROPIC_BASE_URL` must be overridden because Hydra injects a container-local proxy URL into all container environments. Writing via the managed settings file achieves this without touching Zed settings.
- Models are reported dynamically by the SDK — no need to populate `favorite_models` in Zed settings.
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` does not exist in `claude-agent-acp` — do not add it.
- The exact cause of missing model selector / bypass permissions UI should be confirmed by running the agent with and without the env overrides during implementation.
