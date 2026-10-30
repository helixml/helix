# Incident: agent chat ordering and pending questions

**Date:** 2026-10-30
**Status:** Root-caused in code; regression coverage passes; customer deployment not verified
**Affected:** Reported by a customer on a self-managed Helix deployment

## Summary

The report contains two independently reproducible chat-sync defects:

- A delayed assistant entry could be appended after newer entries. The result
  was an older message stranded at the end while newer answers appeared before
  it.
- Helix rendered a pending agent question only when it belonged to the newest
  interaction. A valid question on an older waiting interaction remained in the
  sandbox but was absent from the Helix chat.

The exact customer deployment version and event history are unavailable, so
the report cannot be tied to one production session from live evidence.

## Evidence and root causes

The external-agent bridge uses the ACP transcript entry index as `message_id`.
`MessageAccumulator` kept previously unseen IDs in arrival order. A delayed
entry with ID `2` arriving after IDs `4` and `10` therefore produced `4, 10, 2`
instead of transcript order. Persisted response entries kept the same bad order
across reloads.

Question events were persisted and published against their turn interaction.
The chat then fetched one newest interaction and read `pending_question` only
from that row. Older waiting interactions were consequently invisible even
though their question data was valid.

## Fix

Numeric ACP IDs are inserted by transcript index. Synthetic IDs such as plan
and subagent entries retain their relative arrival order. Restoring persisted
entries normalizes numeric order and rebuilds the flat response content when
needed. Existing completed database rows are not rewritten; normalization
happens when the accumulator is restored.

When entry positions shift, the server emits full replacements at offset zero.
Both frontend patch consumers accept that replacement and clear metadata from
the entry that was displaced, preventing old tool labels from following a new
message.

Pending questions are now queried with `pending_question=true`. The server and
both stores filter for waiting interactions with a non-null question before
counting and pagination. AgentChat polls this focused query independently of
the five-message transcript window, so older active questions appear and
resolved questions disappear on refresh.

## Regression coverage

- Accumulator tests cover delayed numeric IDs, mixed synthetic IDs, and
  out-of-order persisted entries.
- WebSocket patch tests cover shifted IDs and full replacements.
- Frontend tests cover patch metadata clearing, question visibility beyond the
  loaded transcript, action-card ordering, and question removal after refresh.
- Memory-store, handler, and isolated PostgreSQL tests cover pending-question
  filtering before pagination.

The 51 focused frontend tests passed. The combined Go WebSocket suite, handler,
accumulator, and memory-store checks passed. Isolated PostgreSQL pending-filter
and question-lifecycle tests passed. The full frontend TypeScript check remains
blocked by a pre-existing filename casing conflict between
`WorkspaceReviewMessage.tsx` and `workspaceReviewMessage.ts`.

## Scope and verification limits

No customer deployment, Prime environment, browser session, release, or live
API was available for verification on 2026-10-30. No historical data was
deleted or rewritten.
