# Usage task and agent attribution

## Problem

Session-scoped code-agent API keys already carry the Helix session, project,
and spec-task IDs. The OpenAI and Anthropic proxy handlers replaced that
session ID with a generated response ID (or `n/a`), and `UsageLogger` did not
persist `LLMCall.SessionID` at all. Proxy metrics therefore retained their task
and project, but lost the join needed for session, app, and code-agent runtime
breakdowns.

The Usage page also exposes the task breakdown without exposing the same task
dimension as a filter.

## Design

- Persist `session_id` and the current session app on every proxy usage metric.
  For a session-scoped key, resolve the session at request time and treat its
  `ParentApp` as the app snapshot. This remains correct after an in-place agent
  switch, unlike copying the app onto the longer-lived API key.
- Build all Usage dimensions from shared session and app SQL expressions. The
  expressions prefer immutable metric snapshots, then interaction data. For
  historical rows only, they fall back to the spec task's current planning
  session/app because the original values were never recorded.
- Add a `task_id` API filter using the same identity as the Task breakdown:
  trigger configuration for scheduled tasks, otherwise spec task.
- Apply that same task identity to sandbox compute through the sandbox's spec
  task or owning trigger session.
- Return task filter options and add a Task selector to the Usage filter bar.

## Verification

- Unit-test proxy attribution resolution and `UsageLogger` persistence.
- Store-test direct session snapshots, historical spec-task fallback, runtime
  attribution, and task filtering.
- Test handler query parsing and build the generated API client/frontend.
- Exercise the Usage page against the local Helix stack.

Verified live on the `unmanned-org` Usage page for task
`spt_01kzx26k7nnwwrpjpxjvh4a2fb`: the Task filter narrowed the report to one
session, Agent `OpenCode`, harness `opencode`, and the task's DeepSeek model.
Sandbox compute narrowed from the org total to the task's one running sandbox.
Clearing the filter removed `task_id` from the URL and restored the full report.
