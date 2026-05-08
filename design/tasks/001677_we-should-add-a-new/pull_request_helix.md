# PR-ready notifications: link to PR + auto-ack browser notifications + orchestrator specs_pushed fix

## Summary

Three notification-system improvements (the user grouped them):

1. **PR-ready notifications now link directly to the external PR.** When Helix creates a PR on an external provider, fire the existing `pr_ready` attention event immediately from the workflow handler, instead of waiting for the orchestrator's polling loop to detect it. Idempotency-keyed by PR ID, so the orchestrator's later emission collapses cleanly. The notification carries `pr_url` in metadata and the panel renders a small `ExternalLink` icon button next to dismiss — clicking it opens the PR in a new tab. Clicking the notification body still navigates to the in-app task page (so users can inspect the task in Helix without leaving).

2. **Browser notifications mark themselves read on click.** Previously clicking a desktop alert ("Agent finished working", "Spec ready for review", …) navigated you into Helix but left the notification unread, so the badge persisted. Now the click also calls the acknowledge mutation. For grouped notifications (specs_pushed + agent_interaction_completed), both event IDs are acknowledged.

3. **Fix missing `specs_pushed` on orchestrator-driven SpecReview transitions.** The git-push path in `git_http_server.go` already emits `specs_pushed` per design-doc commit. The orchestrator polling loop (`spec_task_orchestrator.go` `handleSpecGeneration`) sets the same status without emitting — so tasks that land in SpecReview via the orchestrator (e.g. cloned tasks, races, retries) silently went unread. Idempotency-keyed by task ID.

## Changes

### Backend
- `api/pkg/server/spec_task_workflow_handlers.go`: after `CreatePullRequest` succeeds in `ensurePullRequestForRepo`, fire `AttentionEventPRReady` via a small helper (`emitPRReadyEvent`) with the PR ID as idempotency qualifier and `pr_id`/`pr_url` in metadata.
- `api/pkg/services/spec_task_orchestrator.go`: emit `AttentionEventSpecsPushed` when `handleSpecGeneration` transitions a task to `SpecReview`.

### Frontend
- `frontend/src/hooks/useAttentionEvents.ts`: extend `AttentionEventType` with the missing `'ci_passed'`/`'ci_failed'` (the backend types existed but the union didn't include them).
- `frontend/src/components/system/GlobalNotifications.tsx`:
  - For `pr_ready` events with `metadata.pr_url`, render an `ExternalLink` icon button next to the dismiss X. Body click → in-app task; icon click → external PR in a new tab. `e.stopPropagation()` keeps the two actions separate.
  - Browser notification click callbacks now `acknowledge()` each event in the group, then navigate in-app.

## Why we didn't add a new event type

An earlier draft of this PR added a separate `pr_opened` type. That was a mistake — `pr_ready` already had the same metadata, near-identical title/description, and same trigger semantics. The orchestrator picks up Helix-created PRs whether we emit a parallel event or not, so the user would have got two notifications for one event. Reverted to a single type.

## Test plan

- [ ] Trigger a PR open via Helix → confirm a `pr_ready` notification appears immediately
- [ ] Click the notification body → in-app task detail page opens
- [ ] Click the small ExternalLink icon → external PR opens in a new tab
- [ ] Confirm the orchestrator's later poll does NOT produce a second notification (idempotency)
- [ ] Enable browser notifications, trigger a non-PR event (e.g. agent finished), click the desktop notification → in-app navigation happens AND the badge clears
- [ ] Trigger an orchestrator-driven SpecReview transition (e.g. clone a task that already has spec docs) → confirm `specs_pushed` notification appears
