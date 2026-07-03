# Org-wide project-manager bot: cross-project spec-task management

## Summary
Completes the org-graph project-manager (PM) bot: an org-wide Bot that manages
the spec tasks of *other* projects in its own organization (never another org),
driven by the spec-task notification events those projects already emit.

The triggering side already existed (`AttentionService` → per-project
`KindSpecTask` topic → dispatch). The gap was the editing/discovery side: the
spec-task MCP tools were hard-scoped to the Bot's *own* project, there was no way
to discover projects, and the notification payload buried everything in `Extra`.

## Changes
- **Cross-project targeting:** every spec-task MCP tool now accepts an optional
  `project_id`. Empty = the Worker's own project (unchanged); non-empty targets
  another project, with a **hard cross-org block** enforced in
  `runtime/helix/spectasks.go` (`resolveProject` asserts
  `project.OrganizationID == caller org`; `ownedTask` chains task→project→org).
  The `projectID` is threaded through the `runtime.SpecTasks` port, the
  `application/spectasks` service, and the tool adapters.
- **Project discovery:** new `list_projects` / `get_project` MCP tools over a new
  `runtime.Projects` port (`runtime/helix/projects.go`) + `application/projects`
  service, both org-scoped (list filters by org; get asserts org).
- **Defensive org-membership check:** `application/spectasks` +
  `application/projects` verify the caller Bot is a member of its org
  (`GetBot`), taking identity only from the authenticated invocation. (Already
  enforced at the MCP mount; this is depth. Optional verifier — nil in tests.)
- **`request_spectask_changes` no longer drops the comment:** new
  `SpecTaskWorkflow.RequestChanges` delivers it to the task's agent via the exact
  REST design-review mechanism (`BuildRevisionInstructionPrompt` +
  `sendMessageToSpecTaskAgent`).
- **Notification field coercion:** `attentionTopicPublisher` now maps
  `Title→Subject`, `Description→Body`, `SpecTaskID→ThreadID`, `ID→MessageID`
  (keeping `event_type`/`project_id`/names in `Extra`) — so filter predicates and
  consumers use first-class `streaming.Message` fields.
- **Connection uses existing primitives (no new tools):** extracted
  `EnsureSpecTaskTopic` (shared, idempotent) so a project's topic can be
  pre-created; connection = existing `create_bot`/`subscribe` + optional filter
  processor. Added the `/pm-bot` prompt that drafts a PM bot: discover projects,
  grant the tools, subscribe to the projects' spec-task topics.

## Tests
- Unit + in-process HTTP MCP e2e (`TestProjectDiscoveryOverMCP`) covering
  cross-project/cross-org rejection, org-scoped discovery, membership rejection,
  filter routing of spec-task events, field coercion, comment delivery,
  `EnsureSpecTaskTopic` idempotency, and prompt/tool registration.
- Full `go build .` and `go test ./pkg/org/...` green.
