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
- [~] Wait for indexing to complete (poll UI; investigate any PDF extraction warnings in API logs)
- [ ] Take screenshot `screenshots/02-indexing-completed.png`
- [ ] Run a knowledge query relevant to the paper content and verify at least one result returns
- [ ] Take screenshot `screenshots/03-query-results.png`
- [ ] Commit the go.mod / go.sum change with a clear message and push
- [ ] Open a PR on `helixml/helix` linking to the kodit v1.3.7 release notes and the screenshots
- [ ] Watch CI on Drone; fix any breakage rather than papering over
