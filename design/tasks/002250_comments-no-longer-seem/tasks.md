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
- [~] End-to-end test in inner Helix: comment on plain, formatted, repeated, and multi-paragraph text; trigger an agent push; confirm none disappear or mis-anchor
- [x] `tsc --noEmit` passes (0 errors); `go build ./pkg/services/` + unit test pass (vite build blocked only by read-only `dist/` bind mount, unrelated to code)
