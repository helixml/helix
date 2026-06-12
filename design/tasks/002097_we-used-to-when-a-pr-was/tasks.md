# Implementation Tasks: Show PR State and CI Status in Spec Task PR Dropdown

- [~] In `frontend/src/components/tasks/SpecTaskActionButtons.tsx`, add a small `PRStateBadge` helper that maps `pr_state` (`open` / `merged` / `closed`) to an MUI `<Chip>` (info / success / default, outlined, size="small"). Treat empty `pr_state` as `open`. Also extend the local `RepoPR` interface with `ci_status?: string` and `ci_url?: string` so `CIStatusIcon` can consume the entries.
- [ ] Import `CIStatusIcon` from `./CIStatusIcon` in the same file so each PR row can render its own CI verdict via `<CIStatusIcon prs={[pr]} />`.
- [ ] Extract a `PRMenuItem` subcomponent inside the file that renders a single `<MenuItem>` containing repo name, `#PR-number`, `<PRStateBadge>`, and `<CIStatusIcon>`, with `opacity: 0.7` when `pr.pr_state === 'closed'`. Open the `pr.pr_url` on click in a new tab (mirror existing behaviour).
- [ ] Replace the inlined `<MenuItem>` JSX in both the `isInline` and full-width branches of the multi-PR dropdown (≈lines 825-838 and 872-886) with `<PRMenuItem pr={pr} idx={idx} onSelect={() => setPrMenuAnchor(null)} />`.
- [ ] Update the single-PR button (≈lines 753-799) to render a `<PRStateBadge>` and `<CIStatusIcon prs={[pr]} />` adjacent to the existing label, and switch button `color` from `"secondary"` to `"success"` when `pr.pr_state === 'merged'`. Keep the existing href / target / rel behaviour untouched.
- [ ] Verify the filter on line ≈737 (`task.repo_pull_requests?.filter(pr => pr.pr_url)`) is preserved — do NOT add a state-based filter. Closed PRs must remain in the list.
- [ ] Verify no other component drops closed PRs: grep for `repo_pull_requests` in `frontend/src/` and confirm no `.filter(pr => pr.pr_state !== 'closed')` (or similar) was added since this spec was written.
- [ ] Run `cd frontend && yarn build` to confirm types and bundling pass.
- [ ] Manual test in the inner Helix at `http://localhost:8080`: register / log in, find or create a spec task with at least one merged PR and one closed PR. Confirm the dropdown shows correct chips and CI icons, and that the closed PR row is muted but clickable.
- [ ] Manual test the single-PR variant: pick a task with exactly one PR (open, then merged) and confirm the button reflects state and CI icon correctly.
- [ ] Confirm tasks in `backlog` / `in_progress` / earlier statuses render unchanged (no regression in action button area).
- [ ] Commit with conventional-commit format (e.g. `feat(frontend): show PR state and CI status in spec task PR dropdown`) and open a PR.
