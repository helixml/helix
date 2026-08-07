# Requirements: Add users:read and users:read.email Scopes to Helix Meta Org Slack App

## Background

Priya Samuel reported in Slack that the Helix Slack app has `chat:write` but not
`users:read` / `users:read.email`. As a result `users.lookupByEmail` and
`users.list` both return `missing_scope`, and there is no way to resolve a
GitHub author into a Slack `@Uxxxx` mention. Phil Winder confirmed he is not an
admin of the ML community's Slack apps — Luke Marsden is the only admin — and
Luke agreed to make the change.

**This is an operational task, not a code change.** Investigation of the `helix`
repo shows the codebase already requests both scopes:

- `api/pkg/server/helix_org_slack.go` → `defaultSlackBotScopes` includes
  `users:read` and `users:read.email` (lines 169-170).
- `frontend/src/components/dashboard/slackManifest.ts` → `BOT_SCOPES` includes
  both, and a unit test (`helix_org_slack_scopes_test.go`) asserts their
  presence.
- Both were added in commit `2ddc59944` *"feat(org): deliver human messages
  through Slack"*.

The existing design doc `helix/design/2026-07-15-org-human-slack-delivery.md`
already anticipated this exact situation:

> Existing Slack installations must be reauthorized to grant newly added scopes.

The Helix Meta Org Slack app was created **before** that commit, so its
installed OAuth grant is stuck on the old, narrower scope set. The fix is to
update the app configuration at `api.slack.com/apps` and reauthorize the
install — no repository change is required.

## User Stories

### US1 — Add the missing bot token scopes

**As** Luke Marsden (the only admin of the ML community Slack apps),
**I want** `users:read` and `users:read.email` added to the Helix Meta Org
Slack app's bot token scopes,
**so that** Helix can call `users.lookupByEmail` and `users.list` without a
`missing_scope` error.

Acceptance criteria:

- [ ] A browser session is authenticated to Slack as an account with admin
      rights over the Helix Meta Org Slack app.
- [ ] The Helix Meta Org app is located at `https://api.slack.com/apps`.
- [ ] Under **OAuth & Permissions → Scopes → Bot Token Scopes**, both
      `users:read` and `users:read.email` are present and saved.
- [ ] The app's Bot Token Scopes are audited against `defaultSlackBotScopes`
      in `api/pkg/server/helix_org_slack.go`; any other missing scopes are
      recorded (see US3) but **not** silently added.

### US2 — Reauthorize the install so the live token carries the new scopes

**As** a Helix operator,
**I want** the workspace install reauthorized after the scope change,
**so that** the bot token Helix actually holds carries the new scopes rather
than the stale pre-change grant.

Acceptance criteria:

- [ ] The app is reinstalled / reauthorized against the Helix Meta Org
      workspace after saving the scopes.
- [ ] Slack's consent screen shows the two new permissions before approval,
      confirming the grant is genuinely being widened.
- [ ] If the app is connected to a live Helix deployment, the Helix-side
      Slack connection is refreshed via **Org Settings → Connect workspace**,
      so the stored bot token is replaced with the newly scoped one.
      (Per the design doc, reconnecting an already-installed team refreshes
      that install and its token rather than creating a duplicate.)

### US3 — Verify and report

**As** Priya Samuel,
**I want** confirmation that `users.lookupByEmail` now succeeds,
**so that** I can build the GitHub-author → `@Uxxxx` resolution she needs.

Acceptance criteria:

- [ ] `users.lookupByEmail` is called with a known workspace email and
      returns `ok: true` with a `U…` user ID — not `missing_scope`.
- [ ] The final OAuth scope list, and any scopes still missing relative to
      `defaultSlackBotScopes`, are reported back to the user.
- [ ] Screenshots of the before and after scope lists are saved under
      `screenshots/` in this task directory as evidence.

## Non-Goals

- Changing any code in the `helix` repo. The scope lists there are already
  correct; editing them would be a no-op for this problem.
- Building the GitHub-author → Slack-user mapping feature itself. This task
  only unblocks it by granting the API permission. (Note: the `helix` org
  domain already stores `github` and `slack_user_id` side by side in the human
  node identity map, patched via the `set_human_contact` MCP tool — so the
  mapping is data-driven once lookup works. No new mapping code exists yet.)
- Touching any Slack app other than the Helix Meta Org one.
- Granting user-token scopes. Only **bot** token scopes are in scope; the
  Helix integration uses an `xoxb-` bot token.

## Open Questions

1. **Credentials and consent.** Only Luke Marsden is an admin. How should the
   login happen — will Luke type his own Slack credentials and 2FA into the
   Chrome window and then hand over, or is there a shared/stored session? An
   agent should not be handling an admin's Slack password. The assumption in
   the design is: **the human performs the login and any 2FA step, then the
   agent drives the post-login navigation.**
2. **Exact app identity.** Is "Helix Meta Org" the Slack **app** name, the
   workspace name, or the Helix org name? The app list at `api.slack.com/apps`
   may show several Helix-branded apps across workspaces, and picking the wrong
   one would grant scopes to an unrelated install. Which workspace should it be
   filtered to?
3. **Scope of the fix.** Should we add *only* the two scopes Priya asked for,
   or bring the app fully in line with `defaultSlackBotScopes` (which also
   includes `channels:join`, `chat:write.customize`, `reactions:write`,
   `files:write`, `groups:history`, `groups:read`, `im:history`, `im:write`,
   `app_mentions:read`, `channels:history`, `channels:read`)? The spec
   currently assumes **add only the two requested scopes, and report the rest**
   — a wider change means a wider reinstall consent prompt for the workspace.
4. **Reinstall blast radius.** Reinstalling revokes and reissues the grant.
   Is there a live Helix deployment currently using this app's bot token, and
   is a brief interruption acceptable? If yes, which deployment/org needs the
   **Connect workspace** refresh, and is there a maintenance window?
5. **Workspace admin approval.** Some workspaces require a separate workspace-
   admin approval step for newly requested scopes. Is app-install approval
   enabled on this workspace, and if so, who approves?
6. **Verification identity.** Which email address should be used for the
   `users.lookupByEmail` smoke test? (A known ML-community member's address is
   needed; using an unknown address returns `users_not_found`, which is a
   different result from `missing_scope` but a weaker signal.)
