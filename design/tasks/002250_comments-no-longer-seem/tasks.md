# Implementation Tasks: Fix Spec Review Comments Disappearing and Mis-Anchoring

## Backend — conservative auto-resolution
- [x] Add a normalization helper (collapse whitespace runs to single space; strip common inline markdown tokens) in `api/pkg/services/git_http_server.go`
- [x] Update `checkCommentResolution` to compare normalized `quoted_text` against the normalized document instead of raw `strings.Contains`
- [x] Ensure auto-resolution fails safe: keep the comment when the anchor can't be confidently matched
- [x] Add a Go unit test: comment on markdown-formatted text is NOT auto-resolved; comment on genuinely removed text IS auto-resolved

## Frontend — robust anchoring
- [~] In `findQuotedTextPosition`, search `markdownRef.current` (rendered markdown) instead of `documentRef.current`
- [~] Normalize whitespace on both the searched text and `quoted_text` before matching so cross-block selections resolve
- [~] Disambiguate repeated phrases so a bubble anchors to the intended occurrence

## Frontend — fail-safe rendering
- [~] Stop silently dropping unresolved comments that lack an inline position; guarantee they remain visible in `CommentLogSidebar`
- [~] Flag comments whose quoted text could not be located ("couldn't locate quoted text")

## Optional (recommended) — stable offsets
- [ ] Compute the selection's character offset into the normalized rendered document in `handleCreateComment` and send `start_offset`
- [ ] Use stored `start_offset` as the tiebreaker/fallback when disambiguating occurrences

## Verification
- [ ] End-to-end test in inner Helix: comment on plain, formatted, repeated, and multi-paragraph text; trigger an agent push; confirm none disappear or mis-anchor
- [ ] `cd frontend && yarn build` passes; `go build ./pkg/services/ ./pkg/server/` passes
