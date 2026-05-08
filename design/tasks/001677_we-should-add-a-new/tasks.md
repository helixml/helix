# Implementation Tasks

## Backend: PR-opened notification (reuse existing `pr_ready`)

- [x] Emit `AttentionEventPRReady` immediately from `ensurePullRequestForRepo` after `CreatePullRequest` succeeds, with `prID` as the idempotency qualifier and `pr_url`/`pr_id` in metadata
- [x] Confirmed: orchestrator's later emission collapses with this one (same idempotency key); no duplicate notifications

## Frontend: Add external-PR-link button to pr_ready items (body click stays in-app)

- [x] In `AttentionEventItem`, render an `ExternalLink` icon button next to the dismiss X for `pr_ready` events that carry `metadata.pr_url`. Click `stopPropagation()` + `window.open(prURL, '_blank', 'noopener,noreferrer')`
- [x] Body click on the notification still navigates to the in-app task detail page (original behaviour — overloading the body as an external link removed the "inspect task in Helix" affordance)
- [x] Browser notification click also navigates in-app (a desktop alert can only carry one action)

## Frontend: Auto-acknowledge on browser notification click

- [x] In `GlobalNotifications.tsx`, the `fireNotification()` `onClick` callbacks now `acknowledge()` each event ID in the group (single + grouped paths)

## Backend: Fix missing `specs_pushed` on orchestrator SpecReview transition

- [x] In `spec_task_orchestrator.go` `handleSpecGeneration`, emit `AttentionEventSpecsPushed` after the SpecReview transition (idempotency = task ID)

> Discovery: the original Part 3 problem statement was inaccurate — the git-push path in `git_http_server.go` (line ~1581) already emits `specs_pushed` correctly. Only the orchestrator polling path was missing it. `HandleSpecGenerationComplete` in `spec_driven_task_service.go` is dead code unrelated to either real path; left alone.

> Discovery: the originally-planned new `pr_opened` event type duplicated `pr_ready` (same metadata, same trigger semantics — orchestrator picks up Helix-created PRs anyway). Reverted the new type and instead emit `pr_ready` earlier from the workflow handler so the user gets it immediately rather than at the next poll.
