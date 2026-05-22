# Goose integration — Phase 2 (project recipes + spec-task params)

Date: 2026-05-22
Branch: `feature/002041-integrate-goose-ai-agent`
Status: implemented, ready for end-to-end smoke

## What shipped in Phase 1 (recap)

- `goose` CLI installed into the desktop image, ACP-spawnable from the
  Zed external agent runtime (`code_agent_runtime: goose_code`).
- Settings-sync-daemon writes a base `${XDG_CONFIG_HOME}/goose/config.yaml`
  per session so goose picks up provider + model selected on the agent.
- No project-level config — every session was vanilla goose.

## What Phase 2 adds

End-to-end story:

```
 project.yaml                                                  Goose thread
 ┌────────────────────┐    ┌──────────────────┐    ┌────────────────────┐
 │ spec.agent.goose:  │    │ ProjectGooseRecipe│    │ /triage  ← slash    │
 │   recipe_repo_url  │──▶ │  on AssistantConf │──▶ │ /fix-flaky-test    │
 │   recipes: [...]   │    │                  │    │                    │
 └────────────────────┘    └──────────────────┘    └────────────────────┘
            │                       │                        ▲
            │                       │                        │
            │            ┌──────────┴──────────┐             │
            │            │ goose_recipes_      │             │
            │            │  handlers.go        │             │
            │            │ GET /v1/projects/   │ Spec-task   │
            │            │   {id}/goose-recipes│ form lists  │
            │            └──────────┬──────────┘ them ───────┘
            │                       │
            │                       ▼
            │            ┌────────────────────┐
            │            │ Spec-task creation │  user picks
            │            │  ── stores         │  /triage + fills
            │            │ GooseRecipeName    │  ci_url, branch
            │            │ GooseRecipeParams  │
            │            └──────────┬─────────┘
            ▼                       ▼
 ┌────────────────────┐  ┌──────────────────────┐
 │ zed_config_handlers│  │ applySpecTaskGooseRec│
 │ resolveGooseRecipes│  │  reads recipe, bakes │
 │ Into Config        │  │  params via          │
 │  → CodeAgentConfig │  │  goose.Bake(),       │
 │  GooseRecipes []   │  │  attaches as         │
 │                    │  │  GooseBakedRecipe    │
 └─────────┬──────────┘  └──────────┬───────────┘
           │                        │
           └────────────┬───────────┘
                        ▼
            ┌──────────────────────┐
            │ settings-sync-daemon │  on session start
            │  writeGooseConfig()  │
            │   → baked-recipes/   │
            │     <name>.yaml      │
            │   → goose.yaml       │
            │     slash_commands[] │
            └──────────────────────┘
```

## File map

| Layer | File | What it does |
|---|---|---|
| Types | `api/pkg/types/project.go` | `ProjectAgentGoose` on `ProjectAgent`; YAML → ProjectSpec round-trip |
| Types | `api/pkg/types/types.go` | `CodeAgentConfig.GooseRecipes []GooseRecipeRef`, `GooseBakedRecipe` |
| Types | `api/pkg/types/simple_spec_task.go` | `SpecTask.GooseRecipeName`/`Params`; `CreateTaskRequest.GooseRecipeName`/`Params` |
| Project apply | `api/pkg/server/project_handlers.go` | Validates `spec.agent.goose.recipes`, persists onto `AssistantConfig` |
| Recipe parser | `api/pkg/goose/recipe.go` | `Parse(yaml)`, `Bake(yaml, params)` — Jinja `{{var}}` substitution + required-param enforcement |
| Recipe parser tests | `api/pkg/goose/recipe_test.go` | covers required/optional/default, missing/extra params, malformed YAML |
| Recipes API | `api/pkg/server/goose_recipes_handlers.go` | `GET /v1/projects/{id}/goose-recipes` returns parsed recipe schema for the project's default agent |
| Zed config | `api/pkg/server/zed_config_handlers.go` | `resolveGooseRecipesIntoConfig` (project recipes) + `applySpecTaskGooseRecipe` (spec-task baked recipe) |
| Service | `api/pkg/services/spec_driven_task_service.go` | Threads `GooseRecipeName/Params` from create request into `SpecTask` row |
| Daemon | `api/cmd/settings-sync-daemon/main.go` | `writeGooseConfig` materializes recipes onto disk and registers slash_commands |
| Frontend | `frontend/src/components/app/GooseRecipesEditor.tsx` | Project settings UI for managing the recipe list on the agent |
| Frontend | `frontend/src/components/tasks/GooseRecipeSelector.tsx` | Spec-task form: recipe dropdown + dynamic parameter form |
| Frontend | `frontend/src/components/tasks/NewSpecTaskForm.tsx` | Wires `GooseRecipeSelector` in when selected agent is `goose_code` |
| Example | `examples/project_goose.yaml`, `examples/goose_recipes/triage.yaml` | Reference YAML + recipe |

