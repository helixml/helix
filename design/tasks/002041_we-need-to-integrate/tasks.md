# Implementation Tasks: Integrate Goose AI Agent into Zed via ACP

## Phase 1 — Base runtime (US-1, US-2, US-3)

- [ ] Add `GOOSE_COMMIT=<sha>` to `sandbox-versions.txt` (pin to a `main` SHA that includes PR #8925 — verify with `grep AvailableCommand crates/goose/src/acp/server.rs` at that SHA)
- [ ] Add a goose build stage to `Dockerfile.ubuntu-helix`: clone goose at `$GOOSE_COMMIT`, `cargo build --release -p goose-cli`, copy the binary into the runtime image's `/usr/local/bin/goose`. Mirror the existing Zed build-stage pattern in the same Dockerfile
- [ ] Disable Goose telemetry/auto-update in the image (mirror the `~/.qwen/settings.json` and `~/.gemini/settings.json` pattern)
- [ ] Verify `goose --version` and `goose acp` start cleanly in a freshly built `helix-ubuntu` container
- [ ] Add `CodeAgentRuntimeGooseCode CodeAgentRuntime = "goose_code"` to `api/pkg/types/task_management.go`
- [ ] Add a `case "goose_code":` branch in `generateAgentServerConfig` in `api/cmd/settings-sync-daemon/main.go` that emits the plain `agent_servers.goose` entry with `command: "goose"`, `args: ["acp"]`, and the right env vars (`GOOSE_PROVIDER`, `GOOSE_MODEL`, provider-specific `*_API_KEY`, `*_BASE_URL` with `rewriteLocalhostURL` applied)
- [ ] Extend the frontend runtime union (`frontend/src/types.ts`, `frontend/src/contexts/apps.tsx`, regen `frontend/src/api/api.ts` via `./stack update_openapi`) to include `'goose_code'` with display name "Goose"
- [ ] Add "Goose" as a selectable runtime in `Onboarding.tsx` and `ProjectSettings.tsx` (follow the existing `qwen_code` pattern)
- [ ] Manual end-to-end test in the inner Helix: create a project with Goose runtime, open Zed, start a "Goose" thread, send a prompt, confirm a tool call executes

## Phase 2a — Slash-command recipes (US-4 v1, ships with `GOOSE_COMMIT` pinned to a `main` SHA that includes PR #8925)

### Backend (YAML path)

- [ ] Add `ProjectAgentGoose` + `ProjectAgentGooseRecipe` types to `api/pkg/types/project.go` (fields: `RecipeRepoURL`, `Recipes`) and wire into `ProjectAgentSpec.Goose`
- [ ] Extend `applyProject` in `api/pkg/server/project_handlers.go` to validate the new `agent.goose` block:
  - resolve `RecipeRepoURL` via `GetGitRepositoryByExternalURL(orgID, url)`; reject with 400 + attach instructions if not found
  - check it's attached to the project (or org-shared) — reject if not
  - reject duplicate recipe names
  - `filepath.Clean` containment check on every recipe `path`
- [ ] Extend `CodeAgentConfig` in `api/pkg/types/types.go` with `GooseRecipes []CodeAgentGooseRecipe` and `GooseRecipeRootDir string` (absolute container path)
- [ ] In `api/pkg/external-agent/zed_config.go` (`buildCodeAgentConfig`), look up `GitRepository.LocalPath` for the resolved recipe repo (or fall back to the primary repo's LocalPath when `RecipeRepoURL` is empty), join with each recipe `Path`, and populate `CodeAgentConfig.GooseRecipes`
- [ ] In `settings-sync-daemon`, when the runtime is `goose_code` and recipes are present, write a goose config file (e.g. `~/.config/goose/config.yaml`) registering each project recipe in the `slash_commands` map: `{ <slug>: { path: <absolute-recipe-path> } }`. Set `GOOSE_RECIPE_PATH=<GooseRecipeRootDir>` on the single `agent_servers.goose` entry so subrecipes/fragments resolve
- [ ] Smoke-test in Zed: open the Goose thread, press `/`, confirm the recipe names appear in autocomplete with the right descriptions; invoke one and confirm "Running recipe: …" plus the recipe's extensions activate
- [ ] Add an annotated example block (commented out) to `examples/project.yaml` showing `repositories:` + `agent.goose.recipe_repo_url` + `recipes`

### Frontend (UI path — equal-priority to YAML, can start in parallel with backend)

The UI surface is independent of upstream's recipe-loading mechanism — it just edits YAML fields and previews already-attached repos. Safe to build during the Phase 2 wait.



- [ ] In `ProjectSettings.tsx`, add a "Goose recipes" `<Card>` shown only when `runtime === 'goose_code'` (mirror the conditional rendering pattern already used for `claude_code` subscription mode)
- [ ] Build a recipe-repo picker component: dropdown of repos returned by `GET /api/v1/projects/{id}/repositories` plus a secondary list of org-scoped attached repos not yet on this project (with "attach to this project" action)
- [ ] Wire an "Attach a recipe repo" button that opens the existing `LinkExternalRepositoryDialog` (no new dialog) and pre-selects the new repo in the picker on success
- [ ] Recipe list editor: rows of (`name`, `path`) with add/remove. On save, PATCH the project's `agent.goose` config — same persistence path the YAML uses
- [ ] Round-trip test: configure recipes through the UI, export the project YAML, re-import, confirm identical state

## Phase 2b — Per-recipe agent_servers (GATED on upstream goose [#7596](https://github.com/aaif-goose/goose/issues/7596))

- [ ] Re-validate upstream status: confirm #7596 is merged into `main`; identify the flag or protocol-extension that lets `goose acp` start with a recipe pre-loaded
- [ ] Bump `GOOSE_COMMIT` to a `main` SHA that includes #7596
- [ ] Change `settings-sync-daemon` from one `agent_servers.goose` + slash commands to one `agent_servers.<slug>` per recipe with the new args (e.g. `["acp", "--recipe", "<abs-path>"]`)
- [ ] Project YAML schema and `ProjectSettings` UI stay unchanged — only the daemon's emit side changes
- [ ] Update user docs to reflect that recipes now appear as separate "New <Recipe>" entries in Zed's agent panel

## Phase 3 — Iteration DX & polish (US-5)

- [ ] Smoke-test the iteration loop in the inner Helix: commit a recipe to a test project, open it in Zed, edit, run `goose recipe validate`, restart the Goose thread (or just type `/` again to refresh the list), confirm changes take effect
- [ ] Document the recipe-iteration workflow in `docs/` (one short page: where to put recipes, how to validate, how to reload)

## Phase 4 — Follow-up

- [ ] Open a separate task to decide whether to delete `Dockerfile.sway-helix` + `desktop/sway-config/` + the experimental-desktop gate, or mirror the Goose install there
- [ ] When goose cuts a stable release that includes PR #8925, switch from source build to `download_cli.sh` install (drop the cargo build stage from `Dockerfile.ubuntu-helix`)
