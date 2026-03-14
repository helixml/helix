# Requirements: Fix React Error #290 (String Ref)

## Problem

The Helix frontend throws a React error #290 in production:

> Element ref was specified as a string ({ref}) but no owner was set.

This error originates from `react-markdown` component overrides that spread `{...rest}` or `{...props}` onto DOM elements. The `rest`/`props` object can contain a `ref` property passed by `hast-util-to-jsx-runtime` (used internally by `react-markdown` v9), which React interprets as a legacy string ref and throws.

The error appears in the browser console and can crash the React render tree if not caught by an error boundary.

## User Stories

- As a user, I want chat responses with code blocks to render without console errors or crashes.
- As a user, I want the Code Intelligence wiki/search tab to render markdown without errors.
- As a user, I want the design review content to render markdown without errors.

## Acceptance Criteria

- [ ] No React error #290 appears in the browser console when rendering markdown content (chat messages, wiki pages, design reviews).
- [ ] Code blocks (fenced and inline) render correctly after the fix.
- [ ] `JsonWindowLink` properly forwards its ref to a DOM element (or drops `forwardRef` if unused).
- [ ] `yarn build` completes without errors.

## Affected Files

| File | Pattern | Risk |
|------|---------|------|
| `frontend/src/components/session/Markdown.tsx` (L1148-1158) | `{...rest}` spread on `<code>` in `MemoizedMarkdownRenderer` | High — renders every chat message |
| `frontend/src/components/git/CodeIntelligenceTab.tsx` (L135-163) | `{...props}` spread on `<code>` in `markdownComponents` | Medium |
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` (L1184-1207) | `{...props}` spread on `<code>` and `<SyntaxHighlighter>` | Medium |
| `frontend/src/components/widgets/JsonWindowLink.tsx` | `forwardRef` used but ref never forwarded to DOM | Low — causes silent failures, not error #290 |

## Root Cause

`react-markdown` v9.0.3 uses `hast-util-to-jsx-runtime` v2.3.2 to convert hast nodes to React elements. When custom `components` are provided (e.g., `code`), the component receives all hast properties as props — including `node` and potentially `ref`. The common pattern of `const { children, className, node, ...rest } = props` leaves `ref` in `rest`. Spreading `{...rest}` onto a DOM element like `<code>` causes React to treat it as a legacy string ref, triggering error #290.