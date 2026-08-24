# Trigger configuration UX

Status: design, awaiting approval
Date: 2026-08-24

## Problem

Trigger settings are not discoverable. Across the three surfaces that show a
Trigger — the New/Edit drawer, the org-chart side pane, and the detail page —
a user cannot see what settings a kind accepts, what those settings are
currently set to, how to actually fire the Trigger, or where its credentials
live. For five of eight kinds the entire settings UI on at least one surface is
a textarea seeded with `{}` and labelled "Advanced settings only", which is
untrue: it is the only settings surface there is.

Two kinds are not merely undiscoverable but non-functional from the UI.

## Evidence

Probed live against the dev stack (`localhost:8080`, org `test`).

Create requests carrying exactly what the UI sends:

```
email  {"inbound_address":"probe@example.com"}
  -> 400 "trigger transport: email transport: alias is required"
slack  {}
  -> 400 "trigger transport: slack transport: service_connection_id is required"
```

`TriggerFormDialog` sends `inbound_address`; `transport.EmailConfig` requires
`alias`. **Email Triggers cannot be created from the UI at all.** Slack offers a
`{}` textarea for a required field it never names and provides no picker for.

The generic webhook Trigger's inbound URL exists and works, but is unguessable
and undocumented:

```
POST /api/v1/orgs/test/webhooks/org_01m0mfs4eg4e5qqhm3s3m30p9y/tr-f4aa8f96-...
Authorization: Bearer <helix api key>
-> 200 {"id":"e-d24a2424-...","triggerId":"tr-f4aa8f96-..."}
```

The org segment appears twice and the second must be the canonical `org_…` id;
the slug 404s, because `webhook.go` reads `PathValue("org")` raw instead of
resolving it. This URL is displayed nowhere in the product.

The list page carries 10 rows for this org, 6 of which are internal plumbing
(per-worker transcripts, a DM pair, a team channel, the event bus).

### Backend contract vs. what the UI shows

| Kind | Backend config fields | Create dialog | Detail page |
|---|---|---|---|
| `local` | — | hidden | "no additional settings" |
| `helix_events` | — | not offered (system-managed) | "no additional settings" |
| `webhook` | `outbound_url` (**outbound**) | raw `{}` textarea | raw JSON |
| `email` | `alias` (required) | wrong field name — **broken** | raw JSON |
| `slack` | `service_connection_id` (required), `channel_id` | raw `{}` — **broken** | `channel_id` only |
| `cron` | `schedule` (required), `message` | guided | guided |
| `github` | `repo`, `events` (req), `branches`, `webhook_id`, `webhook_html_url` | guided | repo + branches; **events editor dropped** |
| `gitlab` | `repository_id` (req), `repo`, `events` (req), `webhook_*` | hardcodes `["Merge Request Hook"]`, 1 of 4 supported | raw JSON |

`webhook`'s only field being **outbound** is the sharpest trap: a user
configuring a webhook Trigger is thinking inbound, and the single field on offer
does the opposite thing with no label saying so.

## Decisions

1. **Per-kind settings are declared in Go and rendered generically, with
   per-kind overrides for rich controls.** The alternative — hand-written React
   per kind — duplicates field names and required-ness in TypeScript, which is
   precisely how the email bug arose.
2. **The webhook route bug is fixed and the URL displayed. Auth stays the Helix
   API key.** No per-Trigger signing secret in this pass.
3. **One `TriggerConfig` component renders all three surfaces at different
   densities**, so the side pane can never again show less than the page.
4. **Config visibility only.** No prerequisite gating of unusable kinds, no
   "send test event" button, no liveness probes.
5. **Internal ids stop reaching the outside world**, via a public `slug`. The
   list's row membership is unchanged.

## Design

### 1. `transport.Descriptor` — one per Kind

`api/pkg/org/domain/transport` gains a `Describer` interface alongside the
existing `Strategy`. Each kind's file grows a `Describe()` immediately adjacent
to its `Validate()`.

