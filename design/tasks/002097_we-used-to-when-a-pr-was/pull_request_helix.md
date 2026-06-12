# feat(frontend): show PR state + CI status in spec task PR dropdown

## Summary

The spec task PR dropdown used to be a flat list of `repo #N` rows — once a
PR was closed or merged on GitHub there was no visual signal in the UI, so
the user had to click through to GitHub to see what had happened.

The backend has been populating `RepoPR.PRState` (`open` / `closed` /
`merged`) and `RepoPR.CIStatus` on every orchestrator poll cycle for a
while now. This PR surfaces both fields in the dropdown and in the
single-PR button on the spec task detail page.

## Changes

- `frontend/src/components/tasks/SpecTaskActionButtons.tsx`
  - Add `PRStateBadge` (small outlined `<Chip>`: open=info, merged=success, closed=default).
  - Import the existing `CIStatusIcon` so each PR row can render its own CI verdict.
  - Extract a `PRMenuItem` and use it in both the inline and full-width multi-PR `<Menu>` branches. Closed PRs are visually muted (`opacity: 0.65`) but stay clickable.
  - In the single-PR variant, render the state badge + CI icon next to / under the button, and switch the button colour: merged → `success`, closed → `inherit`, otherwise unchanged.
  - Extend the local `RepoPR` interface with `ci_status` and `ci_url` so `CIStatusIcon` (which expects `TypesRepoPR`) accepts the entries.
- No backend / API / type-schema changes. No new dependencies.

## Notes for reviewers

- Closed PRs are no longer hidden — the only filter on the list is still `pr.pr_url` (rows with no URL are skipped, same as before).
- All new code paths sit behind the existing `task.status === "pull_request" || task.status === "done"` gate, so action buttons in earlier statuses are unchanged.
- `cd frontend && yarn build` passes locally.

## Test plan

- [ ] Open a spec task with 2+ PRs in mixed states (open + merged + closed). Confirm the dropdown shows the right state chip and CI icon per row, and the closed row is muted.
- [ ] Open a spec task with a single PR. Watch it transition open → merged → confirm button colour switches to green and the badge updates.
- [ ] Confirm tasks in `backlog` / `in_progress` / `planning` / `implementation` render their action button area unchanged.
