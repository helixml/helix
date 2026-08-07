# Design: Add users:read and users:read.email Scopes to Helix Meta Org Slack App

## Approach

A browser-driven runbook, not a code change. Chrome is driven through the
`chrome-devtools` MCP server to navigate `api.slack.com/apps`, add two bot
token scopes, and reauthorize the install. Screenshots are captured through the
`helix-desktop` MCP server as before/after evidence.

Nothing in the `helix` repository changes. That is the central design decision
and it is worth restating because it is counter-intuitive: the natural instinct
on reading "the Slack app is missing a scope" is to go add the scope to a
config file. Here the config file is already correct — the drift is entirely in
Slack's server-side app record.

## Key Decisions

### D1 — Fix in Slack's console, not in the repo

`defaultSlackBotScopes` (`api/pkg/server/helix_org_slack.go:157-171`) and
`BOT_SCOPES` (`frontend/src/components/dashboard/slackManifest.ts`) both
already list `users:read` and `users:read.email`, and
`helix_org_slack_scopes_test.go` pins them. These lists govern what the
*"Install to Slack"* OAuth flow **requests** at install time and what a
*freshly generated* manifest declares. They have no retroactive effect on an
app that was created and installed before commit `2ddc59944`.

The Helix Meta Org app is one of those pre-existing installs. Editing the Go
or TypeScript list again would change nothing observable.

### D2 — Edit scopes directly rather than replacing the app manifest

Two routes exist to add scopes:

- **App Manifest editor** — paste the manifest that `buildManifest()` produces
  and let Slack diff it. Wholesale, and would also rewrite
  `display_information`, `event_subscriptions.request_url`, `redirect_urls`,
  and `socket_mode_enabled` to whatever this deployment generates. For an app
  already wired to a live workspace with hand-set URLs, that risks clobbering
  working configuration.
- **OAuth & Permissions → Bot Token Scopes** — additive, minimal, reversible.

Chosen: the second. Minimum blast radius on a production app we cannot easily
roll back, and it matches exactly what Priya asked for.

### D3 — Reauthorize, then refresh the Helix-side connection

Saving a scope in the Slack console does **not** widen the token already issued.
Slack shows a "reinstall your app" banner precisely because the existing grant
is frozen at its original scope set. So the runbook has two mandatory follow-on
steps:

1. **Reinstall to Workspace** in the Slack console — re-runs consent and issues
   a grant carrying the new scopes.
2. **Connect workspace** in Helix Org Settings — the design doc
   (`helix/design/2026-07-15-org-human-slack-delivery.md`) records that
   reconnecting an already-installed team *"refreshes that install and its token
   instead of creating a duplicate"*. This is the deterministic way to get the
   newly scoped `xoxb-` token into Helix's `ServiceConnection` store. Relying on
   Slack happening to preserve the same token string across a reinstall is an
   assumption we should not build on.

Step 2 is conditional on there being a live Helix deployment bound to this app
(see Open Question 4 in requirements.md). If the app is only used ad hoc via a
manually held token, that token must be re-copied instead.

### D4 — Human performs the login; the agent drives afterwards

The Chrome instance currently has no Slack session (`list_pages` returns only
`about:blank`). Slack admin login involves a password and almost certainly a
second factor. The agent should navigate to the login page and then **stop and
hand control to the human**, resuming automation once the session is
established. An agent should not be typing an admin's Slack password.

### D5 — Additive only; report, don't fix, other drift

The app may also be missing other scopes from `defaultSlackBotScopes`
(`channels:join`, `reactions:write`, `files:write`, `chat:write.customize`, …).
Adding those would widen the reinstall consent prompt the whole workspace sees,
which is a bigger decision than Priya's request. The audit is performed and the
gap reported; adding them is a separate, user-approved follow-up.

## Runbook Outline

| # | Step | Tool |
|---|---|---|
| 1 | Open `https://api.slack.com/apps` | `chrome-devtools` `new_page` |
| 2 | Human completes login + 2FA; agent pauses | — |
| 3 | Locate and open the Helix Meta Org app | `take_snapshot`, `click` |
| 4 | Screenshot the app list to confirm the right app | `helix-desktop` |
| 5 | Navigate to **OAuth & Permissions** | `click` |
| 6 | Screenshot the *before* Bot Token Scopes list | `helix-desktop` |
| 7 | Audit current scopes vs `defaultSlackBotScopes`; record gaps | reasoning |
| 8 | **Add an OAuth Scope** → add `users:read` | `click`, `fill` |
| 9 | **Add an OAuth Scope** → add `users:read.email` | `click`, `fill` |
| 10 | Screenshot the *after* scope list | `helix-desktop` |
| 11 | **Reinstall to Workspace**, confirm consent shows both new perms | `click` |
| 12 | If a live Helix deployment uses this app: **Connect workspace** in Org Settings | `chrome-devtools` |
| 13 | Verify: `users.lookupByEmail` with a known email returns `ok: true` | Slack API tester |
| 14 | Report final scope list + remaining gaps to the user | — |

