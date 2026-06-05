# Implementation Tasks: Open Chat Links in New Tab

- [x] Add an `a` component override to `markdownComponents` in `frontend/src/components/session/Markdown.tsx` (~line 1151), placed alongside the existing `code` override
- [x] In the override: leave `href="#"` / `href.startsWith("#")` links untouched (preserves filter-mention and doc-group-link handlers and in-page anchors)
- [x] In the override: preserve any pre-existing `target` attribute (keeps `processDocumentIds` doc-citation links rendering with their explicit `target="_blank"`)
- [x] In the override: default external links to `target="_blank"` and `rel="noopener noreferrer"` for security
- [x] Run `cd frontend && yarn build` to confirm there are no TypeScript or build errors
- [x] Manually verify in the inner Helix (`http://localhost:8080`): plain external markdown link rendered by the live `InteractionMarkdown` component gets `target="_blank"` and `rel="noopener noreferrer"` (verified by importing the actual component into the running page and inspecting the rendered DOM — see `screenshots/01-rendered-anchors.png` and the verification block in `design.md`)
- [x] Same renderer covers spec task chat surfaces (`InlineCommentBubble`, `CommentLogSidebar`) — they all flow through `MessageWithToolCalls` → `InteractionMarkdown`, so the single override applies uniformly. Verified by importing `Markdown.tsx`'s default export (`InteractionMarkdown`) directly.
- [x] Filter mention (`<a href="#" class="filter-mention">`) is left untouched — verified: rendered output has no `target`, no `rel`, class preserved.
- [x] Document citation behaviour preserved — verified by including `<a href="https://existing.com" target="_self">` in the test; the override respected the pre-existing target. The same code path applies to `processDocumentIds` citations which set `target="_blank"`.
- [x] In-page anchor (`[Top](#top)`) — verified: rendered as `<a href="#top">` with no target, scrolls within the same tab.
- [x] `rel="noopener noreferrer"` confirmed in DevTools on the rendered external link.
- [x] Commit with a conventional commit message and push the `feature/002044-open-chat-links-in-new` branch (commit `15d5851eb`)
