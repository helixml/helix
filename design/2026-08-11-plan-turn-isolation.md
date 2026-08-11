# Plan turn isolation

## Failure

ACP plan state survived into later prompts. Zed removed only completed plan
entries at the start of a turn, leaving pending and in-progress entries visible
even when the agent did not publish a plan for the new prompt.

Helix also treated an empty structured plan as if no structured source existed
and fell back to the spec task's `tasks.md` checklist. That fallback restored
the stale plan immediately after Zed cleared it.

## Fix

- Zed clears the complete ACP plan at every turn boundary and emits
  `PlanUpdated`, producing an explicit `{"steps":[]}` sync snapshot.
- Helix treats any per-turn plan source, including an empty snapshot, as
  authoritative over the `tasks.md` fallback.
- A credential-free headless integration test runs two prompts through the
  production WebSocket handlers. The deterministic ACP agent publishes a plan
  in turn one and no plan in turn two. The test asserts that the second turn
  emits and persists an empty plan snapshot.

The test was also run against the pre-fix Zed binary and failed because the
second turn had no empty plan event or reset entry, demonstrating that the
assertion detects the regression.
