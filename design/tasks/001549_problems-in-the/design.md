# Design: Fix Streaming Responses in Comment Bubbles + Migrate response_message Consumers

## Scope

This task covers three things in one pass:
1. Fix tool call rendering in comment bubbles/sidebar
2. Fix oversized ThinkingWidget in comment context
3. Migrate all `response_message` consumers to `response_entries` and stop sending the redundant flat string over the wire

## Architecture Context

The Go server (`websocket_external_agent_sync.go`) sets **both** fields on interaction completion:
- `ResponseMessage` — flat concatenated string of all entry content (legacy)
- `ResponseEntries` — ordered JSON array of `{type, content, tool_name, tool_status}` entries (new)

Both are currently saved to DB and serialised into every WebSocket event (`interaction_update`, `session_update`). The goal is to make `ResponseEntries` the sole source of truth and stop sending/setting `ResponseMessage` for new interactions.

---

## Fix 1: Tool Call Rendering in Comments

**Root cause:** `InlineCommentBubble.tsx` and `CommentLogSidebar.tsx` use plain `InteractionMarkdown` (`Markdown` component), which doesn't render `CollapsibleToolCall` widgets.

**Fix:** Replace with `MessageWithToolCalls` (from `InteractionInference.tsx`), which uses structured `responseEntries` when provided, or falls back to regex-parsing the flat text for `**Tool Call: <name>**\nStatus: <status>` blocks.

**Streaming path — pass structured entries:** `DesignReviewContent.tsx` currently tracks only flat strings in `entryContents: string[]`, losing type info. It should instead track `ResponseEntry[]` (same shape as the frontend type: `{type, content, tool_name, tool_status}`). The `interaction_patch` event already carries `ep.type`, `ep.tool_name`, and `ep.tool_status` alongside the patch — these just need to be stored and forwarded. The `streamingResponse` state should become `{ commentId, entries: ResponseEntry[] }` so `InlineCommentBubble` can pass them directly to `MessageWithToolCalls`.

**Completion path — read from entries:** In the `interaction_update` and `session_update` handlers in `DesignReviewContent.tsx`, read `interaction.response_entries` (parsed JSON) instead of `interaction.response_message`.

**Persisted display (`comment.agent_response`):** This DB field currently stores the flat text (set in `spec_task_design_review_handlers.go` from `interaction.ResponseMessage`). After migration, `ResponseMessage` will be empty; the handler should reconstruct text from `ResponseEntries` by joining `type=text` entries. For rendering, `MessageWithToolCalls` with the regex fallback will still work since the persisted text is produced from entries anyway — but ideally the handler saves a new `comment.agent_response_entries` JSON field alongside `agent_response` so the sidebar can render tool calls in completed (non-streaming) state too. Whether to add this DB field is at the implementer's discretion; the regression risk of not doing it is that completed tool call blocks show as bold text in the sidebar (same bug we already have), so worth doing.

---

## Fix 2: ThinkingWidget Size

**Root cause:** `ThinkingWidget.tsx` has hardcoded dimensions designed for the wide main chat: outer `minHeight: 120`, collapsed inner `height: 120`, open inner `maxHeight: 200`.

**Fix:** Add a `compact?: boolean` prop. When true: outer `minHeight: 60`, inner collapsed `height: 60`, inner open `maxHeight: 120`. Thread the prop through `Markdown.tsx` (which instantiates `ThinkingWidget`) as `compactThinking?: boolean`. Pass it from `InlineCommentBubble` and `CommentLogSidebar` when calling `MessageWithToolCalls` → `Markdown`.

**Simpler alternative:** Just reduce the global defaults (120→80) if the smaller size looks fine in the main chat. Avoid if main chat looks cramped.

---

## Fix 3: Stop Sending response_message — Backend Consumer Migration

### Consumers that must be migrated before response_message can be dropped

**A. MCP server (`api/pkg/session/mcp_server.go`)**

Lines 815–817, 911–913, 983–985 read `ResponseMessage` to build conversation history for external LLM agents (Zed). These are the `get_turn`, `get_turns`, and `get_interaction` MCP endpoints.

