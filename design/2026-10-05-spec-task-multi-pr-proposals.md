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
- **User** sees a card above the chat composer (same affordance as agent
  questions), can edit head/base/title/body and add a note, then approves or
  rejects. A `pr_proposal` attention event (Needs Attention / Slack) is raised and
  dismissed on decision. Approval requires the approver's GitHub/GitLab OAuth
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
- **Task lifecycle**: an opened PR moves the task to `pull_request` (reopening a
  `done` task). The task is `done` when every tracked PR is merged or closed, at
  least one merged, and no proposal is pending/approved/failed.
- **Request PR** (was "Open PR" / "New PR") on external repos only asks the agent
  to push and propose; the endpoint answers 202. Internal repos keep the
  server-side merge on Accept.
- `create_spectask_prs` is blocked on spec-task surfaces (it would bypass
  approval); org Bots keep it.

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

Not exercised live: an LLM agent choosing to call the tool (blocked by the model
access issue above).
