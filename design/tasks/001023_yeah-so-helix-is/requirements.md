# Requirements: Chat Widget Streaming Performance

## Problem Statement

Helix chat widget becomes unusably slow when rendering long agent responses. The root cause is O(n²) complexity: on every token from the agent, the entire conversation is serialized and sent to Helix, then fully re-processed and re-rendered.

## User Stories

1. **As a user**, I want to chat with agents without the UI becoming sluggish during long responses.
2. **As a user**, I want streaming responses to feel smooth and responsive, regardless of conversation length.

## Acceptance Criteria

1. Streaming a 10,000 token response should not cause noticeable UI lag
2. CPU usage during streaming should remain relatively constant (not grow with message length)
3. Memory allocations during streaming should be incremental, not quadratic
4. No visual regression - streaming cursor, thinking widgets, and markdown rendering still work correctly

## Current Behavior (O(n²))

For each token received:
1. **Zed** iterates all entries after last user message, collecting cumulative content
2. **Zed** sends full cumulative content via WebSocket (`MessageAdded` event)
3. **Helix** receives full content, stores it
4. **Helix** `MessageProcessor.process()` runs regex transformations on full text
5. **Helix** `react-markdown` parses and renders full markdown

**Result**: Token N causes O(N) work → Total work = O(N²)

## Constraints

- Must maintain compatibility with existing WebSocket protocol
- Cannot break Helix sessions that don't use external agents (Zed)
- Must handle reconnection gracefully (may need full state sync on reconnect)