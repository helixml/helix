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
- [ ] Suite test: `restartSessionContainer` calls `StopDesktop` then `StartDesktop` (gomock `InOrder`), not `SendMessage`; preserves `ZedThreadID`; resets crashed prompts; kicks pending prompts.
- [ ] Test: `/sessions/{id}/restart-agent` reaches the shared primitive.
- [ ] Test: worker restart endpoint with an existing session reaches the shared primitive (drives StopDesktop+StartDesktop on the mock executor).
- [ ] Test: worker restart endpoint with no session falls back to activate/start.
- [ ] Test: auth failure (no update access) → 403 on both endpoints.

## Verify
- [ ] `CGO_ENABLED=1 go test ./api/pkg/server/...` (install `gcc libc6-dev`) and `go build` the affected packages.
- [ ] `cd frontend && yarn build`.
- [ ] End-to-end in inner Helix: click each restart button (worker page, in-chat, spec-task) and confirm a new desktop container is created.
- [ ] Push branch, confirm Drone CI green.