Migration: when `ResponseEntries` is non-empty, reconstruct text by iterating entries and joining all `type == "text"` content fields. Keep fallback to `ResponseMessage` for old interactions.

**B. Summary service (`api/pkg/server/summary_service.go`)**

Lines 51, 454–456 read `ResponseMessage` to build summarisation prompts. Same migration: reconstruct from entries with fallback.

**C. Design review handlers (`api/pkg/server/spec_task_design_review_handlers.go`)**

Lines 902, 976–977, 1092–1093 set `comment.AgentResponse = interaction.ResponseMessage`. After migration, reconstruct text from `ResponseEntries`.

**D. Triggers (Slack, Teams, Azure DevOps, Crisp, etc.)**

Each trigger bot reads `ResponseMessage` to send the final reply. Same migration pattern.

**E. Other minor consumers**

`controller_external_agent.go`, `sessions.go`, `question_set_handlers.go`, `session_toc_handlers.go`, `scheduler/workload.go`, CLI spectask — each reads `ResponseMessage` for various purposes (displaying output, checking non-empty, etc.). Audit each and apply the same text-reconstruction fallback.

### Stopping population of ResponseMessage

Once all consumers are migrated to read from `ResponseEntries` with fallback for old interactions, stop setting `targetInteraction.ResponseMessage` in `websocket_external_agent_sync.go` (line 1033). The field should remain in the DB schema and Go struct for backwards compat (old rows), but no longer be written for new interactions.

### Wire protocol

`response_message` is currently serialised with no `omitempty` tag. Two options:
- Add `omitempty` to the `json` tag — it will be absent from the wire when empty. Simplest.
- Create a wire DTO struct that excludes the field. More work, unnecessary.

Prefer `omitempty`. After stopping population, the field will be `""` for all new interactions and absent from JSON, reducing wire payload size proportionally to the interaction text length.

---

## Files to Change

### Frontend

| File | Change |
|------|--------|
| `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` | Replace `InteractionMarkdown` with `MessageWithToolCalls`; accept `streamingEntries?: ResponseEntry[]` prop |
| `frontend/src/components/spec-tasks/CommentLogSidebar.tsx` | Replace `InteractionMarkdown` with `MessageWithToolCalls`; accept persisted `agent_response_entries` if available |
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | Track `ResponseEntry[]` during streaming (not just strings); pass entries to comment bubbles; read from `response_entries` on completion events |
| `frontend/src/components/session/ThinkingWidget.tsx` | Add `compact` prop with halved dimensions |
| `frontend/src/components/session/Markdown.tsx` | Accept and forward `compactThinking` prop to `ThinkingWidget` |

### Backend (Go)

| File | Change |
|------|--------|
| `api/pkg/session/mcp_server.go` | Reconstruct text from `ResponseEntries` with fallback to `ResponseMessage` |
| `api/pkg/server/summary_service.go` | Same reconstruction pattern |
| `api/pkg/server/spec_task_design_review_handlers.go` | Reconstruct text from `ResponseEntries`; optionally store entries in new `agent_response_entries` field |
| `api/pkg/trigger/*/` (slack, teams, azure, crisp, cron) | Same reconstruction pattern |
| `api/pkg/controller/`, `api/pkg/cli/`, `api/pkg/server/` misc files | Audit each; apply reconstruction pattern where reading ResponseMessage |
| `api/pkg/server/websocket_external_agent_sync.go` | Stop setting `ResponseMessage` once consumers are migrated |
| `api/pkg/types/types.go` | Add `omitempty` to `response_message` json tag |

### Helper (shared reconstruction)

Add a helper function in `api/pkg/types/` or a shared package:

```go
// TextFromInteraction returns the plain-text response, preferring ResponseEntries over ResponseMessage.
func TextFromInteraction(i *Interaction) string {
    if len(i.ResponseEntries) > 0 {
        var entries []wsprotocol.ResponseEntry
        if err := json.Unmarshal(i.ResponseEntries, &entries); err == nil {
            var sb strings.Builder
            for _, e := range entries {
                if e.Type == "text" {
                    sb.WriteString(e.Content)
                }
            }
            return sb.String()
        }
    }
    return i.ResponseMessage
}
```

All consumer migration tasks call this helper instead of duplicating the logic.
