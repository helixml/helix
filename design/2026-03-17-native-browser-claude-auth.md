# Native Browser Claude OAuth Login

**Date:** 2026-03-17
**Status:** Implementing
**Branch:** `feature/native-browser-claude-auth`

## Problem

When users connect their Claude subscription during onboarding or from the account settings page, the current flow spins up a full Ubuntu desktop container (GNOME), runs `claude auth login` inside it, and expects the user to authenticate through the remote desktop viewer's embedded browser. This fails because the in-VM browser doesn't open — users see a spinning "Waiting for login..." with only the GNOME wallpaper visible.

## Solution

Capture the OAuth URL that `claude auth login` wants to open and open it in the user's **native browser** instead of the in-VM browser.

### Key Insight

Credentials don't need to pass from the host to the VM. Claude Code's modern OAuth uses `platform.claude.com/oauth/code/callback` (not localhost), so the CLI polls Anthropic's servers for auth completion. The host browser is only a UI surface for the user to authenticate. Tokens are exchanged directly between the container's Claude CLI and Anthropic.

### How It Works

1. A `helix-capture-browser` script in the container image captures the OAuth URL to `/tmp/claude-auth-url.txt` instead of opening a browser.
2. `claude auth login` is run with `BROWSER=/usr/local/bin/helix-capture-browser`.
3. The `poll-login` API endpoint reads both the credentials file and the URL file.
4. The frontend opens the URL in the user's native browser via `window.open`.
5. User completes the magic link flow in their native browser.
6. Claude CLI polls Anthropic, gets tokens, writes credentials.
7. Existing credential polling picks them up.

### Magic Link Flow

The auth uses a magic link flow: user enters email → Claude sends a magic link → user clicks the link in their normal email client → gets a 6-digit code → pastes it back in the browser tab → auth completes. This works much better with the native browser because the user can access their email naturally.

## Changes

- `desktop/shared/helix-capture-browser.sh` — new URL capture script
- `Dockerfile.ubuntu-helix` — COPY the script into the image
- `api/pkg/server/claude_subscription_handlers.go` — add `URL` field to poll response, extract `execInContainer` helper
- `frontend/src/components/account/ClaudeSubscriptionConnect.tsx` — replace desktop viewer dialog with native browser flow
- `frontend/src/components/account/ClaudeSubscription.tsx` — same changes
