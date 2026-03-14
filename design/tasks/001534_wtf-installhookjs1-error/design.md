# Design: Fix React Error #290 (String Ref)

## Summary

Fix three instances of unsafe prop spreading in `react-markdown` component overrides that cause React error #290 ("Element ref was specified as a string ({ref}) but no owner was set"). Also fix `JsonWindowLink` which accepts a `forwardRef` but never forwards it.

## Architecture

No architectural changes. This is a targeted bug fix across four files in the frontend.

## Key Decision: Destructure `ref` Out of Spread

The fix is to explicitly destructure `ref` (and `node`) out of props before spreading the remainder onto DOM elements. This is the standard pattern recommended for `react-markdown` v9 custom components.

**Before (broken):**
```tsx
code({ className, children, node, ...rest }) {
  return <code {...rest} className={className}>{children}</code>
  //          ^^^^^^^^ rest may contain `ref` from hast-util-to-jsx-runtime
}
```

**After (fixed):**
```tsx
code({ className, children, node, ref, ...rest }) {
  return <code {...rest} className={className}>{children}</code>
  //                                   ref is now excluded from rest
}
```

### Why not use `ref`?

`hast-util-to-jsx-runtime` may pass a `ref` as a regular prop (not a React ref callback or object). Forwarding it would still cause the same error. The safe approach is to discard it — `react-markdown` does not rely on refs for its component overrides.

## Codebase Patterns Discovered

- **react-markdown v9.0.3** with **hast-util-to-jsx-runtime v2.3.2** — components receive `node` (hast AST node) and potentially `ref` as props. Both must be excluded from DOM element spreads.
- **rehype-raw** is enabled — raw HTML in markdown content is parsed and rendered. DOMPurify sanitizes non-code HTML, but code blocks bypass sanitization (they're replaced with placeholders). This means raw HTML inside code blocks can introduce unexpected hast properties.
- **Three separate markdown rendering locations** exist: chat messages (`Markdown.tsx`), code intelligence (`CodeIntelligenceTab.tsx`), and design review (`DesignReviewContent.tsx`). All use the same broken pattern independently.
- **`JsonWindowLink`** uses `forwardRef` but the `ref` parameter is destructured and silently dropped. MUI `Tooltip` wraps `JsonWindowLink` in `SessionToolbar.tsx` — `Tooltip` needs its child to accept a ref for positioning. Fix: either forward the ref to the root `ClickLink`/fragment, or remove `forwardRef` and don't use it inside `Tooltip` children.

## Fix Details

### 1. `Markdown.tsx` — `MemoizedMarkdownRenderer` (L1147-1158)

Destructure `ref` from props in the `code` component override:

```tsx
code(props: any) {
  const { children, className, node, ref, ...rest } = props;
  // ... same logic, rest no longer contains ref
}
```

### 2. `CodeIntelligenceTab.tsx` — `markdownComponents` (L135-163)

Same fix — add `ref` to the destructure:

```tsx
code: ({ className, children, node, ref, ...props }) => {
  // ... existing logic unchanged
}
```

### 3. `DesignReviewContent.tsx` — inline `components` (L1184-1210)

Same fix:

```tsx
code({ node, inline, className, children, ref, ...props }: any) {
  // ... existing logic unchanged
}
```

### 4. `JsonWindowLink.tsx` — fix or remove `forwardRef`

Option A (preferred): Remove `forwardRef` since the ref is never used. This is the honest approach — the component doesn't support refs.

Option B: Forward the ref to the root `<>` fragment — but fragments can't take refs. Would need to wrap in a `<span>` or forward to `ClickLink`, which would require `ClickLink` to also use `forwardRef`.

**Decision: Option A** — remove `forwardRef`, remove the unused `ref` parameter. If `Tooltip` wrapping is needed in `SessionToolbar.tsx`, wrap the `JsonWindowLink` in a `<span>` there (MUI Tooltip works with any DOM element child).

## Testing

- `cd frontend && yarn build` — must pass with no errors
- Manual: open a chat session with code blocks in responses, check browser console for error #290
- Manual: navigate to Code Intelligence tab wiki view, check console
- No unit tests needed — this is a prop-passing fix, not logic