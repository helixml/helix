# Design

## Summary

This is a dependency bump. Kodit is consumed by Helix as a Go library imported under several `github.com/helixml/kodit/domain/...`, `application/service`, and `infrastructure/provider` packages (12 helix files import it — see findings below). The release notes for v1.3.7 indicate **no breaking API changes** — all 9 PRs are bug fixes, internal refactors, or doc updates. So the upgrade should be a one-line change in `go.mod` followed by `go mod tidy`.

## Current State (findings from exploration)

- `api/go.mod:45` → `github.com/helixml/kodit v1.3.6`
- `api/go.sum:543-544` → matching v1.3.6 hashes
- 12 Go files in `api/` import kodit packages (controller/knowledge, server/kodit_*, services/kodit_*, rag/rag_kodit, etc.)
- Kodit runs in-process; Helix's docker-compose only spins up `vectorchord-kodit` (Postgres + pgvector) as the storage backend — there is no separate kodit container image to bump
- `.drone.yml:1808-1813` syncs the kodit version from `/drone/src/go.mod` into the E2E test server's `go.mod` automatically — no manual sync needed
- Helm chart `charts/helix-controlplane/values.yaml` references kodit only for config (db, enrichment, vectorchord) — no version pin
- `frontend/` contains kodit admin UI and status pills but no version coupling

## Approach

1. **Edit `api/go.mod`** — change `github.com/helixml/kodit v1.3.6` → `v1.3.7`.
2. **Run `cd api && go mod tidy`** to regenerate `go.sum` with v1.3.7 hashes. (CGO not required for tidy.)
3. **Build verification:** `cd api && CGO_ENABLED=1 go build ./...` to catch any compile breakage from the new version (kodit pulls in tree-sitter; CGO is required).
4. **Restart the dev stack** so the new library code is picked up: `docker compose -f docker-compose.dev.yaml restart api`. Air will rebuild with the new dependency.
5. **Tail API logs** for 30 s to confirm no kodit-related errors at startup.
6. **Manual UI test** via Chrome DevTools MCP:
   - Open `http://localhost:8080`, register/login as `test@helix.ml` / `helixtest` per CLAUDE.md.
   - Complete onboarding (create org).
   - Navigate to Agents, create or open an agent, open the Knowledge tab.
   - Add a knowledge source with URL `https://arxiv.org/pdf/2604.25927v1`.
   - Wait for indexing to complete (poll the UI; PDF extraction in v1.3.7 uses PDFium and surfaces errors instead of silently continuing).
   - Run a knowledge query (e.g., a phrase from the paper title/abstract once visible) and verify results return.
   - Capture screenshots at each milestone.
7. **If indexing fails:** check API logs for the new "PDF extraction failure" warnings introduced in kodit#561/#562. These should now surface real errors rather than silent empty-content. Investigate root cause; do not work around.

## Key Decisions

- **Why not bump anything else?** The `.drone.yml` script at line 1808 dynamically reads the kodit version from helix's go.mod and syncs it into the Zed E2E test server's go.mod at CI time. There is no second pin to update.
- **Why CGO required for build?** CLAUDE.md notes tree-sitter requires CGO for any package under `pkg/server`. Verified locally with `CGO_ENABLED=1`.
- **Why test via the inner Helix browser?** This is a spec task agent environment (helix-in-helix). The inner instance at port 8080 is the real running stack; testing there is the closest to a production smoke test.
- **Why arxiv PDF specifically?** v1.3.7 specifically swaps the PDF text extractor (PR #562) and adds an inline-trailer regression test (PR #563). An arxiv PDF is a real-world, non-trivial document that exercises the new path.

## Risk / Rollback

- **Risk:** A subtle behavior change in kodit's storage refactor (PR #560 unifies BM25 and embedding repositories) could break existing indexed knowledge. Mitigation: test against an existing repo too if time permits.
- **Rollback:** Revert the single-line go.mod change and `go mod tidy` again; no schema migrations were called out in the release notes.

## Notes for Future Agents (Helix-in-Helix discoveries)

- Kodit is **not** a separate container in Helix — it is a Go library compiled into the controlplane binary. The docker-compose `vectorchord-kodit` service is just Postgres+pgvector for kodit's storage.
- The `.drone.yml` E2E pipeline auto-syncs kodit versions between helix and the Zed WS test server. Do not manually edit the E2E server's `go.mod`.
- Kodit imports are under `github.com/helixml/kodit/{application,domain,infrastructure}/...` — 12 files in helix's `api/` use them.
- Inner Helix dev creds: `test@helix.ml` / `helixtest` (per CLAUDE.md). Register first; first user becomes admin.
