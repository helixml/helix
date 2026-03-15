# Requirements: Fix Claude Subscription OAuth Redirect URI Error

## Problem

When users on Helix v2.8.3 attempt to authenticate a Claude Pro/Max subscription, the OAuth flow fails immediately with:

> "Redirect URI http://localhost:37907/callback is not supported by client."

**Root cause (confirmed)**: The Helix v2.8.3 production image contains a stale version of the `claude` CLI that uses a random localhost port as the OAuth redirect URI. Anthropic's OAuth server no longer accepts this.

Newer versions of the CLI (≥ 2.1.73, confirmed by running `claude auth login` inside the current helix-ubuntu image) use `https://platform.claude.com/oauth/code/callback` instead — a server-side callback that gives the user a code to paste into the terminal. This completely avoids the localhost port problem.

`Dockerfile.ubuntu-helix` line 929 installs with `npm install -g @anthropic-ai/claude-code@latest`, but Docker layer caching means "latest" was frozen at an old version when the v2.8.3 image was built.

**Reference**: [GitHub issue #1911](https://github.com/helixml/helix/issues/1911)

## What Helix Does Today

1. User clicks "Connect" in the Claude Subscription UI
2. API creates a desktop container session (`HELIX_SKIP_ZED=1`)
3. Once container is running, frontend sends `claude auth login` via the exec API (with `WAYLAND_DISPLAY=wayland-0`)
4. Old CLI: opens a browser and starts a local HTTP callback server on a random port → Anthropic rejects it
5. New CLI (≥ 2.1.73): opens a browser to a URL with `redirect_uri=https://platform.claude.com/oauth/code/callback`, then waits for the user to paste a code from the browser back into the terminal

## User Stories

- **As a Helix user with a Claude Pro/Max subscription**, I want to authenticate my Claude account without hitting an OAuth redirect URI error.
- **As a Helix user**, I want a manual credential paste fallback in case the browser flow is unusable.

## Acceptance Criteria

1. The helix-ubuntu image is rebuilt with the current `claude` CLI version so the OAuth flow uses the server-side callback.
2. The login flow in the Helix UI works end-to-end: the embedded desktop shows the terminal waiting for a code, the user completes the flow in the browser, pastes the code, and credentials are captured.
3. A "paste credentials JSON" fallback exists in the UI for users who cannot use the embedded desktop flow.
