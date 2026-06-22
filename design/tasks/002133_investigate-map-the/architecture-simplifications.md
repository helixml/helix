# Architecture Simplifications — WebSocket Sync Correlation Surface

Running list of major simplifications spotted while implementing the #2641/#2642/#2643
fixes. Captured for Luke's review (requested 2026-06-22). These are recommendations, NOT
part of the approved fix scope — flagged separately so the fix PR stays focused.

Ranked by leverage (impact ÷ risk).

## 1. Collapse the five correlation maps into a two-phase, DB-backed resolver (highest leverage)
Today routing state lives in five in-memory maps under one mutex — `contextMappings`,
`requestToSessionMapping`, `requestToInteractionMapping`, `interactionToPromptMapping`,
`sessionToCommenterMapping` — plus a separate `streamingContexts` map under its own mutex.
The maps are a caching layer that has accidentally become a source of truth — which is why
a restart (which empties them) loses correlation (#2641/#2642/#2643 all share this cause).

**Correction (Luke, 2026-06-22):** `acp_thread_id` does NOT identify an interaction — it is
1:1 with a *session* only. So the resolver is two phases, and only the first is keyed on the
thread id:

- **Phase 1 — session:** `acp_thread_id -> session`. Already clean and persisted via
  `session.Metadata.ZedThreadID` (`findSessionByZedThreadID`). 1:1, survives restart.
- **Phase 2 — interaction:** `(session, event) -> interaction`. The thread id is no help
  here. Today this is a heuristic stack — `request_id` mapping if present, else
  most-recent-`waiting`, else restart-recovery — and that guesswork is where all three bugs
  live.

The robust Phase 2 uses **explicit turn-state**, not the thread id. Invariant: an ACP thread
runs **one turn at a time** (no new turn until the previous completes, modulo interrupt), so
a session has at most one *active* interaction at any instant. Make that explicit and
persisted:

- the session carries a **current-turn pointer** (e.g. an `active_interaction_id` column),
  set when a turn starts and cleared on completion;
- `resolveInteraction(session)` returns that pointer — no `request_id`, no most-recent-waiting
  scan, no restart guess, and it survives restart because it's a DB column.

`request_id` then isn't needed for routing at all (at most a sanity assertion). The one
wrinkle — Zed's wrapper flushing buffered events from a *previous* turn under a stale id —
is a **content-dedup** concern (the accumulator already drops replays by content/`message_id`),
not a routing concern, so it doesn't argue for keeping `request_id` as the key.

**Simplification:** one `resolveSession(acpThreadID)` + one `resolveInteraction(session)`
(explicit turn pointer) chokepoint, with the in-memory maps demoted to a pure optimisation
that may be empty without correctness loss. This is the "explicit turn-state behind one
swappable chokepoint" direction from the rewrite-strategy doc; done wholesale it deletes most
of the ~130 critical sections.

## 2. State-based idempotency instead of the consumed-sentinel ("") dedup
Duplicate `message_completed` events are currently de-duped by writing `""` into
`requestToInteractionMapping[request_id]` and checking for it later. This only works while
the map is alive and correctly consumed, and it's the source of the "stale request_id
rebind" hazard. The same guarantee falls out of a state check: *if the interaction is
already in a terminal state (Complete/Error/Interrupted), ignore the completion.* Idempotent,
survives restart, no bookkeeping. (Folded into the #2643 fix.)

## 3. `request_id` is literally the interaction ID for Helix-initiated prompts
`NotifyExternalAgentOfNewInteraction` sets `"request_id": interaction.ID`. So
`requestToInteractionMapping[request_id]` is an *identity map* for every Helix-initiated
turn — pure redundancy. The map only carries information for Zed-initiated turns. Routing on
`acp_thread_id` makes it redundant there too.

## 4. Unify the two prompt-delivery paths
The chat path (`NotifyExternalAgentOfNewInteraction`) and the queue path
(`sendQueuedPromptToSession`) build subtly different `chat_message` payloads and register
mappings differently. That divergence *is* #2642 (one set `role:"user"`, the other didn't).
**Simplification:** one function that builds the outbound prompt command, used by both
ingress paths.

## 5. The asymmetry that causes the fragility: `message_added` has no `request_id`, `message_completed` does
This single protocol asymmetry forces all the most-recent-waiting guesswork. Both events
carry `acp_thread_id`. Keying on it removes the asymmetry; `request_id` becomes optional
disambiguation metadata. (This is the crux the re-key addresses.)

## 6. Blunt global reset vs scattered per-thread recovery
On startup `ResetRunningInteractions` marks **every** waiting interaction → `error`/`"Interrupted"`
in one UPDATE, then several reconnect-time branches try to undo that per-thread (e.g.
`getOrCreateStreamingContext:1653-1671`). A blunt global reset fighting local recovery is
hard to reason about. **Simplification options:** (a) mark interactions "needs-reconciliation"
rather than "error", and let the per-thread reconnect handler decide; or (b) when ACP v2's
`state_update` lands, reconcile authoritatively from the live thread state instead of guessing.

## 7. Most of the accumulator/priorEntries machinery exists to absorb a Zed protocol quirk
Zed's `flush_streaming_throttle` resends ALL thread entries on every event, so Helix carries
a `MessageAccumulator` + `priorEntries` content-dedup + flush timers to reassemble deltas.
If Zed sent deltas (or stable per-entry ids that never renumber), much of this disappears.
Worth raising with the Zed/ACP-v2 work rather than the Helix side.

---
Will expand/confirm these as implementation proceeds and present a consolidated
recommendation at the end.
