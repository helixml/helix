# Helix-org Slack integration (ServiceConnection-tiered + processor routing)

Date: 2026-06-23. Supersedes the abandoned `feat/helix-org-slack-transport`
design (`design/2026-06-16-helix-org-slack-stream.md`), which embedded a
bespoke router in the transport and stored installs in the config registry.

## Why

Org mode needs Slack as a first-class transport. The first attempt was judged
"completely wrong" after review: it put per-agent Slack apps in Socket Mode with
no routing, then later grew an in-transport Broadcast/Fuzzy router. The agreed
model instead:

- **One global Slack app per deployment** — secrets never in source.
  - **SaaS**: REST/Events-API app; org admins OAuth-install it into their
    workspace (no Socket Mode 10-connection ceiling).
  - **Self-hosted / Meta**: operator's own app in Socket Mode (one workspace).
- **Slack is a Topic transport.** A Topic binds to a workspace + channel.
- **Routing is the existing `domain/processor` filter layer**, not a bespoke
  router. One global app = one bot = a firehose; a filter predicate
  (`{{ if contains "!qa-bot" .Message.body }}1{{ end }}`) routes to the Worker.
- **Outbound**: one app posts as many personas via per-message
  `username`/`icon_url`.

## Credential model (three tiers, all on the `service_connections` table)

| Tier | type | scope | holds |
|---|---|---|---|
| Global app | `slack_app` | global (`organization_id=""`, helix-admin) | client_id/secret, signing_secret, app_token, bot_token, ingress_mode |
| Workspace install | `slack_workspace` | **org** (many per org) | team_id (indexed), team_name, bot_token, bot_user_id, app_id |
| User token (unchanged) | `OAuthConnection` | user | agent curl skills — untouched |

A Slack Topic's transport config is `{service_connection_id, channel}` —
*which* org workspace + *which* channel. One Topic ↔ one channel.

**Multi-tenancy:** `slack_workspace` rows are org-scoped and only ever read/written
through org-scoped handlers (`lookupOrg` + `authorizeOrgMember`, filtered by org).
The admin Service Connections panel manages only `slack_app`.

## Code map

- **`api/pkg/serviceconnection/slack/`** — generic, reusable protocol layer, zero
  org concepts: client builder, Events-API HMAC verify + parse (`EventsAPIHandler`),
  Socket Mode connector + single-owner pg lock, OAuth `CodeExchanger`, `PostAs`
  persona, `AuthTest`. The obvious home for future Slack ServiceConnection
  consumers (e.g. unifying `api/pkg/trigger/slack` later).
- **`api/pkg/org/domain/transport/slack.go`** — `KindSlack` + `SlackConfig`.
- **`api/pkg/org/infrastructure/transports/slack/`** — org wiring: `Ingest`
  (team_id→workspace→org→matching Topics→`Publishing.Publish`; **no router**),
  `Outbound` (persona post), `Provisioner` (channel join). Defines the
  `Workspaces` port; never imports the helix store.
- **`api/pkg/server/helix_org_slack.go`** — `slackWorkspaces` (Workspaces impl over
  the ServiceConnection store + decryption), OAuth install handlers, org-scoped
  workspace list/delete, `runSlackSocketMode`.
- **`api/pkg/server/helix_org.go`** — composition root: registers the slack
  outbound emitter + provisioner, builds the REST Events handler + socket runner.
- **`api/pkg/server/server.go`** — mounts `/api/v1/slack/events` (insecure),
  `/api/v1/slack/oauth/callback` (insecure), org-scoped
  `/api/v1/orgs/{org}/slack/{oauth/start,workspaces}`.

## Inbound flow

`POST /api/v1/slack/events` (or Socket Mode) → verify signature vs the global
`slack_app` signing secret → `Ingest.OnEvent`: drop bot events; `team_id →
slack_workspace → org`; match org Topics by (service_connection_id, channel);
`Publishing.Publish(org, topic, from="", msg)` each → existing dispatcher →
processor/filter layer → Worker activation. Outbound worker publishes are emitted
by the registered `KindSlack` emitter; inbound (`source=""`) is not re-emitted, so
no loop.

## Status

- Backend complete; unit tests for signature/dispatch + ingest routing.
- **Verified e2e on localhost:8080** (REST): admin creates `slack_app` (encrypted),
  signed `url_verification` → 200/challenge, bad signature → 401, signed message
  with `team_id`→ published onto the bound Topic (`org_events` row, `extra.slack_*`
  populated), unknown team → 200 + dropped, org-scoped workspace list isolated.
- Frontend: admin `slack_app` form in `ServiceConnectionsTable` done.

## Follow-ups

- Frontend org "Install to Slack" surface (button → `/orgs/{org}/slack/oauth/start`;
  list `/orgs/{org}/slack/workspaces`) and the Topic transport picker (`kind=slack`
  → choose workspace + channel). Backend endpoints exist and are tested.
- Optional `"slack"` CredentialProvider so agents can curl Slack (mirror
  `infrastructure/transports/github/credential_provider.go`).
- Unify `api/pkg/trigger/slack` onto `api/pkg/serviceconnection/slack`.
- Socket Mode multi-replica: a single replica holds the socket today. Multi-replica
  would need a cross-replica owner lock (e.g. a pg advisory lock) so exactly one
  replica opens the connection; not built yet.
