# Spec-task model switch lifecycle

## Failure modes

- Usage attribution resolved the app after a turn completed, so changing a task override could label the old turn with the new or base model.
- A model switch created a new ACP thread but attempted to create another `SpecTaskWorkSession`, conflicting with the task/session 1:1 invariant.
- Canceled threads retained in-memory thread, request, and dispatch mappings. Buffered events and stale request IDs could then bind to the new handoff interaction.
- Long sessions trimmed the newest pending interaction from the inference context. `/sessions/chat` created a Waiting row, returned `no user message found`, and a CLI retry created a duplicate interaction.

## Invariants

- Every external-agent interaction snapshots its effective app, provider, model, runtime, and credential type before dispatch.
- A task model switch keeps the Helix session and work session, replaces the ACP thread, drains all Waiting turns, and removes all routing state owned by the superseded thread.
- The handoff request ID is the handoff interaction ID; canceled request IDs are never reused.
- Context limiting always includes the newest pending user interaction.

## Live verification

Tested against `spt_01kzm4attrr5ds1tfdzymkd9x5` and its connected Zed session:

- Luna and Terra handoffs completed on fresh ACP threads.
- A live Terra `sleep 60` turn was interrupted by a switch and retained a Terra snapshot with `usage_known=false`.
- The immediate Luna turn completed without receiving canceled Terra output.
- The final Terra turn returned `TERRA_FINAL_E2E_OK` on its first `/sessions/chat` attempt.
- No Waiting interactions remained, and every usage row with a configuration snapshot matched the snapshot model and spec-task ID.
