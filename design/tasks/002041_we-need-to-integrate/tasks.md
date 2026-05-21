# Implementation Tasks: Integrate Goose AI Agent into Zed via ACP

- [ ] Pin a Goose CLI version (`GOOSE_VERSION`) and install it in `Dockerfile.ubuntu-helix` via the `download_cli.sh` script with `CONFIGURE=false`
- [ ] Disable Goose telemetry/auto-update in the image (mirror the `~/.qwen/settings.json` and `~/.gemini/settings.json` pattern)
- [ ] Verify `goose --version` and `goose acp` start cleanly in a freshly built `helix-ubuntu` container
- [ ] Add `CodeAgentRuntimeGooseCode CodeAgentRuntime = "goose_code"` to `api/pkg/types/task_management.go`
- [ ] Add a `case "goose_code":` branch in `generateAgentServerConfig` in `api/cmd/settings-sync-daemon/main.go` that emits `agent_servers.goose` with `command: "goose"`, `args: ["acp"]`, and the right env vars (`GOOSE_PROVIDER`, `GOOSE_MODEL`, provider-specific `*_API_KEY`, `*_BASE_URL` with `rewriteLocalhostURL` applied)
- [ ] Extend the frontend runtime union (`frontend/src/types.ts`, `frontend/src/contexts/apps.tsx`, `frontend/src/api/api.ts` via `./stack update_openapi`) to include `'goose_code'` with display name "Goose"
- [ ] Add "Goose" as a selectable runtime in `Onboarding.tsx` and `ProjectSettings.tsx` (follow the existing `qwen_code` pattern)
- [ ] Manual end-to-end test in the inner Helix: create a project with Goose runtime, open Zed, start a "Goose" thread, send a prompt, confirm a tool call executes
- [ ] Update `api/cmd/settings-sync-daemon/main.go` doc comment block listing the supported runtimes
- [ ] Open a follow-up task: decide whether to delete `Dockerfile.sway-helix` + `desktop/sway-config/` + the experimental-desktop gate, or mirror the Goose install there
