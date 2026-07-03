# Design: Single Org-Wide Helix Events Topic (Replace Per-Project Spec-Task Topics)

## Summary
Replace the per-project `spectask` topic with **one generic `helix_events` topic
per org**, created by a reconciler on bootstrap, receiving all Helix events with
the event family/type in a structured envelope, and routed to bots by filter
processors. Remove the per-project find-or-create path and clean up legacy rows.

## What exists today (verified)
- **Transport registry:** `api/pkg/org/domain/transport/` — one file per Kind,
  registered in `strategies` + `kindOrder` in `transport.go`; `KindSpecTask`
  ("spectask") in `spectask.go` with `SpecTaskConfig{ProjectID}`.
- **Publisher:** `attentionTopicPublisher` in
  `api/pkg/server/spec_task_attention_publisher.go` — `PublishAttentionEvent`
  calls `EnsureSpecTaskTopic` (per-project lazy find-or-create) then publishes a
  `streaming.Message` (Subject/Body/ThreadID/MessageID coerced;
  `specTaskEventExtra` = {spec_task_id, event_type, project_id, project_name,
  spec_task_name}). Wired at `helix_org.go:722`.
- **Reconciler pattern:** `application/reconcile` (topology) and
  `application/slackrouting` (auto-router routes). Slack uses a **deterministic
  per-org topic id** (`slackWorkspaceTopicID` = `s-slack-ws-<connID>`, in
  `helix_org_slack.go`). Reconcilers run at bootstrap in
  `helix_org_middleware.go` (`rec.ReconcileAll`, `s.slackRoutes.Reconcile`,
  `botsSvc.Reconcile`) and are built in the composition root `helix_org.go`.
- **Filter routing:** `processor.KindFilter` renders a Go template predicate
  against the message (`.Message.extra` raw JSON, `.Message.thread_id`, etc.) and
  republishes matches to output topics; bots `subscribe` to those.
- **UI:** `frontend/src/pages/HelixOrgTopics.tsx` `TRANSPORT_KINDS` lists only
  local/webhook/github/postmark/cron — `spectask` and `slack` are already
  excluded, so a new `helix_events` kind is user-uncreatable by omission.
- **Topic store read path:** `gorm/stream.go` maps `transport_kind` to a plain
  `transport.Kind(string)` with **no validation on read** — so removing the
  `KindSpecTask` constant does not break loading legacy rows for cleanup.

## Target architecture

### 1. New transport kind `helix_events`
New file `api/pkg/org/domain/transport/helixevents.go`:
- `const KindHelixEvents Kind = "helix_events"`.
- `HelixEventsConfig struct{}` with `Validate() error { return nil }` — org-wide,
  inbound-only, no per-topic config.
- `helixEvents` strategy + `HelixEventsConfig()` accessor (mirroring siblings).
- Register in `strategies` map and `kindOrder` in `transport.go`.
- Add a `helixevents_test.go` (kind-in-values, empty-config-valid).

### 2. Deterministic per-org topic + reconciler
- **Deterministic id:** `helixEventsTopicID()` → `streaming.TopicID("s-helix-events")`.
  Unique per org via the `(id, orgID)` composite key (same pattern as Slack).
- **New reconciler** `api/pkg/org/application/helixevents/reconciler.go`, built
  in the composition root next to `slackrouting.New`, wired into
  `helix_org_middleware.go` bootstrap alongside the others:
  - `Reconcile(ctx, orgID)`: **ensure** the single `helix_events` topic exists —
    get-or-create the deterministic id, race-safe re-read on create conflict
    (copy the idempotent get-or-create shape from
    `reconcile.ensureTopicWithMembers`). Name "Helix events", description "Helix
    event bus for this org", transport `{Kind: KindHelixEvents}`. Nothing else —
    it does **not** touch legacy `spectask` rows (operator cleans those up
    manually, per review).
  - Narrow deps: `store.Topics` only (+ `now`, logger). Nil-safe no-op like the
    other reconcilers.
- Keep the reconciler in `application/` (org reconcilers live there); the
  deterministic-id helper can live with the publisher or in the reconciler
  package (single source of truth, shared with the publisher).

### 3. Publisher change
Rewrite `attentionTopicPublisher` in `spec_task_attention_publisher.go`:
- Drop `EnsureSpecTaskTopic` and the per-project ensure. `PublishAttentionEvent`
  resolves the org's single topic by `helixEventsTopicID()` and publishes.