```go
type Descriptor struct {
    Label, Summary string
    Fields         []Field
    Activation     Activation
    Secrets        []SecretRef
    SystemManaged  bool // not offered in New Trigger
}

type Field struct {
    Name, Label, Help, Placeholder string
    Type      FieldType // string | url | cron | stringlist | github_repo |
                        // github_events | gitlab_repo | gitlab_events |
                        // slack_workspace | slack_channel
    Required  bool
    ReadOnly  bool      // server-managed: webhook_id, webhook_html_url
    Direction Direction // Inbound | Outbound
}

type Activation struct {
    Summary, Verb, URLTemplate, AuthHeader, AddressTemplate, Note string
}

type SecretRef struct {
    Label, SettingKey, Location string
}
```

`Direction` exists solely to name the `outbound_url` trap in the UI.

Adding a Kind remains "one new file + one map entry": `Describe()` sits beside
`ParseConfig` and `Validate` in that same file, and `DescribeAll()` walks
`kindOrder`.

**Parity test.** A table test asserts that for every Kind, every `Field` marked
`Required` causes `Validate()` to fail when that key is absent from the config.
This test fails today against `email`'s `inbound_address` and is the mechanism
that keeps the descriptor honest.

### 2. API

- `GET /api/v1/orgs/{org}/trigger-kinds` returns the descriptor list in
  `kindOrder` with templates resolved for the calling org.
- `TriggerDTO` gains `slug` and `activation` — the resolved fire-this recipe for
  *this* Trigger (concrete URL, composed email address, schedule summary), so
  the frontend does no templating.
- **Webhook route fix.** `server.go` registers `POST /webhooks/{org}/{triggerID}`
  on the org mux; mounted under the authRouter's `/orgs/{org}/` prefix this
  yields the doubled path, and `PathValue("org")` beats the already-resolved
  context org. The handler was written for two deployment modes
  (`if orgID == "" { orgID = OrgIDFromContext(...) }`) but the empty case is
  unreachable when embedded. Register the org-less sibling `POST
  /webhooks/{triggerID}`, so embedded requests fall through to the resolved
  context org. Standalone keeps its `{org}` form. Resulting public URL:

  ```
  POST /api/v1/orgs/{org}/webhooks/{trigger-slug}
  ```

  Only `webhook_test.go` references the doubled form; no client, frontend, or
  doc constructs it, so this is not a breaking change in practice.
- Trigger routes resolve `{handle}` as slug-then-id through one helper, matching
  the id-or-slug contract `lookupOrg` already establishes.

### 3. Public handle: `slug`

Internal ids are unchanged. Every `s-` family is a **derived, deterministic
string** computed at its call site rather than looked up —
`"s-transcript-"+workerID`, `"s-dm-"+pair`, `"s-team-"+managerID`,
`"s-slack-ws-"+connID`, and the `s-helix-events` const. They are load-bearing
addresses; re-minting them would mean adding a lookup at every derivation site.
They stay, and stop being displayed.

- New `slug` column on `org_triggers` with `uniqueIndex idx_trigger_org_slug`.
- Derived on create from `Name` via the existing `data.SlugifyName`,
  deconflicted with a `-2`/`-3` suffix.
- Backfilled from `Name` for existing rows. `Name` is already unique per org
  (`idx_trigger_org_name`), so collisions are rare and the deconflictor covers
  them.
- **Slug does not follow later renames**, because it appears in live webhook
  URLs. It is directly editable on the detail page, with a warning on webhook
  kinds that the payload URL changes.
- System Triggers get readable slugs too: `transcript-b-sam`, `helix-events`,
  `dm-b-sam-chief-of-staff`.

Where the id stops appearing:

| Surface | Today | After |
|---|---|---|
| Detail URL | `/triggers/tr-afe1fefc-dc64-...` | `/triggers/hello` |
| Detail page heading | the id, monospace h5; name a grey subtitle | the **name**; id copyable under Advanced |
| List page | id in monospace under every name | dropped from default columns |
| Side pane | full uuid under the title | name + kind |
| Webhook payload URL | `.../webhooks/{ORG_ID}/tr-afe1fefc...` | `.../webhooks/hello` |
| Attached agents | raw `b-sam` chips | agent display names |

