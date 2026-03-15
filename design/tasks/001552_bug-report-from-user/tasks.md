# Implementation Tasks

- [ ] Bust the Docker layer cache for the `npm install -g @anthropic-ai/claude-code@latest` line in `Dockerfile.ubuntu-helix:929` (add/update a cache-bust comment), run `./stack build-ubuntu`, and verify `claude auth login` inside the new container uses `https://platform.claude.com/oauth/code/callback` (not a localhost port)
- [ ] Test the full login flow end-to-end: click Connect in the UI, complete the OAuth in the embedded desktop browser, paste the code in the terminal, confirm credentials are captured
- [ ] Add a "Paste credentials JSON" fallback to `ClaudeSubscriptionConnect.tsx`: a text area where the user can paste their `~/.claude/.credentials.json`, wired to `POST /api/v1/claude-subscriptions` (no backend changes needed)
