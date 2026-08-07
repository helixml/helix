# Implementation Tasks: Add users:read and users:read.email Scopes to Helix Meta Org Slack App

## Prerequisites

- [ ] Confirm with the user which Slack app is "Helix Meta Org" (app name vs workspace name)
- [ ] Confirm who will perform the admin login (Luke Marsden is the only admin per the thread)
- [ ] Confirm whether a live Helix deployment currently uses this app's bot token, and whether a brief interruption is acceptable
- [ ] Confirm whether to add only the two requested scopes or the full `defaultSlackBotScopes` set
- [ ] Obtain a known workspace email address to use for the `users.lookupByEmail` verification

## Slack login

- [x] Create the `screenshots/` directory in this task folder
- [x] Open `https://api.slack.com/apps` in Chrome via the `chrome-devtools` MCP server
- [~] Pause and hand the browser to the human for Slack login and 2FA — do not enter admin credentials on their behalf
- [ ] Confirm the session is authenticated and the app list is visible

## Locate the app

- [ ] Take a snapshot of the app list and identify the Helix Meta Org app
- [ ] Screenshot the app list as `screenshots/01-app-list.png`
- [ ] Open the app and confirm the associated workspace name matches Helix Meta Org before making any change
- [ ] Navigate to **OAuth & Permissions**

## Audit current scopes

- [ ] Screenshot the current Bot Token Scopes as `screenshots/02-scopes-before.png`
- [ ] Record the current bot scope list verbatim
- [ ] Diff it against `defaultSlackBotScopes` in `helix/api/pkg/server/helix_org_slack.go` and note every missing scope

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
