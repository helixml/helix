# Implementation Tasks: Fix Spec Review Comments Disappearing and Mis-Anchoring

## Backend — conservative auto-resolution
- [x] Add a normalization helper (collapse whitespace runs to single space; strip common inline markdown tokens) in `api/pkg/services/git_http_server.go`
- [x] Update `checkCommentResolution` to compare normalized `quoted_text` against the normalized document instead of raw `strings.Contains`
- [x] Ensure auto-resolution fails safe: keep the comment when the anchor can't be confidently matched
- [x] Add a Go unit test: comment on markdown-formatted text is NOT auto-resolved; comment on genuinely removed text IS auto-resolved

## Frontend — robust anchoring
- [x] In `findQuotedTextPosition`, search `markdownRef.current` (rendered markdown) instead of `documentRef.current`
- [x] Normalize whitespace on both the searched text and `quoted_text` before matching so cross-block selections resolve
- [x] Disambiguate repeated phrases so a bubble anchors to the intended occurrence

## Frontend — fail-safe rendering
- [x] Stop silently dropping unresolved comments that lack an inline position; render them (stacked at top) instead of `return null`
- [x] Flag comments whose quoted text could not be located ("⚠ couldn't locate the quoted text")

## Optional (recommended) — stable offsets
- [x] Compute the selection's character offset into the normalized rendered document in `handleCreateComment` and send `start_offset`
- [x] Use stored `start_offset` as the tiebreaker/fallback when disambiguating occurrences

## Verification
- [x] End-to-end in inner Helix: created spec task → design review; commented on a unique phrase and on the **4th** `localStorage` (inline-code, repeated). Both render; Comment B anchored to y1012 (4th occurrence at 996), NOT the 1st (265) — disambiguation via stored `start_offset` (500 and 1437) confirmed. No "couldn't locate" warnings, no console errors. Screenshot: `screenshots/01-comments-anchored-wide.png`
- [x] `tsc --noEmit` passes (0 errors); `go build ./pkg/services/` + unit test pass; API rebuilt by Air (log line numbers match edits). Note: backend auto-resolution-on-agent-push path is covered by the unit test, not driven E2E (the agent asked clarifying questions rather than editing docs). `vite build` blocked only by read-only `dist/` bind mount, unrelated to code.
