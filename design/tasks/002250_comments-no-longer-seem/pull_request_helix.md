# Fix spec review comments disappearing and mis-anchoring

## Summary

Comments on spec design documents were disappearing and anchoring to the wrong
text. Root cause: a comment's anchor is stored only as `quoted_text` — the string
from `window.getSelection().toString()`, i.e. **rendered** text with markdown
syntax stripped and whitespace normalized — but that value was compared against
two different, un-normalized sources.

- **Backend** auto-resolved a comment whenever `strings.Contains(rawMarkdown,
  quoted_text)` was false. Selections spanning block boundaries (captured `\n`)
  or overlapping markdown that isn't a simple wrapping token failed the check and
  the comment was silently resolved on the next agent push.
- **Frontend** located the bubble via `documentRef.textContent.indexOf(quoted_text)`
  — the wrong container (it includes the comment bubbles), first-occurrence only
  (repeated phrases mis-anchored), and `-1` for cross-block selections (bubble
  silently dropped).

## Changes

- **`api/pkg/services/git_http_server.go`**: added `normalizeForCommentMatch`
  (resolves links/images to visible text, strips inline markdown tokens,
  collapses whitespace) and apply it to both the document and `quoted_text`
  before comparing. Fail-safe: a quote that normalizes to empty is kept.
- **`api/pkg/services/git_http_server_comment_resolution_test.go`**: unit test
  covering bold/inline-code/link/list/cross-paragraph text (kept) and genuinely
  removed text (resolved).
- **`frontend/.../DesignReviewContent.tsx`**: `findQuotedTextPosition` now matches
  a whitespace-normalized copy of the **markdown element** and maps back to an
  exact DOM Range; picks the occurrence by stored `start_offset`, else by
  occurrence ordinal for repeated phrases. Captures `start_offset`/`end_offset`
  on selection. Fail-safe: after retries, an un-locatable comment is stacked and
  flagged instead of being dropped.
- **`frontend/.../InlineCommentBubble.tsx`**: `unlocated` prop renders a "⚠
  couldn't locate the quoted text" caption.

## Testing

End-to-end in a live inner Helix: created a spec task, opened its design review,
and commented on a unique phrase and on the 4th occurrence of the repeated
inline-code phrase `localStorage`. Both bubbles render; the second anchored next
to the 4th occurrence (y≈1012) rather than the first (y≈265), confirming
occurrence disambiguation via the stored offset (500 and 1437). No console errors.
Backend normalization is covered by the unit test.

## Screenshots

![Both comments anchored correctly in wide view](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002250_comments-no-longer-seem/screenshots/01-comments-anchored-wide.png)
