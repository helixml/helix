# Architecture Simplifications — WebSocket Sync Correlation Surface

Running list of major simplifications spotted while implementing the #2641/#2642/#2643
fixes. Captured for Luke's review (requested 2026-06-22). These are recommendations, NOT
part of the approved fix scope — flagged separately so the fix PR stays focused.

Ranked by leverage (impact ÷ risk).

## 1. Collapse the five correlation maps into one DB-backed resolver (highest leverage)
Today routing state lives in five in-memory maps under one mutex — `contextMappings`,
`requestToSessionMapping`, `requestToInteractionMapping`, `interactionToPromptMapping`,
`sessionToCommenterMapping` — plus a separate `streamingContexts` map under its own mutex.
Every inbound Zed event ultimately needs just two facts: **which session** and **which
interaction**. Both are already derivable from persisted state:
- session ← `acp_thread_id` via `session.Metadata.ZedThreadID` (`findSessionByZedThreadID`)
- interaction ← most-recent-waiting (or restart-recovered) interaction for that session

The maps are a caching layer that has accidentally become a source of truth — which is why
a restart (which empties them) loses correlation (#2641/#2642/#2643 all share this cause).
**Simplification:** one `resolve(acpThreadID) -> (session, interaction)` chokepoint backed by
the DB, with the maps demoted to a pure optimisation that may be empty without any
correctness loss. This is the re-key direction; doing it wholesale (not just for #2643's
path) would delete most of the ~130 critical sections.

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
