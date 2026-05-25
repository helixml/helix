# feat(goose): integrate Goose agent + project recipes + spec-task parameter capture

## Summary

Adds **Goose** (the AAIF/Block open-source AI coding agent) as a fourth code-agent runtime alongside `zed_agent`, `qwen_code`, and `claude_code`. Goose speaks ACP natively, so it slots into Zed's agent panel via the same `agent_servers` plumbing the other runtimes use — no new protocol work needed.

This PR ships both phases of the integration:

- **Phase 1** — install the Goose CLI in the desktop image and wire it into the runtime selector, giving every Goose project a single "Goose" thread in Zed.
- **Phase 2** — project YAML declares a recipe library; the spec-task creation form renders a parameter capture UI per recipe; Helix bakes captured values into a per-task `agent_servers.<slug>` entry so the agent starts with a ready-to-run slash command. No free-text prompting, no interactive back-and-forth to gather context.

Sister docs PR: [helixml/helix-next#96](https://github.com/helixml/helix-next/pull/96).

## Why build from `main`, not a release?

Upstream's slash-command-discovery support in `goose acp` ([aaif-goose/goose#8925](https://github.com/aaif-goose/goose/pull/8925)) merged on 2026-05-12 but is not in the latest stable v1.34.1 — release branch was cut before the merge, verified by inspecting `crates/goose/src/acp/server.rs` at the tag. Helix already pins-and-builds upstream Rust projects from specific commits (`ZED_COMMIT`, `QWEN_COMMIT`), so adding `GOOSE_COMMIT` follows the same pattern.

When goose cuts a stable release containing #8925, the cargo build stage can be dropped in favour of `download_cli.sh` (tracked as a Phase 4 follow-up task).

## Changes

### Phase 1 — base runtime

- **`sandbox-versions.txt`** — pin `GOOSE_COMMIT=ca26f01d3acd9871691fa8981f05d19aed9a3b82` (current `main` HEAD, verified to contain PR #8925).
- **`Dockerfile.ubuntu-helix`** — new `goose-build` stage that clones goose at `$GOOSE_COMMIT` and `cargo build --release -p goose-cli` with Rust 1.92 + upstream's documented Linux build deps (now including `libclang-dev` and `cmake` for the `llama-cpp-sys` transitive dep). Binary copied into the runtime image at `/usr/local/bin/goose`. BuildKit cache mounts (`/root/.cargo/registry`, `/root/.cargo/git`, `/build/target`) keep re-builds fast across commit bumps. `OTEL_SDK_DISABLED=true` added to the global `ENV` block as the telemetry kill switch.
- **`api/pkg/types/task_management.go`** — new `CodeAgentRuntimeGooseCode CodeAgentRuntime = "goose_code"` constant.
- **`api/cmd/settings-sync-daemon/main.go`** — new `case "goose_code":` in `generateAgentServerConfig` emits `agent_servers.goose` with `command: "goose"`, `args: ["acp"]`, and provider-aware env vars (`GOOSE_PROVIDER`, `GOOSE_MODEL`, plus the matching `*_API_KEY` and `*_BASE_URL`). `rewriteLocalhostURL` applied to base URLs so the daemon can reach the Helix API proxy from inside the container.
- **Frontend (Vite SPA)** — `'goose_code'` added to the runtime union in `types.ts`, `contexts/apps.tsx`, and the local types in `AppSettings.tsx` / `CodingAgentForm.tsx`. New "Goose" `MenuItem` in `CodingAgentForm.tsx` — `Onboarding.tsx`, `ProjectSettings.tsx`, `CreateProjectDialog.tsx`, and `AgentSelectionModal.tsx` all delegate to that form, so the option appears everywhere automatically.

### Phase 2 — project recipes + spec-task parameter capture

- **`api/pkg/types/project.go`** — new `ProjectAgentGoose` block on the project agent config: `recipe_repo_url` (optional — defaults to the primary repo when omitted) and `recipes: [{name, path}]`. `applyProject` validates names are unique, paths are repo-relative, and the referenced repo is attached to the project.
- **`api/pkg/goose/recipe.go` + `recipe_test.go`** — recipe parser. Reads the YAML at the declared path, validates `version`/`title`/`prompt`/`parameters[]`, substitutes Jinja-style `{{ var }}` placeholders, and returns the rendered prompt/instructions. Required vs optional + `default` handling, plus structural validation that surfaces a warning per missing/malformed recipe rather than failing the whole agent.
- **`api/pkg/server/goose_recipes_handlers.go`** — new `GET /api/v1/apps/:app_id/goose-recipes/schema` endpoint exposes the parameter schema to the frontend so the spec-task form can render the right input per recipe.
- **`api/pkg/server/zed_config_handlers.go`** — when a spec task carries a `CodeAgentBakedRecipe`, the daemon writes a per-task `agent_servers.<slug>` entry (in addition to the bare `agent_servers.goose`) so Goose picks up the baked recipe at thread start. Slash-command discovery in `~/.config/goose/config.yaml` `slash_commands:` map advertises every project recipe for ad-hoc interactive use.
- **`api/pkg/types/simple_spec_task.go`** — `CodeAgentBakedRecipe` field on the spec task captures the recipe name and the user's parameter values at task creation time; serialised onto the row, picked up by the daemon at session start. File-typed parameters are stored as bare filenames; the daemon rewrites them to absolute paths inside the agent's workspace (`/workspace/.helix-attachments/<filename>`) before handing the recipe to Goose.
- **`api/pkg/services/spec_driven_task_service.go`** — attachment-write path resolves file-typed recipe params against the spec-task's `pendingAttachments`, so the agent sees an ordinary file path it can `Read` with normal tool calls (no inline embedding, no size limit beyond the attachment limit itself).
- **Frontend (Vite SPA)** —
  - `GooseRecipesEditor.tsx` — declarative recipe table in `ProjectSettings` for adding/removing recipe entries.
  - `GooseRecipeSelector.tsx` — recipe dropdown + dynamic per-recipe parameter form on `NewSpecTaskForm`. Selecting a recipe fetches its schema via the new endpoint and renders one input per declared parameter (string, select, file). The file-input branch reads from `pendingAttachments` rather than a separate upload widget — picking a file there immediately populates the dropdown.
  - Plumbed `pendingAttachments` from `NewSpecTaskForm` through to the selector so the file picker has the freshly-staged attachments without a refresh.

### Examples + docs

- **`examples/project_goose.yaml`** — minimal runnable project that wires up Goose with `recipe_repo_url`, a `recipes:` list, and the matching code-agent runtime.
- **`examples/goose_recipes/`** — six runnable starter recipes covering all parameter types: `triage.yaml` (two required strings), `release-notes.yaml` (strings + default + `select`), `review-spec.yaml` (file parameter), `fix-flaky-test.yaml`, `triage-error-log.yaml`, `implement-from-spec.yaml`. Each is self-contained and can be copied into a downstream project as-is.
- **`design/2026-05-22-goose-integration-phase-2.md`** — design note covering the recipe baking model, the per-task `agent_servers.<slug>` synthesis, and the file-parameter attachment-resolution flow.

### Fixes + housekeeping

- **`api/pkg/server/zed_config_handlers_test.go`** — updated for the new `projectID` arg on `buildCodeAgentConfig` (resolves the CI failure on this PR).
- **`api/pkg/server/swagger.{json,yaml}`, `swagger.json`, `openapi.json`, `frontend/swagger/swagger.yaml`** — regenerated for the new goose-recipes endpoint and `ProjectAgentGoose` types.

## Test plan

- [x] `./stack build-ubuntu` succeeds with the new goose-build stage (image tag `eb09f6`, bumped after libclang/cmake fix)
- [x] Inside a fresh `helix-ubuntu` container: `goose --version` returns `1.35.0`; `goose acp --help` prints expected usage
- [x] Create a project in the inner Helix with code-agent runtime "Goose" — selector now shows Goose alongside the existing options
- [x] Phase 1 smoke: spec task with prompt "Just say hello and tell me what files exist in the current directory" — agent responds, executes a directory-listing tool call, returns the file listing
- [x] Confirm `~/.config/zed/settings.json` inside the container contains the expected `agent_servers.goose` block with `command=goose`, `args=[acp]`, and the right `GOOSE_PROVIDER` / `GOOSE_MODEL` / `ANTHROPIC_*` env vars
- [x] Phase 2 smoke: attach `examples/goose_recipes/triage.yaml` to a Goose project; the spec-task creation form's **Goose Recipe** dropdown lists it; selecting it renders `ci_url` + `branch` text inputs; submitting bakes the values into `~/.config/zed/settings.json` `agent_servers.triage` and the agent starts with the rendered prompt — no free-text re-entry
- [x] File-input recipe (`review-spec.yaml`): stage a file in the form's **Attachments** section; the `spec_doc` dropdown is populated from staged attachments; at session start the parameter resolves to the absolute path under `/workspace/.helix-attachments/`; agent `Read`s it with a normal tool call
- [x] Slash-command discovery: `/triage`, `/release-notes`, … available inside the Goose thread for interactive use (independent of the per-task baked recipe)
- [x] CI green on this PR after the `buildCodeAgentConfig` test fix

## Screenshots

Goose option in the runtime selector during project creation:

![Goose in runtime selector](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002041_we-need-to-integrate/screenshots/01-goose-in-runtime-selector.png)

End-to-end Goose session with a tool call executing inside Zed:

![Goose tool call](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002041_we-need-to-integrate/screenshots/04-goose-tool-call-success.png)

Spec-task form screenshots showing the recipe parameter UI are embedded in the [helix-next docs PR](https://github.com/helixml/helix-next/pull/96) (and the new `/docs/goose` page) at 2x DPI: dropdown, two-string `triage`, string+default+select `release-notes`, and file-parameter `review-spec`.

## Out of scope (Phase 4 follow-up)

- Drop the cargo build stage once goose cuts a stable release containing PR #8925 — replace with `download_cli.sh` like Zed/Qwen.
- In-Helix recipe editor (recipes are authored in your normal repo review flow; the project UI is read-only declarative).

Spec docs (requirements / design / tasks) live in [`helix-specs/design/tasks/002041_we-need-to-integrate/`](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002041_we-need-to-integrate/).
