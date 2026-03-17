# Requirements: Fix Claude OAuth Login on macOS App

## Problem

During onboarding on the Helix macOS app, the "Connect Claude" step launches a Ubuntu desktop container and attempts to open a browser inside it (streamed via `ExternalAgentDesktopViewer`). The browser does not open, leaving the user stuck on:

> "A browser will open below. Sign in to your Claude account and complete the authentication flow in the browser."

## User Stories

**US-1 — Fix broken login**
As a macOS app user during onboarding, when I click to connect Claude, I want the authentication to complete successfully without getting stuck on a spinner.

**US-2 — Native browser flow**
As a macOS app user, when I need to sign in to Claude, I want the OAuth URL to open in my native macOS browser (Safari/Chrome), so I can log in in a familiar environment and benefit from existing sessions/passwords/passkeys.

**US-3 — Already-signed-in fast path**
As a user who is already signed in to Claude in my browser, I want the flow to complete in one click with no extra steps (Claude Code handles this well via the native browser — if already authenticated, the callback fires immediately).

**US-4 — Non-macOS / web context preserved**
As a user on the web version of Helix, I want the existing embedded-desktop login flow to remain available as a fallback, since the native-browser approach requires Wails/macOS-specific integration.

## How credentials flow into spec task sessions

This is a separate, already-working mechanism that this task does not change:

1. **Login flow (this task):** User authenticates → `claude auth login` writes `~/.claude/.credentials.json` in the temporary login container → the Helix API polls for it → stores the OAuth tokens encrypted in the `claude_subscriptions` DB table.

2. **Session runtime flow (unchanged):** When a real spec task / desktop session starts, the `settings-sync-daemon` runs inside the Ubuntu container. It calls `GET /api/v1/sessions/{id}/claude-credentials`, which decrypts and returns the stored tokens. The daemon writes them to `~/.claude/.credentials.json` inside that container. Claude Code (subscription mode, no `baseURL`) reads from this file to authenticate directly with Anthropic. The daemon also watches for token refreshes and pushes updated tokens back to the API via `PUT /api/v1/sessions/{id}/claude-credentials`.

So fixing the login flow (step 1) automatically makes credentials available in all subsequent sessions (step 2) — there is no additional work needed to "pass" credentials into the VM.

## Acceptance Criteria

- [ ] Clicking "Connect" during onboarding completes the Claude OAuth flow successfully on the macOS app
- [ ] The OAuth URL opens in the user's native macOS browser (not inside a streamed container)
- [ ] If the user is already logged in to Claude, the flow completes automatically without requiring re-entry of credentials
- [ ] After successful auth, the onboarding step is marked complete and the user can proceed
- [ ] Cancelling the flow cleans up any temporary session/container
- [ ] The existing embedded-desktop login still works for the web context (no regression)
- [ ] Re-authentication (expired token) also uses the same improved flow
- [ ] After login, a new spec task using the Claude Code runtime has working credentials (via settings-sync-daemon — no change needed here)
