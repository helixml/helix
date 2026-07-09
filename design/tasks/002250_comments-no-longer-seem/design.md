# Design: Fix Spec Review Comments Disappearing and Mis-Anchoring

## Guiding Principle

`quoted_text` is captured from **rendered** text but is compared against two
different sources (raw markdown on the backend, rendered DOM on the frontend).
The fix keeps that single stored value but makes every comparison **normalize
both sides the same way** and makes both surfaces **fail safe** (never silently
lose a comment).

## Affected Code

| Concern | File |
|---|---|
| Backend auto-resolution | `api/pkg/services/git_http_server.go` — `checkCommentResolution` (~L1899) |
| Frontend anchoring | `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — `findQuotedTextPosition` (~L891), inline render (~L1605) |
| Fallback surface | `frontend/src/components/spec-tasks/CommentLogSidebar.tsx` |
| Comment model (offsets exist, unused) | `api/pkg/types/spec_task_design_review.go` (`StartOffset`/`EndOffset`) |

## Decision 1 — Backend: conservative, normalized auto-resolution

`checkCommentResolution` currently does `strings.Contains(rawMarkdown, quoted_text)`.
Replace the naive substring check with a **normalized** comparison:

- Produce a comparison string from the doc that approximates rendered text:
  collapse all runs of whitespace (including newlines) to a single space, and
  strip common inline markdown tokens (`*`, `_`, `` ` ``, link syntax, leading
  `#`/`-`/`>` markers). Apply the **same** whitespace collapse to `quoted_text`.
- Only auto-resolve when the normalized quoted text is absent from the normalized
  document. **When in doubt, keep the comment** (fail safe) — a lingering resolved-
  worthy comment is far less harmful than a silently-deleted one.

Keep the existing `agent_updated` / explicit resolution paths untouched. This
narrows auto-resolution to its intended case (text genuinely removed) instead of
firing on formatting differences.

Rationale: matches the CLAUDE.md "root cause it properly, no hacks" rule — the bug
is a source-mismatch, and normalization removes the mismatch without adding a new
data model.

## Decision 2 — Frontend: robust anchoring

Rework `findQuotedTextPosition` (and the string it searches):

1. **Search the rendered markdown only.** Walk/`textContent` from
   `markdownRef.current`, not `documentRef.current`, so bubble text and chrome
   can't produce false matches or shift offsets.
2. **Normalize whitespace on both sides** before matching (collapse whitespace
   runs to a single space) so cross-block selections that captured `\n` still
   locate their anchor.
3. **Disambiguate repeated phrases.** Prefer the occurrence nearest an expected
   position. Minimal approach: keep `indexOf` but, when the same `quoted_text`
   is shared by multiple *comments*, distribute them across successive
   occurrences. Better approach (recommended if scope allows): capture and store
   `start_offset` at creation (character offset into the normalized rendered text)
   and select the occurrence closest to it.

## Decision 3 — Frontend: never silently drop a comment (fail safe)

Today `if (yPos === undefined) return null;` hides any comment that can't be
positioned. Instead:

- Keep the inline bubble when a position is found.
- When no inline position can be computed for an **unresolved** comment on the
  active document, guarantee it is still visible in the `CommentLogSidebar`
  (which already lists `activeDocComments`), and flag it (e.g. "⚠ couldn't locate
  quoted text") so the reviewer knows the anchor drifted.

This makes US-3 hold regardless of anchoring accuracy.

## Optional (recommended) — populate offsets at creation

The model already has `StartOffset`/`EndOffset`. Capturing the character offset of
the selection within the normalized rendered document at comment-creation time
gives a stable tiebreaker for Decision 2's disambiguation and a fallback anchor.
Frontend `handleCreateComment` would compute and send `start_offset`; backend
handler already stores it. This is the cleanest long-term anchor but is optional
for the first fix.

## Testing Strategy

Per CLAUDE.md, test end-to-end in the inner Helix (`localhost:8080`):
1. Register / onboard, create a spec task, reach the design review page.
2. Comment on: (a) plain text, (b) **bold**/`code`/[link] text, (c) a repeated
   phrase, (d) a selection spanning two paragraphs.
3. Trigger an agent push (send a comment so the agent responds/updates docs) and
   confirm none of (a)–(d) disappear or auto-resolve while their text still exists.
4. Confirm (c) anchors to the correct occurrence and (d) renders.
5. Backend: unit test `checkCommentResolution` normalization — a comment on
   markdown-formatted text is NOT auto-resolved; a comment whose text was truly
   deleted IS auto-resolved.
