# feat(goose): Phase 1 — install Goose CLI + base runtime wiring

## Summary

Adds **Goose** (the AAIF/Block open-source AI agent) as a fourth code-agent runtime alongside `zed_agent`, `qwen_code`, and `claude_code`. Goose speaks ACP natively, so it slots into Zed's agent panel via the same `agent_servers` plumbing the other runtimes use — no new protocol work needed.

This PR ships Phase 1 (base runtime — single "Goose" thread in Zed). Phase 2 (per-recipe custom agents driven from project YAML + the spec-task creation form) will follow in a separate PR.

## Why build from `main`, not a release?

Upstream's slash-command-discovery support in `goose acp` ([aaif-goose/goose#8925](https://github.com/aaif-goose/goose/pull/8925)) merged on 2026-05-12 but is not in the latest stable v1.34.1 — release branch was cut before the merge, verified by inspecting `crates/goose/src/acp/server.rs` at the tag. Helix already pins-and-builds upstream Rust projects from specific commits (`ZED_COMMIT`, `QWEN_COMMIT`), so adding `GOOSE_COMMIT` follows the same pattern.

When goose cuts a stable release containing #8925, the cargo build stage can be dropped in favour of `download_cli.sh` (tracked as a Phase 4 follow-up task).

## Changes

- **`sandbox-versions.txt`** — pin `GOOSE_COMMIT=ca26f01d3acd9871691fa8981f05d19aed9a3b82` (current `main` HEAD, verified to contain PR #8925).
- **`Dockerfile.ubuntu-helix`** — new `goose-build` stage that clones goose at `$GOOSE_COMMIT` and `cargo build --release -p goose-cli` with Rust 1.92 + upstream's documented Linux build deps. Binary copied into the runtime image at `/usr/local/bin/goose`. BuildKit cache mounts (`/root/.cargo/registry`, `/root/.cargo/git`, `/build/target`) keep re-builds fast across commit bumps. `OTEL_SDK_DISABLED=true` added to the global `ENV` block as the telemetry kill switch.
- **`api/pkg/types/task_management.go`** — new `CodeAgentRuntimeGooseCode CodeAgentRuntime = "goose_code"` constant.
- **`api/cmd/settings-sync-daemon/main.go`** — new `case "goose_code":` in `generateAgentServerConfig` emits `agent_servers.goose` with `command: "goose"`, `args: ["acp"]`, and provider-aware env vars (`GOOSE_PROVIDER`, `GOOSE_MODEL`, plus the matching `*_API_KEY` and `*_BASE_URL`). `rewriteLocalhostURL` applied to base URLs so the daemon can reach the Helix API proxy from inside the container.
- **Frontend** — `'goose_code'` added to the runtime union in `types.ts`, `contexts/apps.tsx`, and the local types in `AppSettings.tsx`/`CodingAgentForm.tsx`. New "Goose" `MenuItem` in `CodingAgentForm.tsx` — `Onboarding.tsx`, `ProjectSettings.tsx`, `CreateProjectDialog.tsx`, and `AgentSelectionModal.tsx` all delegate to that form, so the option appears everywhere automatically.

## Test plan

- [x] `./stack build-ubuntu` succeeds with the new goose-build stage (image tag `eb09f6`)
- [x] Inside a fresh `helix-ubuntu` container: `goose --version` returns `1.35.0`; `goose acp --help` prints expected usage
- [x] Create a project in the inner Helix with code agent runtime "Goose" — selector now shows Goose alongside the existing options
- [x] Spec task with prompt "Just say hello and tell me what files exist in the current directory" — agent responds with greeting, executes a directory-listing tool call, returns the file listing
- [x] Confirm `~/.config/zed/settings.json` inside the container contains the expected `agent_servers.goose` block with `command=goose`, `args=[acp]`, and the right `GOOSE_PROVIDER` / `GOOSE_MODEL` / `ANTHROPIC_*` env vars

## Screenshots

Goose option in the runtime selector during project creation:

![Goose in runtime selector](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002041_we-need-to-integrate/screenshots/01-goose-in-runtime-selector.png)

End-to-end Goose session with a tool call executing inside Zed:

![Goose tool call](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002041_we-need-to-integrate/screenshots/04-goose-tool-call-success.png)

## Out of scope (deferred to Phase 2)

- Project YAML `agent.goose.recipe_repo_url` + `recipes` fields
- Recipe-driven per-thread agent_servers entries
- Spec-task creation form with recipe parameters
- `~/.config/goose/config.yaml` synthesis with `slash_commands` map

Spec docs (requirements / design / tasks) live in [`helix-specs/design/tasks/002041_we-need-to-integrate/`](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002041_we-need-to-integrate/).
