# Implementation Tasks

## Gap 0: Jobs developer UI (`/jobs` page)
- [ ] Add `/jobs` route in the Helix frontend (hidden — not in nav bar)
- [ ] Project list view: select existing project or create new one (reuse existing project creation components)
- [ ] Job file editor: three text boxes that write to `helix-specs` branch `job/` folder on save
- [ ] Run management: start button (creates unmanaged session via API), stop button, job detail view embedding `EmbeddedSessionView` + `ExternalAgentDesktopViewer` (only need session ID, no SpecTask object)
- [ ] Cron config: UI to set up cron triggers for scheduled runs
- [ ] API call display: show the equivalent curl/JSON at each interaction point

## Gap 1: Unmanaged session mode (bypass spec task orchestrator)
- [ ] Add `SessionRole string` field (or `Managed bool`) to `SessionChatRequest` in `api/pkg/types/types.go`
- [ ] In `startChatSessionHandler`, set `SessionMetadata.SessionRole = "job"` for unmanaged sessions
- [ ] Add `role` / `exclude_roles` query parameter to `GET /api/v1/sessions` list handler
- [ ] Ensure unmanaged sessions still support desktop streaming and the embedded session viewer

## Gap 2: Cron triggers for external agent sessions
- [ ] Add `AgentType string` field to `CronTrigger` in `api/pkg/types/types.go`
- [ ] Add `ProjectID string` field to `CronTrigger` (so agent inherits project config)
- [ ] Update `trigger_cron.go` execution logic to pass `agent_type` and `project_id` when creating the session
- [ ] Test that cron-triggered Zed sessions get project MCP servers, startup script, and secrets

## Gap 3: Persistent agent state on helix-specs branch (1 job = 1 project)
- [ ] On unmanaged (job) session start, reuse existing machinery to check out `helix-specs` branch into `~/work/helix-specs`
- [ ] Agent works from `~/work/helix-specs/job/` where its state files live (persona, task list, notes, log)
- [ ] On session completion, auto-commit and push any changes back to the `helix-specs` branch
- [ ] Reuse existing git clone/push machinery from spec task service
- [ ] Ensure this is transparent to the agent — Helix handles restore/commit, not the agent

## Gap 4: Cron prompt from file reference
- [ ] Add `InputFile string` field to `CronTrigger` type
- [ ] In cron execution, if `InputFile` is set, read the file from the `helix-specs` worktree (`~/work/helix-specs/job/`)
- [ ] Use file contents as the session prompt; fall back to `Input` if `InputFile` is empty

## Gap 5: Webhook callback on completion
- [ ] Add `CallbackURL string` field to `CronTrigger` and `SessionChatRequest`
- [ ] Create `WebhookNotifier` in `api/pkg/notification/` alongside `EmailNotifier`
- [ ] On session completion, POST to callback URL with `{session_id, status, output}`
- [ ] Register `WebhookNotifier` in the trigger manager

## Gap 6: Session output endpoint
- [ ] Add `GET /api/v1/sessions/{id}/output` handler in `session_handlers.go`
- [ ] Return last interaction's response message, status, and duration
- [ ] Include any file artifacts from the session's filestore folder
- [ ] Single-call retrieval — no need to fetch full session + filter interactions
