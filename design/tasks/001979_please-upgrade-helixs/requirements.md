# Requirements

## Background

Helix uses [helixml/kodit](https://github.com/helixml/kodit) as a Go library (in-process, not a separate container) for code intelligence and knowledge indexing. The helix backend currently pins `github.com/helixml/kodit v1.3.6` in `go.mod`. Upstream has released `v1.3.7` (2026-05-01), which contains 9 PRs of bug fixes and improvements — notably a swap of the PDF text extractor to PDFium and better error handling for PDF extraction failures.

## User Story

As a Helix maintainer, I want to upgrade the bundled Kodit library from v1.3.6 to v1.3.7 so that improvements (especially PDF extraction reliability and BM25 storage refactor) reach Helix users, and confirm the Knowledge feature in agents indexes and queries an arxiv PDF without regression.

## Acceptance Criteria

1. `api/go.mod` declares `github.com/helixml/kodit v1.3.7`.
2. `api/go.sum` contains matching `v1.3.7` hashes.
3. `go build ./...` succeeds with no compile errors after the bump.
4. `go vet ./...` reports no new issues from the bump.
5. The Helix dev stack (`docker compose -f docker-compose.dev.yaml`) starts and the API container reports a healthy startup with no kodit-related errors in the first 30 s of logs.
6. Manual UI test passes: in the inner Helix at `http://localhost:8080`, navigate to **Agents → (an agent) → Knowledge**, add a knowledge source pointing to `https://arxiv.org/pdf/2604.25927v1`, wait for indexing to complete, then run a knowledge query and confirm at least one relevant result is returned.
7. Screenshots are captured for: (a) knowledge source added, (b) indexing completed, (c) query returning results — saved under `screenshots/` in this task folder.
8. Any breakage discovered during build/test is either fixed in the same PR or, if it requires deeper work, documented as a follow-up issue rather than papered over.

## Out of Scope

- Bumping the kodit version in any vendored or pinned location *other* than helix's `api/go.mod` / `api/go.sum`. (The `.drone.yml` E2E test step at line 1808 already syncs the kodit version automatically — no manual change needed.)
- Refactoring helix's kodit integration code.
- Changes to the Knowledge UI itself.

## Reference

- Kodit v1.3.7 release notes: https://github.com/helixml/kodit/releases/tag/v1.3.7
- Test PDF: https://arxiv.org/pdf/2604.25927v1
