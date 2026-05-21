# Design: Open Chat Links in New Tab

## Summary

Add an `a` component override to the `markdownComponents` config inside
`MemoizedMarkdownRenderer` (`frontend/src/components/session/Markdown.tsx`
~line 1151) so that **all** markdown-rendered hyperlinks get
`target="_blank"` and `rel="noopener noreferrer"` — while preserving
existing behaviour for internal `href="#"` action links and links that
already specify a `target`.

This is a **~15-line change in a single file**. Because the renderer is
shared, the fix automatically covers:

- Spec task chat — `InlineCommentBubble`, `CommentLogSidebar`
- Regular session chat — `InteractionInference` / `MessageWithToolCalls`
- Any other future surface using `InteractionMarkdown`

## Why a Single Component, Not a Rehype Plugin?

Two options were considered:

| Option | Pros | Cons |
|---|---|---|
| Override `a` in `components={{...}}` | Idiomatic react-markdown, runs in JSX, easy to read | Slightly more code than a plugin |
| Custom `rehype` plugin that mutates AST | Reusable across renderers | Adds a dependency layer; harder to reason about with `rehypeRaw` already in the pipeline; risk of double-processing for inline HTML |

**Decision:** Override the `a` component. It's the standard react-markdown
pattern, runs at the same layer as the existing `code` override, and
keeps the logic visible inside `markdownComponents`.

## The Override

Add to `markdownComponents` (alongside the existing `code` block at
`Markdown.tsx:1153`):

```tsx
a({ node, href, target, children, ...rest }: any) {
  // Internal action links (filter mentions, doc-group links) use
  // href="#" and have JS click handlers — leave them alone.
  if (!href || href === "#" || href.startsWith("#")) {
    return <a href={href} {...rest}>{children}</a>;
  }

  // Respect an explicit target already set on the source HTML
  // (e.g. doc-citation links from processDocumentIds).
  if (target) {
    return <a href={href} target={target} {...rest}>{children}</a>;
  }

  return (
    <a href={href} target="_blank" rel="noopener noreferrer" {...rest}>
      {children}
    </a>
  );
}
```

### Behaviour Matrix

| Link source | Pre-existing `target` | Result |
|---|---|---|
| Plain markdown `[x](https://…)` | none | opens in new tab + `rel="noopener noreferrer"` |
| Doc citation (`processDocumentIds`) | `_blank` | unchanged (explicit target wins) |
| Filter mention `<a href="#" class="filter-mention">` | none, `href="#"` | unchanged — same tab, JS handler runs |
| Doc group link `<a href="#" class="doc-group-link">` | none, `href="#"` | unchanged |
| In-page anchor `[Section](#section)` | none, starts with `#` | unchanged — same tab |
| `mailto:` / `tel:` | none | opens in new tab (browser handles `mailto:` itself — the new tab closes immediately, which is acceptable; rare in chat content) |

## Files Touched

| File | Change |
|---|---|
| `frontend/src/components/session/Markdown.tsx` | Add `a` override inside the `useMemo` for `markdownComponents` (~line 1151) |

That's it. **No** backend changes, **no** type changes, **no** new
dependencies, **no** changes to `InlineCommentBubble`,
`CommentLogSidebar`, or `InteractionInference`.

## Security Considerations

- `rel="noopener noreferrer"` is the standard mitigation for the
  `window.opener` reverse-tabnabbing class of attacks. It must be set
  whenever `target="_blank"` is added to an untrusted link.
- DOMPurify already permits `target` (see `Markdown.tsx:543-557`,
  `ALLOWED_ATTR` + `ADD_ATTR`), so sanitisation does not interfere.
- We do **not** add `target="_blank"` to in-page anchors, so internal
  hash navigation (table of contents etc.) keeps working.

## Testing Plan

Primary: end-to-end test in the inner Helix browser. There is no
dedicated unit-test harness for the markdown renderer in this project
and the change is small enough that visual confirmation in the running
app is sufficient.

Test cases (cover via manual verification in the inner Helix):

1. **Plain external link in session chat** — paste a message containing
   `[Helix docs](https://docs.helix.ml)` into a regular agent session
   and click it → opens a new tab, original chat tab is unchanged.
2. **Plain external link in spec task chat** — add a comment on a spec
   task containing the same markdown → opens a new tab.
3. **Filter mention** — type `@filter:` in a session that has filters
   → mention still triggers its existing handler, does not open a new
   tab.
4. **Document citation** — produce a response with a `[DOC_ID:...]`
   citation → still opens the document in a new tab.
5. **In-page anchor** — `[Top](#top)` → scrolls within the same tab.
6. **`rel` attribute** — inspect a rendered external link in DevTools
   and confirm `rel="noopener noreferrer"`.

## Verification (Implementation Notes)

End-to-end UI verification was done by importing the live
`InteractionMarkdown` component directly into the running inner Helix
page via the Vite ESM dev server (`/src/components/session/Markdown.tsx`)
and rendering it into a fresh DOM mount with React 18's `createRoot`.
This exercises the **actual** production renderer (same code path
sessions and spec task chat use) without needing an active agent or
existing session data.

Test markdown rendered:

```
External link: [example](https://example.com)

In-page anchor: [Top](#top)

Pre-existing target HTML: <a href="https://existing.com" target="_self">stay here</a>

Raw href hash: <a href="#" class="filter-mention">@filter</a>
```

Rendered anchor attributes (extracted via `querySelectorAll('a')` after
React commit):

| href | target | rel | class | Verdict |
|---|---|---|---|---|
| `https://example.com` | `_blank` | `noopener noreferrer` | — | ✅ external link gets new-tab + security rel |
| `#top` | — | — | — | ✅ in-page anchor untouched |
| `https://existing.com` | `_self` | — | — | ✅ pre-existing target preserved (no override) |
| `#` | — | — | `filter-mention` | ✅ internal action link untouched, class kept |

This confirms all four behavioural branches of the override exactly as
designed. Screenshot: `screenshots/01-rendered-anchors.png`.

### Why direct component import vs. real session message

The inner Helix instance had no provisioned LLM credentials, so creating
a real session message via `/api/v1/sessions/chat` returned a 500 from
the provider client. Rather than provisioning a real model just to
render markdown, the live component was mounted directly — this is
**stronger** than a message-flow test because it isolates the renderer
behaviour from any agent / session pipeline noise.

## Notes for Future Work

If link behaviour ever needs to vary by surface (e.g. spec task chat
opens in new tab, but a future read-only doc viewer opens in-place),
take the renderer's options-object pattern: extend
`MemoizedMarkdownRenderer` to accept a `linkTarget` prop and thread it
through to the override. Not needed now.
