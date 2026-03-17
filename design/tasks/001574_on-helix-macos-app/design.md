# Design: Fix Claude OAuth Login — Native Browser Flow

## Current Architecture

The "Connect Claude" flow (`ClaudeSubscriptionConnect.tsx`) works as follows:

1. `POST /api/v1/claude-subscriptions/start-login` → backend creates a Ubuntu desktop container (`DesktopAgent` with `HELIX_SKIP_ZED=1`)
2. Frontend shows `ExternalAgentDesktopViewer` streaming the container desktop
3. Once `isRunning`, frontend sends two exec commands to the container:
   - `npm install -g @anthropic-ai/claude-code@latest` (blocking, up to 300s)
   - `claude auth login` (background, with `WAYLAND_DISPLAY=wayland-0`)
4. `claude auth login` is supposed to open a browser inside the container
5. Frontend polls `GET /api/v1/claude-subscriptions/poll-login/{sessionId}` every 3s for `~/.claude/.credentials.json`

## Root Cause of the Bug

The browser does not open inside the container for several likely reasons:

- **Timing**: The 3-second delay after `isRunning` may not be enough for GNOME/Wayland to be fully ready. `claude auth login` fails silently if the display server isn't ready.
- **No visible browser**: Even if the browser launches, the display stream may not render it correctly in all environments.
- **npm install duration**: On slow networks, the blocking npm install can time out or take longer than expected, and failures aren't surfaced to the user.

The container-embedded-browser approach is inherently fragile for authentication: the display is a stream, not a real browser, so it lacks password managers, passkeys, existing sessions, and smooth UX.

## Proposed Solution: Extract OAuth URL, Open in Native Browser

The `claude auth login` CLI, when it cannot find a browser (or when explicitly requested), prints the OAuth URL to stdout/stderr for manual completion. We exploit this to get the URL, then open it via the macOS-native `BrowserOpenURL` Wails function.

### How `claude auth login` handles no-browser environments

`claude auth login` checks for `BROWSER` env var and falls back to `xdg-open`. When neither succeeds, it prints:
```
Please open the following URL in your browser:
https://claude.ai/oauth/authorize?...
```

We pass `BROWSER=echo` (or similar) to force it to print the URL without launching a browser.

### New Flow

```
User clicks "Connect Claude"
  → POST /api/v1/claude-subscriptions/start-login  (same as before — starts container)
  → Container reaches isRunning state
  → Frontend sends exec: claude auth login (with BROWSER=echo or --print-redirect-uri)
  → Backend streams stdout → frontend receives the OAuth URL
  → Frontend calls BrowserOpenURL(url) via Wails [macOS] OR window.open(url) [web]
  → User authenticates in native browser
  → Claude CLI writes ~/.claude/.credentials.json (same as before)
  → Frontend polls poll-login endpoint (same as before)
  → Credentials captured → subscription created
```

The container is still needed to:
- Run `claude auth login` which manages the OAuth PKCE state and credential writing
- Poll `~/.claude/.credentials.json` after completion

But the browser interaction moves to the user's native environment.

### Context Detection

The Helix frontend runs inside a Wails WKWebView on macOS, and in a regular browser on web. We need to detect which context we're in to use the right "open URL" mechanism.

**Approach**: Check for `window.__wails` (injected by Wails runtime) to detect macOS app context. In Wails context, import and call `BrowserOpenURL` from `../../wailsjs/runtime/runtime`. On web, use `window.open(url, '_blank')`.

**Important**: `BrowserOpenURL` and `wailsjs` are only available in the for-mac frontend build. The main Helix frontend (served from the VM, loaded in the iframe) does NOT have Wails runtime. Therefore:

- Option A: Add a new Go method to the `App` struct, exposed via Wails bindings, and call it from the for-mac frontend — but the Connect Claude component runs in the main Helix frontend inside the iframe, not the for-mac frontend.
- Option B: **Use `window.open(url, '_blank')`** in the main Helix frontend. In WKWebView, `window.open` is intercepted by Wails and routes to `BrowserOpenURL` natively. This works seamlessly and requires no separate Wails binding.

**Chosen approach**: Use `window.open(url, '_blank')` in `ClaudeSubscriptionConnect.tsx`. Wails automatically intercepts this in the macOS app context and opens the URL in the system browser.

