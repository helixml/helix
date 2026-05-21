# feat(frontend): open chat markdown links in a new tab

## Summary

Markdown links rendered in the spec task chat (inline comment bubbles
and the comment log sidebar) and in regular session chat used to open
in the **same tab**, navigating users away from Helix and discarding
their place in the chat. This PR fixes that.

The fix is one override in the shared `InteractionMarkdown` renderer in
`frontend/src/components/session/Markdown.tsx`, so it covers every
surface that uses the renderer with a single change.

## Changes

- Add an `a` component override to `markdownComponents` inside
  `MemoizedMarkdownRenderer` (`Markdown.tsx`).
- Plain markdown links (`[text](https://…)`) now render with
  `target="_blank"` and `rel="noopener noreferrer"` (the standard
  mitigation for reverse-tabnabbing).
- Existing `processDocumentIds` doc-citation links (which already set
  `target="_blank"` explicitly) are preserved — the override respects
  any pre-existing `target` on the anchor.
- Internal action links (`href="#"` filter mentions, doc-group links)
  and in-page anchors (`href="#section"`) are left untouched so their
  JS handlers and same-tab navigation continue to work.

## Why

The user's report: *"Links in the spec task chat should open a new tab.
To be honest, this is probably a common interaction display component
that's used in sessions as well."* — and yes, both the spec task chat
and the session chat flow through the same `InteractionMarkdown`
component, so a single override fixes both.

## Testing

- `cd frontend && yarn build` — passes (no type or build errors).
- Manual verification in the inner Helix browser: posting a markdown
  link in a session message renders an anchor with `target="_blank"`
  and `rel="noopener noreferrer"`; clicking opens a new tab. Existing
  citation, filter-mention, and in-page-anchor behaviour unchanged.
