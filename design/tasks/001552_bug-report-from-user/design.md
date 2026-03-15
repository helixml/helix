# Design: Fix Claude Subscription OAuth Redirect URI Error

## Key Files

| File | Role |
|------|------|
| `frontend/src/components/account/ClaudeSubscriptionConnect.tsx` | Sends `claude auth login` command, polls for creds |
| `api/pkg/server/claude_subscription_handlers.go:447` | `startClaudeLogin` — creates desktop container |
| `api/pkg/server/claude_subscription_handlers.go:550` | `pollClaudeLogin` — reads credentials from container |
| `desktop/shared/start-zed-core.sh:174` | `HELIX_SKIP_ZED=1` path — skips Zed for login-only sessions |

## Why This Breaks

`claude auth login` picks a random local port (e.g., 37907) and constructs a redirect URI `http://localhost:37907/callback`. On a normal desktop this works: RFC 8252 (OAuth for Native Apps) lets authorization servers accept any loopback port, so Anthropic's server accepts it for legitimate CLI installs.

Inside Helix's container it fails. The most likely explanation is that the version of `claude` CLI installed in the helix-ubuntu image is outdated. Newer CLI versions may use a different OAuth client ID or a different auth flow that Anthropic currently supports. The investigation task below should confirm which version is installed and whether upgrading fixes it.

## Fix Strategy

### Fix 1 (Primary): Use `claude` CLI's non-browser auth flow

Investigate whether the `claude` CLI supports a flag to bypass the local callback server and instead use a device/manual code flow. Common patterns:
- `claude auth login --no-launch-browser` — prints a URL for the user to visit manually, then waits for a code to be pasted
- `claude auth login --device` — device authorization grant flow

If such a flag exists, update the exec command in `ClaudeSubscriptionConnect.tsx` (line 333) from:
```
['claude', 'auth', 'login']
```
to:
```
['claude', 'auth', 'login', '--no-launch-browser']   // or whichever flag works
```

The embedded desktop viewer already shows the GNOME terminal output, so the user can paste the auth code back in.

**Note**: The current UI alert already hints at this flow: *"Claude will email you a link — click it to get a code, then paste the code back here to authenticate."* This suggests the non-browser flow was the intended approach.

### Fix 2 (Fallback): Add manual credentials paste in Helix UI

Add a "Paste credentials" option to `ClaudeSubscriptionConnect.tsx`. The user can:
1. Run `claude auth login` on their own machine (where it works)
2. Copy `~/.claude/.credentials.json`
3. Paste it into the Helix UI text field

The API already accepts raw credentials JSON via `POST /api/v1/claude-subscriptions` with the `credentials` field. No backend changes needed — the frontend just needs a text area + submit button.

This is a robust fallback that doesn't depend on container OAuth at all.

### Fix 3 (Investigation): Update claude CLI version

Check which version of the `claude` CLI is installed in the helix-ubuntu image (see `desktop/ubuntu/Dockerfile` or similar). Compare with the latest release. A newer version may have fixed the port handling or switched to a registered fixed port.

Update the installation step if a newer version resolves the issue.

## Codebase Patterns

- Frontend: uses `api.getApiClient()` generated client — see `v1ExternalAgentsExecCreate` for the exec call pattern
- Backend: credentials stored as `ClaudeOAuthCredentials` in `api/pkg/types/claude_subscription.go`; encrypted at rest
- Settings-sync-daemon writes credentials to `/home/retro/.claude/.credentials.json` for in-container claude CLI use
- Rebuilding the desktop image requires `./stack build-ubuntu`

## Decision

Implement **Fix 1** first (change the auth command flags) since it's the smallest change. Add **Fix 2** (manual paste) in the same PR as a permanent fallback regardless of Fix 1's outcome. Skip Fix 3 unless Fix 1 is blocked.
