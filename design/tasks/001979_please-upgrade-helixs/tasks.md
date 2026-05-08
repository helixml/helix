# Implementation Tasks

- [x] Edit `go.mod` (repo root, line 45): change `github.com/helixml/kodit v1.3.6` to `v1.3.7`
- [x] Run `go mod tidy` to update `go.sum`
- [x] Verify the build: `CGO_ENABLED=1 go build` of kodit-importing packages (services, server, controller/knowledge, rag) — full `./...` requires pkg-config + glib for unrelated go-gst, skipped
- [x] Restart the inner Helix API container: `docker compose -f docker-compose.dev.yaml restart api`
- [x] Tail API logs for ~30 s and confirm no kodit-related errors at startup — saw "Using Kodit for RAG" + "Registered MCP backend kodit", no errors. KODIT_ENABLED=false in dev (in-process kodit disabled), but RAG/MCP path uses kodit fine
- [x] Open `http://localhost:8080` in Chrome MCP, register/login as `test@helix.ml` / `helixtest`, complete onboarding (create org)
- [x] Navigate to Agents → create or open an agent → Knowledge tab
- [x] Add a knowledge source with URL `https://arxiv.org/pdf/2604.25927v1`
- [x] Take screenshot `screenshots/01-knowledge-source-added.png`
- [x] Switch from web URL to file upload (web URL errored, user requested file upload as the test path)
- [x] Enable Kodit (`KODIT_ENABLED=true` in `.env`) — was disabled by default in dev compose; recreated API container
- [x] Re-trigger indexing via "Refresh knowledge and reindex data" button
- [x] Wait for indexing to complete — status went `error` → `indexing` → `ready`. Kodit v1.3.7 produced 27 code embeddings + 13 page image embeddings via PDFium extractor. No errors.
- [x] Take screenshot `screenshots/03-indexing-ready.png`
- [x] Switched the Optimus agent's 4 model slots to `claude-haiku-4-5-20251001` (default `claude-opus-4-6` had no runner; unrelated to kodit)
- [x] Run a knowledge query relevant to the paper content — agent invoked `KnowledgeQuery` tool against the indexed PDF (proves end-to-end Knowledge tool works against kodit v1.3.7-indexed data)
- [x] Take screenshot `screenshots/05-knowledge-query-working.png`
- [x] Commit the go.mod / go.sum change with a clear message and push to feature branch
- [x] CI runs on the public `helixml/helix` repo only after the platform creates the GitHub PR from this push (no Drone creds in this inner Helix env). User opens PR via the spec UI; CI status will surface there.

Notes:
- The Optimus agent's onboarding-defaulted models (`claude-opus-4-6`) are not registered with any runner in dev — got 500 "no runner has model". Switched to `claude-haiku-4-5-20251001` (the same model the system uses for chat default) and the agent successfully invoked the `KnowledgeQuery` tool against the kodit-indexed PDF. This is unrelated to the kodit upgrade itself.
- Web URL knowledge sources (HTTP scraping path) errored on arxiv.org redirect handling — unrelated to kodit, may want to investigate separately. File upload path is the supported route for PDFs.
