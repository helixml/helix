# Single org-wide Helix events topic (replace per-project spec-task topics)

## Summary
Redesigns the Helix→org event bridge from **per-project** spec-task topics to
**one generic, org-wide "Helix events" topic** per org. All Helix events flow
onto this single bus; the event family/type is carried in the message envelope
(`domain` + `event_type`), so future event kinds (project lifecycle, PR, CI,
membership) can ride the same topic. Spec-task attention events are the first
`domain`. Routing to individual bots is done with the existing filter-processor +
subscribe primitives keyed on `domain`/`event_type`/`project_id` — no per-project
topics.

Follows on from spec task 002209, which introduced the bridge as a per-project
`spectask` topic created lazily on the first attention event. That shape is
removed.

## Changes
- **New transport kind `helix_events`** (`api/pkg/org/domain/transport/helixevents.go`):
  inbound-only, empty always-valid config, registered in `strategies` + `kindOrder`.
- **New reconciler** `api/pkg/org/application/helixevents/reconciler.go`: ensures
  exactly one topic per org with the deterministic id `s-helix-events`
  (idempotent, race-safe get-or-create), mirroring the Slack workspace-topic
  pattern. Built in the composition root and wired into org bootstrap in
  `helix_org_middleware.go` alongside the topology/Slack/role reconcilers.
- **Publisher rewrite** (`spec_task_attention_publisher.go`): publishes each
  attention event onto the single `helixevents.TopicID`, ensuring the topic
  defensively for brand-new orgs. Envelope renamed `specTaskEventExtra` →
  `helixEventExtra` with `domain` (`"spectask"`) + `event_type`; first-class
  Message fields (Subject/Body/ThreadID=SpecTaskID/MessageID) preserved.
- **Deleted** the per-project path outright (no deprecation): `EnsureSpecTaskTopic`,
  `transport/spectask.go` + `SpecTaskConfig` + strategy/accessor, and the
  `KindSpecTask` registry entries. Transport tests updated.
- Legacy per-project `spectask` topic rows are left in place for manual operator
  cleanup (per review); the topic read path stores kind as a plain string, so
  they load harmlessly.

## Not user-creatable
`helix_events` is absent from the New Topic UI (`frontend/src/pages/HelixOrgTopics.tsx`
`TRANSPORT_KINDS`), so operators can't create it — it is system-managed.

## Testing
- Unit tests: transport kind registration + empty-config validity
  (`helixevents_test.go`); reconciler creates exactly one topic, idempotent,
  nil-safe (`reconciler_test.go`); publisher targets the single topic, generic
  envelope, single-topic reuse across projects, org-empty no-op
  (`spec_task_attention_publisher_test.go`). All pass.
- Browser E2E on inner Helix (`localhost:8080`): the bootstrap reconciler created
  exactly one "Helix events" topic (`s-helix-events`, kind `helix_events`);
  creating two projects did not create any per-project topics (topic count stayed
  at 1); the New Topic dialog does not offer `helix_events`. Screenshots included.
- The publish→dispatch→activation path is unchanged from 002209 (only the topic
  identity changed); the live bot-activation flow was not re-run end-to-end (it
  requires the full spec-task agent/sandbox run) and is covered by the publisher
  unit tests.

## Screenshots
![Single Helix events topic](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002215_redesign-the-helixorg/screenshots/01-single-helix-events-topic.png)
![New Topic dialog omits helix_events](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002215_redesign-the-helixorg/screenshots/02-new-topic-no-helix-events.png)
