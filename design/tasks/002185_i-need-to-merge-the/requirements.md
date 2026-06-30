# Requirements: Merge Role and Worker into a single Bot concept

## Background

The `helix-org` subsystem (`api/pkg/org/`, `frontend/src/pages/HelixOrg*.tsx`)
currently models an org chart with **two** entities:

- **Role** — a job description: markdown `Content` + a `Tools` list +
  `Topics` manifest. Many Workers can share one Role.
- **Worker** — a live participant: holds a `role_id`, a `kind`
  (`human`/`ai`), and `identity_content`. Reporting lines, subscriptions,
  transcript/team/DM streams, runtime state (project/agent/session), and
  activations are all **Worker-anchored**.

This two-level model (Role = capability template, Worker = instance) is the
source of most of the complexity in the QA plan (`api/pkg/org/QA.md`):
per-Worker identity, kind, the role→worker binding, "live propagation" of
Role edits to every Worker, "specialisation" of two Workers in one Role, etc.

## Goal

Collapse Role and Worker into **one** aggregate called a **Bot**. A Bot
*is* a Role that also participates directly in the org graph. It has **no
identity beyond its name (id)** — no `kind`, no separate `identity_content`,
no role binding. Everything that was Worker-specific is either dropped or
re-anchored onto the Bot.

helix-org is **pre-release** — no data migration/backfill is required (we
follow the precedent of `0004_drop_org_positions`: wipe helix-org tables,
let AutoMigrate recreate, re-bootstrap on next request).

## User Stories

### US-1 — Operator manages bots (not roles + workers)
As an org operator, I want a single **Bots** surface so that I create,
edit, and delete one kind of thing instead of juggling Roles and Workers.
- **AC1** The middle sidebar shows **Chart / Bots / Streams / Settings**
  (Roles and Workers tabs are gone, replaced by one **Bots** tab).
- **AC2** Creating a bot is a single action (`New bot`): id + content
  (markdown) + optional parent. No `kind` selector, no identity field, no
  role dropdown.
- **AC3** The bot detail page merges the old Role detail (content + tools
  editor) and Worker detail (subscriptions, reporting, inline chat,
  project/agent links, restart) into one page.
- **AC4** Deleting a bot is one action that cascades its reporting lines,
  subscriptions, streams, and runtime teardown (the old `Fire` + `DeleteRole`
  merged). Deletion stays REST-only (the LLM must not delete bots from chat).

### US-2 — Chart shows bots and reporting edges
As an operator, I want the Chart to show bots as nodes wired by reporting
and subscription edges.
- **AC1** Bots are plain nodes (no Role group-frames containing Workers).
- **AC2** Reporting edges (bot→bot), subscription edges (bot→stream), and
  the drag interactions behave as before but operate on bots.
- **AC3** Cycle guard, multi-manager, and the reparent-unsubscribe fix
  (QA §12.3a) still hold.

### US-3 — Bot-scoped MCP surface
As a bot's agent, I want MCP tools that speak in terms of bots.
- **AC1** `create_role`+`hire_worker` → **`create_bot`**;
  `update_role`+`update_identity` → **`update_bot`**;
  `read_roles`+`read_workers` → **`list_bots`** / **`get_bot`**.
- **AC2** `managers`, `reports`, `dm`, `subscribe`, `unsubscribe`,
  `publish`, `read_events` operate on bots; baseline read tools are injected
  on bot creation as today.
- **AC3** A bot's live tool surface is resolved from the bot's own `Tools`
  list (no role indirection).

### US-4 — Bot-scoped REST + runtime
As the React client and runtime, I want bot-shaped REST and persistence.
- **AC1** `/api/v1/orgs/{org}/roles` + `/workers` → `/bots`;
  `WorkerDTO`+`RoleDTO` → `BotDTO`. `/workers/{id}/parents`,
  `/activate`, `/chat`, `/identity` move to `/bots/{id}/…` (identity edit
  folds into the bot content update).
- **AC2** Tables `org_roles` + `org_workers` → **`org_bots`**;
  `org_subscriptions`, `org_reporting_lines`, `org_worker_runtime_state`
  re-keyed bot↔bot (`bot_id`). Composite `(id, org_id)` PK preserved.
- **AC3** Streams/transcripts keep deterministic ids keyed on bot id
  (`s-transcript-<botID>`, `s-team-<botID>`, `s-dm-<a>-<b>`).

### US-5 — Multi-tenancy and lifecycle integrity preserved
As a security-conscious operator, I want the merge to preserve every
isolation and topology guarantee.
- **AC1** All QA.md tenancy guarantees (§16: read + write isolation,
  colliding ids, spawner stamps the bot's own org) still pass.
- **AC2** Topology side-effects on hire/reparent/fire (transcript
  observership, team streams, DM channels) still reconcile correctly.
- **AC3** Worker-anchored → bot-anchored subscriptions still die on delete
  and do not auto-inherit.

### US-6 — Verified end-to-end via the local UI
As the implementer, I must prove the new flows work against the in-sandbox
Helix instance.
- **AC1** `api/pkg/org/QA.md` is rewritten to the merged Bot model.
- **AC2** Every rewritten QA scenario is exercised against the local Helix
  UI (localhost) with the `helix-org` alpha flag, including the load-bearing
  bot-chat (§10) and tenancy (§16) gates.

## Out of Scope

- Any non-`helix-org` use of the words "role"/"worker" (e.g. `authz` RBAC
  `Role`, GPU runner workers) — untouched.
- Data backfill / reversible migration (pre-release; wipe + recreate).
- New features beyond the merge itself.

## Constraints / Notes

- Follow **DDD**: change the `orgchart` domain aggregate first, then ripple
  outward (application → infrastructure → interfaces → frontend).
- Follow **TDD**: update/extend the existing `_test.go` suites first.
- Keep small interfaces (`helix-org` philosophy: ≤4 collaborators per
  service; the reconciler depends only on the narrow repos it touches).
