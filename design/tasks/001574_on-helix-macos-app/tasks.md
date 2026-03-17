# Implementation Tasks

## Backend

- [x] Add `GET /api/v1/claude-subscriptions/get-login-url?session_id={id}` endpoint in `claude_subscription_handlers.go` that execs `BROWSER=echo claude auth login` in the running container, captures stdout, extracts the OAuth URL line, and returns `{"login_url": "https://..."}`
- [x] Register the new route in the server router (same file as other claude-subscription routes)
- [x] Regenerate OpenAPI client: `./stack update_openapi`

## Frontend

- [x] In `ClaudeSubscriptionConnect.tsx`, after the container reaches `isRunning`, call the new `get-login-url` endpoint to retrieve the OAuth URL
- [x] Open the OAuth URL with `window.open(url, '_blank')` (works in both web and macOS/Wails WKWebView context)
- [x] Remove the `ExternalAgentDesktopViewer` from the Claude login dialog (no longer showing the desktop stream)
- [x] Update dialog UI states: "Starting session..." → "Installing Claude CLI..." → "Opening browser..." → "Waiting for sign-in..." with fallback URL link
- [x] Keep the polling logic unchanged (still polls `poll-login/{sessionId}` every 3s)
- [x] Show fallback link if browser didn't open (the `loginUrl` shown as clickable link)

## Testing

- [ ] Manually test on macOS app: onboarding → Connect Claude → browser opens natively → auth completes → step marked complete
- [ ] Manually test on web: Connect Claude → new tab opens (or fallback to embedded desktop)
- [ ] Test re-authentication (expired token) follows the same improved flow
- [ ] Test cancel button: dialog closes, container session is stopped
- [ ] Test already-signed-in fast path: OAuth URL opens, callback fires immediately, no re-entry needed
