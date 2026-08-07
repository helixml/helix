# Implementation Tasks: Add users:read and users:read.email Scopes to Helix Meta Org Slack App

## Prerequisites

- [x] Confirm with the user which Slack app is "Helix Meta Org" — resolved: app `Helix Meta Org` A0BDUQTBLF4 in MLOps.community
- [x] Confirm who will perform the admin login — Luke completed Google SSO + device 2FA
- [~] Confirm whether a live Helix deployment currently uses this app's bot token — YES, redirect URL is `https://meta.helix.ml/api/v1/slack/oauth/callback`
- [~] Confirm whether to add only the two requested scopes or also `im:write`
- [ ] Obtain a known workspace email address to use for the `users.lookupByEmail` verification

## Slack login

- [x] Create the `screenshots/` directory in this task folder
- [x] Open `https://api.slack.com/apps` in Chrome via the `chrome-devtools` MCP server
- [x] Pause and hand the browser to the human for Slack login and 2FA — do not enter admin credentials on their behalf
- [x] Confirm the session is authenticated and the app list is visible

## Locate the app

- [x] Take a snapshot of the app list and identify the Helix Meta Org app
- [x] Screenshot the app list as `screenshots/01-app-list.png`
- [x] Open the app and confirm the associated workspace name matches Helix Meta Org before making any change (A0BDUQTBLF4 / MLOps.community)
- [x] Navigate to **OAuth & Permissions**

## Audit current scopes

- [x] Screenshot the current Bot Token Scopes as `screenshots/02-scopes-before.png`
- [x] Record the current bot scope list verbatim (11 scopes, see design.md)
- [x] Diff it against `defaultSlackBotScopes` and note every missing scope — 3 missing: `users:read`, `users:read.email`, `im:write`

## Add the scopes

- [ ] Add `users:read` under **Scopes → Bot Token Scopes → Add an OAuth Scope**
- [ ] Add `users:read.email` under the same section
- [ ] Confirm the changes are saved (Slack saves scope additions immediately)
- [ ] Screenshot the updated scope list as `screenshots/03-scopes-after.png`

## Reauthorize the install

- [ ] Click **Reinstall to Workspace** (or follow the "reinstall your app" banner)
- [ ] Verify the consent screen lists the two new permissions before approving
- [ ] Screenshot the consent screen as `screenshots/04-reinstall-consent.png`
- [ ] Approve the install
- [ ] If a workspace-admin approval gate blocks the install, stop and report who needs to approve

## Refresh the Helix-side connection

- [ ] Skip this section if no live Helix deployment uses this app
- [ ] Open the relevant Helix org's **Settings → Slack integration**
- [ ] Run **Connect workspace** to refresh the stored bot token with the newly scoped grant
- [ ] Confirm the connection reports success and did not create a duplicate connection row
- [ ] If the deployment uses a manually pasted bot token instead of OAuth, re-copy the new `xoxb-` token from **OAuth & Permissions → Bot User OAuth Token** and update it

## Verify

- [ ] Call `users.lookupByEmail` with the agreed known email via `https://api.slack.com/methods/users.lookupByEmail/test`
- [ ] Confirm the response is `ok: true` with a `U…` user ID, not `missing_scope`
- [ ] Screenshot the successful response as `screenshots/05-lookup-verified.png`
- [ ] Optionally confirm `users.list` also returns `ok: true`

## Report

- [ ] Report the final bot scope list to the user
- [ ] Report any scopes still missing relative to `defaultSlackBotScopes`, and ask whether to add them as a follow-up
- [ ] Confirm to Priya Samuel in the Slack thread that `users.lookupByEmail` now works
