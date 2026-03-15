# Implementation Tasks

- [ ] Pin the `claude` CLI to an explicit version in `Dockerfile.ubuntu-helix:929`: change `@latest` to the current release (check `npm show @anthropic-ai/claude-code version`), run `./stack build-ubuntu`, and verify `claude auth login` inside the new container uses `https://platform.claude.com/oauth/code/callback` (not a localhost port)
- [ ] Test the full login flow end-to-end: click Connect in the UI, complete the OAuth in the embedded desktop browser, paste the code in the terminal, confirm credentials are captured
- [ ] Add a "Paste credentials JSON" fallback to `ClaudeSubscriptionConnect.tsx`: a text area where the user can paste their `~/.claude/.credentials.json`, wired to `POST /api/v1/claude-subscriptions` (no backend changes needed)
- [ ] Document the claude CLI version pin in a comment in `Dockerfile.ubuntu-helix` explaining why it's pinned and how to update it (e.g. "bump this when Anthropic releases a new version; check release notes for auth flow changes")
