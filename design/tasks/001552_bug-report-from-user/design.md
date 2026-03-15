# Design: Fix Claude Subscription OAuth Redirect URI Error

## Key Files

| File | Role |
|------|------|
| `Dockerfile.ubuntu-helix:929` | Installs claude CLI — cache-bust here to pick up latest version |
| `frontend/src/components/account/ClaudeSubscriptionConnect.tsx` | Sends `claude auth login` command, polls for creds |
| `api/pkg/server/claude_subscription_handlers.go:447` | `startClaudeLogin` — creates desktop container |
| `api/pkg/server/claude_subscription_handlers.go:550` | `pollClaudeLogin` — reads credentials from container |

## Why This Breaks

The Helix v2.8.3 production image was built with a stale `claude` CLI. Old versions used a random localhost port for OAuth callback (`http://localhost:37907/callback`), which Anthropic's server now rejects.

**Confirmed by testing inside the current helix-ubuntu image**: `claude` 2.1.73 (installed here) runs:

```
claude auth login
→ redirect_uri=https://platform.claude.com/oauth/code/callback
```

No localhost port. Instead the user visits a URL, gets a code from claude.ai, and pastes it back into the terminal. The production v2.8.3 image has an older version where this was different.

## Fix

Bust the Docker layer cache for the `npm install` line in `Dockerfile.ubuntu-helix:929` and rebuild with `./stack build-ubuntu`. The rebuilt image will install the current CLI version which uses the server-side callback.

The existing UI flow in `ClaudeSubscriptionConnect.tsx` already embeds the desktop terminal via `ExternalAgentDesktopViewer`, so the user can paste the code back. The alert text on line 431 already says to paste a code — this matches the new CLI behaviour.

Also add a "paste credentials JSON" fallback (see tasks) as insurance against future CLI regressions.

## Codebase Patterns

- Frontend: uses `api.getApiClient()` generated client — see `v1ExternalAgentsExecCreate` for the exec call pattern
- Backend: credentials stored as `ClaudeOAuthCredentials` in `api/pkg/types/claude_subscription.go`; encrypted at rest
- Rebuilding the desktop image requires `./stack build-ubuntu`
- The helix-ubuntu image version shown by `cat sandbox-images/helix-ubuntu.version`
