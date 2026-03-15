# Requirements: Fix Claude Subscription OAuth Redirect URI Error

## Problem

When users attempt to authenticate a Claude Pro/Max subscription in Helix, the OAuth flow fails immediately after the browser opens with:

> "Redirect URI http://localhost:37907/callback is not supported by client."

**Root cause**: Unknown — needs investigation. On a normal desktop, `claude auth login` works fine with a random port because RFC 8252 (OAuth for Native Apps) allows authorization servers to accept any loopback port. The same approach fails inside Helix's container. Likely causes:

- The version of `claude` CLI installed in the helix-ubuntu image is outdated and uses an OAuth client ID whose registration does not include the loopback pattern.
- OR the container environment causes `claude auth login` to construct the redirect URI in a way that differs from what Anthropic expects (e.g., different hostname, different path).
- OR Anthropic recently tightened their OAuth policy so that only specific registered ports are accepted, breaking an older CLI version that used to work.

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