- Defensive get-or-create of the deterministic topic on publish (covers a
  brand-new org whose bootstrap reconcile hasn't run yet) — same idempotent
  ensure the reconciler uses, shared. This keeps publishing robust without
  reintroducing per-project topics.
- Rename `specTaskEventExtra` → generic `helixEventExtra` and add the family
  discriminator:

  ```go
  type helixEventExtra struct {
      Domain       string `json:"domain"`        // "spectask" (event family)
      EventType    string `json:"event_type"`    // type within the family
      ProjectID    string `json:"project_id,omitempty"`
      SpecTaskID   string `json:"spec_task_id,omitempty"`
      ProjectName  string `json:"project_name,omitempty"`
      SpecTaskName string `json:"spec_task_name,omitempty"`
  }
  ```
  For spec-task events `Domain = "spectask"`. First-class Message fields
  (Subject/Body/ThreadID=SpecTaskID/MessageID) unchanged. `OrganizationID`
  no-op guard unchanged; `ProjectID` empty is now allowed (routing handles it).

### 4. Routing (no new code)
Consumers attach a `KindFilter` processor with input = the Helix events topic and
a predicate over `.Message.extra` (`domain` / `event_type` / `project_id`) or
`.Message.thread_id`, routing matches to the bot's inbox topic (existing
`application/processors` + `subscribe`). This is documented, not coded.

### 5. Delete the per-project path (no deprecation)
- **Delete** `transport/spectask.go` + `spectask_test.go` entirely, including
  `SpecTaskConfig`, the `specTask` strategy, and the `SpecTaskConfig()`
  accessor; remove `KindSpecTask` from `strategies` and `kindOrder`. No
  deprecated stub or compatibility shim is left behind.
- Update `transport_test.go` (kind count/order assertions, drop `KindSpecTask`).
- **Delete** `EnsureSpecTaskTopic` and its tests; update
  `spec_task_attention_publisher_test.go` for the single-topic behavior.
- Legacy `spectask` topic rows are **left in place** for manual operator cleanup
  (per review) — not deleted by this change. The read path stores kind as a
  plain string with no read-time validation, so those rows still load harmlessly
  after `KindSpecTask` is removed.

### 6. No documentation / prompt changes
Per review, **no documentation is updated** — the 002209 design docs and the
`api/pkg/org/application/prompts/templates/pm_bot.md` prompt are left as-is. See
requirements Open Question 6: the prompt currently references the now-deleted
per-project "Spec tasks: <projectId>" topics and will be stale after this ships.

## Key decisions
- **Deterministic id, no config.** Org-wide singleton → deterministic id
  (`s-helix-events`) keyed by org; no `project_id` config to carry. Mirrors the
  Slack workspace-topic pattern.
- **Reconciler owns creation, publisher defensively ensures.** Bootstrap
  reconcile guarantees the topic for every org; the publisher's idempotent
  get-or-create covers the brand-new-org race — neither creates per-project
  topics.
- **Generic envelope with `domain` + `event_type`.** One bus for all Helix
  events; spec-task is just the first `domain`. Future domains add a value, not a
  topic.
- **No automated legacy cleanup.** Per review, legacy per-project `spectask`
  topic rows are left untouched; the operator removes them manually. The change
  neither migrates nor deletes them.
- **Delete `KindSpecTask` outright (no deprecation).** Read path stores kind as a
  plain string, so legacy rows still load for the delete scan without the
  constant.

## Testing
- Unit: `HelixEventsConfig.Validate`; kind registered in `KindValues`;
  `helixEventsTopicID` determinism.
- Reconciler: creates exactly one topic; idempotent on re-run; deletes legacy
  `spectask` rows; no-op on nil deps.
- Publisher: publishes onto the single deterministic topic; envelope carries
  `domain="spectask"` + `event_type` + ids; org-empty is a no-op; coerced Message
  fields intact.
- Filter predicate over `.Message.extra` (`project_id`/`event_type`/`domain`)
  selects/drops correctly.
- Transport registry tests updated for the kind swap.
- **Browser E2E (inner Helix `localhost:8080`) — must be verified in the browser
  UI, not just via API/CLI:** two projects in one org; confirm in the Topics page
  that exactly one "Helix events" topic exists and no per-project topics appear;
  confirm the New Topic dialog does not offer `helix_events`; wire a PM bot via a
  filter processor keyed on `project_id` through the UI; trigger attention events
  on each project via the UI; confirm the bot is activated and acts on the right
  project, and the events are visible on the Helix events topic in the browser.
