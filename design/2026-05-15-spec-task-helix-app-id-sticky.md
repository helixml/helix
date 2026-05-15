# spec_tasks.helix_app_id is sticky — reassigning the project default doesn't reach existing tasks

Date: 2026-05-15
Status: backlog

## Observation

On the SaaS deployment, the agent that an in-flight spec task uses is whichever
agent app id was on the project at the moment the task was created. Once
`spec_tasks.helix_app_id` is written, changing `projects.default_helix_app_id`
later does NOT propagate to existing tasks — only to tasks created after the
change.

Surfaced while debugging `agent_misconfigured` on SaaS: the user reassigned the
project's default agent, but the failing task kept returning the same 422
because it was still pinned to the original (misconfigured) app id.

## Why it stays this way for now

Auto-reinheriting a task's `helix_app_id` from the project default is the
intuitive expectation only when the task is in a pre-spec state (`backlog`,
`spec_revision`). Once spec generation has run, the design docs, the planning
session, and the queued implementation all carry assumptions about the agent
that produced them — silently swapping the agent under those would be worse
than the current sticky behaviour.

So a proper fix has to:
- limit re-inheritance to pre-spec states (statuses where no LLM call has run
  yet for the task),
- happen at the entry handler (start-planning, approve-specs) so the new
  agent is validated, not at random read paths,
- be conservative about what counts as "still pre-spec" — `task.status ==
  backlog` is the safe set; `spec_revision` deserves its own discussion
  because design docs may already exist.

## Workaround

Direct DB write when needed:

```sql
UPDATE spec_tasks
SET    helix_app_id = '<new-app-id>',
       updated_at   = NOW()
WHERE  id = '<task-id>';
```

Then retry `start-planning`. Validator picks up the new app id on next call.

## Related

PR #2440 fixes the symptom on the originally-reported instance by stopping the
validator from 422'ing subscription-credential agents. This document is the
separate follow-up for the reassignment ergonomics.
