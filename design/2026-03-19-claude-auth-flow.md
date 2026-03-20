# Claude Auth Flow — Design Notes (2026-03-19)

## Problem
Claude CLI's `claude auth login` starts a local HTTP server on a random port
and expects the OAuth callback at `http://localhost:PORT/callback`. Claude's
OAuth server only allows `localhost:*` redirect URIs. Since the CLI runs inside
a container, the user's browser can't reach that localhost.

## Verified facts
1. Claude CLI does NOT read auth codes from stdin — only via HTTP callback server or TUI input
2. Claude CLI's TUI (Ink-based) has a TextInput for manual code entry, but tmux send-keys doesn't reach Ink's input handler
3. The callback server WORKS when curled from inside the container (subscription created successfully)
4. The auth code is bound to the redirect_uri it was issued with — codes from platform.claude.com can't be used with localhost callback (400 error, redirect_uri mismatch)
5. Claude's OAuth rejects non-localhost redirect URIs ("not supported by client")
6. Anthropic bans third-party OAuth — client fingerprinting blocks non-official clients (Jan 2026)
7. Device flow (RFC 8628) is NOT supported — open feature request: https://github.com/anthropics/claude-code/issues/22992
8. popup.location.href is cross-origin blocked even for localhost (different ports = different origins)
9. Claude Code has a `handleManualAuthCodeInput` that uses `isManual=true` (correct redirect_uri for platform.claude.com), but it's only callable from the TUI input or the IDE WebSocket `claude_oauth_callback` message
10. The WebSocket IDE integration is NOT available in `claude auth login` mode — it only runs in the full interactive session
11. socat PTY causes Node.js assertion failures (ResetStdio)
12. Using Claude Code's client_id ourselves is a ToS violation + blocked by fingerprinting
13. Patching the CLI's redirect_uri handling violates RFC 6749

## What works
- **In-container browser flow**: claude auth login opens a browser inside the container's desktop. The user interacts via the desktop stream. localhost:PORT is reachable within the container. This is the officially supported flow.
- **Direct curl to callback server**: curling `localhost:PORT/callback?code=CODE&state=STATE` from inside the container works when the code was issued for the localhost redirect_uri.

## Approaches tried (all blocked)
| Approach | Why it fails |
|----------|-------------|
| Custom redirect_uri on Helix API | Claude OAuth rejects non-localhost URIs |
| Popup window URL interception | Cross-origin blocks (different ports = different origins) |
| Named pipe / stdin to feed code | Claude's TUI doesn't read from stdin for the code |
| tmux send-keys | Ink's TUI doesn't process injected keystrokes |
| socat PTY relay | Node.js crashes (assertion failure on ResetStdio) |
| Own OAuth client_id | Anthropic ToS violation + client fingerprinting |
| Patch CLI with sed | Violates RFC 6749 (redirect_uri mismatch in token exchange) |
| WebSocket claude_oauth_callback | WS server not available in `auth login` mode |
| Code paste from platform.claude.com | redirect_uri mismatch — code issued for platform.claude.com, callback server exchanges with localhost |
| SSH port forwarding | Callback server only listens on ::1 (IPv6 loopback) — not accessible from outside |

## Anthropic redirect_uri bug (2026-03-19)

**Root cause (confirmed via URL analysis):** The bug is on Anthropic's
**server side**, not in Claude Code's URL construction. Claude Code constructs
the correct redirect_uri (`http://localhost:PORT/callback`), visible in the
initial login URL as double-encoded `http%253A%252F%252Flocalhost`. However,
during the login→authorize server-side redirect, Anthropic's server decodes the
`returnTo` parameter and **mangles** the redirect_uri — `http://localhost`
becomes `http:/localhost` (single slash). The authorize endpoint then rejects
the malformed URI.

URL trace:
```
1. Initial (correct): redirect_uri=http%3A%2F%2Flocalhost%3A32873%2Fcallback → http://localhost:32873/callback ✓
2. After login redirect (mangled): redirect_uri=http:/localhost:32873/callback → rejected ✗
```

This affects ALL environments (Linux desktop, macOS, Docker, VS Code extension,
Chrome extension). Not a headless/container issue — any user going through the
login→authorize flow hits it.

**Bug reports:**
- https://github.com/anthropics/claude-code/issues/36015 (Mac, opened 2026-03-19)
- https://github.com/anthropics/claude-code/issues/34917 (Docker/headless, 2026-03-16)
- https://github.com/anthropics/claude-code/issues/34662 (VS Code Linux, closed as dupe)

**Our workaround:** Chrome extension (`chrome-oauth-fix/`) loaded via
`--load-extension` in the Chromium/Chrome wrapper. A content script runs on
`claude.ai/*` pages at `document_start` and fixes the mangled redirect_uri
before the error page renders. The fix is a regex that adds the missing second
slash (`http:/localhost` → `http://localhost`). Safe no-op when Anthropic fixes
this — the regex won't match correct URLs.

Pinning Claude Code to 2.1.75 (pre-bug-report version) was tested and made
no difference — confirms this is purely server-side and version-independent.

## Recommendation
1. **Ship now**: In-container browser flow with Chrome extension fix
2. **Feature request**: Upvote https://github.com/anthropics/claude-code/issues/22992 for device flow support
3. **Contact Anthropic**: Report the server-side URL mangling bug with reproduction steps
4. **Remove workaround later**: Extension is a safe no-op once the bug is fixed
