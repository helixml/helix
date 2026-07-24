# Spec-task review WIP capacity incident

## Incident

Project `prj_01kx8kxa6q5qzsgcgb1xjxa7y5` launched eight planning sessions on
2026-07-14 and all eight reached `spec_review`. Each session kept its Zed
desktop alive, producing eight independent gopls processes and exhausting host
memory and swap.

The eight tasks were created in 134 ms and their sessions were created in
135 ms. They were started through the helix-org spec-task runtime while the
project had `auto_start_backlog_tasks=false`. The UI displayed the default WIP
limits (planning 3, review 2), but the runtime wrote every task directly to
`queued_spec_generation`.

## Root cause

WIP enforcement existed only in the backlog auto-start path. The orchestrator
treated `queued_spec_generation` as unconditional permission to launch a
desktop, so callers that queued work directly bypassed the check.

The review limit was not read by the backend. Planning slots were released as
tasks entered review, allowing more planners to start even though review tasks
retain the same live desktop.

## Fix

The orchestrator now enforces capacity at the queued-to-running boundary, the
last common path before any planning desktop is launched. It serializes the
check per project, reloads current task state, and changes the selected task to
`spec_generation` before starting the asynchronous desktop launch. That status
change reserves the slot for concurrent queued tasks.

Planning admission also reserves future review capacity. A planning task may
start only when both conditions hold:

- active planning work is below the planning limit;
- active planning plus review work is below the review limit.

The second condition is required because every successful planner becomes a
review task and keeps its desktop alive. With planning 3 and review 2, at most
two planning/review desktops can be admitted before a reviewer advances one.

## Verification

- `go test ./pkg/services -count=1`
- `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
- Regression tests cover backlog admission, direct queued-task admission, a
  full review column, and reservation of the last future review slot.

## Usage telemetry repair

Tasks 230–236 had completed interactions and ACP `message_completed` events
with exact token payloads, but no `usage_metrics` rows. ACP subscription usage
recording was present in the checked-out code but the running API had not
reloaded it before those tasks completed. The API restarted at 06:09 UTC and
recorded task 237's next completion normally at 06:10 UTC.

Eight missing completed-interaction rows (tasks 230–237) were backfilled from
the exact API-log payload whose timestamp matched each interaction's persisted
`completed` timestamp. The repair used the recorder's normal
`acp/<session_id>:<interaction_id>` idempotency key. The authenticated batch
usage endpoint then returned non-zero usage for all eight July 14 tasks,
including 633,245 tokens for task 230.

Task 217 completed on July 11 before Zed emitted an ACP usage payload. Its log
contains no token counts, so it was deliberately not backfilled rather than
inventing a value.
