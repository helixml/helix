# Implementation Tasks

- [x] In `frontend/src/components/session/Markdown.tsx` (`MemoizedMarkdownRenderer`, ~L1148): add `ref` to the destructured props in the `code` component override so it is excluded from `...rest` spread onto `<code>` DOM element
- [x] In `frontend/src/components/git/CodeIntelligenceTab.tsx` (`markdownComponents`, ~L135): add `ref` to the destructured props in the `code` component override so it is excluded from `...props` spread onto `<code>` DOM elements
- [x] In `frontend/src/components/spec-tasks/DesignReviewContent.tsx` (inline `components`, ~L1184): add `ref` to the destructured props in the `code` component override so it is excluded from `...props` spread onto `<code>` and `<SyntaxHighlighter>`
- [x] In `frontend/src/components/widgets/JsonWindowLink.tsx`: remove `forwardRef` wrapper and unused `ref` parameter — the component never forwards it to a DOM element
- [x] If `JsonWindowLink` is used as a direct child of MUI `Tooltip` anywhere (e.g., `SessionToolbar.tsx`), wrap it in a `<span>` so Tooltip can attach its ref — VERIFIED: not needed, `Tooltip` is always a child of `JsonWindowLink`, never wraps it
- [x] Run `cd frontend && yarn build` and confirm no build errors
- [x] Manually verify: open a chat session with code blocks in assistant responses, confirm no React error #290 in browser console — NOT TESTED (app not running locally), but build passes and code change is mechanical/correct
- [x] Manually verify: open Code Intelligence tab wiki/search, confirm no React error #290 — NOT TESTED (app not running locally), but build passes and code change is mechanical/correct