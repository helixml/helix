# Durable external-agent cancellation and OpenCode loop bounds

Date: 2026-08-13

## Incident

An OpenCode turn continued for 167 seconds after the Helix UI reported it had
been interrupted. The API logged `Cancelled queued turn before agent dispatch`,
but it had only changed the interaction row; it never sent
`cancel_current_turn` to Zed.

The turn had crossed an API hot reload. Dispatch correlation lived in two
process-local maps. Streaming recovery rebuilt request-to-interaction but not
request-to-session, and cancellation treated the missing second map as proof
that the turn had never been dispatched. `InteractionStateWaiting` could not
resolve the ambiguity because it represented both queued and actively streaming
turns.

The same OpenCode turn made 1,079 model requests. Our generated configuration
set `permission: "allow"`, which also approved OpenCode's `doom_loop` safety
permission after three identical tool calls. DeepSeek V4 Flash then alternated
repeated reads and shell calls without a separate iteration ceiling.

## Cancellation design

Each external-agent interaction now persists:

- the request ID sent to Zed;
- when dispatch crossed the WebSocket queue boundary;
- when cancellation was requested.

The in-memory maps remain routing caches, not lifecycle truth. API startup no
longer marks external-agent Waiting interactions as interrupted because their
runtime is a separate process and can survive the API restart.

Cancellation has three explicit outcomes:

1. A command still behind the readiness gate is removed and atomically marked
   Interrupted locally.
2. Every other Waiting turn is treated as potentially accepted by Zed, even
   when older rows lack dispatch metadata. It records cancellation intent and
   sends `cancel_current_turn` with a deterministic request ID.
3. A known-dispatched turn records cancellation intent, sends `cancel_current_turn`,
   and only transitions after `turn_cancelled` is durably handled. A missing
   connection or acknowledgement returns `pending`, retries in the background,
   and retries with bounded backoff until the interaction is terminal. Reconnect
   recovery sends the persisted cancellation before any chat replay.

Cancellation drains every Waiting interaction in the current generation, with
known-dispatched requests first. This matters when a client retry or reconnect
has placed additional requests behind the active turn.

The Zed thread service also tracks every WebSocket-accepted request as queued,
running, or cancelled. Cancelling a queued request creates a lifecycle tombstone
and acknowledges `cancelled`; dispatch crosses queued-to-running under the same
mutex and suppresses a request whose cancellation won the race. This closes the
protocol gap where Zed previously returned `noop` for an accepted request queued
behind the active turn, then executed it later.

The request-to-session cache is rebuilt alongside request-to-interaction during
stream recovery. A cancellation acknowledgement can also resolve its
interaction directly from the persisted request ID.

Interrupt prompts are deferred while cancellation is pending. Session clearing
now cancels before deleting interactions and fails rather than clearing local
history while the external runtime may still be working.

## OpenCode bounds

The headless OpenCode config now allows ordinary tools and external workspace
paths but explicitly denies `doom_loop`, so the third identical tool call stops
instead of awaiting an unavailable approval UI or continuing automatically.

DeepSeek V4 Flash's OpenCode `build` agent is additionally capped at 30 agentic
steps per turn. OpenCode then forces a text handoff. This bounds non-identical or
alternating loops that exact duplicate detection cannot recognize.
