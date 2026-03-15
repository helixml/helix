# Implementation Tasks

## Frontend — Tool Call Rendering

- [~] In `DesignReviewContent.tsx`: update the `interaction_patch` handler to track `ResponseEntry[]` (type, content, tool_name, tool_status) instead of bare `string[]`; update `streamingResponse` state shape to carry `entries: ResponseEntry[]` alongside the comment ID
- [~] In `DesignReviewContent.tsx`: update the `interaction_update` and `session_update` completion handlers to parse `interaction.response_entries` JSON instead of reading `interaction.response_message`
- [~] In `InlineCommentBubble.tsx`: accept `streamingEntries?: ResponseEntry[]` prop; replace `InteractionMarkdown` with `MessageWithToolCalls`, passing entries for structured rendering and flat text as fallback
- [~] In `CommentLogSidebar.tsx`: replace `InteractionMarkdown` with `MessageWithToolCalls`; pass `agent_response_entries` (if present) as structured entries

## Frontend — ThinkingWidget Size

- [ ] In `ThinkingWidget.tsx`: add `compact?: boolean` prop; when true use `minHeight: 60`, inner collapsed `height: 60`, open `maxHeight: 120`
- [ ] In `Markdown.tsx`: accept `compactThinking?: boolean` prop and forward it to `ThinkingWidget`
- [ ] Pass `compactThinking={true}` from `InlineCommentBubble` and `CommentLogSidebar` through to their `MessageWithToolCalls` / `Markdown` calls

## Backend — Shared Helper

- [ ] Add `TextFromInteraction(i *Interaction) string` helper in `api/pkg/types/` (or a shared util): unmarshal `ResponseEntries`, join `type=text` content, fall back to `ResponseMessage` if entries are empty or unmarshal fails

## Backend — Migrate response_message Consumers

- [ ] `api/pkg/session/mcp_server.go`: replace direct reads of `ResponseMessage` in `get_turn`, `get_turns`, `get_interaction` handlers with `TextFromInteraction()`
- [ ] `api/pkg/server/summary_service.go`: replace `ResponseMessage` reads with `TextFromInteraction()`
- [ ] `api/pkg/server/spec_task_design_review_handlers.go`: replace `interaction.ResponseMessage` assignments to `comment.AgentResponse` with `TextFromInteraction()`; also marshal `interaction.ResponseEntries` into a new `comment.AgentResponseEntries` jsonb DB column (required — without it tool calls disappear from completed comments in new sessions)
- [ ] `api/pkg/trigger/slack/`, `teams/`, `azure/`, `crisp/`, `cron/`: replace `ResponseMessage` reads with `TextFromInteraction()`
- [ ] Audit remaining consumers (`controller_external_agent.go`, `sessions.go`, `question_set_handlers.go`, `session_toc_handlers.go`, `scheduler/workload.go`, CLI spectask) — apply `TextFromInteraction()` wherever reading `ResponseMessage` for display/output

## Backend — Stop Writing response_message

- [ ] In `api/pkg/server/websocket_external_agent_sync.go`: remove the line that sets `targetInteraction.ResponseMessage = acc.Content` (line ~1033) — entries are already saved; flat string is no longer needed
- [ ] In `api/pkg/types/types.go`: add `omitempty` to the `json:"response_message"` tag so the now-empty field is omitted from wire events

## Verification

- [ ] `go build ./api/pkg/...` — no compile errors
- [ ] `cd frontend && yarn build` — no TypeScript errors
- [ ] Test in browser: submit a comment on a design review doc; confirm tool call blocks render as collapsible widgets during streaming and after completion
- [ ] Confirm ThinkingWidget fits the comment bubble without excess height
- [ ] Verify Zed MCP conversation history still works (get_turn/get_turns return correct text after migration)
- [ ] Check network traffic: `response_message` absent (or empty) in WebSocket frames for new interactions
