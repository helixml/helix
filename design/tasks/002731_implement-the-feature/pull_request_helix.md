# Show agent questions in Helix and let users answer them

## Summary

An ACP agent can ask the user a question mid-turn — Claude Code does this with its
`AskUserQuestion` tool, which `claude-agent-acp` turns into an ACP elicitation that
**blocks the turn** until the client answers. Helix never showed those questions: it
rendered only the dead tool-call stub emitted just before the elicitation, and the
interaction sat in `waiting` until someone interrupted it, killing the turn with
`Tool permission request failed: Tool use aborted`.

This adds the Helix half of the loop: the question is recorded, shown inline in the
conversation with all its options, answerable from the UI, and reconciled with the agent.

Pairs with the Zed PR (`ZED_COMMIT` bumped in `sandbox-versions.txt`).

## Changes

**Storage** — new `agent_elicitations` table, authoritative for status, plus a mirror
inline in the interaction's `response_entries` as a new entry type `elicitation` so the
question renders in conversation order and stays in the transcript once answered. Both are
written by the same handler so they cannot drift. The row exists because status races need
a conditional `WHERE status IN (…)` write, authorisation needs an indexed lookup by id, and
"which tasks are blocked on a human" needs an indexed query.

**Sync handlers** (`websocket_external_agent_elicitation.go`) — `elicitation_requested`,
`elicitation_resolved`, `elicitation_resync`, `elicitation_response_ack`.

**A reconnect is never treated as evidence that the agent is gone.** `agent_ready` fires on
every WebSocket reconnect and the commonest cause is the API restarting while the desktop
container, Zed and its `respond_tx` all survive. Instead the agent re-affirms outstanding
questions on a heartbeat and the reaper cancels only ones whose affirmations stopped for
longer than the grace window (60 s, `HELIX_ELICITATION_RESYNC_GRACE_SECONDS`).

**Endpoint** — `POST /sessions/{id}/elicitations/{elicitation_id}/respond` plus a
`GET /sessions/{id}/elicitations`. Session comes from the URL only. `pending → submitting`
is a conditional claim, so two clients answering at once yields one answer and one clean
409; a failed send rolls the claim back.

**Auto-wake gate** — `maybeAutoWake` now skips an interaction with a live question. Waking
it would cancel the question and replace the user's pending answer with a "continue"
prompt.

**Prompt queue** — `processPromptQueue` deferred any non-interrupt prompt while the newest
interaction was `waiting`. A pending question holds that state indefinitely, so a user who
typed a message instead of answering would have had it stranded forever. It now dispatches,
matching Zed (starting a turn cancels outstanding elicitations).

**Notification** — `agent_question` attention event, keyed on the elicitation id so
heartbeats cannot re-notify, dismissed per-event on any terminal status. Deliberately not
suppressed when the user is active in the session.

**Frontend** — generic JSON-Schema form renderer, an answerable card rendered outside the
collapsible activity summary, and the "Skip" control (per the adapter, declining returns
empty answers and the turn continues — it is not an abort).

The parser matches on metadata **shape**, never on literal key names: against the deployed
adapter (0.66.0) the real `_meta` keys are `_askUserQuestionCustomAnswer` and
`_claude/askUserQuestionOption`, not the `claudeCode/*` names originally specified. Answer
folding mirrors `applyAskElicitationResponse` exactly, including custom-answer precedence
and the custom-only "none of the above" case.

Design doc: `design/2026-08-11-agent-questions-elicitation.md`.

## Testing status

- `go build ./pkg/...` — **passes**.
- Frontend schema-parser unit tests written (`elicitationSchema.test.ts`), **not yet run**:
  the frontend's `node_modules` is not installed in this environment and `yarn install`
  did not complete under the machine's load (average 350–500).
- `yarn build` — **NOT run**, same reason.
- Go unit tests for the new handlers — **NOT written yet**.
- Live end-to-end verification in the inner Helix — **NOT done**. This requires a Zed
  binary built with the paired changes (`./stack build-zed`), which did not complete here.

**This has not been exercised end-to-end. Do not merge on the strength of the build alone.**
