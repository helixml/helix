# Spec tasks: N pull requests per task, each opened from an approved proposal

## Problem

A spec task could ship exactly one PR per repository, and Helix opened it on its
own: on the "Open PR" click (`approveImplementation`), on agent pushes while the
task was in `pull_request`, from orchestrator retries, and (since #3324-era work)
from the "New PR" follow-up path. Agents could not split work into several PRs,
and nothing asked the user before a branch name or PR appeared upstream.

## Model

Every PR a spec task opens starts as a `SpecTaskPRProposal` (table
`spec_task_pr_proposals`) that a user approves. Helix never opens one by itself.

```
pending ─approve─▶ approved ─(branch has commits beyond base)─▶ opened
   │                  │  ▲                                          
   │                  ▼  │ retry                                    
   └─reject─▶ rejected  failed ─reject─▶ rejected
```

- **Agent** calls `propose_pull_request` (helix-session MCP, mounted for every
  agent; identity = the session's owner + `session.Metadata.SpecTaskID`). Head
  defaults to the task branch, base to `TaskTargetBranch`. `list_pull_request_proposals`
  shows state. Re-proposing a pending head updates it in place.
- **User** sees a card at the end of the chat thread (alongside agent
  questions; both scroll with the thread so any number stay reachable), can edit head/base/title/body and add a note, then approves or
  rejects. A `pr_proposal` attention event (Needs Attention / Slack) is raised and
  dismissed on decision. When a proposal needs the user (pending, or failed), the task view
  opens the chat panel once per proposal: it expands a collapsed chat on wide
  screens and switches to the Chat tab on narrow ones. Collapsing it again sticks. Approval requires the approver's GitHub/GitLab OAuth
  connection; the PR is opened with their credentials.
- **Push rights** (pre-receive allow-list, now per repository): `helix-specs`, the
  task branch, plus the head branch of every approved/opened/failed proposal.
  Any other branch is refused, and the hook hint tells the agent to propose it.
  Only *new* branch names need approval; the task branch remains pushable.
- **Opening**: on approval if the head already has commits beyond the base;
  otherwise when the agent pushes it (post-push hook → `OnBranchPushed`), with the
  PR poller's `Reconcile` as a backstop. Title/body are the approved ones plus the
  usual Helix footer; `pull_request*.md` sync only touches legacy PRs.
- **Agent feedback**: every outcome (opened + link, approved-awaiting-push with the
  exact branch to use, failed + error, rejected + note) is enqueued as a
  non-interrupting message. Edits made by the user are spelled out.
- **Task lifecycle**: the first opened PR moves the task to `pull_request`
  (reopening a `done` task). Merges never complete it — see "Completion".
- **Request PR** (was "Open PR" / "New PR") on external repos only asks the agent
  to push and propose; the endpoint answers 202. Internal repos keep the
  server-side merge on Accept.
- `create_spectask_prs` is blocked on spec-task surfaces (it would bypass
  approval); org Bots keep it.

## Completion

A task is done only when someone says so; PR merges never complete it (an agent
often has follow-up work after a merge — deploy, test, fix). This replaces the
"all PRs merged" rule and the branch-merged-into-main rule, both removed.

- **Agent**: `mark_task_complete(summary)` (helix-session MCP) records a
  completion request (`spec_tasks.completion_requested_at` + summary), raises a
  `completion_request` attention event, and shows a card in the chat with **Mark
  done** / **Send back** (note goes to the agent). Refused while a PR proposal is
  pending/approved/failed. Prompts (planning, implementation handoff, Just Do It,
  Request PR) tell the agent not to call it while its PRs are open: people merge
  on GitHub/GitLab/ADO, and the agent tells them which PRs are waiting.
- **PR settled**: the PR poller tells the agent when a tracked PR merges or
  closes (non-interrupting message), listing what is still open and, when none
  are, suggesting `mark_task_complete`.
- **User**: **Mark done** at any time, with a confirmation dialog — a button on
  Pull Request tasks (header and card) and a menu item on any task an agent works
  on (spec generation → pull request), which is also how a task is abandoned.
  `POST /spec-tasks/{id}/completion/decide` (`approve` works without a request;
  `reject` needs one).
- **Org bots**: `complete_spectask` (bot-only; blocked on spec-task surfaces).
- **Auto-approve** (the task's PR auto-approve flag) also completes the task
  immediately when the agent asks.
- **Done** stops the desktop, ends a waiting turn (so auto-wake does not boot it
  again) and drops messages queued before completion. **Starting** a done task's
  agent (Start, restart, upload, or a queued message waking it) reopens the task
  — to `pull_request` if it has PRs, else `implementation` — because a done
  task's desktop is stopped on its next update. Reopen does the same.
- Columns: Implementation = no PR yet; Pull Request = at least one PR (stays
  after merges); Done (was "Merged") = someone said it is finished.

Next step (not built): let the agent merge its own PRs through the provider.

## Auto-approval

Three controls, one rule: **each task's own flag decides**
(`spec_tasks.auto_approve_pull_requests`, approving as
`auto_approve_pull_requests_by`, whose provider credentials push and open PRs).

- **Project setting** `projects.auto_approve_pull_requests` (Board Automation →
  Automations) is the *default for new tasks*: it pre-ticks the box in both
  creation forms and is applied server-side to tasks created by API, bots,
  agents, cron and clone (`SpecTask.InitAutoApprovePullRequests`). Changing it
  does not touch existing tasks.
- **Creation forms** (new-task composer and Kanban form) show "Auto-approve PRs"
  for projects with an external repo; an explicit choice overrides the default.
- **Approval card** "Auto-approve future pull requests for this task" turns it on
  as the approving user (ignored on reject).
- **Task Details → Pull requests** shows the setting and whose credentials it
  uses, and turns it on/off.

An auto-approved proposal runs the same validation, OAuth check and
compare-and-set as a click, is marked `auto_approved`, raises no approval
request, and its outcome (PR link / push now / failed) is the
`propose_pull_request` result. If the approver has no provider connection the
proposal stays pending for a human.

## Verified end to end (2026-10-05)

Inner Helix + real GitHub repo (`lukemarsden/helix-pr-proposals-e2e`), agent side
driven through the agent's own git credentials and MCP endpoint because the inner
stack had no working model access (outer token rejected with "code-agent task has
no API provider selected"): unapproved push refused → first PR proposed/approved
→ PR #1; second slice proposed, renamed in the UI, old name refused, new name
pushed → PR #2 auto-opened; third proposal rejected → push refused; merge #1 →
task stays open; merge #2 → done; Request PR on done → 202 + instruction;
follow-up proposal → push → PR #3 reopens task; pending proposal blocks
completion after #3 merged; rejecting it → done.

Auto-approval, live: project switch on → composer pre-ticked → task created with
auto-approve as its creator → agent proposal opened PR #4 immediately with no
approval request; Details switch off → next proposal pending; approving it with
"auto-approve future" ticked → flag back on → next proposal auto-approved;
Kanban form pre-ticked, unticking created a task with auto-approve off.

Not exercised live: an LLM agent choosing to call the tool (blocked by the model
access issue above).

## Verified end to end: completion (2026-10-07)

Inner Helix, agent side via the helix-session MCP as the session owner:
`mark_task_complete` refused while proposals were outstanding → after clearing
them, request recorded + attention event + card at the end of the chat (open-PR
warning) → Send back with a note → request cleared, note queued to the agent →
second request → Mark done on the card → done, all attention dismissed, waiting
turn interrupted, desktop stopped → Start desktop on the done task → reopened to
`pull_request`, desktop stayed up → header Mark done (dialog) → done → board
shows "Done" → auto-approve on + request → done immediately → card-menu Mark
done on a PR-less task → done → Needs Attention entry opens the task with the
card visible.

Not exercised live: the PR-merged message to the agent (needs a merge on
GitHub; covered by the orchestrator unit test).
