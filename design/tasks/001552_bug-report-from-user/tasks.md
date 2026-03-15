# Implementation Tasks

- [ ] Bust the Docker layer cache for the `@anthropic-ai/claude-code` install in `Dockerfile.ubuntu-helix` line 929 (e.g., add/update a cache-bust comment above the `npm install` line), then run `./stack build-ubuntu` and test the OAuth flow
- [ ] Add a "Paste credentials JSON" fallback to `ClaudeSubscriptionConnect.tsx`: a text area where the user pastes their `~/.claude/.credentials.json` contents, wired to `POST /api/v1/claude-subscriptions` (no backend changes needed)
- [ ] If the OAuth flow still fails after the CLI upgrade, investigate `claude auth login --help` for a `--no-launch-browser` or device-flow flag and update the exec command in `ClaudeSubscriptionConnect.tsx` line ~333
