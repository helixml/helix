# Org human Slack delivery

Date: 2026-07-15
Status: implemented, awaiting review
Branch: feat/org-human-slack-delivery

## Problem

`ask_human` previously wrote only to the Helix notification inbox. Human org
members already had an identity map and Helix already supported Slack workspace
OAuth, Slack API credentials, an automated workspace router, and inbound thread
following, but those capabilities were not connected into human delivery.

The feature lets an org owner choose Helix or Slack as their preferred contact
route. A Bot continues to call the single `ask_human` primitive; delivery code
selects the configured route and reports whether it used Helix, a Slack DM, or a
Slack channel.

## Reused capabilities

- Human nodes and their existing `Identity` map.
- `ask_human` as the only Bot-facing delivery action.
- Org-scoped Slack workspace connections and decrypted bot tokens.
- `mint_credential` for the Chief of Staff to resolve user, team, and channel
  IDs through Slack's API during setup.
- The automated Slack router and its per-Worker managed outputs.
- The domain-event-backed Slack thread participant log and inbound
  thread-follow routing.
- Helix attention events for notification-panel delivery.

No new table, transport abstraction, or Slack-specific MCP sending tool was
added.

## Human identity and route selection

The human node identity map uses these keys:

| Key | Meaning |
|---|---|
| `preferred_contact` | `helix` or `slack`; missing is treated as `helix` |
| `slack_user_id` | Canonical Slack user ID; required for Slack delivery |
| `slack_channel_id` | Optional canonical channel ID; missing selects a DM |
| `slack_team_id` | Optional canonical workspace ID used to select an org workspace |
| `email` | Existing contact identity, preserved by contact patches |
| `github` | Existing contact identity, preserved by contact patches |

Route selection is exact:

- Missing or `helix`: create the existing attention event for the linked Helix
  user.
- `slack` with no channel: open a DM with `slack_user_id`, then post there.
- `slack` with a channel: post to `slack_channel_id` and mention
  `slack_user_id` in the message.
- Any other value: return an error.

Slack failures do not fall back to Helix. A silent fallback would claim the
person was contacted through their selected route when they were not.

## `set_human_contact`

The new MCP primitive patches a human node's identity map. It preserves keys
not present in the patch, trims keys and values, and removes a key when its
patched value is empty. It rejects non-human targets, unsupported preferred
routes, and Slack preference without `slack_user_id`.

The tool is included in `OwnerBotTools`, so the seeded Chief of Staff receives
it and existing Chief of Staff Bots receive it during seed reconciliation. The
restriction is capability-based: the tool implementation scopes all access to
the caller's org, but it does not perform a second hard-coded caller-role check.
Any Bot explicitly granted this tool can use it.

## Slack delivery and replies

Slack delivery resolves a workspace only from Slack service connections owned
by the caller's org. When `slack_team_id` is present it must match that
workspace. When it is absent, delivery accepts exactly one distinct Slack team,
including when that team has duplicate connection rows, and rejects multiple
distinct teams as ambiguous. For duplicate rows belonging to the selected team,
an OAuth-installed connection is preferred over a manual-token connection.

Messages include the sending Bot's display name. Replyable messages also tell
the human to reply in the Slack thread. Before posting a replyable message,
delivery verifies that the workspace's automated router exists, has thread
following enabled, and has a managed output for the sending Bot. It fails
before posting if that reply path is unavailable.

After Slack returns the root message timestamp, delivery records the sending
Bot as a participant under the existing router-scoped thread subject. The
record operation is idempotent. A later inbound Slack thread reply then follows
the existing thread-follow path to the Bot's managed output topic. Non-replyable
messages do not require or register thread routing.

If Slack accepts the post but participant recording fails, `ask_human` returns
an error and does not fall back; the external message may already be visible.

## Slack OAuth

Both backend default scopes and the generated Slack manifest now include:

- `chat:write` to post the message.
- `im:write` to open a DM.
- `users:read`, required by Slack alongside email-based user lookup.
- `users:read.email` so setup can resolve the human's canonical Slack user ID
  from the email they provide.

Existing Slack installations must be reauthorized to grant newly added scopes.
The Chief of Staff must not claim Slack setup is complete until the workspace
is installed and `set_human_contact` succeeds.

## Onboarding and UI

The seeded Chief of Staff prompt now asks what the org is for, who its key
people are, and whether future contact should use Helix or Slack. For Slack it
asks for an email and optional channel name, then uses `mint_credential` and
Slack's `users.lookupByEmail` and `conversations.list` APIs to resolve canonical
IDs before calling `set_human_contact`.

The human detail page adds a preferred-delivery selector and fields for Slack
user, channel, and workspace IDs. It requires a Slack user ID when Slack is
selected and preserves unrelated identity fields on save. This UI is a manual
configuration surface; the conversational Chief of Staff flow avoids asking a
human for opaque IDs unless Slack lookup fails.

The REST mutation used by this UI permits a human to update their own identity
or an org owner to update another human in the org. Other org members cannot
change another person's contact route.

## Security and multi-tenancy

- Human lookup and mutation use the caller's org ID.
- Human identity REST mutation is authorized only for the human themselves or
  an owner of that org.
- Slack workspace lookup lists connections for that same org and optionally
  filters by `slack_team_id`; another org's bot token is never selected. An
  omitted team ID is rejected when more than one distinct team is installed.
- Reply-router validation uses the resolved workspace connection ID and the
  sending Bot's managed route.
- Slack tokens remain in the existing encrypted service-connection path and
  are not stored in the human identity map.
- Channel delivery mentions only the configured canonical Slack user ID.

## Verification

Run in `/Users/psamuel/helix/helix-worktrees/org-human-slack-delivery`:

- `git diff --check`: passed.
- ASCII check of this document: passed with no non-ASCII characters.
- `go test ./api/pkg/org/interfaces/mcptools ./api/pkg/org/application/slackrouting ./api/pkg/server -count=1`: passed all three packages in 0.851 s, 0.498 s, and 3.670 s.
- `go test ./api/pkg/org/interfaces/server/api ./api/pkg/server ./api/pkg/org/infrastructure/transports/slack -count=1`: passed all three packages in 1.967 s, 4.040 s, and 0.256 s.
- `go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/`: passed.
- `cd frontend && yarn test --run src/components/dashboard/slackManifest.test.ts`: passed 1 file and 5 tests.
- `cd frontend && yarn build`: passed, 21,709 modules in 18.41 s.

The tests exercise contact patching and validation, route reporting, Helix
reply metadata, Slack DM and channel selection, no-fallback behavior, reply
router preflight, participant idempotency, inbound reply fan-out, and required
OAuth scopes.

## NOT tested

- NOT tested: local browser end-to-end onboarding and human-detail UI. A
  fresh-worktree `./stack start` successfully built Zed, then could not start
  services because host networking and inotify setup required an interactive
  macOS sudo password. Playwright MCP navigation was also attempted, but its
  shared Chrome profile was already in use.
- NOT tested: real Slack workspace installation, OAuth reauthorization, user or
  channel lookup, DM/channel delivery, and a live threaded reply.
