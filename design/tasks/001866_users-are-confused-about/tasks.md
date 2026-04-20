# Implementation Tasks

- [x] Add `validateSetupToken()` function in `ClaudeSubscriptionConnect.tsx` that checks for API key prefix (`sk-ant-api`), valid setup token prefix (`sk-ant-oat01-`), and minimum length
- [x] Wire validation into the token dialog: show error alert below text field, disable "Connect" button when validation fails
- [x] Change text field label from "Your Token" to "Claude Code Setup Token" and update placeholder text
- [x] Add backend validation in `createClaudeSubscription` handler (before encryption) to reject API keys and non-`sk-ant-oat01-` tokens with HTTP 400
- [x] Test: paste Anthropic API key (`sk-ant-api03-...`) — should see specific error about wrong credential type
- [x] Test: paste random text — should see generic "not a valid setup token" error
- [x] Test: paste valid-format setup token (`sk-ant-oat01-...`) — should be accepted
- [x] Test: OAuth flow still works unchanged (OAuth flow is a separate code path — `createReq.Credentials.ClaudeAiOauth` branch — and our validation only touches the `SetupToken != ""` branch, so it's unaffected)
