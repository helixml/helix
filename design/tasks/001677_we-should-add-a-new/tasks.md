# Implementation Tasks

## Backend: New `pr_opened` notification type

- [x] Add `AttentionEventPROpened = "pr_opened"` constant to `api/pkg/types/attention_event.go`
- [x] Add title ("Pull request opened") and description to the switch in `api/pkg/services/attention_service.go`
- [x] Emit `AttentionEventPROpened` in `api/pkg/server/spec_task_workflow_handlers.go` after a PR is successfully created (pass `prID` as qualifier and `pr_url`/`pr_id` in metadata)

## Frontend: Render and navigate `pr_opened` notifications

- [x] Add `'pr_opened'` to the `AttentionEventType` union in `frontend/src/hooks/useAttentionEvents.ts`
- [x] Add `GitPullRequest` icon for `pr_opened` in the `eventIcon()` switch in `GlobalNotifications.tsx`
- [x] Add indigo (`#6366f1`) left-border color for `pr_opened` in the event item styling
- [x] In `handleNavigate()`, handle `pr_opened`: open `event.metadata.pr_url` in a new tab via `window.open(..., '_blank', 'noopener,noreferrer')`; fall back to task-detail navigation if no URL
- [x] Apply the same new-tab external-link behavior to `pr_ready` events that have `pr_url` in metadata
- [x] Add `pr_opened` case to browser notification firing logic — uses generic title/body path; click opens external PR URL when present

## Frontend: Auto-acknowledge on browser notification click

- [x] In `GlobalNotifications.tsx`, update the `onClick` callback passed to `fireNotification()` to also call `acknowledgeMutation.mutate(eventId)` for each event ID in the notification group

## Backend: Fix missing `specs_pushed` notifications on SpecReview transition

- [x] In `spec_task_orchestrator.go` `handleSpecGeneration`, emit `AttentionEventSpecsPushed` via `attentionService.EmitEvent()` after the task status is set to `SpecReview`; use task ID as idempotency qualifier
- [x] Update generated swagger / TS API client to include `pr_opened` enum entry (kept `swagger.json`/`swagger.yaml`/`docs.go`/`api.ts` in sync without running `./stack update_openapi` since Docker isn't available locally)

> Note: the git push path in `git_http_server.go` (line ~1581) already emits `specs_pushed` correctly, and `HandleSpecGenerationComplete` in `spec_driven_task_service.go` is a separate concern (dead code unrelated to either real path) — leaving it untouched in this task.
