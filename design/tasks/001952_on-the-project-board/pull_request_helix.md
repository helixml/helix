# Show real agent activity (Working / Idle) on In Progress task cards

## Summary

The "In Progress" badge with the green dot and ticking timer on a project-board task card was driven purely by `task.status === "implementation"`. That meant the timer kept ticking long after the agent had finished writing code and was idle, sitting in the In Progress column waiting for human approval. This change derives an honest `agent_work_state` (`working` / `idle` / `done`) on the backend and uses it to drive the card label and timer, so a card that says "In Progress" really is in progress.

The column, the column label, and which column a task lives in are unchanged — only the badge on the card itself changes.

## Changes

**Backend** (`api/`):
- `pkg/store/store.go`, `pkg/store/store_interactions.go`, `pkg/store/store_mocks.go`: add `GetLatestInteractionsForSessions` — one batched `SELECT DISTINCT ON (session_id)` to fetch the newest interaction per session, used by the spec-task list handler. Indexed lookup, sub-ms hot.
- `pkg/server/spec_driven_task_handlers.go`: in the existing `listTasks` enrichment loop that already populates `SessionUpdatedAt` and `SandboxState`, batch-fetch latest interactions and call new helper `deriveAgentWorkState(task, latestInteraction)` to populate `task.AgentWorkState` per the state machine:
  - `sandbox=absent / starting` → `""` (UI shows sandbox hint)
  - `sandbox=running, latest interaction=Waiting` → `working`
  - `sandbox=running, latest interaction=Complete/Error/none` → `idle`
  - `task.Status` ∈ {`implementation_review`, `pull_request`, `done`} → `done`
- `pkg/server/spec_driven_task_handlers_test.go`: 9 unit tests covering every branch of the state machine.

**Frontend** (`frontend/`):
- `src/components/tasks/TaskCard.tsx`:
  - The `useRunningDuration` hook is now enabled by `task.agent_work_state === "working"` instead of `task.status === "implementation"`. The 1-second `setInterval` only runs while the agent is actually streaming.
  - New `getImplementationLabel(task)` helper picks the implementation-phase label: `Sandbox stopped` (sandbox absent), `Starting…` / `sandbox_status_message` (sandbox starting), `Idle` (agent idle), or the existing `In Progress` (agent working / unknown). Other phases are unchanged.

## Why this approach

The unimplemented `design/2025-12-22-external-agent-state-reconciliation.md` proposed a new `external_agent_activity` table plus reconciler loop and write hooks across `handleMessageCompleted`, `NotifyExternalAgentOfNewInteraction`, etc. That's the right shape if/when we also need continue-prompt-on-restart, but it's overkill for "show the right label on the card." The `AgentWorkState` Go type and swagger entry already existed (left over from that design); we just needed to populate the field from data we already have. No new tables, no new endpoints, no migrations.

## Performance

Real boards have ~29 tasks in In Progress, polled every 30s. One extra batched query against an indexed column per `listTasks`. Frontend goes from 29 always-on 1-Hz timers to a handful (only the truly-working subset). Net win.

## Testing

- Go unit tests: `CGO_ENABLED=1 go test -run TestDeriveAgentWorkState ./api/pkg/server/` — all 9 cases pass.
- Backend builds clean (`go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/`) and the dev `air` container rebuilt without error.
- Frontend builds clean (`cd frontend && yarn build`).

**Live UI verification not done in the dev sandbox** — the inner Helix instance has zero spec tasks, and creating one needs a full project + repo + agent + streaming run. The outer Helix instance with ~29 in-progress tasks is the natural place to eyeball the change.
