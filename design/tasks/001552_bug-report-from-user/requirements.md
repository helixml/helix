# Requirements: Fix Claude Subscription OAuth Redirect URI Error

## Problem

When users attempt to authenticate a Claude Pro/Max subscription in Helix, the OAuth flow fails immediately after the browser opens with:

> "Redirect URI http://localhost:37907/callback is not supported by client."

**Root cause**: Unknown — needs investigation. On a normal desktop, `claude auth login` works fine with a random port because RFC 8252 (OAuth for Native Apps) allows authorization servers to accept any loopback port. The same approach fails inside Helix's container. Likely causes:

- Most likely: the `claude` CLI in the helix-ubuntu image is stale. `Dockerfile.ubuntu-helix` line 929 runs `npm install -g @anthropic-ai/claude-code@latest`, but Docker layer caching means "latest" is only re-fetched when the cache is busted. A newer CLI version likely fixes the OAuth flow.
- OR Anthropic recently tightened their OAuth policy, breaking older CLI versions.

**Reference**: [GitHub issue #1911](https://github.com/helixml/helix/issues/1911)

## What Helix Does Today

1. User clicks "Connect" in the Claude Subscription UI
2. API creates a desktop container session (`HELIX_SKIP_ZED=1`)
3. Once container is running, frontend sends `claude auth login` via the exec API (with `WAYLAND_DISPLAY=wayland-0`)
4. `claude auth login` opens a browser inside the GNOME/Wayland session and starts a local HTTP callback server on a **random port**
5. Anthropic's OAuth server rejects the callback URI → user sees the error

## User Stories

- **As a Helix user with a Claude Pro/Max subscription**, I want to authenticate my Claude account so that Helix can use it for coding sessions, without hitting an OAuth redirect URI error.
- **As a Helix user whose OAuth flow has broken**, I want a fallback manual credential entry method so I can connect even when the browser OAuth fails.

## Acceptance Criteria

1. A user on Helix v2.8+ can successfully authenticate a Claude Pro/Max subscription without seeing the "Redirect URI not supported" error.
2. If the browser-based OAuth flow fails, a manual fallback exists (e.g., pasting credentials JSON directly).
3. The embedded desktop session terminal is usable as a fallback path (secondary issue).