Screenshots go to
`/home/retro/work/helix-specs/design/tasks/002669_you-are-going-to-help-me/screenshots/`
as `01-app-list.png`, `02-scopes-before.png`, `03-scopes-after.png`,
`04-reinstall-consent.png`, `05-lookup-verified.png`.

## Verification

The single decisive check is a `users.lookupByEmail` call that returns
`ok: true` and a `U…` ID. `missing_scope` means the reinstall did not take.
`users_not_found` means the scope *is* granted but the test email is not in the
workspace — a weaker pass, so prefer a known-good address.

Slack's API tester at `https://api.slack.com/methods/users.lookupByEmail/test`
runs this in-browser against the installed token without needing a terminal.

## Risks

- **Reinstall interrupts the live bot.** Reissuing the grant can invalidate the
  token Helix holds until step 12 completes. Coordinate a window if the app
  serves live traffic.
- **Wrong app selected.** Several Helix-branded Slack apps may be visible.
  Confirm the workspace name on the app page before touching scopes — screenshot
  at step 4 exists for this reason.
- **Workspace app-approval gate.** If the workspace requires admin approval for
  new scopes, step 11 stalls pending someone's approval and the task is blocked,
  not failed.
- **Login handoff blocks automation.** If the human is not present at step 2 the
  runbook cannot proceed; there is no agent-side workaround and none should be
  invented.

## Notes for Future Agents

- The `helix` Slack integration lives in two places: the legacy trigger bot
  (`api/pkg/trigger/slack/`) and the newer org transport
  (`api/pkg/org/infrastructure/transports/slack/`). Scope and OAuth wiring for
  the org path is in `api/pkg/server/helix_org_slack.go`.
- `helix/design/` contains dated design docs that are unusually detailed about
  operational consequences —
  `2026-07-15-org-human-slack-delivery.md` predicted this exact reauthorization
  need. Read `helix/design/` before assuming an integration question is
  undocumented.
- Pattern worth remembering: when a Slack/OAuth integration reports
  `missing_scope`, check whether the scope list in code is already correct
  before editing it. Scope lists govern *new* installs only; pre-existing
  installs need manual reauthorization.
- `frontend/src/components/dashboard/slackManifest.ts` derives `BOT_EVENTS`
  from `BOT_SCOPES` via a `SCOPE_EVENT` map, so `message.*` events stay in sync
  with `*:history` scopes automatically. Do not hand-maintain the event list.

## Implementation Notes (live findings)

### Confirmed app identity

- **App:** `Helix Meta Org`, App ID `A0BDUQTBLF4`, workspace **MLOps.community** (`T7FHA770F`).
- Six other Helix-branded apps exist in the same workspace (`FindOS`, `HelixOS`,
  `Helix Launchpad`, `Helix`, `Helix Meta`), so the exact-name match mattered.
- Admin login is `luke.marsden@gmail.com` via Google SSO, with a Google
  device-push 2FA step that only the human can complete.
- Slack now redirects `api.slack.com/apps/<id>/oauth` to the newer in-client
  settings UI at `app.slack.com/app-settings/<team>/<app>/oauth`. Same page,
  different chrome.

### This app IS bound to a live deployment

Redirect URL is `https://meta.helix.ml/api/v1/slack/oauth/callback`. So the app
backs the live `meta.helix.ml` Helix deployment — the reinstall in D3 will cycle
its bot token, and the Helix-side **Connect workspace** refresh is required, not
optional.

### Scope audit result — three scopes missing, not two

Bot Token Scopes currently on the app (11):

```
app_mentions:read   channels:history   channels:join   channels:read
chat:write          chat:write.customize   files:write  groups:history
groups:read         im:history         reactions:write
```

Diffed against `defaultSlackBotScopes` (`api/pkg/server/helix_org_slack.go`),
**three** are missing, not the two Priya reported:

| Missing scope | Why it matters |
|---|---|
| `users:read` | Priya's request — required alongside email lookup |
| `users:read.email` | Priya's request — `users.lookupByEmail` |
| `im:write` | **Not reported by anyone.** Required to open a DM. Without it the `ask_human` Slack DM delivery path cannot work — it can only post to channels. |

`im:write` is a latent bug in this install: the org human Slack delivery feature
(`design/2026-07-15-org-human-slack-delivery.md`) documents `im:write` as
required "to open a DM", and the app has `im:history` but not `im:write`. The
install predates that feature exactly as suspected.

Gotcha for future agents: the "Reinstall to <workspace>" link on the OAuth page
carries the currently-granted scope list in its `scope=` query parameter. That
is the fastest way to read what the *live token* actually holds, as opposed to
what the app config lists.
