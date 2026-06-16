# Implementation Tasks: Reliable Backend Restart of Agent Session Desktop Container

## Backend — shared restart primitive
- [x] Extract `restartSessionContainer(ctx, user, session) (resetCount int, err error)` on `HelixAPIServer` from the current body of `restartCrashedAgentThread` (StopDesktop → resumeSessionInternal → ResetCrashedPromptsForSession → kick pending prompts).
- [x] Refactor `restartCrashedAgentThread` (`POST /sessions/{id}/restart-agent`) into a thin wrapper: auth → load session → call `restartSessionContainer`.

## Backend — worker-scoped restart endpoint
- [x] Add `RestartSession(ctx, sessionID)` to `inProcHelixClient` (calls the `/sessions/{id}/restart-agent` handler, which delegates to `restartSessionContainer`).
- [x] Add `POST /api/v1/orgs/{org}/workers/{id}/restart-agent` handler in `api/pkg/org/interfaces/server/api/workers.go`: resolve worker → resolve session id (via `WorkerRuntime.State`) → if session exists call restart primitive (new `SessionRestarter` port); if none, fall back to `Activations.Activate`.
- [x] Wire the new `SessionRestarter` dependency (`inProcClient`) through the org api Deps in `helix_org.go`.
- [x] Register the route and add swagger annotations.

## Frontend re-wiring
- [x] `./stack update_openapi` to regenerate the API client (`v1OrgsWorkersRestartAgentCreate`).
- [x] Add `useRestartWorkerAgent` hook in `helixOrgService.ts`; switch `HelixOrgWorkerDetail.tsx` "Restart agent session" button from `useActivateWorker` to it.
- [x] In `SpecTaskDetailContent.tsx` replace `handleRestartSession`'s stop + `setTimeout` + resume with a single `v1SessionsRestartAgentCreate(sessionId)` call (keep dialog + snackbars).

## Tests (TDD)
- [x] Suite test: `restartSessionContainer` calls `StopDesktop` then `StartDesktop` (gomock `InOrder`), not a SendMessage continuation; resets crashed prompts (returns count); kicks pending prompts (`processAnyPendingPrompt`). (`restart_session_container_test.go`)
- [x] Test: `/sessions/{id}/restart-agent` handler auth — 403 when not owner; 400 when not a zed_external session.
- [x] Test: worker restart endpoint with an existing session calls the `SessionRestarter` port (not DispatchManual). (`restart_worker_test.go`)
- [x] Test: worker restart endpoint with no session falls back to `Activations.Activate` (DispatchManual fires).
- [x] Test: worker restart endpoint 404 on unknown worker.

Note: `ZedThreadID` preservation is inherent — `restartSessionContainer`/`resumeSessionInternal` reuse the same session row and never mutate the thread id. It's deliberately not asserted in the unit test because a non-empty `ZedThreadID` makes `resumeSessionInternal` spawn an async `open_thread` goroutine that would call mocks after `ctrl.Finish()` and crash the test binary; covered by E2E instead.

## Verify
- [x] `go build ./...` (api) and new+related suites pass: `restart_session_container_test.go`, `restart_worker_test.go`, `TestActivateWorker*`, `TestExploratorySessionActivationSuite` (`CGO_ENABLED=1`, installed `gcc libc6-dev`).
- [x] Frontend `tsc -b` passes (0 errors); vite transform succeeds (build's only failure is an unrelated EACCES on the root-owned `dist/` bind mount).
- [ ] End-to-end in inner Helix: click each restart button (worker page, in-chat, spec-task) and confirm a new desktop container is created.
- [ ] Push branch, confirm Drone CI green.

## PR descriptions
- [x] `pull_request_helix.md`
