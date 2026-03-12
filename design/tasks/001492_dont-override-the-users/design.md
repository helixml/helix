# Design: Don't Override User's Chosen Theme in Zed

## Root Cause Analysis

There are two bugs working together to clobber user settings:

### Bug 1: `syncFromHelix()` ignores persisted user overrides

In `api/cmd/settings-sync-daemon/main.go`, the `syncFromHelix()` function (called on daemon startup and `/reload`):

1. Resets `d.userOverrides = make(map[string]interface{})` (line 768) — blows away in-memory overrides
2. Calls `d.writeSettings(d.helixSettings)` (line 789) — writes Helix settings directly, no merge

Meanwhile, the poll path in `checkHelixUpdates()` correctly calls `d.mergeSettings(d.helixSettings, d.userOverrides)` before writing. The fix is making `syncFromHelix()` follow the same pattern.

The user overrides ARE persisted to the API (via `POST /zed-config/user` in `syncToHelix()`), but `syncFromHelix()` never fetches them back. It needs to call the API to restore overrides on startup.

### Bug 2: Server hardcodes `"Ayu Dark"` unconditionally

In `api/pkg/external-agent/zed_config.go:169`, `GenerateZedMCPConfig` always sets `config.Theme = "Ayu Dark"`. This means the Helix API always returns `theme: "Ayu Dark"` in the zed-config response, and the daemon always includes it in `d.helixSettings`.

Since `mergeSettings()` applies user overrides on top of Helix settings, this is fine IF the user's theme choice is in `userOverrides`. But it means theme is always "contested" — every poll cycle re-asserts the Helix default, which only gets overridden if `userOverrides` has a `theme` key.

This is actually correct behavior once Bug 1 is fixed. The server provides a default; user overrides win. No change needed here.

## Solution

### Change 1: Fetch user overrides in `syncFromHelix()` (daemon side)

Add an API call to fetch persisted user overrides before writing settings. The daemon already has an endpoint for saving overrides (`POST /zed-config/user`), but needs a corresponding `GET` to restore them.

**Option A — Add a GET endpoint on the server for `/zed-config/user`**
The server already has `GetUserZedOverrides()` in `zed_config.go`. We just need to wire up a GET handler.

**Option B — Include user overrides in the existing `/zed-config` response**
Piggyback on the existing config endpoint by adding a `user_overrides` field to `ZedConfigResponse`.

**Decision: Option B.** It avoids an extra HTTP round-trip on startup and keeps the daemon's initial sync as a single API call. The `ZedConfigResponse` already returns all config the daemon needs; user overrides belong there.

### Change 2: Merge before writing in `syncFromHelix()`

After fetching user overrides from the API response, use the existing `mergeSettings()` function before calling `writeSettings()`, exactly like `checkHelixUpdates()` does.

### Change 3: Also read existing on-disk overrides as fallback

On the very first startup (before any overrides have been persisted to the API), the user may have already customized `settings.json` from a previous daemon run. Read the on-disk file and extract overrides as a fallback when the API returns no overrides.

## Key Files

| File | Change |
|------|--------|
| `api/cmd/settings-sync-daemon/main.go` | `syncFromHelix()`: fetch user overrides from API response, populate `d.userOverrides`, call `mergeSettings()` before `writeSettings()` |
| `api/pkg/server/zed_config_handlers.go` | `getZedConfig()`: include persisted user overrides in response |
| `api/pkg/external-agent/zed_config.go` | No change needed (server-side merge helper already exists) |
| `api/pkg/types/types.go` | Add `UserOverrides` field to `ZedConfigResponse` |
| `api/cmd/settings-sync-daemon/main_test.go` | Add tests for merge-on-initial-sync behavior |

## Merge Precedence (unchanged)

```
settings.json = merge(helixSettings, userOverrides)
```

- Helix-managed keys (context_servers, language_models, agent, etc.) come from the API
- User overrides win for all non-protected keys (theme, font_size, vim_mode, etc.)
- `SECURITY_PROTECTED_FIELDS` (telemetry, agent_servers) are never synced to the API
- `context_servers` are deep-merged (user can add servers but Helix servers are preserved)

## Risks

- **Race condition on first startup**: If Zed writes a settings change before the daemon finishes initial sync, the daemon might overwrite it. This is an existing race (not introduced by this fix) and mitigated by the debounce timer + file watcher.
- **Stale overrides from API**: If the user's overrides reference a theme that no longer exists, Zed handles this gracefully (falls back to default theme). No special handling needed.