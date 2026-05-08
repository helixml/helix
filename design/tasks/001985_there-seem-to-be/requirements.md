# Requirements: Agent Settings Don't Persist

## Problem

When a user updates an agent configuration at `/orgs/<org>/agent/<app_id>?tab=settings`, the changes appear to save (no error), but on reload the values revert. Reproduces against `app_01kqx5wk13n0ej2av2xrz3rsad` in the `mola` org.

## User Story

As a user editing an agent's settings (System Instructions, model, agent type, advanced parameters like max_tokens / temperature), when I make a change in the Settings tab, **my change must still be there after refreshing the page or navigating away and back**.

## Acceptance Criteria

1. **System Instructions field** — typing into the textarea and waiting for the autosave (≤1s) persists across page reload.
2. **Model / Provider picker** — selecting a different model persists.
3. **Agent Type selector** (helix_agent / zed_external) — switching persists.
4. **External agent config** (resolution, desktop_type, zoom, refresh rate) — changes persist.
5. **Advanced settings** (max_iterations, temperature, max_tokens, top_p, frequency/presence penalty, reasoning_effort, context_limit) — changes persist when shown via `?showAdvanced=true`.
6. **Code agent runtime** (zed_agent / qwen_code / claude_code) and Claude credential mode (subscription vs api_key) — changes persist.
7. After save, the form does **not** silently reset other fields back to their stored values mid-edit (e.g. typing in one field must not blank out another).
8. Network: the `PUT /api/v1/apps/{id}` request body contains the user's edits, the 200 response body reflects the saved state, and the next `GET /api/v1/apps/{id}` returns the same.
9. Backend tool/knowledge validation (lines 1052-1078 of `app_handlers.go`) operates on the **same** struct that gets persisted, with no field divergence between the validated and the saved object.
10. A regression test exists that round-trips an app update and asserts the saved fields match the request.

## Out of Scope

- Skills / tools tabs (separate flow via `onSaveApiTool` etc.)
- Knowledge tab (separate `onUpdateKnowledge` flow)
- Triggers tab
- Permissions tab
- Anything outside the Settings tab on this URL

## Open Questions (resolve during repro)

- Which specific field(s) does the user observe failing? "Invariably don't seem to persist" suggests broad failure, but the answer affects the priority of the candidate root causes below.
- Does the failure happen on first save, or only after subsequent edits in the same session?
