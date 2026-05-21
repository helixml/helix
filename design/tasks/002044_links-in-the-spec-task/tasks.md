# Implementation Tasks: Open Chat Links in New Tab

- [~] Add an `a` component override to `markdownComponents` in `frontend/src/components/session/Markdown.tsx` (~line 1151), placed alongside the existing `code` override
- [~] In the override: leave `href="#"` / `href.startsWith("#")` links untouched (preserves filter-mention and doc-group-link handlers and in-page anchors)
- [~] In the override: preserve any pre-existing `target` attribute (keeps `processDocumentIds` doc-citation links rendering with their explicit `target="_blank"`)
- [~] In the override: default external links to `target="_blank"` and `rel="noopener noreferrer"` for security
- [ ] Run `cd frontend && yarn build` to confirm there are no TypeScript or build errors
- [ ] Manually verify in the inner Helix (`http://localhost:8080`): plain external markdown link in a session message opens in a new tab
- [ ] Manually verify in the inner Helix: plain external markdown link in a spec task comment (`InlineCommentBubble` / `CommentLogSidebar`) opens in a new tab
- [ ] Manually verify in the inner Helix: filter mention (`@filter:`) still triggers its existing JS handler and does NOT open a blank new tab
- [ ] Manually verify in the inner Helix: a document citation (`[DOC_ID:...]`) still opens its document in a new tab (no regression)
- [ ] Manually verify in the inner Helix: in-page anchor (e.g. `[Top](#top)`) scrolls within the same tab
- [ ] Inspect a rendered external link in DevTools and confirm `rel="noopener noreferrer"` is present
- [ ] Commit with a conventional commit message (e.g. `feat(frontend): open chat markdown links in new tab`) and push; verify CI passes
