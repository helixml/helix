# Requirements: Never Show "PR Could Not Be Created" When Work Landed on an Internal Repo

## Background

Spec task `spt_01kzg669penpt2rg9b40zvpfd6` (a HelixOS GTM bot, `type=bot_run`) shows a
permanent red banner:

> Pull request could not be created - the agent may not have pushed the feature
> branch. Check the agent session for errors.

Nothing failed. The bot's work goes to `chris-outreach`, an **internal** Helix-hosted
repo, where the equivalent of "open a PR" is a server-side fast-forward merge into the
default branch. `tryAutoMergeBotRun` did exactly that at 04:51:34 and set
`merged_to_main=t`. The task was nonetheless moved to `pull_request` at 04:54:05, and
~5 minutes later the orchestrator's timeout stamped the error into
`spec_tasks.metadata.error`, where nothing ever clears it.

Three independent defects produce this:

| # | Defect | Location |
|---|---|---|
| A | The "is a PR even possible?" check asks a *project-wide* question ("does any repo in this project have an ExternalURL?") instead of "can the repo this task's branch actually went to host a PR?". HelixOS has two external repos, so it answers `true` and the blame-the-agent message wins. | `spec_task_orchestrator.go` `projectHasExternalRepo` (~1014) |
| B | The task should never have entered `pull_request`. `approveImplementation` decides the entire workflow branch from `project.DefaultRepoID` (external) — the same project-default assumption `70ab4b28a` already had to fix on the merge side. | `spec_task_workflow_handlers.go` (~143) |
| C | The error is written under `if _, hasErr := task.Metadata["error"]; !hasErr` and **nothing anywhere clears it**. Here it was written 8 minutes *after* `merged_to_main` was already true. | `spec_task_orchestrator.go` `handlePullRequest` (~1060) |

## User Stories

### US-1 — A bot whose work lands on an internal repo is never reported as failed
**As** someone monitoring a HelixOS bot, **I want** the task page to stay clean when the
bot pushed and the branch was fast-forward merged, **so that** I do not go hunting a
failure that does not exist.

Acceptance criteria:
- [ ] A task whose work landed only in internal (Helix-hosted) repos never receives the
      message "Pull request could not be created - the agent may not have pushed the
      feature branch." — not as a fallback, not on timeout, not ever.
- [ ] After the 5-minute PR timeout window elapses on such a task, no error banner is
      present on the task page.
- [ ] The task's `merged_to_main` state reflects that the branch landed.

### US-2 — A genuine "the agent never pushed" failure still reports accurately
**As** a user of a project backed by an external GitHub/GitLab/ADO repo, **I want** the
existing message when the agent really did fail to push, **so that** a real failure is
still surfaced.

Acceptance criteria:
- [ ] External, PR-capable repo + branch never pushed + >5 min in `pull_request` ⇒ the
      original message is written, unchanged.
- [ ] The internal-repo message added by `77726b750` is still used when *no* repo the
      task could touch supports pull requests.

### US-3 — A diverged branch is diagnosed accurately, not generically
**As** an operator, **I want** a branch that cannot fast-forward to say so, **so that**
the remedy is obvious.

Acceptance criteria:
- [ ] When the task's branch exists in an internal repo but is not contained in the
      default branch and cannot fast-forward, the surfaced message names that condition
      and the remedy: the agent must merge the base branch into its feature branch and
      push again.
- [ ] This diagnosis takes precedence over both generic messages — an accurate
      diagnosis is never degraded into a generic one.

### US-4 — A stale error is cleared once the work demonstrably landed
**As** a user looking at a task whose row already says `merged_to_main=t`, **I want** any
previously-written error gone, **so that** the row does not contradict itself.

Acceptance criteria:
- [ ] When the task's branch is contained in the default branch of every repo it has
      work in, `metadata.error` is removed and the change is persisted.
- [ ] Existing rows carrying a stale error self-heal the next time the task is
      evaluated. **No migration and no one-off repair for
      `spt_01kzg669penpt2rg9b40zvpfd6`.**

### US-5 — The decision uses one repo-resolution, not a second divergent one
**As** a maintainer, **I want** a single shared helper answering "where did this task's
work land?", **so that** the merge path and the surfacing path cannot drift apart again.

Acceptance criteria:
- [ ] One helper, in one place, used by both the orchestrator and the workflow handler.
- [ ] No fallback chain, no duplicated second code path, no dead code (CLAUDE.md).
- [ ] The frontend is **not** used to hide the banner; the fix is at the source
      (`spec_tasks.metadata.error` and the status transition).

## Non-Goals

- Retroactively merging the five stranded `chris-outreach` branches. Report only.
- Touching `chris-outreach` contents, branches, or history (live GTM/prospecting memory).
- Removing the original message for external repos.

## Report-Only Investigation

Confirm and write up in the PR (do **not** necessarily fix): a diverged branch in
`tryAutoMergeBotRun` gets a `log.Warn()` and nothing else — no task error, no attention
event, no user-visible signal. If that silent-failure hole still exists in current code,
say so explicitly in the PR description.

## Verification (per CLAUDE.md — earn the confidence)

1. Go unit tests: internal-repo task never gets the PR-failure message; external-repo
   task still does when the branch really was not pushed; a stale error is cleared once
   the branch is merged; a diverged branch gets the diverged message.
2. `cd api && go build ./...` (+ `cd frontend && yarn build` only if the UI is touched —
   it should not need to be).
3. **End-to-end in the inner Helix at `http://localhost:8080`**: a project whose default
   repo is external, plus a task whose branch lands in an internal repo, running as
   `bot_run`. Push the branch, let it auto-merge, wait past the 5-minute timeout window,
   confirm no error banner on the task page.
4. Confirm the genuine-failure case end-to-end too: external-repo task, branch never
   pushed, still gets the original message.
5. Anything not achievable ⇒ write `NOT tested: <what and why>` in the PR. Never write
   "covered by unit tests".

Branch, build, test, push, open a PR against `helixml/helix` main with full PR URLs. Do
not merge it.

## Open Questions

1. **How is `chris-outreach` reachable from the task?** The brief says its
   `git_repositories` row has an empty `project_id`, so it is not in the project's repo
   list. But `ListGitRepositories{ProjectID}` filters through the **`project_repositories`
   junction table**, not that column — and `handleFeatureBranchPush` found the task only
   via `GetProjectsForRepository(repo.ID)`, which reads the same junction. If the
   auto-merge ran (it did), the repo must be attached to the task's project in the
   junction, and the legacy `project_id` column is simply unused. **Please confirm on the
   meta DB:** `SELECT * FROM project_repositories WHERE repository_id =
   'code-chris-outreach-01kvtnnpgaz5eg525et25tx62f';`. If that returns nothing, the
   candidate repo set must be widened (org-scoped repos / app-attached repos) and the
   design changes materially. The design below assumes the junction row exists.
2. **Should a landed internal-only task leave `pull_request`?** `77726b750` deliberately
   does *not* set autonomous runs to `done` (HelixOS bots are long-running, one
   persistent task each; completing it would stop the bot). Current assumption: clear the
   error and record the merge, but do not change status for autonomous runs. Is that the
   wanted behaviour, or should non-autonomous internal-only tasks go to `done`?
3. **Attention event on divergence?** Should the diverged case also emit an attention
   event (making it visible on the board), or is `metadata.error` enough? Assumed: error
   only, and the attention-event gap is reported, not fixed.
4. **E2E timing.** The 5-minute `StatusUpdatedAt` window plus the 1-minute poll makes the
   live test slow but feasible. Assumption: wait it out rather than shortening the
   timeout for the test. Acceptable?
