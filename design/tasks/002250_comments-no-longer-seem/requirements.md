# Requirements: Fix Spec Review Comments Disappearing and Mis-Anchoring

## Background

When reviewing a spec task's design documents (Requirements / Technical Design /
Implementation Plan), a reviewer can select text and attach an inline comment.
The comment renders as a bubble anchored next to the quoted text.

Users report that comments **no longer show up correctly**: they appear detached
from the text they were made against, and often **disappear completely**.

## Root Cause Summary

A comment's anchor is stored only as `quoted_text` — the string returned by the
browser's `window.getSelection().toString()`, i.e. the **rendered** text with all
markdown syntax stripped and whitespace normalized. That single value is then
compared against two *different* text sources, and both comparisons are fragile:

1. **Backend auto-resolution** (`checkCommentResolution` in
   `api/pkg/services/git_http_server.go`) runs on every push while the task is in
   SpecGeneration / SpecReview / SpecRevision / Implementation. It marks a comment
   resolved if `strings.Contains(rawMarkdown, quoted_text)` is false. Because
   `quoted_text` came from rendered text, any comment whose selection overlapped
   inline markdown (`**bold**`, `*italic*`, `` `code` ``, `[links](…)`, list
   bullets, heading `#`s) or spanned a block boundary (captured `\n`) is **not**
   found verbatim in the raw `.md` file, so it is silently auto-resolved on the
   next agent push — and vanishes. This is the primary "disappear completely"
   cause.

2. **Frontend anchoring** (`findQuotedTextPosition` in
   `DesignReviewContent.tsx`) locates the bubble with
   `documentRef.current.textContent.indexOf(quoted_text)`:
   - It searches `documentRef` (which also contains the comment bubbles and other
     chrome) instead of the rendered markdown element (`markdownRef`).
   - `indexOf` returns the **first** occurrence, so a repeated phrase anchors the
     bubble to the wrong place ("not attached to the right bit of text").
   - Cross-block selections contain `\n` that `textContent` concatenation does not,
     so `indexOf` returns `-1`, position is `null`, and the bubble is silently
     dropped (`if (yPos === undefined) return null;`).

The `start_offset` / `end_offset` columns exist on the comment model but the
frontend never populates them (only `document_type`, `quoted_text`, `comment_text`
are sent), so there is no robust positional anchor to fall back on.

## User Stories

### US-1: Comments stay visible
As a reviewer, when I comment on any text — including bold/italic/code/linked text
or a selection that spans paragraphs — my comment stays visible and does not get
silently auto-resolved or dropped when the agent pushes an update.

**Acceptance Criteria**
- Commenting on text containing inline markdown formatting keeps the comment
  unresolved after a subsequent push, as long as the underlying text still exists.
- A comment is only auto-resolved when its quoted text is genuinely no longer
  present in the document (after normalization), not merely because of markdown
  syntax or whitespace differences.
- If the anchor cannot be confidently located, the comment is **kept**, never
  silently resolved.

### US-2: Comments anchor to the correct text
As a reviewer, my comment bubble appears next to the text I actually selected, even
when the same phrase occurs multiple times in the document.

**Acceptance Criteria**
- A comment on a repeated phrase anchors to the specific occurrence that was
  selected, not always the first one.
- A comment on a selection spanning multiple paragraphs still anchors and renders.

### US-3: Comments never silently disappear
As a reviewer, if the app cannot locate the exact quoted text in the rendered
document, the comment is still shown to me (e.g. surfaced in the comment log /
sidebar), rather than vanishing.

**Acceptance Criteria**
- An unresolved comment whose text cannot be positioned inline is still visible in
  the comment log sidebar (and ideally flagged as "couldn't locate exact text").
- No unresolved comment for the active document is ever completely absent from the
  UI.

## Non-Goals

- No change to the comment→agent request/response streaming flow.
- No redesign of the bubble stacking / layout algorithm beyond what's needed to
  stop dropping comments.
- Full fuzzy/diff-based re-anchoring (prefix/suffix context matching) is a possible
  future enhancement, not required here.

## Open Questions

1. **Regression trigger.** The `strings.Contains` auto-resolution logic appears to
   have arrived with the go-git HTTP server rewrite (`db890cc94` /`d12efe6ff`). Is
   the reported regression believed to stem from that rewrite making
   auto-resolution fire more reliably/eagerly, or from a separate change to
   document rendering? Confirming the trigger would help scope the fix.
2. **Auto-resolution behaviour.** Should backend auto-resolution be made
   conservative (normalize + strip markdown before comparing, keep on doubt), or
   disabled entirely and left to the agent to explicitly resolve via
   `agent_updated`? Recommendation in design.md is "make conservative".
3. **Preferred anchoring strategy.** Is normalizing + occurrence-disambiguation
   acceptable for now, or do you want character offsets into the source markdown
   captured at creation time (more robust, more work)?
4. **Fallback surface.** For comments that can't be anchored inline, is the comment
   log sidebar the right place to guarantee visibility, or should they pin to the
   top of the document column?
