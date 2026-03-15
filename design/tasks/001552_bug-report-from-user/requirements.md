# Requirements: Fix Claude Subscription OAuth Redirect URI Error

## Problem

When users attempt to authenticate a Claude Pro/Max subscription in Helix, the OAuth flow fails immediately after the browser opens with:

> "Redirect URI http://localhost:37907/callback is not supported by client."

**Root cause**: The `claude auth login` CLI command (run inside Helix's desktop container) starts a local HTTP callback server on a **random/dynamic port**. Anthropic's OAuth server only accepts pre-registered redirect URIs, and a random port will never match.

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
