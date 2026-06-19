# Requirements: Forensic Map of request_id Routing in WebSocket Sync

## Context

The Helix↔Zed WebSocket sync server routes agent events by correlating `request_id` values. Three live production bugs (#2641, #2642, #2643) are all symptoms of the same root cause: `claude-agent-acp` buffers events and flushes them with a **stale** `request_id`, so Helix routes them to the wrong interaction, an already-complete one, or nothing at all.

The agreed direction is to re-key routing on the stable, persisted `acp_thread_id` instead of the in-memory `request_id`. Before that refactor can start, we need a precise, evidence-backed map of what exists today.

## User Stories

### As an engineer starting the `acp_thread_id` routing refactor:
- I need to know **exactly which maps exist**, what they key/value, every write and read site, and what happens on a miss — so I know what state I must preserve and what I can delete.
- I need a **request_id lifecycle trace** end-to-end so I can identify where the stale-id window opens and close it surgically.
- I need to understand the **dual delivery paths** (#2642) so I can remove the `role:"user"` drop on the Zed side safely.
- I need to understand the **turn-state inference** sites so I can design an explicit state machine to replace them.
- I need a **restart-survival matrix** showing what is preserved vs lost on API restart — this determines which state a thread-id-keyed design would recover for free.

## Acceptance Criteria

1. Every map claim is backed by `file:line`.
2. Every write site and every read site is listed for each map.
3. The not-found / else branch of every map lookup is named explicitly (the bugs live there).
4. The request_id lifecycle is traced for one prompt end-to-end.
5. The dual delivery paths (#2642) are mapped with the exact Zed-side drop point cited.
6. All turn-state inference sites are pinned by file:line.
7. The auto-wake worker's trigger, what it re-sends, the retry cap, and every state transition it performs are documented.
8. `acp_thread_id` availability on inbound events and its DB persistence are confirmed with file:line.
9. 1–3 chokepoint candidate functions are identified.
10. The restart-survival matrix covers all 6 state pieces.
11. A "discrepancies vs prior docs" section flags divergence between the April-28 architecture review and the current code.
12. A "smallest-first refactor seams" section names concrete edit points (no implementation).
