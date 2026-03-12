# Requirements: Helix Chat Sidebar Polish

## Problem Statement

The Helix chat sidebar (visible in the frontend when viewing agent sessions) has three critical UX issues that make it unreliable and messy:

1. **Messages often don't get finished** — the assistant response appears truncated, as if the final streaming tokens were lost
2. **Tool calls need to be collapsed** — raw tool call markdown (e.g. `**Tool Call: edit**\nStatus: Completed\n...`) is displayed inline, cluttering the chat
3. **Thread/session switching causes a mess** — when the user switches between sessions or Zed switches threads, the sidebar shows stale content, duplicates, or blank messages

## User Stories

### US-1: Complete Message Delivery
**As a** Helix user viewing the chat sidebar  
**I want** every assistant message to display its full content  
**So that** I can read the complete response without missing the end

**Acceptance Criteria:**
- When a `message_completed` event arrives, the frontend displays the final content from the DB (not stale streaming state)
- The final `flush_streaming_throttle` on the Zed side always sends pending content before `message_completed`
- If the Go API receives `message_completed` but the interaction's `response_message` is shorter than what was last streamed, it logs a warning (data loss indicator)
- No blank flash between streaming and completed states (the `lastKnownMessage` fallback in `useLiveInteraction` works correctly)

### US-2: Collapsed Tool Calls
**As a** Helix user reading an agent response  
**I want** tool call details to be collapsed by default, showing only a summary line  
**So that** I can focus on the actual response text without scrolling past tool output

**Acceptance Criteria:**
- Tool call blocks in the response markdown are rendered as a single collapsed line (e.g. "🔧 edit file.py" or "🔧 Ran terminal command")
- Clicking the collapsed line expands to show full tool call details (status, input, output)
- Multiple consecutive tool calls are grouped visually
- The collapsing works for both live-streaming and completed (historical) messages

### US-3: Clean Session/Thread Switching
**As a** Helix user switching between different sessions or threads  
**I want** the chat sidebar to cleanly show the correct conversation  
**So that** I don't see stale messages, duplicates, or blank content from a previous session

**Acceptance Criteria:**
- When `currentSessionId` changes in the streaming context, all previous streaming state (currentResponses, stepInfos, patchContentRef) is fully cleared
- The `useLiveInteraction` hook correctly resets `lastKnownMessage` when the interaction ID changes
- Switching sessions triggers a fresh data fetch (not just cache read) to avoid showing stale interactions
- No race condition where a late-arriving WebSocket event for session A updates the display while viewing session B

## Scope

### In Scope
- Frontend streaming context cleanup on session switch (`streaming.tsx`)
- `useLiveInteraction` hook hardening for interaction ID transitions
- Tool call markdown detection and collapsible rendering in `InteractionInference.tsx` or `Markdown.tsx`
- Go API: ensure `flushAndClearStreamingContext` writes final content before marking complete
- Zed: verify `flush_streaming_throttle` timing relative to `message_completed` send

### Out of Scope
- Zed-side thread view rendering (tool calls in Zed's own UI — that's upstream)
- Redesigning the overall chat layout or adding new features
- Backend message persistence changes (the `MessageAccumulator` is already correct)
- Mobile-specific layout issues