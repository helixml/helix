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

No localhost port. The CLI tries to open a browser to the OAuth URL; after the user authenticates, the flow continues via `https://platform.claude.com/oauth/code/callback`.

**What we don't yet know** (requires interactive testing with a real Claude account):
- Does the new flow complete entirely in the browser (CLI polls silently), or does the user need to paste a code back into the terminal?
- If terminal input is required, the current login dialog (`ExternalAgentDesktopViewer` in stream mode) shows the desktop but may not support interactive terminal input — that would need a UI change.
- Is the old browser-based flow (where the user enters a code in the browser UI) actually broken for everyone, or just for this one user on a very old image?

The production v2.8.3 image has an older CLI version where this was different — but the exact UX of the new flow needs to be confirmed before deciding on UI changes.

## Fix

Two changes, with a required investigation step first:

**0. Interactive test (before writing code)**

With a real Claude account: run `claude auth login` inside the container, open the printed URL, and observe exactly what the user needs to do to complete the flow. This determines whether any UI changes are needed beyond the CLI upgrade.

**1. Upgrade claude CLI before login (primary fix for existing deployments)**

In `ClaudeSubscriptionConnect.tsx`, before sending `claude auth login`, send a prior exec command to upgrade the CLI:

```
npm install -g @anthropic-ai/claude-code@latest
```

This self-heals even on old Mac app VM images that are already deployed and can't be updated without a full app release. No additional requirements — internet access is already a prerequisite for Claude OAuth to work at all. Adds a few seconds of latency to the login flow.

**2. Pin the version in the Dockerfile (fix for future builds)**

Change `Dockerfile.ubuntu-helix:929` from `@latest` to an explicit version so dev and prod images are identical and regressions are reproducible. Bump the pin deliberately when upgrading. The runtime upgrade in step 1 means the pinned version is a floor, not a ceiling.

**3. UI changes (if needed — determined by step 0)**

If the new flow requires terminal input, the login dialog may need to expose an interactive terminal in addition to (or instead of) the desktop stream. Scope TBD after testing.

Also add a "paste credentials JSON" fallback (see tasks) as insurance against future CLI regressions.

## Codebase Patterns

- Frontend: uses `api.getApiClient()` generated client — see `v1ExternalAgentsExecCreate` for the exec call pattern
- Backend: credentials stored as `ClaudeOAuthCredentials` in `api/pkg/types/claude_subscription.go`; encrypted at rest
- Rebuilding the desktop image requires `./stack build-ubuntu`
- The helix-ubuntu image version shown by `cat sandbox-images/helix-ubuntu.version`
