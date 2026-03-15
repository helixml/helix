# Implementation Tasks

- [ ] Reproduce the bug: trigger the Claude subscription login flow on the current build and confirm the "Redirect URI not supported" error appears
- [ ] Check the installed `claude` CLI version inside the helix-ubuntu image (`claude --version`) and compare with the current `@anthropic-ai/claude-code` release on npm
- [ ] If the version is stale: bust the Docker layer cache for the install in `Dockerfile.ubuntu-helix` line 929, run `./stack build-ubuntu`, and re-test to confirm the OAuth flow succeeds
- [ ] If the OAuth flow still fails after the CLI upgrade, investigate `claude auth login --help` for a `--no-launch-browser` or device-flow flag and update the exec command in `ClaudeSubscriptionConnect.tsx` line ~333
- [ ] Add a "Paste credentials JSON" fallback to `ClaudeSubscriptionConnect.tsx`: a text area where the user pastes their `~/.claude/.credentials.json` contents, wired to `POST /api/v1/claude-subscriptions` (no backend changes needed)
