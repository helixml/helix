# Implementation Tasks: Integrate Goose AI Agent into Zed via ACP

## Phase 1 — Base runtime (US-1, US-2, US-3)

- [ ] Pin a Goose CLI version (`GOOSE_VERSION`) and install it in `Dockerfile.ubuntu-helix` via the `download_cli.sh` script with `CONFIGURE=false`
- [ ] Disable Goose telemetry/auto-update in the image (mirror the `~/.qwen/settings.json` and `~/.gemini/settings.json` pattern)
- [ ] Verify `goose --version` and `goose acp` start cleanly in a freshly built `helix-ubuntu` container
- [ ] Add `CodeAgentRuntimeGooseCode CodeAgentRuntime = "goose_code"` to `api/pkg/types/task_management.go`
- [ ] Add a `case "goose_code":` branch in `generateAgentServerConfig` in `api/cmd/settings-sync-daemon/main.go` that emits the plain `agent_servers.goose` entry with `command: "goose"`, `args: ["acp"]`, and the right env vars (`GOOSE_PROVIDER`, `GOOSE_MODEL`, provider-specific `*_API_KEY`, `*_BASE_URL` with `rewriteLocalhostURL` applied)
- [ ] Extend the frontend runtime union (`frontend/src/types.ts`, `frontend/src/contexts/apps.tsx`, regen `frontend/src/api/api.ts` via `./stack update_openapi`) to include `'goose_code'` with display name "Goose"
- [ ] Add "Goose" as a selectable runtime in `Onboarding.tsx` and `ProjectSettings.tsx` (follow the existing `qwen_code` pattern)
- [ ] Manual end-to-end test in the inner Helix: create a project with Goose runtime, open Zed, start a "Goose" thread, send a prompt, confirm a tool call executes

## Phase 2 — Custom Goose agents from project repo (US-4)

- [ ] Probe upstream: does `goose acp` accept `--recipe` or `GOOSE_RECIPE`? If not, test `goose run --recipe <file> --interactive` over stdio with an ACP client to confirm what works
- [ ] Add `ProjectAgentGoose` + `ProjectAgentGooseRecipe` types to `api/pkg/types/project.go` (fields: `RecipeRepo`, `RecipeRepoBranch`, `Recipes`) and wire into `ProjectAgentSpec.Goose`
- [ ] Extend `applyProject` in `api/pkg/server/project_handlers.go` to validate and persist the new `agent.goose` block: recipe-name uniqueness, `filepath.Clean` containment check on recipe paths, and auto-register `recipe_repo` as a `GitRepository` (same flow as primary project repos)
- [ ] Extend `CodeAgentConfig` in `api/pkg/types/types.go` with `GooseRecipes []CodeAgentGooseRecipe` and `GooseRecipeRootDir string` (absolute path inside the container)
- [ ] In `api/pkg/external-agent/zed_config.go` (`buildCodeAgentConfig`), ensure the recipe-repo clone is current, then resolve each recipe `Path` to an absolute path under that checkout (or the primary repo when `recipe_repo` is unset) and populate `CodeAgentConfig.GooseRecipes`
- [ ] In `settings-sync-daemon`, for each `CodeAgentConfig.GooseRecipes` entry, emit an additional `agent_servers.<slug>` entry using the flag confirmed in the upstream probe; export `GOOSE_RECIPE_PATH=<GooseRecipeRootDir>` on every Goose entry so sibling subrecipes/fragments resolve
- [ ] Add an annotated example block (commented out) to `examples/project.yaml` showing `agent.goose` with `recipe_repo` + `recipes`

## Phase 3 — Iteration DX & polish (US-5)

- [ ] Smoke-test the iteration loop in the inner Helix: commit a recipe to a test project, open it in Zed, edit, run `goose recipe validate`, close+reopen the thread, confirm changes take effect
- [ ] Document the recipe-iteration workflow in `docs/` (one short page: where to put recipes, how to validate, how to reload)

## Phase 4 — Follow-up

- [ ] Open a separate task to decide whether to delete `Dockerfile.sway-helix` + `desktop/sway-config/` + the experimental-desktop gate, or mirror the Goose install there
