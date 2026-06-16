# Design: Reliable Backend Restart of Agent Session Desktop Container

## Current State (verified in `helix` repo, `main`)

| Surface | Frontend call | Backend | Recreates container? |
|---|---|---|---|
| Worker detail page | `useActivateWorker` → `POST /orgs/{org}/workers/{id}/activate` | `activations.Activate` → dispatcher → spawner → `ensureSession` → `EnsureAndSend` → `SendMessage` | **No** — sends a message to the existing session |
| In-chat prompt input | `v1SessionsRestartAgentCreate` → `POST /sessions/{id}/restart-agent` | `restartCrashedAgentThread` | **Yes** — `StopDesktop` → `resumeSessionInternal` (`StartDesktop`) → `ResetCrashedPromptsForSession` |
| Spec-task detail page | `stop-external-agent` + `setTimeout(1000)` + `resume` (in `handleRestartSession`) | two endpoints, orchestrated by the browser | Yes, but logic is in the frontend |

Key files:
- `api/pkg/server/session_handlers.go` — `restartCrashedAgentThread` (~L2339),
  `resumeSessionInternal` (~L1963), `stopExternalAgentSession`,
  `cancelSessionTurn`.
- `api/pkg/org/infrastructure/runtime/helix/sessions.go` — `EnsureAndSend`
  (existing-session → `SendMessage`; this is why activate doesn't recreate).
- `api/pkg/org/infrastructure/runtime/helix/spawner.go` — `ensureSession`.
- `api/pkg/server/helix_org.go` — `orgWorkerRuntime.SessionID(...)` already
  resolves a worker → its persisted session id (via `LoadState`).
- `api/pkg/org/interfaces/server/api/workers.go` — `activateWorker` handler.
- `frontend/src/pages/HelixOrgWorkerDetail.tsx` (~L468 button),
  `frontend/src/services/helixOrgService.ts` (`useActivateWorker`),
  `frontend/src/components/common/RobustPromptInput.tsx` (`handleRestartAgent`),
  `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
  (`handleRestartSession` ~L732).

## Decision: one canonical backend restart primitive

Extract the body of `restartCrashedAgentThread` into a single shared method on
`HelixAPIServer`:

```
func (s *HelixAPIServer) restartSessionContainer(ctx, user, session) (resetCount int, err error)
```

Behaviour (the only definition of "restart" in the system):
1. Validate `session.Metadata.AgentType == "zed_external"` and executor present.
2. `StopDesktop(sessionID)` — best-effort (container may already be gone).
3. `resumeSessionInternal(ctx, user, session)` — recreates the container via
   `StartDesktop`, preserving `ZedThreadID` (context restored) and clearing
   `PausedScreenshotPath`; re-sends `open_thread`.
4. `ResetCrashedPromptsForSession(sessionID)` and kick
   `processAnyPendingPrompt` so reset prompts re-dispatch on the new container.

`restartCrashedAgentThread` (the `/sessions/{id}/restart-agent` handler)
becomes a thin wrapper: auth → resolve session → `restartSessionContainer`.
The in-chat button is unchanged on the wire and keeps working.

## Worker-page restart → new worker-scoped endpoint

The worker page must recreate the container, but it only knows the worker id,
not the session id (and `WorkerDetailDTO` does not expose the session id).
Rather than leak session resolution into the frontend, add a worker-scoped
restart endpoint that resolves the session **in the backend** and delegates:

```
POST /api/v1/orgs/{org}/workers/{id}/restart-agent   (helix-org API)
```

Handler logic:
1. Resolve org + worker (404 if missing).
2. Resolve the worker's current session id (reuse `orgWorkerRuntime.SessionID`
   / `LoadState`).
3. If a session exists → load it and call the shared
   `restartSessionContainer` primitive.
4. If **no** session exists → fall back to the existing activation/start path
   (`Activations.Activate`) so first-time start still works.

The helix-org API package (`api/pkg/org/interfaces/server/api`) reaches Helix
session operations through the in-proc client / a small port. We expose the
restart primitive to it the same way `StopExternalAgent` / `StartSession` are
exposed today on `inProcHelixClient` (add a `RestartSession(ctx, sessionID)`
method that calls `s.server.restartSessionContainer(...)` with a resolved
user). This keeps "complicated restart logic in the backend" and the org layer
free of session internals.

The worker page button switches from `useActivateWorker` to a new
`useRestartWorkerAgent` hook calling the generated client method for the new
endpoint (regenerate the API client via `./stack update_openapi`).

## Spec-task page → call the backend, drop the frontend orchestration

Replace `handleRestartSession`'s `stop-external-agent` + `setTimeout` +
`resume` sequence with a single `v1SessionsRestartAgentCreate(sessionId)` call
(the canonical `/sessions/{id}/restart-agent` endpoint). Removes the forbidden
`setTimeout` and moves all orchestration server-side. Confirmation dialog and
snackbars stay.

## Why not just change `EnsureAndSend` / activation to always recreate?

A normal activation of a **healthy** running worker should send a message, not
destroy its container — that is correct and must be preserved (Out of Scope in
requirements). "Restart" is a distinct, explicit operator intent and deserves
its own endpoint, not a behavioural overload of activate. Hence a dedicated
worker restart endpoint rather than mutating the activation path.

## TDD plan (the "so it doesn't happen again" requirement)

Use the existing `pkg/server` suite pattern (gomock `MockStore` +
`MockExecutor`, `testify/suite`). Tests assert ordering with
`gomock.InOrder`:

- `restartSessionContainer` calls `StopDesktop` **then** `StartDesktop`
  (recreate), not `SendMessage`.
- `ZedThreadID` is preserved across restart; `open_thread` is re-sent.
- `ResetCrashedPromptsForSession` is invoked and pending prompts are kicked.
- `/sessions/{id}/restart-agent` and the worker restart endpoint both reach the
  same primitive (e.g. both drive `StopDesktop`+`StartDesktop` on the mock
  executor).
- Worker restart with **no** session falls back to the activate/start path.
- Auth: non-owner without update access → 403.

Note: `go test ./pkg/server/...` needs CGo for tree-sitter
(`CGO_ENABLED=1`, install `gcc libc6-dev`). Store tests need Postgres, so mock
the store. Push and confirm CI (Drone) green.

## Risks / Gotchas

- `StopDesktop` is best-effort by design — do not fail restart if the container
  is already gone; the workspace volume (threads.db + agent state) persists and
  is remounted on `StartDesktop`.
- `resumeSessionInternal` re-fetches the session after `StartDesktop` to avoid
  clobbering executor-written metadata — keep that.
- Regenerate the OpenAPI client after adding the new endpoint, or the frontend
  hook won't have a typed method.
