# Design: Claude Agent UX Parity in Zed

## Root Cause

The daemon's `generateAgentServerConfig` (helix: `api/cmd/settings-sync-daemon/main.go` ~line 188) writes:

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

Zed's model selector and bypass-permissions toggle are only rendered when the Claude Code ACP server reports `modes`, `models`, and `config_options` in its session initialization. These env vars (particularly `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`) suppress those reports, so Zed has nothing to render.

Zed's own native "Claude Agent" uses no such env overrides and shows the full UI.

## Solution

### For subscription mode: write nothing to `agent_servers.claude`

The daemon already writes OAuth credentials to `~/.claude/.credentials.json`. Claude Code reads this file natively. If we stop writing `agent_servers.claude`, Zed falls back to its built-in Claude Code config — which reports the full set of modes, models, and config options to Zed, giving users the model selector and bypass-permissions toggle.

**The one remaining problem:** Hydra injects `ANTHROPIC_BASE_URL` into all container environments (pointing at the Helix API proxy). Claude Code would inherit this and hit the proxy instead of `api.anthropic.com`, breaking subscription auth.

**Fix:** Unset `ANTHROPIC_BASE_URL` in the shell environment for subscription-mode sessions *before* Zed starts, rather than patching it per-agent in `agent_servers.claude`. The cleanest options, in order of preference:

1. **Hydra doesn't inject `ANTHROPIC_BASE_URL` in subscription-mode containers** — handled at container creation; the daemon never needs to care about it. (Cleanest; requires Hydra change.)
2. **Daemon unsets it in `~/.config/zed/settings.json` at the `lsp.env` / top-level env level** — Zed propagates this to all child processes.
3. **Daemon writes `unset ANTHROPIC_BASE_URL` to `~/.bashrc` or `~/.profile`** — Zed inherits the shell environment when launching Claude Code.
4. **Write `ANTHROPIC_BASE_URL=https://api.anthropic.com` to Claude Code's own config** (`~/.claude/settings.json`) — keeps it out of Zed settings entirely.

Option 1 is preferred; option 4 is a self-contained fallback if Hydra changes are out of scope.

### For API key mode

API key mode sets `ANTHROPIC_BASE_URL` (Helix proxy) and `ANTHROPIC_API_KEY` to route traffic through Helix. This also suppresses the ACP UI, but that's a separate concern — users on API key mode may or may not expect the same full UI. Out of scope for this task.

## Key Files

| File | Change |
|---|---|
| `helix/api/cmd/settings-sync-daemon/main.go` | In the `claude_code` subscription branch, return `nil` (no `agent_servers` config) instead of the current env block |
| `helix/api/pkg/...` (Hydra container creation) | Don't set `ANTHROPIC_BASE_URL` for subscription-mode sessions (preferred); OR |
| `helix/api/cmd/settings-sync-daemon/main.go` | Write `ANTHROPIC_BASE_URL=https://api.anthropic.com` to `~/.claude/settings.json` instead of via Zed settings |

## Notes for Future Agents

- The model selector and bypass-permissions toggle are advertised by the Claude Code ACP server via the ACP protocol (`modes`, `models`, `config_options` in session init response). They are not Zed-side features that can be configured in `agent_servers.claude`.
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` is very likely what suppresses these reports — remove it and the UI should appear.
- The daemon's `ClaudeCredentialsPath` (`~/.claude/.credentials.json`) and the `os.Stat` gate before writing subscription config are important correctness mechanisms — keep the credentials-sync logic, just stop writing `agent_servers.claude` env overrides.
- Zed's built-in Claude Code uses the npm package `@zed-industries/claude-code-acp`; no `command` or `args` override is needed in `agent_servers.claude` for the built-in package to work.
