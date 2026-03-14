# Design: Add Support for Thinking Tags in SpecTask Chat

## Current Architecture

The SpecTask chat rendering path is:

```
SpecTaskDetailContent
  └─ EmbeddedSessionView
       └─ Interaction
            └─ InteractionInference
                 └─ MessageWithToolCalls
                      └─ Markdown (MessageProcessor)
                           └─ ThinkingWidget (when <think> tags found)
```

The `Markdown` component (`frontend/src/components/session/Markdown.tsx`) already handles `<think>...</think>` tags via `processThinkingTags()` and renders them as `ThinkingWidget`. This pipeline is fully wired for SpecTask sessions — the frontend is not the problem.

## Root Cause

Claude Code agents output thinking content through the Anthropic SDK's structured protocol as `ThinkingBlock { type: 'thinking', thinking: string }` with `thinking_delta` streaming events. These flow through the Zed → Helix external agent WebSocket protocol (`websocket_external_agent_sync.go`).

The `handleMessageAdded` function in the external agent sync accumulates raw text via `MessageAccumulator` but has no handling for structured thinking blocks. As a result, thinking content is either:
- Passed through as literal text (showing raw XML tags or structured data), or
- Silently dropped

The internal agent path (`inference_agent.go`) correctly wraps thinking response types in `<think>` tags before storing, but this code path is not used for external Claude Code agents.

## Solution

### Step 1: Determine the Exact Format

First, add logging or inspect real SpecTask agent output to confirm exactly how thinking content arrives in `handleMessageAdded`. The format could be:
- Literal `<thinking>...</thinking>` XML in the text stream
- Structured `thinking_delta` events in the ACP WebSocket protocol
- Some other Zed-specific format

This can be done by adding a temporary debug log in `handleMessageAdded` to print raw message content.

### Step 2: Transform Thinking Content to `<think>` Tags

**Preferred location: Backend (`websocket_external_agent_sync.go`)**

In `handleMessageAdded` (or the ACP message parsing layer), detect thinking content and wrap it in `<think>...</think>` tags before passing to `MessageAccumulator`. This mirrors what `inference_agent.go` already does for internal agents:

```go
// inference_agent.go lines 358, 363-366, 379-385 (reference implementation)
case ResponseTypeThinkingStart:
    write("<think>")
case ResponseTypeThinking:
    write(resp.Message)
case ResponseTypeThinkingEnd:
    write("</think>")
```

Apply equivalent logic in the external agent path when a thinking block is detected.

**Alternative: Frontend format support**

If the thinking content arrives as a different XML format (e.g., `<thinking>` instead of `<think>`), the simpler fix is to extend `MessageProcessor.processThinkingTags()` in `Markdown.tsx` to also recognize the `<thinking>` tag variant. This avoids backend changes but only works if the tags arrive as literal text.

### Preferred Approach

Backend transformation is preferred because:
1. It's consistent with how internal agents work.
2. It keeps rendering logic isolated in the frontend `Markdown` component (single source of truth).
3. It correctly handles streaming thinking deltas as they arrive.

## Key Files

| File | Purpose |
|------|---------|
| `api/pkg/server/websocket_external_agent_sync.go` | External agent message ingestion — add thinking detection here |
| `api/pkg/agent/response.go` | Defines `ResponseTypeThinkingStart/End` (reference) |
| `api/pkg/controller/inference_agent.go` | Reference: how internal agents wrap thinking in `<think>` |
| `frontend/src/components/session/Markdown.tsx` | `processThinkingTags()` — extend if frontend fix needed |
| `frontend/src/components/session/ThinkingWidget.tsx` | Collapsible widget — no changes needed |

## Codebase Patterns

- Helix uses Go backend + React/TypeScript frontend
- WebSocket-based streaming with structured `ResponseEntry[]` for messages
- External agents connect via Zed's Agent Client Protocol (ACP)
- Internal agents use typed `ResponseType*` constants; external agents send raw text
- Frontend `Markdown.tsx` is the single rendering pass for all message content; special tags are extracted during `MessageProcessor` processing before react-markdown renders
- `ThinkingWidget` auto-expands during streaming and collapses on completion — no changes needed there

## Risk / Investigation Note

The exact format of thinking content from Claude Code agents **must be verified** before implementing. A 15-minute debugging session (add a log line to `handleMessageAdded`, run a SpecTask, inspect output) will confirm which approach is correct. Both approaches above are low-risk and localized changes.
