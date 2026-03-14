# Implementation Tasks

- [ ] In `InlineCommentBubble.tsx`: replace `InteractionMarkdown` with `MessageWithToolCalls` for rendering `displayResponse` (pass `EMPTY_SESSION`, `getFileURL={() => '#'}`, `showBlinker={isStreaming}`, `isStreaming={isStreaming}`)
- [ ] In `CommentLogSidebar.tsx`: replace `InteractionMarkdown` with `MessageWithToolCalls` for rendering `comment.agent_response` (pass `EMPTY_SESSION`, `getFileURL={() => '#'}`, `showBlinker={false}`, `isStreaming={false}`)
- [ ] Verify the agent produces `**Tool Call: <name>**\nStatus: <status>` format in comment responses; if the regex in `parseToolCallBlocks` doesn't match, adjust the pattern in `CollapsibleToolCall.tsx`
- [ ] Fix `ThinkingWidget` sizing: either add `compact` prop with smaller dimensions (minHeight 60, inner height 60, maxHeight 120) and thread it through `Markdown.tsx`, OR reduce the global default sizes if they look acceptable in the main chat
- [ ] Build frontend (`cd frontend && yarn build`) and test in browser: submit a comment on a design review doc, confirm tool call blocks render as collapsible widgets and thinking block fits the comment bubble
