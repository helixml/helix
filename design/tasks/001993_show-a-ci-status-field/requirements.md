# Requirements: CI Status on Kanban Cards

## User Story

As a project owner watching the Kanban board, I want to see at a glance whether CI is **running**, **failed**, or **passed** for each task's PR — without making the card any taller (it's already too tall) — and I want the running agent to be **automatically notified** the moment CI finishes so it can react (fix failures, move on after success) without me having to nudge it.

## Acceptance Criteria

### Card display
- [ ] When a task has at least one open or recently-merged PR, its card shows a small CI status indicator (a colored icon, no extra row).
- [ ] States rendered: **running** (yellow spinner / pulse), **passed** (green check), **failed** (red x), **none** (no indicator at all — silent if no CI is configured).
- [ ] Indicator is placed inline with the existing status row (next to phase + duration). **Card height does not increase.**
- [ ] Hovering the indicator shows a tooltip with: provider name, conclusion, and a clickable link to the CI run / checks tab on the provider.
- [ ] If multiple PRs / multiple check suites exist, the indicator reflects the **worst** state (failed > running > passed). Tooltip lists all.

### Backend polling
- [ ] CI status is fetched for any task in the **implementation**, **pull_request**, or recently-merged phases that has at least one PR tracked in `repo_pull_requests`.
- [ ] GitHub: uses the **Combined Status API** + **Check Runs API** for the PR's head SHA.
- [ ] GitLab: uses the **Pipelines API** for the MR's head SHA.
- [ ] Azure DevOps: uses the **Build/Status API** for the PR's head commit.
- [ ] Bitbucket: out of scope for v1 (no indicator, treated as "none"). Add follow-up task.
- [ ] Polling runs on the **same 30s loop** as `pollPullRequests` in `spec_task_orchestrator.go` — no new goroutine.
- [ ] CI state cached on the task row (extends `RepoPR` with `ci_status`, `ci_url`, `ci_updated_at`). Frontend reads it via the existing task list endpoint — no new API call from the card.

### Agent notification
- [ ] When CI transitions from **running → passed** for the task's PR, the running agent receives a chat message: `"CI passed for PR #N (<repo>). <url>"`.
- [ ] When CI transitions from **running → failed**, the running agent receives: `"CI failed for PR #N (<repo>). Check the logs: <url>. Please investigate and push a fix."`.
- [ ] Notifications use the existing `sendChatMessageToExternalAgent()` path with `interrupt: false` (don't interrupt mid-turn; queue if busy).
- [ ] Notifications fire **at most once per (PR, transition)** — tracked via `ci_status` cached on `RepoPR`. No duplicates if the orchestrator restarts.
- [ ] If the agent is offline, the message is persisted as a waiting interaction (existing fallback) and delivered when the agent reconnects.
- [ ] Human is **also** notified via `AttentionService.EmitEvent()` with new event types `ci_passed` / `ci_failed` (so it shows the red dot on the card and optionally goes to Slack).

### Out of scope (follow-ups)
- Webhook-driven CI updates (we poll for v1 — simpler, same code path as PRs, ~30s latency is fine).
- Bitbucket CI status.
- Configurable per-project notification toggles (always on for v1).
- Rendering individual check names — tooltip shows aggregate + link only.
