# Implementation Tasks

## US-1: API walkthrough document (do first)
- [x] Write a standalone markdown document walking Phil through the full Jobs API workflow
- [x] Show exact API calls (endpoint, method, request/response JSON) for each step: create project, configure MCP/secrets, write job files, start session, stream/poll results, configure cron
- [x] Mark each API as "exists today" or "proposed/new"
- [x] Cover both ad hoc and recurring job scenarios

## Go client updates (`api/pkg/client/`)
- [x] Add `GetSession(ctx, sessionID)` method — `GET /sessions/{id}`
- [x] Add `StopExternalAgent(ctx, sessionID)` method — `DELETE /sessions/{id}/stop-external-agent`
- [x] Add `CreateProjectSecret(ctx, projectID, req)` method — `POST /projects/{id}/secrets`
- [x] Add `ProjectID` field to `SessionFilter` and wire it into `ListSessions` query params
- [x] Fix `ListSessions` query param names (`page`/`page_size` instead of `offset`/`limit`)
- [x] Fix `ListSessions` return type to `*types.PaginatedSessionsList`
- [x] Add `WriteGitFile(ctx, repoID, req)` method — `PUT /git/repositories/{id}/contents`
- [x] Add `ReadGitFile(ctx, repoID, path, branch)` method — `GET /git/repositories/{id}/contents`
- [x] Update `Client` interface with all new methods

## Gap 0: Jobs developer UI (`/jobs` page)
- [x] Add `/jobs` route in the Helix frontend (hidden — not in nav bar, `drawer: false`)
- [x] Project list view: select existing project via dropdown
- [x] Job file editor: three text boxes for `job/persona.md`, `job/tasks.md`, `job/notes.md` on `helix-specs` branch
- [x] Run management: start button (creates `zed_external` session), stop button, desktop viewer + prompt input
- [x] Cron config: Schedule tab with link to project settings for trigger configuration
- [x] API call display: shows curl commands with project ID populated

## Gap 1: Unmanaged session mode (bypass spec task orchestrator)
- [x] Add `SessionRole string` field to `SessionChatRequest` in `api/pkg/types/types.go`
- [x] In `startChatSessionHandler`, set `SessionMetadata.SessionRole = "job"` for unmanaged sessions
- [x] Add `session_role` / `exclude_roles` query parameter to `GET /api/v1/sessions` list handler
- [x] Ensure unmanaged sessions still support desktop streaming and the embedded session viewer

## Gap 2: Cron triggers for external agent sessions
- [x] Add `AgentType string` field to `CronTrigger` in `api/pkg/types/types.go`
- [x] Add `ProjectID string` — cron trigger already has `AppID` which maps to the project
- [x] Update `trigger_cron.go` execution logic to create external agent sessions via `ExternalAgentStarter`
- [x] Wire `ExternalAgentStarter` implementation into cron system via trigger manager
- [ ] Test that cron-triggered Zed sessions get project MCP servers, startup script, and secrets

## Gap 3: Persistent agent state on helix-specs branch (1 job = 1 project)
- [x] On unmanaged (job) session start, reuse existing machinery to check out `helix-specs` branch into `~/work/helix-specs`
- [x] Agent works from `~/work/helix-specs/job/` where its state files live
- [x] On session completion, auto-commit and push any changes back to the `helix-specs` branch (Docker exec in `DeleteDevContainer`)
- [x] Reuse existing git clone/push machinery from spec task service

## Gap 4: Cron prompt from file reference
- [x] Add `InputFile string` field to `CronTrigger` type
- [x] In cron execution, if `InputFile` is set, read the file from the `helix-specs` branch via git helpers
- [x] Use file contents as the session prompt; fall back to `Input` if `InputFile` is empty

## Gap 5: Webhook callback on completion
- [x] Add `CallbackURL string` field to `CronTrigger` and `SessionChatRequest`
- [x] Create webhook sender in `api/pkg/notification/` — `sendWebhook` dispatched from `Notify`
- [x] On session completion, POST to callback URL with `{session_id, status, output}`

## Gap 6: Session output endpoint
- [x] Add `GET /api/v1/sessions/{id}/output` handler in `session_handlers.go`
- [x] Return last interaction's response message, status, and duration
- [x] Single-call retrieval — no need to fetch full session + filter interactions
