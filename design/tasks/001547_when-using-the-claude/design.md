# Design: Claude Agent UX Parity in Zed

## Root Cause

The settings-sync-daemon's `generateAgentServerConfig` function (helix: `api/cmd/settings-sync-daemon/main.go` ~line 188) writes:

```json
{
  "agent_servers": {
    "claude": {
      "default_mode": "bypassPermissions",
      "env": { ... }
    }
  }
}
```

Zed reads `agent_servers.claude` into `BuiltinAgentServerSettings` (zed: `crates/settings_content/src/agent.rs`). Its model selector (`crates/agent_servers/src/claude.rs`) calls `favorite_model_ids()` which reads `settings.claude.favorite_models`. With that field absent, Zed has no list of models to display → model selector is invisible.

The `default_mode` being hardcoded every 30 seconds also prevents the user from changing their preferred mode via the Zed UI (the daemon immediately overwrites it).

## How Zed Uses These Fields

| Field | Zed behavior when absent | Zed behavior when present |
|---|---|---|
| `favorite_models` | Model selector hidden / empty | Model selector lists these models |
| `default_model` | No model pre-selected | Model pre-selected in dropdown |
| `default_mode` | Agent uses its own default | Agent starts in the specified mode |

## Solution

### 1. Populate `favorite_models` in daemon config

The daemon should write the list of Claude models that the session can use:

- **Subscription mode:** Include the full set of models that Claude Code natively supports (these do not change often): `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5-20251001`. The daemon can keep this as a hardcoded list or receive it from the API alongside the `CodeAgentConfig`.
- **API key mode (Helix proxy):** Same approach — include the models that the Helix assistant is configured to expose. The `CodeAgentConfig` already carries the configured model; the daemon can write it as both `default_model` and the sole entry in `favorite_models`.

### 2. Treat `default_mode` as a user-owned setting

The daemon already has the concept of "user overrides" (settings that are preserved across sync cycles). `default_mode` should be added to the user-owned bucket so the daemon does not clobber it.

On first write (when no user preference exists), the daemon sets `default_mode: "bypassPermissions"` for subscription mode (matching current behaviour). On subsequent syncs it does not touch `default_mode` unless the underlying runtime changes.

### 3. Resulting config shape

**Subscription mode:**
```json
{
  "agent_servers": {
    "claude": {
      "default_mode": "bypassPermissions",
      "default_model": "claude-sonnet-4-6",
      "favorite_models": ["claude-haiku-4-5-20251001", "claude-sonnet-4-6", "claude-opus-4-6"],
      "env": { ... }
    }
  }
}
```

**API key mode (e.g., model = claude-sonnet-4-6):**
```json
{
  "agent_servers": {
    "claude": {
      "default_model": "claude-sonnet-4-6",
      "favorite_models": ["claude-sonnet-4-6"],
      "env": { ... }
    }
  }
}
```

## Key Files

| File | Change |
|---|---|
| `helix/api/cmd/settings-sync-daemon/main.go` | `generateAgentServerConfig`: add `favorite_models` + `default_model`; make `default_mode` user-owned |
| `helix/api/cmd/settings-sync-daemon/main.go` | User-overrides logic: add `agent_servers.claude.default_mode` to protected user fields |

No Zed-side changes needed — Zed's model selector already works correctly once the settings fields are present.

## Notes for Future Agents

- The daemon's "user overrides" mechanism (already implemented for other fields) is the right pattern here — see `userOverrides` field in `SettingsDaemon` struct and how it's applied during the merge step.
- The model list for subscription mode can be a hardcoded constant in the daemon; it changes rarely and aligns with what `@zed-industries/claude-code-acp` natively exposes.
- `bypassPermissions` is a valid Claude Code ACP session mode ID (not a Zed-invented concept).
- `default_model` in `BuiltinAgentServerSettings` (Zed) is the model ID string as reported by the Claude Code ACP server, not a Zed model ID.
