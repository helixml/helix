# Implementation Tasks

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

## Gap 3: Generic webhook trigger
- [ ] Add `POST /api/v1/apps/{id}/webhook` handler in `app_trigger_handlers.go`
- [ ] Accept `{"prompt": "...", "project_id": "..."}` payload
- [ ] Authenticate via existing API key middleware
- [ ] Create an unmanaged session (role = "job") and return `{"session_id": "..."}`
- [ ] Log execution in `TriggerExecution` table

## Gap 4: Persistent agent working directory (git-backed)
- [ ] Define a "state repo" concept per project — a git repo branch where agent state files live
- [ ] On external agent session start, clone the state repo branch into a known path (e.g., `/workspace/state/`)
- [ ] On session completion, auto-commit and push any changes to the state branch
- [ ] Add `StateRepositoryID` and `StateBranch` fields to `Project` type
- [ ] Wire the clone/push logic into `hydra_executor.go` container setup

## Gap 5: Cron prompt from file reference
- [ ] Add `InputFile string` field to `CronTrigger` type
- [ ] In cron execution, if `InputFile` is set, read the file from the project's primary repo (default branch)
- [ ] Use file contents as the session prompt; fall back to `Input` if `InputFile` is empty

## Gap 6: Webhook callback on completion
- [ ] Add `CallbackURL string` field to `CronTrigger` and `SessionChatRequest`
- [ ] Create `WebhookNotifier` in `api/pkg/notification/` alongside `EmailNotifier`
- [ ] On session completion, POST to callback URL with `{session_id, status, output}`
- [ ] Register `WebhookNotifier` in the trigger manager

## Gap 7: Session output endpoint
- [ ] Add `GET /api/v1/sessions/{id}/output` handler in `session_handlers.go`
- [ ] Return last interaction's response message, status, and duration
- [ ] Include any file artifacts from the session's filestore folder
- [ ] Single-call retrieval — no need to fetch full session + filter interactions
