# Implementation Tasks

## Backend

- [x] Add `GET /api/v1/claude-subscriptions/get-login-url?session_id={id}` endpoint in `claude_subscription_handlers.go` that execs `BROWSER=echo claude auth login` in the running container, captures stdout, extracts the OAuth URL line, and returns `{"login_url": "https://..."}`
- [x] Register the new route in the server router (same file as other claude-subscription routes)
- [x] Regenerate OpenAPI client: `./stack update_openapi`

## Frontend

- [~] In `ClaudeSubscriptionConnect.tsx`, after the container reaches `isRunning`, call the new `get-login-url` endpoint to retrieve the OAuth URL
- [ ] Open the OAuth URL with `window.open(url, '_blank')` (works in both web and macOS/Wails WKWebView context)
- [ ] Remove the `ExternalAgentDesktopViewer` from the Claude login dialog (no longer showing the desktop stream)
- [ ] Update dialog UI states:
  - "Starting authentication..." (container starting, before URL)
  - "Complete sign-in in your browser..." (URL opened, waiting for callback)
  - On success: close dialog, show snackbar, call `onConnected`
- [ ] Keep the polling logic unchanged (still polls `poll-login/{sessionId}` every 3s)
- [ ] If `get-login-url` times out or fails, fall back to showing `ExternalAgentDesktopViewer` with the existing embedded-desktop flow

## Testing

- [ ] Manually test on macOS app: onboarding → Connect Claude → browser opens natively → auth completes → step marked complete
- [ ] Manually test on web: Connect Claude → new tab opens (or fallback to embedded desktop)
- [ ] Test re-authentication (expired token) follows the same improved flow
- [ ] Test cancel button: dialog closes, container session is stopped
- [ ] Test already-signed-in fast path: OAuth URL opens, callback fires immediately, no re-entry needed
