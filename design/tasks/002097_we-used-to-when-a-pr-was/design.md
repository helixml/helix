# Design: Show PR State and CI Status in Spec Task PR Dropdown

## Summary

A small, frontend-only change to `SpecTaskActionButtons.tsx` that enriches the
PR dropdown (and single-PR button) with the PR state (`open` / `merged` /
`closed`) and CI verdict that the backend already tracks on each
`RepoPR`. No API, no schema, no backend service changes.

## Where the data already lives

These fields are already present and populated — verified by reading the
codebase:

- `api/pkg/types/simple_spec_task.go` — `RepoPR` struct:
  - `PRState string` — `"open" | "closed" | "merged"`
  - `CIStatus string` — `"running" | "passed" | "failed" | "none" | ""`
  - `CIURL string` — link to CI run
- `api/pkg/services/spec_task_orchestrator.go:780-834` — poll loop fetches PR
  state via `gitService.GetPullRequest` and updates `PRState` on every cycle.
- `api/pkg/services/spec_task_orchestrator_ci.go` — `pollCIStatusForPR`
  updates `CIStatus` / `CIURL` from the provider.
- Closed PRs are NOT removed from `task.RepoPullRequests`. When all PRs are
  closed (and none merged), the task simply stays in `pull_request` status
  (orchestrator.go:865-870 logs and waits for the user).
- Frontend types: `TypesRepoPR` in `frontend/src/api/api.ts` already exposes
  `pr_state`, `ci_status`, `ci_url`, `ci_updated_at`, `ci_head_sha`.

So the data is there, end-to-end. The dropdown just doesn't render it.

## The change

### File: `frontend/src/components/tasks/SpecTaskActionButtons.tsx`

Today (≈line 737):

```ts
const pullRequests = task.repo_pull_requests?.filter(pr => pr.pr_url) || []
```

Keep this filter as-is. Closed PRs DO have a `pr_url`, so they will be
included — which is what we want.

#### 1. Add a small `PRStateBadge` component (local to this file or a sibling)

```tsx
function PRStateBadge({ state }: { state?: string }) {
  const s = (state || 'open').toLowerCase()
  const cfg = {
    open:   { label: 'open',   color: 'info' as const },
    merged: { label: 'merged', color: 'success' as const },
    closed: { label: 'closed', color: 'default' as const },
  }[s] || { label: s, color: 'default' as const }
  return <Chip label={cfg.label} size="small" color={cfg.color} variant="outlined" />
}
```

#### 2. Reuse the existing CI icon

Import `CIStatusIcon` from `./CIStatusIcon` and render it per-row with a
single-PR array, so its tooltip/colour logic stays the centralised source of
truth (no duplication of CI semantics across components).

```tsx
import CIStatusIcon from './CIStatusIcon'
// ...
<CIStatusIcon prs={[pr]} />
```

#### 3. Replace each `MenuItem` body in the multi-PR menu

Both the `isInline` branch (≈lines 825-838) and the full-width branch
(≈lines 872-886) render the same `<MenuItem>` with `<ListItemText primary=…
secondary=…/>`. Refactor both to:

```tsx
<MenuItem
  key={pr.repository_id || idx}
  component="a"
  href={pr.pr_url}
  target="_blank"
  rel="noopener noreferrer"
  onClick={() => setPrMenuAnchor(null)}
  sx={{ opacity: pr.pr_state === 'closed' ? 0.7 : 1 }}
>
  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
    <ListItemText
      primary={pr.repository_name || `Repository ${idx + 1}`}
      secondary={pr.pr_number ? `#${pr.pr_number}` : undefined}
    />
    <PRStateBadge state={pr.pr_state} />
    <CIStatusIcon prs={[pr]} />
  </Box>
</MenuItem>
```

The `opacity` rule mutes closed PRs without hiding them (US-3).

To avoid duplicating the JSX twice, extract a single `PRMenuItem`
component used by both the inline and full-width menus.

#### 4. Single-PR case (≈lines 753-799)

The existing button renders `"View Pull Request"` or `"PR: <repo>"`. Keep
the label and href, and append a `<CIStatusIcon prs={[pr]}/>` + a small
state chip next to or beneath the label. For the full-width button this can
go in a small row directly under the button; for `isInline` it can sit
adjacent to the `CompactActionButton`.

A minimal version: include the state chip as the button `endIcon` for the
`open|closed` case, and switch the button's `color` from `"secondary"` to
`"success"` when `pr.pr_state === 'merged'` so the merged terminal state
reads clearly.

## Decisions and rationale

- **Frontend-only.** Backend already tracks and persists everything. Adding
  more endpoints or webhook plumbing here would be over-engineering — the
  poll loop is already the source of truth and runs on every orchestrator
  cycle.
- **Reuse `CIStatusIcon`.** It exists, is already used by `TaskCard`, and
  centralises tooltip + icon semantics. Don't reimplement.
- **Don't filter closed PRs.** The user explicitly asked for them to stay
  visible. Today's filter (`pr.pr_url`) does not drop them; we keep that
  filter and add no new one. (We confirmed via grep that no other code path
  silently drops closed PRs from `repo_pull_requests`.)
- **Mute, don't hide.** Closed PRs are de-emphasised with `opacity: 0.7`,
  consistent with how disabled-feeling items are usually rendered in MUI.
- **No new MUI imports of consequence.** `Chip`, `Box`, `ListItemText`,
  `MenuItem` are all already in use in the file.

## Gotchas surfaced during exploration

- `RepoPR.CIStatus` can be `""` when the orchestrator has never observed CI
  for the PR's head SHA. `CIStatusIcon` already handles this gracefully —
  do not special-case it here.
- `RepoPR.PRState` can be empty for PRs created before the polling code
  shipped. Treat empty as `open` in `PRStateBadge` (the orchestrator will
  fill it in on the next poll cycle).
- The dropdown only renders when `task.status` is `pull_request` or `done`
  AND there's at least one PR with a URL. Don't change those gating
  conditions.
- The task type `SpecTaskWithExtras` in `TaskCard.tsx` defines a narrower
  inline shape for `repo_pull_requests` — when adding new fields used in
  the dropdown component, prefer reading from the canonical `TypesRepoPR`
  type to avoid drift.

## Testing

- Manual: in the inner Helix (`http://localhost:8080`), open a spec task
  with at least 2 PRs in different states (one merged, one closed, one
  open) and verify the dropdown shows correct chips + CI icons. Then
  collapse to a single-PR task and verify the single-button variant.
- No automated UI test added — there is no existing UI test harness for
  `SpecTaskActionButtons.tsx`. Component is a small JSX refactor; rely on
  end-to-end visual verification.
- `cd frontend && yarn build` must pass before commit.

## Out of scope

- Webhook-driven real-time PR state updates.
- Reopening closed PRs from the UI.
- Showing per-check CI breakdown.
