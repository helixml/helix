# Requirements: Late-Joiner Catch-Up for Active Streaming Sessions

## Problem

When a user refreshes the page (or reconnects via WebSocket) while an agent is actively streaming a response, they only receive delta patches from that moment onward. Since delta patches are relative to a previous snapshot the new client never received, the client starts from an empty baseline and cannot reconstruct the correct content. The result is the user sees a partial or empty response until the stream completes and a full `interaction_update` is emitted.

This was introduced by PR #1898 which switched from flat-string updates to per-entry delta patches (`interaction_patch` events with `EntryPatch[]`). Delta patches are efficient for clients that were connected from the start, but broken for late joiners.

## User Story

**As a user who refreshes the page while the agent is streaming a response, I want to immediately see everything the agent has streamed so far** — so that I'm not left staring at a blank response area waiting for the next incremental patch.

## Acceptance Criteria

1. When a user WebSocket client connects to `/api/v1/ws/user?session_id=X` for a session that has an active streaming context, the server immediately sends a full-state snapshot to that client before any subsequent delta patches arrive.

2. The snapshot must include all entries accumulated so far (text and tool call entries), with their full content as of the moment the client connects.

3. Subsequent `interaction_patch` delta events continue to work normally — the snapshot initializes the client's baseline so future deltas apply correctly.

4. Clients that connect at the start of streaming (the normal case) are unaffected — they receive no extra event.

5. The catch-up event is sent only to the newly connecting client, not broadcast via pub/sub to all subscribers.

6. A race condition is avoided: the pub/sub subscription is established before the catch-up snapshot is sent, so no patches are missed between subscription and snapshot.
