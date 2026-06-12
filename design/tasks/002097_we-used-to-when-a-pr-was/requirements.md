# Requirements: Show PR State and CI Status in Spec Task PR Dropdown

## Background

Spec tasks can have one or more associated GitHub/GitLab PRs. The "View Pull
Request(s)" control on the Spec Task Detail page shows a flat list with just
the repo name and PR number. When a PR is closed or merged on the provider,
the dropdown gives no visual indication — the entry stays as plain text and
the user has to click through to the provider to see what's going on.

Previously the task view surfaced PR closed/merged state more prominently;
that signal was lost when the list was simplified. The data is still tracked
on the backend (`RepoPR.PRState` is one of `open|closed|merged`, and
`RepoPR.CIStatus` is one of `running|passed|failed|none|""`), it just isn't
displayed in the dropdown.

## User Stories

### US-1: See PR state at a glance in the dropdown
**As** a developer reviewing a spec task with multiple PRs
**I want** each entry in the PR dropdown to show whether the PR is open, merged, or closed
**So that** I can immediately tell which PRs still need attention without clicking through to GitHub.

### US-2: See test/CI status at a glance in the dropdown
**As** a developer reviewing a spec task with open PRs
**I want** each PR row in the dropdown to show its current CI verdict
**So that** I can spot failing builds and merge-ready PRs without opening each one.

### US-3: Don't hide closed PRs
**As** a developer auditing what happened on a task
**I want** closed PRs to remain visible in the dropdown (clearly labelled "closed")
**So that** I can find the abandoned attempt and reopen, replace, or learn from it.

### US-4: Single-PR layout also shows status
**As** a developer on a single-PR task
**I want** the single "View Pull Request" button (the non-dropdown case) to also reflect PR state and CI status
**So that** the signal is consistent regardless of how many PRs the task has.

## Acceptance Criteria

- [ ] When a spec task is in `pull_request` or `done` status and has 2+ PRs, the
      dropdown menu shows, for each PR row: repo name, `#PR-number`, a state
      badge (`open` / `merged` / `closed`), and a CI status icon with tooltip.
- [ ] State badge colours: `open` = info/blue, `merged` = success/green,
      `closed` = neutral/grey. Match colours already used by `CIStatusIcon`
      where applicable.
- [ ] CI status icon reuses the existing `CIStatusIcon` rendering (same icon
      set and tooltip behaviour) so visual language is consistent with task
      cards.
- [ ] Closed PRs are NOT filtered out of the dropdown. The existing
      `pr.pr_url` filter is preserved (rows without a URL are skipped).
- [ ] Merged/closed PRs in the dropdown are visually de-emphasised
      (e.g. slightly muted text) but still clickable through to the provider.
- [ ] In the single-PR case (1 PR only), the button label/subtext includes
      the PR state and a CI status icon — no behaviour change to the click
      action itself.
- [ ] No backend changes required: PRs already carry `pr_state` and
      `ci_status` on the `repo_pull_requests` array (`TypesRepoPR`).
- [ ] No regression for tasks in earlier statuses (`backlog`, `in_progress`,
      etc.) — the action button area continues to render as it does today.
- [ ] Works in both the `isInline` and full-width button variants of
      `SpecTaskActionButtons`.

## Out of Scope

- Adding a GitHub/GitLab webhook to push PR state changes in real time.
  State today is updated by the orchestrator poll loop; that cadence is
  acceptable for this task.
- Showing per-check CI breakdown (individual check names). The aggregate
  `ci_status` is sufficient.
- Reopening or otherwise mutating a closed PR from the UI.
