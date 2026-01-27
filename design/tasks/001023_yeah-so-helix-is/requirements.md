# Requirements: Chat Widget Streaming Performance

## Problem Statement

Helix chat widget becomes unusably slow when rendering long agent responses. There are two O(n²) problems:

1. **Zed side**: On every token, the entire conversation is serialized and sent to Helix
2. **Helix side**: On every update, the entire session (all interactions) is broadcast to the frontend

## User Stories

1. **As a user**, I want to chat with agents without the UI becoming sluggish during long responses.
2. **As a user**, I want streaming responses to feel smooth and responsive, regardless of conversation length.

## Acceptance Criteria

1. Streaming a response with 10+ tool calls should not cause noticeable UI lag
2. CPU usage during streaming should remain relatively constant (not grow with message length)
3. WebSocket message count should be proportional to conversation structure (turns, tool calls), not token count
4. No visual regression - tool call status, completion state, and markdown rendering still work correctly

## Current Behavior (O(n²))

For each token received:
1. **Zed** emits `EntryUpdated` event
2. **Zed** iterates all entries after last user message, collecting cumulative content
3. **Zed** sends full cumulative content via WebSocket (`MessageAdded` event)
4. **Helix** receives full content, stores it
5. **Helix** `MessageProcessor.process()` runs regex transformations on full text
6. **Helix** `react-markdown` parses and renders full markdown

**Result**: Token N causes O(N) work → Total work = O(N²)

## Why Deltas Don't Work

The Zed display is **not append-only**:
- Parallel tool calls update interleaved content
- Tool calls transition from "pending" to "completed", changing rendered output
- Content can change anywhere, not just at the end

## Proposed Behavior (O(n))

### Zed Side
Send updates only at **boundaries**:
- New entry created (user message, tool call, assistant text chunk)
- Tool call status changes (pending → completed)
- Turn completes (stopped, error, refusal)

### Helix Side
Send only the updated interaction, not full session:
- New WebSocket event type `interaction_update` with single interaction
- Frontend surgically updates React Query cache instead of replacing full session
- Keep full session broadcast for initial load and reconnection

Trade-off: Less smooth character-by-character streaming, but dramatically better responsiveness.

## Constraints

- Must maintain compatibility with existing WebSocket protocol (minimal changes)
- Cannot break Helix sessions that don't use external agents (Zed)
- Acceptable to show chunky updates instead of smooth streaming