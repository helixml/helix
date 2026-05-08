# Add pr_opened notification + auto-ack browser notifications + fix orchestrator specs_pushed

## Summary

Three notification-system improvements in one PR (the user grouped them):

1. **New `pr_opened` attention event** — fired when Helix actually creates a PR on an external provider. The notification (in panel and as a browser/desktop alert) opens the external PR in a new tab rather than navigating to the task in Helix. The same external-link behaviour is now also applied to existing `pr_ready` events that carry a `pr_url` in metadata.

2. **Browser notifications now mark themselves read on click.** Previously clicking a desktop alert ("Agent finished working", "Spec ready for review", …) navigated you into Helix but left the notification unread, so the badge persisted. Now the click also calls the acknowledge mutation. For grouped notifications (specs_pushed + agent_interaction_completed), both event IDs are acknowledged.

3. **Fix missing `specs_pushed` notification on orchestrator-driven SpecReview transitions.** The git-push path in `git_http_server.go` already emits `specs_pushed` per design-doc commit. The orchestrator polling loop (`spec_task_orchestrator.go` `handleSpecGeneration`) sets the same status without emitting — so tasks that land in SpecReview via the orchestrator (e.g. cloned tasks, races, retries) silently went unread. Idempotency-keyed by task ID so it can't dupe with itself; the git-push emission uses commit hash so the two won't collide either.

## Changes

### Backend
- `api/pkg/types/attention_event.go`: add `AttentionEventPROpened = "pr_opened"`
- `api/pkg/services/attention_service.go`: add title/description/emoji for `pr_opened`
- `api/pkg/server/spec_task_workflow_handlers.go`: emit `pr_opened` from `ensurePullRequestForRepo` after a successful `CreatePullRequest`. PR id is the idempotency qualifier; metadata carries `pr_id`, `pr_url`, `repo_name`.
- `api/pkg/services/spec_task_orchestrator.go`: emit `specs_pushed` when `handleSpecGeneration` transitions a task to `SpecReview` (idempotency = task ID).
- Swagger / TS API client (`docs.go`, `swagger.json`, `swagger.yaml`, `frontend/src/api/api.ts`): add the new enum entry. Hand-edited because the local environment doesn't have `./stack update_openapi` available — the diff matches what the generator would produce.

### Frontend
- `frontend/src/hooks/useAttentionEvents.ts`: extend `AttentionEventType` union with `'pr_opened'` (also added the missing `'ci_passed'` / `'ci_failed'` while in there — they were in the backend types but absent from the TS union).
- `frontend/src/components/system/GlobalNotifications.tsx`:
  - new `GitPullRequest` icon and indigo (`#6366f1`) accent for `pr_opened`
  - `handleNavigate` opens `metadata.pr_url` in a new tab via `window.open(..., '_blank', 'noopener,noreferrer')` for both `pr_opened` and `pr_ready` (latter falls back to in-app nav if there's no URL)
  - browser notification click callbacks now `acknowledge()` each event in the group, and apply the same external-link behaviour for PR events

## Test plan

- [ ] Trigger a PR open via Helix and confirm a `pr_opened` notification appears
- [ ] Click the panel notification → external PR opens in a new tab; notification marked read
- [ ] Enable browser notifications, trigger a non-PR event (e.g. agent finished), click the desktop notification → in-app navigation happens AND the notification badge clears
- [ ] Trigger an orchestrator-driven SpecReview transition (e.g. clone a task that already has spec docs) → confirm `specs_pushed` notification appears
