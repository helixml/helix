# Implementation Tasks

## Backend: New `pr_opened` notification type

- [ ] Add `AttentionEventPROpened = "pr_opened"` constant to `api/pkg/types/attention_event.go`
- [ ] Add title ("Pull request opened") and description to the switch in `api/pkg/services/attention_service.go`
- [ ] Emit `AttentionEventPROpened` in `api/pkg/server/spec_task_workflow_handlers.go` after a PR is successfully created (pass `prID` as qualifier and `pr_url`/`pr_id` in metadata)

## Frontend: Render and navigate `pr_opened` notifications

- [ ] Add `'pr_opened'` to the `AttentionEventType` union in `frontend/src/hooks/useAttentionEvents.ts`
- [ ] Add `GitPullRequest` icon for `pr_opened` in the `eventIcon()` switch in `GlobalNotifications.tsx`
- [ ] Add indigo (`#6366f1`) left-border color for `pr_opened` in the event item styling
- [ ] In `handleNavigate()`, handle `pr_opened`: open `event.metadata.pr_url` in a new tab via `window.open(..., '_blank', 'noopener,noreferrer')`; fall back to task-detail navigation if no URL
- [ ] Apply the same new-tab external-link behavior to `pr_ready` events that have `pr_url` in metadata
- [ ] Add `pr_opened` case to browser notification firing logic with appropriate title/body; set its click callback to open the external PR URL (not navigate within Helix)

## Frontend: Auto-acknowledge on browser notification click

- [ ] In `GlobalNotifications.tsx`, update the `onClick` callback passed to `fireNotification()` to also call `acknowledgeMutation.mutate(eventId)` for each event ID in the notification group

## Backend: Fix missing `specs_pushed` notifications on SpecReview transition

- [ ] In `spec_task_orchestrator.go` `handleSpecGeneration`, emit `AttentionEventSpecsPushed` via `attentionService.EmitEvent()` after the task status is set to `SpecReview` (~line 557); use task ID as idempotency qualifier
- [ ] In `git_http_server.go`, emit `AttentionEventSpecsPushed` after the task status is set to `SpecReview` on git push (~line 1361); use task ID as idempotency qualifier
- [ ] Delete (or wire up) the dead `HandleSpecGenerationComplete` function in `spec_driven_task_service.go`
