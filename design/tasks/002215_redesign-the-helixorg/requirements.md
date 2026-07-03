# Requirements: Single Org-Wide Helix Events Topic (Replace Per-Project Spec-Task Topics)

## Background

Spec task 002209 (org-wide project-manager bot) added a Helix→org event bridge:
each Helix `AttentionEvent` is published onto an org Topic so subscribed bots are
activated. It was built as a **per-project** topic — transport kind `spectask`,
one "Spec tasks: <projectId>" topic auto-created lazily on the first attention
event for that project (`EnsureSpecTaskTopic` in
`api/pkg/server/spec_task_attention_publisher.go`).

This shape is wrong: it multiplies topics per project, creates them lazily (so
nothing exists to wire against until an event fires), and is scoped to a single
event family. We want **one generic, org-wide event bus** onto which all Helix
events flow, with per-project / per-type routing done by filter processors.

## User Stories

### US1 — One Helix events topic per org
**As** an org operator, **I want** exactly one "Helix events" topic in my org
(not one per project) **so that** all Helix events arrive on a single,
predictable bus I can route from.

**Acceptance criteria**
- Every org has exactly one Topic of transport kind `helix_events`, named
  "Helix events", with a deterministic id.
- The topic is created by a reconciler on org bootstrap/reconcile (not lazily on
  first event), the same way Slack routes and org topology are reconciled.
- Reconcile is idempotent: running it repeatedly never creates a second topic.
- No per-project spec-task topic is ever created.

### US2 — Generic event envelope
**As** a bot author, **I want** each event to declare its family and type in a
structured envelope **so that** the same bus can carry future event kinds
(project lifecycle, PR, CI, membership) and I can select the ones I care about.

**Acceptance criteria**
- Each published event carries an `extra` payload including `domain` (event
  family, e.g. `spectask`) and `event_type` (specific type within the family),
  plus the payload keys for that domain (`project_id`, `spec_task_id`, …).
- First-class `streaming.Message` fields are preserved as coerced today:
  `Subject`=Title, `Body`=Description, `ThreadID`=SpecTaskID, `MessageID`=event
  ID.
- Spec-task attention events are the first `domain` on the bus; the design does
  not special-case them beyond the domain value.

### US3 — Publisher targets the single topic
**As** the system, **I want** the attention-event sink to publish onto the
single org Helix events topic **so that** the existing publish → dispatch →
activation path is preserved.

**Acceptance criteria**
- `attentionTopicPublisher.PublishAttentionEvent` publishes onto the one
  `helix_events` topic for the event's org (resolved by deterministic id),
  keeping org scoping.
- A missing org scope remains a no-op.
- The event still reaches subscribed bots via the unchanged dispatcher.

### US4 — Filter-processor routing (no per-project topics)
**As** a PM-bot operator, **I want** to route events to a bot by `project_id`
and/or `event_type`/`domain` using a filter processor over the single topic
**so that** I never need per-project topics.

**Acceptance criteria**
- A filter processor whose input is the Helix events topic and whose predicate
  keys on `.Message.extra` (`project_id` / `event_type` / `domain`) or
  `.Message.thread_id` routes matching events to a bot's inbox topic.
- The `/pm-bot` prompt describes subscribing to the single Helix events topic +
  filtering, not subscribing to per-project topics.

### US5 — Delete the per-project approach
**As** a maintainer, **I want** the per-project `spectask` code path **fully
deleted** (not deprecated) and any existing per-project topics cleaned up **so
that** there is exactly one code path and no dead code or orphaned rows.

**Acceptance criteria**
- Per-project find-or-create (`EnsureSpecTaskTopic`) is deleted entirely.
- The `spectask` transport kind is **deleted**: `transport/spectask.go`,
  `SpecTaskConfig`, the strategy, the `SpecTaskConfig()` accessor, and its
  registry entries (`strategies`, `kindOrder`) are all removed. No deprecated
  stub or compatibility shim remains.
- The transport registry/tests are updated to drop `KindSpecTask`.
- Existing `spectask` topic rows are cleaned up (deleted) by the reconciler.
- `helix_events` is inbound-only, system-managed, and NOT offered in the New
  Topic UI (`TRANSPORT_KINDS`) or user-creatable via `create_topic`.

## Non-Goals
- **Documentation / prompt changes.** Per review, no documentation is updated:
  the 002209 design docs and the `api/pkg/org/application/prompts/templates/pm_bot.md`
  prompt are left as-is (see Open Question 6).
- Adding new event domains beyond `spectask` (the envelope must *allow* them; we
  don't implement them here).
- New MCP tools (routing uses the existing topic + filter-processor + subscribe
  primitives, per 002209).
- Changing the AttentionService event source or the dispatcher.

## Open Questions
1. **Legacy row handling:** plan is for the reconciler to **delete** existing
   per-project `spectask` topics (and any subscriptions/filter processors that
   pointed at them cascade/break). Since 002209 shipped recently and these
   topics are auto-managed, is destructive cleanup acceptable, or do you want a
   one-time migration that re-points existing subscriptions to the new topic?
2. **`KindSpecTask` deletion (confirmed):** the constant, strategy, config, and
   file are deleted outright — no deprecation. The read path stores kind as a
   plain string, so legacy rows still load for the reconciler's delete scan.
3. **Deterministic topic id:** proposed `s-helix-events` (unique per org via the
   `(id, orgID)` key, mirroring `s-slack-ws-…`). Any preferred naming
   convention?
4. **Transport kind name:** proposed `helix_events`. Confirm this over
   alternatives like `helix` or `events`.
5. **Config shape:** the `helix_events` transport needs no config (org-wide,
   inbound-only). An empty always-valid config is proposed — confirm no per-org
   settings are needed.
6. **Stale `pm_bot.md` prompt (flagged):** per review, no docs/prompts are
   changed. Note that `pm_bot.md` currently instructs the PM bot to subscribe to
   the per-project "Spec tasks: <projectId>" topics, which this change deletes —
   so after this ships the prompt will describe topics that no longer exist.
   Confirm you want the prompt left unchanged despite that.