**MCP root cause.** `create_trigger`'s description tells agents *"Pass `id` as a
short readable handle (e.g. `s-releases`)"* — this is why agent-created Triggers
carry `s-` ids. Remove `id` from the tool's arguments; agents pass `name` and the
server mints both id and slug.

Because the `s-` prefix is currently the only visual cue separating plumbing
rows from user-created ones, system-managed rows gain a small "system"
indicator. Nothing is filtered and row membership is unchanged.

### 4. Frontend

```
components/helix-org/trigger/
  TriggerConfig.tsx           descriptor-driven; density=compact|full, mode=read|edit|create
  TriggerFieldRenderer.tsx    FieldType -> control
  TriggerActivationCard.tsx   "How to fire this" + MarkdownCodeBlock curl
  TriggerSecretsNote.tsx      deep-link to the owning settings key
  fields/SlackWorkspacePicker.tsx
  fields/GitLabRepoPicker.tsx
  fields/GitLabEventsField.tsx
```

Rich field types delegate to the components that already exist:
`CronScheduleFields`, `GitHubRepoPicker`, `GitHubEventsField`,
`GitHubBranchesField`. `SlackWorkspacePicker` sources options from the existing
`GET /orgs/{org}/slack/workspaces`. Populating a field's options is not
readiness gating and remains in scope under decision 4.

Raw JSON survives as a collapsed **Advanced (raw JSON)** accordion with correct
helper text, for fields the descriptor has not yet modelled.

The three surfaces become thin consumers: `TriggerFormDialog` renders
`mode="create"`, `TriggerDetailDrawer` renders `density="compact" mode="read"`,
`HelixOrgTriggerDetail` renders `density="full"`.

### 5. Per-kind outcomes

| Kind | Fixed |
|---|---|
| `email` | `inbound_address` -> `alias`; composed address shown as the activation recipe. Unblocks creation. |
| `slack` | workspace picker for the required `service_connection_id`, on all three surfaces. Unblocks creation. |
| `webhook` | `outbound_url` labelled Outbound; inbound URL, auth, and a copyable curl shown as the activation recipe |
| `github` | events editable after create; `webhook_id` / `webhook_html_url` visible read-only |
| `gitlab` | all four supported events, not just `Merge Request Hook`; `repository_id` visible read-only |
| `cron` | schedule and message on every surface, not only create |
| `local` / `helix_events` | stated as having no settings, with the activation recipe explaining what publishes to them |
| list page | Edit action in the row menu; `kind` disabled *with a stated reason* |

## Out of scope

- Per-Trigger webhook signing secrets (decision 2).
- Prerequisite gating of unusable kinds, test-event buttons, liveness probes
  (decision 4).
- **A GitLab webhook install panel.** `install-webhook` and `webhook-status`
  endpoints exist for GitLab with no UI, but the GitHub equivalent carries a
  live status check, which decision 4 excludes. Follow-up.
- Re-minting internal ids, or renaming the Topic/Stream/Trigger vocabulary in
  routes such as `/topics/{topic_id}/github/webhook`.
- Filtering or regrouping the Triggers list (decision 5).

## Testing

- Go: descriptor/`Validate` parity table test; webhook route resolution test
  covering slug and id in both deployment modes; slug derivation and
  deconfliction tests.
- `cd frontend && yarn build`.
- End-to-end in the browser at `http://localhost:8080`: create one Trigger of
  every kind the org can support, confirm each shows its fields, current values,
  and activation recipe on all three surfaces; fire the webhook Trigger with the
  exact curl the UI displays and confirm the event lands.

## Risks

- **Slug backfill.** Runs over existing rows; a bad slugify or a missed
  collision breaks trigger URLs. Mitigated by the unique index plus the
  deconflictor, and by resolution accepting ids indefinitely.
- **Webhook URL change.** The old doubled path is referenced only by its own
  tests, but any operator who reverse-engineered it would need to re-copy.
- **Descriptor drift.** The parity test covers required-ness; it does not cover
  help text or labels going stale.
