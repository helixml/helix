# Design: CI Status on Kanban Cards

## Approach

Reuse the existing PR-polling loop in `spec_task_orchestrator.go`. For each tracked PR, also fetch CI status from the same provider, cache it on the `RepoPR` row, and detect transitions to fire agent + human notifications. The frontend just reads new fields on the existing task payload.

No new database tables, no new background goroutine, no new API endpoint. The whole feature piggybacks on the PR-tracking machinery that already exists.

## Data model changes

**File:** `api/pkg/types/simple_spec_task.go`

Extend `RepoPR` (it's already a JSONB array on `SpecTask`, so no migration is needed — GORM serializes the new fields automatically):

```go
type RepoPR struct {
    RepositoryID   string
    RepositoryName string
    PRID           string
    PRNumber       int
    PRURL          string
    PRState        string

    // NEW
    CIStatus    string    `json:"ci_status,omitempty"`     // "running" | "passed" | "failed" | "none"
    CIURL       string    `json:"ci_url,omitempty"`        // Link to checks tab / pipeline
    CIUpdatedAt time.Time `json:"ci_updated_at,omitempty"`
    CIHeadSHA   string    `json:"ci_head_sha,omitempty"`   // SHA we last polled — used to detect new commits
}
```

Why these names: `CIStatus` is the normalized verdict (we collapse provider-specific states like `pending`/`queued`/`in_progress` into `running`, and `success`/`neutral` into `passed`, everything else into `failed`). `CIHeadSHA` lets us reset the cached status when the agent pushes a new commit — otherwise we'd notify "CI passed" against stale code.

## Provider client extensions

For each provider, add **one** method that returns a normalized `(status, url)` for a given head SHA / branch. Keep them small.

**File:** `api/pkg/agent/skill/github/client.go`

```go
func (c *Client) GetCIStatus(ctx context.Context, owner, repo, sha string) (status, url string, err error)
```

Implementation: call `Repositories.GetCombinedStatus(sha)` AND `Checks.ListCheckRunsForRef(sha)`, take the worst conclusion across both. URL points at `https://github.com/<owner>/<repo>/pull/<n>/checks` (computed in the orchestrator from PR data, not the client).

**File:** `api/pkg/agent/skill/gitlab/client.go`

```go
func (c *Client) GetCIStatus(ctx context.Context, projectID interface{}, sha string) (status, url string, err error)
```

Uses `Pipelines.ListProjectPipelines` filtered by SHA, take latest.

**File:** `api/pkg/agent/skill/azure_devops/client.go`

```go
func (c *Client) GetCIStatus(ctx context.Context, project, repoID, commitID string) (status, url string, err error)
```

Uses ADO Build API filtered by `sourceVersion`.

**Bitbucket:** skip for v1 — return `("none", "", nil)`. Add TODO.

### Status normalization (shared helper)

`api/pkg/services/ci_status.go` — new file with one tiny helper:

```go
func normalizeCIStatus(provider, raw string) string
```

Maps provider-specific verdicts to the four canonical values. Keeps the orchestrator clean.

## Orchestrator integration

**File:** `api/pkg/services/spec_task_orchestrator.go`

Inside the existing `pollPullRequests(ctx)` (runs every 30s), after we update `PRState` for each PR, also call `pollCIStatus(ctx, task, repo, pr)`:

1. Resolve provider client from `GitProviderConnection` (we already do this for PR polling — reuse).
2. Fetch head SHA of the PR (already in the response from `GetPullRequest` — pass it through).
3. If `pr.CIHeadSHA != newSHA`: reset `CIStatus = ""` (new commit, force re-evaluation, no transition fires until we have a verdict).
4. Call `client.GetCIStatus(...)`.
5. Compare `oldStatus` → `newStatus`:
   - `running → passed`: fire `notifyAgent("CI passed for PR #N. <url>")` + `attention.EmitEvent("ci_passed")`.
   - `running → failed`: fire `notifyAgent("CI failed for PR #N. Check logs: <url>. Please investigate and push a fix.")` + `attention.EmitEvent("ci_failed")`.
   - other transitions: silent update.
6. Persist updated `RepoPR` to the task row.

Idempotency: the cached `CIStatus` itself is the dedup key. If the orchestrator crashes mid-poll, on restart it sees the new status already cached and won't re-notify.

## Agent notification path

Reuse `sendChatMessageToExternalAgent()` from `api/pkg/server/websocket_external_agent_sync.go`. The orchestrator doesn't have direct access to the API server, so we expose it through the existing notifier pattern (`SpecTaskReviewNotifier` is the template).

Add `api/pkg/services/spec_task_ci_notifier.go`:

```go
type CINotifier interface {
    NotifyCIResult(ctx context.Context, task *types.SpecTask, repo, prURL, ciURL, status string) error
}
```

Wired into the orchestrator the same way `SpecTaskReviewNotifier` is. Implementation calls `sendChatMessageToExternalAgent` with `interrupt: false`. If no agent is online, the existing waiting-interaction fallback kicks in.

## Frontend changes

**File:** `frontend/src/components/tasks/TaskCard.tsx`

In the **status row** (around line 964) — already a flex row with phase + assignee — slot a `<CIStatusIcon />` between the phase label and the assignee avatar. The row already exists; we add one ~16px icon, no extra row, no height change.

```tsx
{task.repo_pull_requests?.length > 0 && (
  <CIStatusIcon prs={task.repo_pull_requests} />
)}
```

New component `frontend/src/components/tasks/CIStatusIcon.tsx`:

- Computes worst status across PRs (`failed > running > passed > none`).
- Renders one small icon: `<Sync>` (yellow, animated) / `<CheckCircle>` (green) / `<Cancel>` (red) / nothing.
- Wraps in `<Tooltip>` listing each PR with its CI status + link.
- Click opens the CI URL in a new tab.

**Generated API types** (`frontend/src/api/api.ts`): add `ci_status`, `ci_url`, `ci_updated_at` to the `RepoPR` interface — regenerated via `./stack update_openapi` after the Go struct changes (Helix convention, see CLAUDE.md).

**Memo dependency** (`TaskCard.tsx` line 1599): add `prevProps.task.repo_pull_requests` to the comparator (currently missing — the existing memo wouldn't re-render on PR/CI changes either; pre-existing minor bug, fix in scope since we're touching it).

## Decisions & rationale

- **Polling, not webhooks.** Webhooks are out of scope for v1. The PR-polling loop already runs every 30s and we get CI updates for free on the same cadence with zero new infrastructure. Webhook delivery is also unreliable across self-hosted GitHub Enterprise / GitLab — polling is the more general solution. Add webhooks later as an optimization for github.com.
- **Cache on `RepoPR`, not a new table.** `RepoPR` is already a JSONB array — adding fields is free. A separate `ci_runs` table would be over-engineered for a status indicator.
- **Worst-state aggregation.** A failed check should always win over a passing one — that's the actionable signal. No user has ever wanted "yellow because half-failed".
- **No new height on the card.** Per the user's explicit constraint, the icon goes inline in the existing status row, not a new row. ~16px next to the phase dot.
- **Notify agent with `interrupt: false`.** Don't interrupt mid-turn — let the agent finish what it's doing, then pick up the CI message. Failures are usually not "drop everything" urgent; the agent will see them at the next message-pump tick.
- **Reset `CIStatus` on new SHA.** Without this, an old "passed" status would suppress notification when the next commit fails. `CIHeadSHA` tracking makes the dedup correct under amend/force-push.

## Files touched

| File | Change |
|---|---|
| `api/pkg/types/simple_spec_task.go` | +4 fields on `RepoPR` |
| `api/pkg/agent/skill/github/client.go` | +`GetCIStatus()` |
| `api/pkg/agent/skill/gitlab/client.go` | +`GetCIStatus()` |
| `api/pkg/agent/skill/azure_devops/client.go` | +`GetCIStatus()` |
| `api/pkg/services/ci_status.go` | NEW: `normalizeCIStatus` helper |
| `api/pkg/services/spec_task_ci_notifier.go` | NEW: `CINotifier` (mirrors `SpecTaskReviewNotifier`) |
| `api/pkg/services/spec_task_orchestrator.go` | Extend `pollPullRequests` with CI poll + transition detection |
| `api/pkg/services/attention_service.go` | +`ci_passed` / `ci_failed` event types (string consts) |
| `frontend/src/components/tasks/CIStatusIcon.tsx` | NEW |
| `frontend/src/components/tasks/TaskCard.tsx` | Slot icon into status row, fix memo comparator |
| `frontend/src/api/api.ts` | Regenerated via `./stack update_openapi` |

## OAuth / PAT scopes (reviewer question)

- **GitHub**: Combined Status API (`/repos/{o}/{r}/commits/{sha}/status`) and Check Runs API (`/repos/{o}/{r}/commits/{sha}/check-runs`) are covered by the `repo` scope already required for PR management on private repos, and need no scope on public repos. **No new scope.**
- **GitLab**: Pipelines API is covered by the `api` (or `read_api`) scope already required for MR management. **No new scope.**
- **Azure DevOps**: Build Status / Builds API requires `vso.build` (Build Read). The existing code/PR PATs typically have `vso.code` and may NOT include `vso.build`. **One new scope needed for ADO.** Document in the connection-creation UI hint and in the README; gracefully degrade (treat as `"none"`) if the API returns 401/403 so existing connections don't break.
- **Bitbucket**: Out of scope for v1 — stub returns `"none"`. When implemented later, will need `pullrequest` + `repository` scopes (already required for PR work) — pipelines are covered.

## Notes for the implementer

- The repo has `gomock` (not `testify/mock`) — generate a mock for `CINotifier` with `mockgen` and use it in orchestrator tests. See existing `SpecTaskReviewNotifier` mocks.
- The orchestrator's `pollPullRequests` already resolves the right provider client per repo — don't duplicate that logic.
- Run `cd frontend && yarn build` before committing — this catches type errors that `tsc --noEmit` misses.
- Inner-Helix end-to-end test (per Helix CLAUDE.md): create a spec task, push a branch, open a PR with a workflow, watch the icon transition. Don't ship without doing this.