## Design choices worth knowing

**Recipes are agent-scoped, not session-scoped.** A project may have
several agents; recipes only apply to the goose ones. The schema endpoint
and the spec-task selector both gate on the project's default agent
being `goose_code` (the selector is rendered only when the user picks a
goose agent in the form, regardless of default).

**URLs, not internal IDs, identify the recipe repo.** `recipe_repo_url`
on the agent config matches against one of the project's attached
repositories by upstream URL — same convention used elsewhere for repo
references, so project YAML is portable across Helix instances.

**Recipes that fail to load surface as warnings, not errors.** If the
declared file is missing or YAML-malformed, the schema endpoint returns
the recipe with an `error` field set; the selector UI lists them in a
warning alert so the user can fix project YAML without the whole agent
becoming unusable. `resolveGooseRecipesIntoConfig` does the same on the
session-start path.

**Spec-task params bake at task-creation time, not at session start.**
The `GooseRecipeParams` map is captured into the SpecTask row. When the
session config is built (`getZedConfig`), `applySpecTaskGooseRecipe`
reads the recipe file from disk and calls `goose.Bake` to substitute
`{{var}}` placeholders into both `prompt` and `instructions`, then
attaches the result as `CodeAgentConfig.GooseBakedRecipe`. The daemon
writes that baked recipe into `${XDG}/goose/baked-recipes/<name>.yaml`
and registers a slash command — so the agent sees a ready-to-run
slash command with the user's values pre-filled.

**Why bake instead of writing the recipe and letting goose substitute?**
Goose's runtime Jinja substitution prompts the user for values
interactively. We want the spec-task parameter form (already filled out
in the Helix UI) to be the source of truth — so we substitute first and
hand goose a recipe with no remaining `{{var}}` references.

**Vanilla goose stays an option.** Empty `goose_recipe_name` on the
create request means no `GooseBakedRecipe` is attached and no extra
slash command is registered. The user still sees the project-level
slash commands declared on the agent.

**XDG isolation.** Goose uses etcetera/XDG (no `GOOSE_CONFIG_PATH`),
so each session gets `XDG_CONFIG_HOME=/home/retro/.config/helix-goose`
to keep its config out of the user-level `~/.config/goose`.

## Smoke-test checklist

The end-to-end loop to verify by hand:

1. Create a project in the inner Helix pointing at a repo containing
   `.goose/recipes/triage.yaml` (the `examples/goose_recipes/triage.yaml`
   format).
2. In project settings, set the assistant's runtime to `goose_code`.
3. Add a recipe entry via the GooseRecipesEditor (or update project.yaml
   directly): `name: triage`, `path: .goose/recipes/triage.yaml`.
4. Save. Confirm the API returns the parsed schema:
   `GET /api/v1/projects/{id}/goose-recipes` → `[{name: "triage",
   title: "Triage failing CI", parameters: [...]}]`.
5. New spec task → pick the goose agent → confirm dropdown lists
   `/triage — Triage failing CI`. Pick it, fill in `ci_url` and `branch`.
6. Submit. Wait for the session to come up.
7. In the Goose thread, run `/triage` → confirm the prompt body comes
   through with parameters substituted, no interactive prompts for
   `{{ ci_url }}`.

## Known gaps / out-of-scope

- No recipe-author UX inside Helix — recipes live in the repo and are
  edited via the normal git flow. (Considered scope creep for v1.)
- No retry/cache for the schema endpoint — it re-reads recipe files on
  every request. Fine at current volumes; revisit if it becomes hot.
- Daemon does not re-render the recipe if the user edits parameter
  values mid-session. Restart the session if you want different values.
