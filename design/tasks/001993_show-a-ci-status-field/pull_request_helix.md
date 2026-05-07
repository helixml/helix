# Show CI status on Kanban cards + notify the agent on transitions

## Summary

Adds a small CI status indicator to each Kanban task card (running / passed / failed) and pipes CI transitions through to the running agent + the human attention feed — all without making the card any taller. Polling reuses the existing 30 s PR poll loop in the spec-task orchestrator, so there's no new infrastructure.

Closes the user's request: "Show a CI status field in the Kanban card view. I want to know if CI is running, failed, or passed at a glance. ... we should immediately notify the agent ... we should also be careful not to make the card view any taller."

## Changes

**Backend**
- `RepoPR` extended with `CIStatus` / `CIURL` / `CIUpdatedAt` / `CIHeadSHA` (JSONB — no migration).
- Provider clients gain `GetCIStatus`:
  - **GitHub** combines Combined Status API + Check Runs API, worst-state wins.
  - **GitLab** uses `Pipelines.ListProjectPipelines` filtered by SHA.
  - **Azure DevOps** queries the Build API and matches by `SourceVersion`. **Requires `vso.build` scope**; missing-scope errors degrade gracefully to "none" and emit a one-time warn log so existing PATs don't break the UI.
  - **Bitbucket** stub returns `none` (v2).
- `types.PullRequest.HeadSHA` plumbed through every provider's PR mapper.
- New `GitRepositoryService.GetCIStatus(repoID, prID, headSHA)` dispatcher + `services.NormalizeCIStatus(provider, raw)` helper (one canonical of `running` / `passed` / `failed` / `none`).
- Orchestrator extended: inside the existing `processExternalPullRequestStatus`, every tracked PR now also has its CI status polled. New head SHA → cached state reset (so a stale "passed" doesn't suppress the next failure notification). On `running → passed` and `running → failed`, fires both:
  - A chat message to the running agent via the existing `SpecTaskMessageSender` (offline → waiting interaction queue, same as design-review notifications), `interrupt: false`.
  - A `ci_passed` / `ci_failed` `AttentionEvent` for the human (red dot on the card, optional Slack).
- New `CINotifier` interface + `MessageSenderCINotifier` implementation, wired in `pkg/server/server.go`.

**Frontend**
- New `CIStatusIcon.tsx` component renders a single 14 px MUI icon (animated `Sync` for running, `CheckCircle` for passed, `Cancel` for failed), with a tooltip listing each PR's CI state and clickable through to the provider's CI page. Worst-state aggregation across multiple PRs (failed > running > passed > none).
- Slotted inline into the existing status row in `TaskCard.tsx` between the phase label and the assignee avatar — **no extra row, no height change**.
- Memo comparator updated with a CI signature so cards re-render when CI moves.
- Generated API client regenerated via `./stack update_openapi`.

**Tests**
- `NormalizeCIStatus` unit tests cover every provider's verdicts (including unknown → failed).
- Orchestrator transition tests verify: first-observation silent, `running → passed` notifies once, `running → failed` notifies once with logs link, no-op transitions don't notify, nil notifier doesn't panic.

## Visual verification

Inserted three spec tasks directly into the DB with hand-crafted CI states; the Kanban board renders all three icons inline in the existing status row with no card-height change.

![Kanban cards showing CI status icons](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001993_show-a-ci-status-field/screenshots/01-kanban-with-ci-icons.png)

## Out of scope (follow-ups)

- Webhook-driven CI updates for github.com (latency optimisation; polling at 30 s is fine for v1).
- Bitbucket CI status implementation.
- Per-project notification toggle in settings.
