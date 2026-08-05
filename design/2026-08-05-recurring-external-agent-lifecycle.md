# Recurring external-agent task lifecycle

Recurring external-agent sessions are asynchronous. A completed chat turn is
not evidence that the requested task is complete, so their trigger executions
use an explicit lifecycle:

- A running execution is reserved before the sandbox starts.
- The task system prompt requires the agent to call the session-scoped
  `task_completed` MCP tool after all work is finished.
- `task_completed` is idempotent at the persistence boundary and is the only
  path that marks an external-agent execution successful.
- Agent, thread, sandbox-start, and explicit-stop failures mark the linked
  running execution failed.
- A trigger-row lock serializes scheduled and manual starts. If an execution is
  already running, the new fire records a skipped execution with the active
  execution ID and does not create a session or sandbox.

The execution history renders skipped runs distinctly and displays the stored
failure or skip explanation.
