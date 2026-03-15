# Implementation Tasks

- [ ] In `ClaudeSubscriptionConnect.tsx`, before sending `claude auth login`, send a prior exec command: `npm install -g @anthropic-ai/claude-code@latest` — this upgrades the CLI at login time so old deployed images self-heal without needing a full app release
- [ ] Pin the `claude` CLI to an explicit version in `Dockerfile.ubuntu-helix:929`: change `@latest` to the current release (check `npm show @anthropic-ai/claude-code version`) with a comment explaining the pin and how to bump it
- [ ] Test the full login flow end-to-end: click Connect in the UI, let the upgrade run, complete the OAuth in the embedded desktop browser, paste the code in the terminal, confirm credentials are captured
- [ ] Add a "Paste credentials JSON" fallback to `ClaudeSubscriptionConnect.tsx`: a text area where the user can paste their `~/.claude/.credentials.json`, wired to `POST /api/v1/claude-subscriptions` (no backend changes needed)
