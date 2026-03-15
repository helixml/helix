# Implementation Tasks

- [ ] **Interactive test first**: with a real Claude account, run `claude auth login` inside the container, open the printed URL, and observe exactly what the user must do to complete the flow — does it complete in the browser, or does the user need to paste a code into the terminal? Does the current login dialog (desktop stream) work as-is?
- [ ] In `ClaudeSubscriptionConnect.tsx`, before sending `claude auth login`, send a prior exec command: `npm install -g @anthropic-ai/claude-code@latest` to upgrade the CLI at login time so old deployed images self-heal
- [ ] Pin the `claude` CLI to an explicit version in `Dockerfile.ubuntu-helix:929`: change `@latest` to the current release with a comment explaining the pin and how to bump it
- [ ] If the interactive test reveals the flow requires terminal input: update the login dialog UI to expose an interactive terminal (scope TBD based on findings)
- [ ] Test the full login flow end-to-end and confirm credentials are captured
- [ ] Add a "Paste credentials JSON" fallback to `ClaudeSubscriptionConnect.tsx`: a text area where the user can paste their `~/.claude/.credentials.json`, wired to `POST /api/v1/claude-subscriptions` (no backend changes needed)
