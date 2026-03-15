# Implementation Tasks

- [ ] Run `claude auth login --help` (or check claude CLI docs/source) to confirm whether a `--no-launch-browser` or `--device` flag exists that bypasses the local callback server
- [ ] If a no-browser flag exists: update `ClaudeSubscriptionConnect.tsx` line ~333 to pass that flag in the exec command array
- [ ] Update the UI alert text in the login dialog to match the actual flow (e.g., remove the "enter email" instruction if the flag changes the UX)
- [ ] Add a "Paste credentials JSON" option to `ClaudeSubscriptionConnect.tsx` as a permanent fallback: a text area where the user can paste the contents of `~/.claude/.credentials.json` from their own machine
- [ ] Wire the paste option to `POST /api/v1/claude-subscriptions` (already accepts raw credentials JSON — no backend changes needed)
- [ ] Test the browser login flow and the paste flow end-to-end
- [ ] If the claude CLI version in the helix-ubuntu image is outdated and fixing the version resolves the issue, update the Dockerfile install step and run `./stack build-ubuntu`
