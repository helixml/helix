# external_websocket_sync: stop echoing a dead turn's request_id after an interrupt

## Summary

After a rapid interrupt→resend cycle, a thread could keep tagging **every**
later event with the **first** turn's `request_id` — for as long as the thread
lived. On meta this made Helix drop a genuine `message_completed` as a stale
duplicate, stranding a finished 182,510-character answer in `state=waiting` and
wedging the session for ~5 minutes.

The subscription keeps `turn_request_id` (the id events are tagged with) and
`last_completed_request_id` (the id most recently reported). The assistant-side
rotation only advances the former when the two are equal — "the turn I'm holding
has already been reported, move on".

The `Stopped` handler has a fallback for turns that produced no assistant
entries: it reports the id from the global `THREAD_REQUEST_MAP` instead of the
stale held one. It wrote **only** `last_completed_request_id`, leaving
`turn_request_id` untouched — so after any fallback the two diverged and that
equality could never hold again.

Walking the incident: turn 1 is interrupted; turn 2 produces 0 chars so nothing
rotates `turn_request_id`, and its `Stopped` takes the fallback and reports turn
2's id; turn 3 then streams 182,510 chars and completes **entirely under turn
1's id**, 23 minutes stale.

Note that Zed's protocol state was never wrong — it correctly answered `noop`
and logged "no turn running". Only the id was wrong.

## Changes

- Keep `turn_request_id` in lockstep with the id actually reported when the
  `Stopped` fallback fires. One line; restores rotation for every later turn.
  The existing protection against a mid-turn follow-up poisoning the in-flight
  id is untouched.
- Add a read-only `turn_status` → `turn_status_response` request/response.
  Helix needs to ask "is a turn running on this thread?" during ordinary
  operation to tell a genuine stale-id completion apart from a replay, and the
  only existing way to get that answer was `cancel_current_turn`, which cancels
  a live turn. Answered from the same `ThreadStatus::Generating` check the
  cancel path already uses.
- E2E: new phase 18 exercising the `turn_status` round trip on an idle thread.

## Testing

- `cargo test -p external_websocket_sync` — 57 passed.
- Dockerized e2e (`run_docker_e2e.sh`) against a binary built from this branch:
  **all 18 phases PASSED**, including "Rapid 3-turn cancel", "Mid-stream
  interrupt" and the new turn_status phase. Running it caught a real deadlock in
  the new phase that compiling never would have.

Release Notes:

- N/A
