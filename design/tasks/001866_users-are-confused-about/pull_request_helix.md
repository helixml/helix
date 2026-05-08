# Validate Claude Code setup tokens and reject Anthropic API keys

## Summary
Users frequently paste their Anthropic API key (`sk-ant-api03-...`) into the Claude Code setup token dialog, which silently accepts it and shows a misleading green "Connected" status. The token only fails later at runtime. This adds frontend and backend validation to catch wrong credential types immediately with helpful error messages.

## Changes
- Add `validateSetupToken()` in `ClaudeSubscriptionConnect.tsx` — checks token prefix and shows specific errors for API keys vs random text
- Disable "Connect" button when validation fails
- Rename text field label from "Your Token" to "Claude Code Setup Token" for clarity
- Add backend validation in `createClaudeSubscription` handler to reject `sk-ant-api*` tokens (HTTP 400) and tokens without the `sk-ant-oat01-` prefix

## Screenshots

**Anthropic API key pasted — specific error shown:**
![API key error](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001866_users-are-confused-about/screenshots/02-api-key-error-visible.png)

**Random text pasted — generic error shown:**
![Random text error](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001866_users-are-confused-about/screenshots/03-random-text-error.png)

**Valid setup token — accepted, Connect enabled:**
![Valid token accepted](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001866_users-are-confused-about/screenshots/04-valid-token-accepted.png)

**After connecting — green Connected status:**
![Connected status](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001866_users-are-confused-about/screenshots/05-valid-token-connected.png)
