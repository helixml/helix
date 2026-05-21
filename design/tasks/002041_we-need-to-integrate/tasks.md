# Implementation Tasks: Integrate Goose AI Agent into Zed via ACP

## Phase 1 — Base runtime (US-1, US-2, US-3)

- [x] Add `GOOSE_COMMIT=<sha>` to `sandbox-versions.txt` (pin to a `main` SHA that includes PR #8925 — verify with `grep AvailableCommand crates/goose/src/acp/server.rs` at that SHA)
- [x] Add a goose build stage to `Dockerfile.ubuntu-helix`: clone goose at `$GOOSE_COMMIT`, `cargo build --release -p goose-cli`, copy the binary into the runtime image's `/usr/local/bin/goose`. Mirror the existing Zed build-stage pattern in the same Dockerfile
- [x] Disable Goose telemetry/auto-update in the image (mirror the `~/.qwen/settings.json` and `~/.gemini/settings.json` pattern)
- [ ] Verify `goose --version` and `goose acp` start cleanly in a freshly built `helix-ubuntu` container *(deferred — batched with task 9 at end of Phase 1 to share one image build)*
- [x] Add `CodeAgentRuntimeGooseCode CodeAgentRuntime = "goose_code"` to `api/pkg/types/task_management.go`
- [~] Add a `case "goose_code":` branch in `generateAgentServerConfig` in `api/cmd/settings-sync-daemon/main.go` that emits the plain `agent_servers.goose` entry with `command: "goose"`, `args: ["acp"]`, and the right env vars (`GOOSE_PROVIDER`, `GOOSE_MODEL`, provider-specific `*_API_KEY`, `*_BASE_URL` with `rewriteLocalhostURL` applied)
- [ ] Extend the frontend runtime union (`frontend/src/types.ts`, `frontend/src/contexts/apps.tsx`, regen `frontend/src/api/api.ts` via `./stack update_openapi`) to include `'goose_code'` with display name "Goose"
- [ ] Add "Goose" as a selectable runtime in `Onboarding.tsx` and `ProjectSettings.tsx` (follow the existing `qwen_code` pattern)
- [ ] Manual end-to-end test in the inner Helix: create a project with Goose runtime, open Zed, start a "Goose" thread, send a prompt, confirm a tool call executes

## Phase 2a — Project-level recipe declaration + slash-command UX (US-4)

Project declares recipes; the plain "Goose" thread in Zed exposes them as slash commands for ad-hoc / exploratory use. No spec-task wiring yet.

### Pre-flight validation gates

- [ ] Confirm goose honours `GOOSE_CONFIG_PATH` to override the default config file location (read source: `crates/goose/src/config/` or similar)
- [ ] Confirm goose's config schema supports a system prompt override (search for `system_prompt`, `instructions`, or similar at the config level — if not, fall back to defining a builtin "instructions" extension)
- [ ] Confirm `slash_commands:` config map is honoured by `goose acp` (this is the PR #8925 behaviour)

### Backend (YAML path)

- [ ] Add `ProjectAgentGoose` + `ProjectAgentGooseRecipe` types to `api/pkg/types/project.go` (fields: `RecipeRepoURL`, `Recipes`) and wire into `ProjectAgentSpec.Goose`
- [ ] Extend `applyProject` in `api/pkg/server/project_handlers.go` to validate the new `agent.goose` block:
  - resolve `RecipeRepoURL` via `GetGitRepositoryByExternalURL(orgID, url)`; reject with 400 + attach instructions if not found
  - check it's attached to the project (or org-shared) — reject if not
  - reject duplicate recipe names
  - `filepath.Clean` containment check on every recipe `path`
- [ ] Extend `CodeAgentConfig` in `api/pkg/types/types.go` with `GooseRecipes []CodeAgentGooseRecipe` and `GooseRecipeRootDir string` (absolute container path)
- [ ] In `api/pkg/external-agent/zed_config.go` (`buildCodeAgentConfig`), look up `GitRepository.LocalPath` for the resolved recipe repo (or fall back to the primary repo's LocalPath when `RecipeRepoURL` is empty), join with each recipe `Path`, and populate `CodeAgentConfig.GooseRecipes`
- [ ] In `settings-sync-daemon`, write a goose config file (`~/.config/goose/config.yaml`) registering each project recipe in the `slash_commands` map: `{ <slug>: { path: <absolute-recipe-path> } }`. Set `GOOSE_RECIPE_PATH=<GooseRecipeRootDir>` on the plain `agent_servers.goose` entry so subrecipes/fragments resolve
- [ ] Smoke-test in Zed: open the Goose thread, press `/`, confirm the recipe names appear in autocomplete with the right descriptions; invoke one (with inline args like `/recipe-name arg=value`) and confirm "Running recipe: …" plus the recipe's extensions activate
- [ ] Add an annotated example block (commented out) to `examples/project.yaml` showing `repositories:` + `agent.goose.recipe_repo_url` + `recipes`

### Frontend (UI path)

- [ ] In `ProjectSettings.tsx`, add a "Goose recipes" `<Card>` shown only when `runtime === 'goose_code'` (mirror the conditional rendering pattern already used for `claude_code` subscription mode)
- [ ] Build a recipe-repo picker component: dropdown of repos returned by `GET /api/v1/projects/{id}/repositories` plus a secondary list of org-scoped attached repos not yet on this project (with "attach to this project" action)
- [ ] Wire an "Attach a recipe repo" button that opens the existing `LinkExternalRepositoryDialog` (no new dialog) and pre-selects the new repo in the picker on success
- [ ] Recipe list editor: rows of (`name`, `path`) with add/remove. On save, PATCH the project's `agent.goose` config — same persistence path the YAML uses
- [ ] Round-trip test: configure recipes through the UI, export the project YAML, re-import, confirm identical state

## Phase 2b — Per-recipe agent_servers + spec-task parameter capture (US-5)

This is the automation path: each project recipe becomes a separate "New <Recipe>" entry in Zed's agent panel for spec tasks, with parameters supplied at task creation. No mid-Zed prompting. Built on top of Phase 2a's config-baking primitives.

### Backend

- [ ] Spec-task model: add `GooseRecipeName string` and `GooseRecipeParams map[string]string` to the spec-task type and the corresponding DB column / migration
- [ ] Add `CodeAgentBakedRecipe` to `api/pkg/types/types.go` (`{ Name, Slug, SystemPrompt string, Extensions []..., Settings ... }`) and `CodeAgentConfig.GooseBakedRecipe *CodeAgentBakedRecipe`
- [ ] Recipe parser: in `api/pkg/external-agent/` add a function that loads a recipe YAML from a GitRepository mirror, parses it into a typed struct, validates schema (Goose's recipe spec)
- [ ] Recipe-schema endpoint: `GET /api/v1/projects/{id}/goose/recipes/{name}/schema` — returns the recipe's `parameters:` block in a form-schema shape the frontend can render. 404 if the project doesn't declare a recipe by that name
- [ ] Parameter substitution: at spec-task start, read the recipe + `recipe_params`, do Jinja-style `{{ var }}` substitution on `instructions:`, `prompt:`, `activities:`. Use a regex-based substitutor (recipes are documented as `{{ name }}` only — no control flow)
- [ ] `file:` parameter resolution: join the param value with the primary repo's checkout root, `filepath.Clean` containment check, read the file, substitute its **contents** (matches Goose CLI semantics)
- [ ] Populate `CodeAgentConfig.GooseBakedRecipe` with the substituted system prompt + extensions + settings
- [ ] In `settings-sync-daemon`, when `GooseBakedRecipe` is present, write `~/.config/goose/<slug>.yaml` with the baked content and emit an additional `agent_servers.<slug>` entry pointing at it via `GOOSE_CONFIG_PATH`

### Frontend

- [ ] In the spec-task creation page, when the project runtime is `goose_code` and recipes are declared, show a "Goose recipe" dropdown
- [ ] On recipe selection, fetch the recipe schema endpoint and dynamically render a form: one input per parameter, type per the mapping in US-5. Required fields marked required; optionals pre-fill `default:`
- [ ] For `file:` params, render a text input with placeholder text "repo-relative path (e.g. `src/auth/handler.go`)"; do client-side validation that the path doesn't contain `..`
- [ ] On submit, the spec-task POST body includes `goose_recipe_name` + `goose_recipe_params`

### Cleanup

- [ ] Once Phase 2b ships, document in `docs/` how to author recipes for use as spec-task agents (the difference between "available as a slash command" and "selectable as a spec-task recipe")

## Phase 2c — Revisit when upstream goose [#7596](https://github.com/aaif-goose/goose/issues/7596) lands

- [ ] Re-validate upstream status: confirm #7596 is merged into `main`; identify the flag or protocol-extension that lets `goose acp` start with a recipe pre-loaded natively
- [ ] If it materially improves on the config-baking approach (e.g. handles recipe `prompt:` auto-firing, exposes `activities:` somewhere usable, better error reporting), bump `GOOSE_COMMIT` and replace the daemon's config-baking with the native flag
- [ ] If config-baking is functionally equivalent, document the decision and skip

## Phase 3 — Iteration DX & polish (US-6)

- [ ] Smoke-test the iteration loop in the inner Helix: commit a recipe to a test project, open the project in Zed, edit the recipe YAML, run `goose recipe validate` from a terminal, close+reopen the recipe's thread (or type `/` again to refresh the slash list), confirm changes take effect
- [ ] Document the recipe-iteration workflow in `docs/` (one short page: where to put recipes, how to validate, how to reload, the difference between Phase 2a slash commands and Phase 2b spec-task agents)

## Phase 4 — Follow-up

- [ ] Open a separate task to decide whether to delete `Dockerfile.sway-helix` + `desktop/sway-config/` + the experimental-desktop gate, or mirror the Goose install there
- [ ] When goose cuts a stable release that includes PR #8925, switch from source build to `download_cli.sh` install (drop the cargo build stage from `Dockerfile.ubuntu-helix`)
