# Design: Fix Streaming Responses in Comment Bubbles

## Architecture Context

Agent responses to comments stream via WebSocket in `DesignReviewContent.tsx`. The accumulated text is stored in `streamingResponse: { commentId, content }` state and passed as a plain string to:

- `InlineCommentBubble.tsx` — the yellow bubble next to highlighted doc text
- `CommentLogSidebar.tsx` — the sidebar panel listing all comments

Both components render the response text via `InteractionMarkdown` (i.e. the plain `Markdown` component from `components/session/Markdown.tsx`). This component handles `<think>` tags, citations, and document links, but **does not** render `CollapsibleToolCall` widgets.

The correct component is `MessageWithToolCalls` (from `InteractionInference.tsx`), which:
1. If `responseEntries` (typed array) is provided, renders each entry with the right component.
2. Otherwise falls back to `parseToolCallBlocks(text)` — a regex that matches `**Tool Call: <name>**\nStatus: <status>` and renders `CollapsibleToolCall` per block.

## Fix 1: Tool Call Rendering

**Root cause:** `InlineCommentBubble` and `CommentLogSidebar` use `InteractionMarkdown` instead of `MessageWithToolCalls`.

**Fix:** Replace `InteractionMarkdown` with `MessageWithToolCalls` in both components. The regex fallback path handles the flat text format already used by the comment streaming code (which joins entry contents with `\n\n`). No changes needed to `DesignReviewContent.tsx` or the WebSocket streaming path.

`MessageWithToolCalls` requires a `session` prop (for Markdown's RAG citation features) — pass the existing `EMPTY_SESSION` constant. It also requires `getFileURL`, `showBlinker`, and `isStreaming` which are already available.

Note: `MessageWithToolCalls` uses `parseToolCallBlocks` as its fallback. That regex requires `Status:` on the line immediately after the `**Tool Call:**` header. Verify the agent actually produces this format; if not, the regex in `CollapsibleToolCall.tsx` may need adjustment (separate task/investigation).

## Fix 2: ThinkingWidget Size

**Root cause:** `ThinkingWidget` has hardcoded large dimensions:
- Open/streaming state outer box: `minHeight: 120`
- Collapsed inner scroll box: `height: 120`, `py: 2`
- Open inner text box: `maxHeight: 200`

These are fine in the 700px+ wide chat area but oversized in the 300px comment bubble or 400px sidebar.

**Fix:** Add a `compact?: boolean` prop to `ThinkingWidget`. When `compact=true`, apply reduced dimensions:
- Outer box: `minHeight: 60` (instead of 120)
- Collapsed inner box: `height: 60` (instead of 120), reduce `py`
- Open inner text: `maxHeight: 120` (instead of 200)

Pass `compact={true}` wherever `ThinkingWidget` is rendered inside `InteractionMarkdown` in the comment bubble/sidebar context. Since `Markdown.tsx` creates the `ThinkingWidget`, the `compact` prop needs to flow through `Markdown`'s props.

**Alternative (simpler):** Reduce the absolute sizes globally in `ThinkingWidget` — the current `minHeight: 120` is quite tall even in the main chat. Reducing to `minHeight: 80` and inner height to `80` would help everywhere without adding prop complexity. Prefer this if the smaller size looks acceptable in the main chat.

## Files to Change

| File | Change |
|------|--------|
| `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` | Replace `InteractionMarkdown` with `MessageWithToolCalls` |
| `frontend/src/components/spec-tasks/CommentLogSidebar.tsx` | Replace `InteractionMarkdown` with `MessageWithToolCalls` |
| `frontend/src/components/session/ThinkingWidget.tsx` | Add `compact` prop OR reduce global sizes |
| `frontend/src/components/session/Markdown.tsx` | Pass `compact` to `ThinkingWidget` if going the prop route |

## Pattern Found

This codebase uses `MessageWithToolCalls` as the canonical rich-text renderer for agent responses. New places that display agent responses should use it instead of bare `InteractionMarkdown`/`Markdown`. The `EMPTY_SESSION` pattern for skipping RAG features is established in both `InlineCommentBubble` and `CommentLogSidebar` already.