### Capturing the OAuth URL from exec stdout

The current exec API returns output after the command completes. We need the URL before the command finishes (since `claude auth login` waits for the callback).

**Options**:
1. Run `BROWSER=echo claude auth login` — `echo` immediately prints the URL and exits, `claude auth login` then waits for the callback URL. The exec output (`output` field) will contain the printed URL.
2. Use `--print-default-redirect-uri` flag if available in the installed version.
3. Stream the exec output (existing SSE/streaming infra).

**Recommended**: Use `BROWSER=echo`. The exec returns immediately with the URL in stdout (from the `echo` call), while the `claude auth login` process continues running in the background waiting for the callback. We need `background: true` for the main claude process, and capture its initial stdout.

**Simpler alternative**: Use a two-step exec:
1. `claude auth login --print-url-only` or equivalent (if supported)
2. If not supported: run `claude auth login` with a wrapper that captures and forwards the URL

**Simplest reliable approach**: Pass `BROWSER=cat` — this makes `claude auth login` pipe the URL to `cat`, which prints it to stdout. Run with `background: true` and stream the output, or run with a short timeout just to capture the URL print.

Actually, the cleanest approach: wrap the command as a shell script that captures the URL:
```bash
BROWSER="sh -c 'echo OAUTH_URL:$1' --" claude auth login
```
Then parse the line starting with `OAUTH_URL:` from stdout.

### UI Changes

**While waiting for OAuth URL** (container starting + `claude auth login` running):
- Show spinner: "Starting authentication..."

**Once URL is captured** (`window.open` called):
- Show: "Browser opened. Complete sign-in in your browser window."
- Show: "Waiting for authentication to complete..." with spinner

**On completion** (poll returns `found: true`):
- Show: "Connected!" → close dialog → invalidate queries → call `onConnected`

**If URL capture fails**:
- Fall back to existing embedded-desktop flow (show `ExternalAgentDesktopViewer`)

### Backend Changes

No changes to the Go backend are strictly required. The existing endpoints support this flow:
- `start-login` creates the container (unchanged)
- `poll-login` polls for credentials (unchanged)
- The exec endpoint (`v1ExternalAgentsExecCreate`) already accepts env vars

However, we should add a backend endpoint to **get the OAuth URL** server-side and return it to the frontend, so the frontend doesn't need to parse exec output. This is cleaner and testable:

**New endpoint**: `POST /api/v1/claude-subscriptions/get-login-url`
- Execs `BROWSER=echo claude auth login` in the container with a timeout
- Streams stdout, extracts the URL line
- Returns `{"login_url": "https://claude.ai/oauth/..."}`

This keeps URL-parsing logic in Go (simpler) and the frontend just opens the URL.

## Key Files

| File | Change |
|------|--------|
| `frontend/src/components/account/ClaudeSubscriptionConnect.tsx` | Replace embedded-desktop UI with native-browser flow; add URL capture + `window.open` |
| `api/pkg/server/claude_subscription_handlers.go` | Add `getClaudeLoginURL` handler that execs and captures OAuth URL |
| `api/pkg/server/routes.go` (or equivalent) | Register new route |

## Patterns Found

- The for-mac frontend uses `BrowserOpenURL` from `wailsjs/runtime/runtime` — but the main Helix frontend does NOT have Wails; use `window.open` instead.
- The exec API (`v1ExternalAgentsExecCreate`) supports `env`, `background`, and `timeout` params.
- Credentials polling via RevDial is already implemented and reliable — keep as-is.
- The `ExternalAgentDesktopViewer` + stream approach is expensive (full Ubuntu desktop) just for OAuth; the new flow still uses the container but avoids the streaming viewer.

## Decision Log

- **Keep container**: The `claude` CLI manages PKCE state and credential writing to `~/.claude/.credentials.json`. Running it in the container is the correct approach; we just move the browser interaction outside.
- **`window.open` over `BrowserOpenURL`**: The Claude connect component runs in the main Helix frontend (inside WKWebView iframe), which doesn't have Wails bindings. `window.open` is intercepted by Wails and works identically.
- **New backend endpoint for URL**: Cleaner than having the frontend parse exec output. Keeps credential/URL logic on the server.
- **Preserve embedded-desktop fallback**: Non-macOS users on the web don't benefit from native-browser UX in the same way. The fallback ensures the web flow still works.
