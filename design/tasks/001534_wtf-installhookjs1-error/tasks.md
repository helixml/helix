# Implementation Tasks

- [~] In `frontend/src/components/session/Markdown.tsx` (`MemoizedMarkdownRenderer`, ~L1148): add `ref` to the destructured props in the `code` component override so it is excluded from `...rest` spread onto `<code>` DOM element
- [~] In `frontend/src/components/git/CodeIntelligenceTab.tsx` (`markdownComponents`, ~L135): add `ref` to the destructured props in the `code` component override so it is excluded from `...props` spread onto `<code>` DOM elements
- [~] In `frontend/src/components/spec-tasks/DesignReviewContent.tsx` (inline `components`, ~L1184): add `ref` to the destructured props in the `code` component override so it is excluded from `...props` spread onto `<code>` and `<SyntaxHighlighter>`
- [ ] In `frontend/src/components/widgets/JsonWindowLink.tsx`: remove `forwardRef` wrapper and unused `ref` parameter — the component never forwards it to a DOM element
- [ ] If `JsonWindowLink` is used as a direct child of MUI `Tooltip` anywhere (e.g., `SessionToolbar.tsx`), wrap it in a `<span>` so Tooltip can attach its ref
- [ ] Run `cd frontend && yarn build` and confirm no build errors
- [ ] Manually verify: open a chat session with code blocks in assistant responses, confirm no React error #290 in browser console
- [ ] Manually verify: open Code Intelligence tab wiki/search, confirm no React error #290