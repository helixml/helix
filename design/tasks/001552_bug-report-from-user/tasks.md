# Implementation Tasks

- [x] **Interactive test first**: confirmed — the Helix UI desktop container has an old claude CLI that uses `localhost:<random-port>` OAuth redirect. The current image (2.1.73) uses `https://platform.claude.com/oauth/code/callback`. The old version in the deployed container is what fails.
- [x] In `ClaudeSubscriptionConnect.tsx`, before sending `claude auth login`, send a prior exec command: `npm install -g @anthropic-ai/claude-code@latest` to upgrade the CLI at login time so old deployed images self-heal
- [x] Pin the `claude` CLI to an explicit version in `Dockerfile.ubuntu-helix:929`: changed `@latest` to `@2.1.76` with a comment explaining why and how to bump
- [ ] If the interactive test reveals the flow requires terminal input: update the login dialog UI to expose an interactive terminal (scope TBD based on findings)
- [ ] Test the full login flow end-to-end and confirm credentials are captured
- [ ] Add a "Paste credentials JSON" fallback to `ClaudeSubscriptionConnect.tsx`: a text area where the user can paste their `~/.claude/.credentials.json`, wired to `POST /api/v1/claude-subscriptions` (no backend changes needed)
