# Requirements: Mark Task as Done Without a Pull Request

## User Stories

**As a project user**, I want to mark a task as "done" without going through the full implementation/PR workflow, so that I can close tasks whose outcome doesn't involve merging code.

Helix tasks are not only used for engineering work that produces a pull request. People use tasks to track research, planning, writing, communication, manual operations, and other knowledge work where the "deliverable" lives outside the repo. The current workflow forces every task through implementation → PR → merge, which is wrong for these cases.

### Use Cases

**Engineering**
- Task was investigated and determined to be a non-issue
- The fix was applied manually or through a different process (e.g., a config change in a dashboard)
- The task is a duplicate of work already completed
- The desired behavior already exists — no change needed
- Bug turned out to be environmental, not a code issue

**Research / investigation**
- Used the agent to dig into a question; the answer is recorded in the spec/chat and no code change follows
- Spike to evaluate an approach — decision was "don't do it"

**Writing & communication**
- Drafted a doc, blog post, email, or announcement in the spec/chat, iterated on it, then copy-pasted the result into Notion / Google Docs / Slack / email
- Wrote release notes or a customer reply that ships outside the repo

**Manual / browser-based actions**
- Used the agent's desktop session to perform a one-off action in a web UI (filing a ticket, updating a third-party dashboard, configuring a SaaS tool)
- Followed a procedure manually after the agent surfaced the steps

**Planning**
- Used the task to produce a design or plan that will be executed across several future tasks — this task itself produces no PR

## Acceptance Criteria

1. A "Mark as Done" action is available in the task card's three-dot menu for tasks that are not already done or archived
2. Clicking "Mark as Done" shows a confirmation dialog (consistent with the existing archive confirmation pattern)
3. After confirmation, the task transitions to `done` status with `CompletedAt` set
4. The task appears in the "Completed" column on the kanban board
5. A completed-without-PR task can be reopened using the existing "Reopen" action
6. The action is available from any pre-done status (backlog, spec_review, implementation, pull_request, etc.)

## Out of Scope

- Adding a reason/comment field for why the task was closed without PR (can be added later)
- Changing the backend API (the `PUT /api/v1/spec-tasks/{taskId}` endpoint already supports direct status updates)
- Modifying the task status enum or adding a new status
