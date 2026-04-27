# Implementation Tasks

- [x] In `api/pkg/store/`, confirm whether a batched "latest interaction per session" query already exists; if not, add `GetLatestInteractionsForSessions(ctx, sessionIDs []string) (map[string]*types.Interaction, error)` using `SELECT DISTINCT ON (session_id) ... ORDER BY session_id, created_at DESC`
- [~] In `api/pkg/server/spec_driven_task_handlers.go` `listTasks` enrichment loop (~line 258-318), after the existing `SessionUpdatedAt` / `SandboxState` block, batch-fetch latest interactions for all `PlanningSessionID`s and populate `task.AgentWorkState` per the state-machine table in `design.md`
- [ ] Apply the same derivation to any other handler that returns `SpecTask`s to the frontend (e.g. single-task GET) — search for callers that read `SandboxState` to find the matching set, and extract a small helper `deriveAgentWorkState(task, sandboxState, latestInteraction)` so both paths stay in sync
- [ ] Add a unit test in `api/pkg/server/` for `deriveAgentWorkState` covering: sandbox absent, sandbox starting, running + waiting interaction, running + complete interaction, running + no interaction, post-implementation status
- [ ] Run `go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/` to confirm the backend still compiles
- [ ] In `frontend/src/components/tasks/TaskCard.tsx` (~line 602-605), change the `useRunningDuration` enabled predicate from `task.status === "implementation"` to `task.agent_work_state === "working"`
- [ ] In `frontend/src/components/tasks/TaskCard.tsx` status row (~line 949-997), branch on `agent_work_state` and `SandboxState` when `task.phase === "implementation"` to choose the label (`In Progress` / `Idle` / `Sandbox stopped` / `Starting…`); fall back to `In Progress` for any unexpected state
- [ ] Run `cd frontend && yarn build` to confirm the frontend still compiles and types line up
- [ ] Manual test in the inner Helix at `http://localhost:8080`: register if needed, create a project, kick off a task, watch the card while the agent streams (label = `In Progress`, timer ticking), then after the agent's response completes (label = `Idle`, no timer); stop the sandbox and confirm the card label switches to `Sandbox stopped`
- [ ] Verify no regressions on cards in other phases (`planning`, `review`, `pull_request`, `completed`) — dot color, label, and absence of timer should match today
- [ ] Commit and push; check Drone CI is green
