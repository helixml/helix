# Requirements: Fix Truncated Sentences Before Tool Calls During Streaming

## Problem

During LLM streaming, text content appears truncated in the UI at tool call boundaries. When the assistant finishes a sentence and immediately invokes a tool, the last portion of the text entry is often missing until the full interaction completes. At completion, the content is correct — meaning the data exists but the real-time patches were incomplete.

## User Stories

**US-1:** As a user watching a streaming LLM response, I want to see complete sentences in real time, so that the UI is not confusing or jarring when a tool call begins.

**US-2:** As a developer, I want to understand which layer (Zed or Helix API) is responsible for each gap in the streamed content, so that fixes are applied in the right place.

## Acceptance Criteria

- **AC-1:** Text content in the entry immediately preceding a tool call is fully visible in the frontend before (or simultaneously with) the tool call entry appearing.
- **AC-2:** No regression in streaming throughput — smooth character-by-character streaming must continue to work.
- **AC-3:** The fix works even when multiple tool calls appear in sequence.
- **AC-4:** On interaction completion, content remains correct (no double-send or duplication).
