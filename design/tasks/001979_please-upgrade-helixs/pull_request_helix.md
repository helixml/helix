# Bump kodit to v1.3.7

## Summary

Upgrades the in-process [helixml/kodit](https://github.com/helixml/kodit) Go library from v1.3.6 to v1.3.7. The release contains 9 PRs of bug fixes and internal refactors — notably a swap of the PDF text extractor to PDFium (#562) with empty-content warnings (#561), a deduplication fix for repos with >1000 enrichments (#557), and a unified BM25/embedding storage refactor (#560). No breaking API changes; helix's 12 kodit-importing files compile and run without modification.

## Changes

- `go.mod`: `github.com/helixml/kodit v1.3.6` → `v1.3.7`
- `go.sum`: regenerated via `go mod tidy`

## Verification

- `CGO_ENABLED=1 go build ./api/pkg/services/... ./api/pkg/server/... ./api/pkg/controller/knowledge/... ./api/pkg/rag/...` passes
- API container restarts cleanly with `KODIT_ENABLED=true`; logs show kodit v1.3.7 worker polling and "Using Kodit for RAG"
- End-to-end manual test: uploaded the arxiv paper [2604.25927v1](https://arxiv.org/pdf/2604.25927v1) (629 KB, 7 pages) as a Files knowledge source on an agent. Indexing went `preparing` → `indexing` → `ready`; kodit produced 27 code embeddings + 13 page-image embeddings via the new PDFium extractor with no PDF-extraction warnings. The agent then successfully invoked the `Knowledge_arxiv_pdf` skill (a `KnowledgeQuery` tool call) against the indexed content.

The `.drone.yml` E2E pipeline at line 1808 syncs the kodit version into the Zed WS test server's `go.mod` automatically — no manual sync needed.

## Screenshots

![Knowledge source ready after kodit v1.3.7 indexing](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001979_please-upgrade-helixs/screenshots/03-indexing-ready.png)

![Agent calling KnowledgeQuery against the kodit-indexed PDF](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001979_please-upgrade-helixs/screenshots/05-knowledge-query-working.png)
