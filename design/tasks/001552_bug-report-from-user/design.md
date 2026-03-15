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

Two complementary changes:

**1. Upgrade claude CLI before login (primary fix for existing deployments)**

In `ClaudeSubscriptionConnect.tsx`, before sending `claude auth login`, send a prior exec command to upgrade the CLI:

```
npm install -g @anthropic-ai/claude-code@latest
```

This self-heals even on old Mac app VM images that are already deployed and can't be updated without a full app release. No additional requirements — internet access is already a prerequisite for Claude OAuth to work at all. Adds a few seconds of latency to the login flow.

**2. Pin the version in the Dockerfile (fix for future builds)**

Change `Dockerfile.ubuntu-helix:929` from `@latest` to an explicit version so dev and prod images are identical and regressions are reproducible. Bump the pin deliberately when upgrading. The runtime upgrade in step 1 means the pinned version is a floor, not a ceiling — users always get at least the pinned version, and the login flow upgrades to latest.

The existing UI flow in `ClaudeSubscriptionConnect.tsx` already embeds the desktop terminal via `ExternalAgentDesktopViewer`, so the user can paste the code back. The alert text on line 431 already says to paste a code — this matches the new CLI behaviour.

Also add a "paste credentials JSON" fallback (see tasks) as insurance against future CLI regressions.

## Codebase Patterns

- Frontend: uses `api.getApiClient()` generated client — see `v1ExternalAgentsExecCreate` for the exec call pattern
- Backend: credentials stored as `ClaudeOAuthCredentials` in `api/pkg/types/claude_subscription.go`; encrypted at rest
- Rebuilding the desktop image requires `./stack build-ubuntu`
- The helix-ubuntu image version shown by `cat sandbox-images/helix-ubuntu.version`
