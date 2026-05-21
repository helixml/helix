# Design: Integrate Goose AI Agent into Zed via ACP

## Summary

Add Goose as a fourth code-agent runtime alongside `zed_agent`, `qwen_code`,
`claude_code` (and the stubbed `gemini_cli` / `codex_cli`). The integration
is a thin wrapper: install the `goose` CLI into the desktop image, then
have `settings-sync-daemon` write the right `agent_servers.goose` block
into Zed's settings.json for users who choose the Goose runtime.

## Architecture

```
Helix API (chosen runtime, provider, model, API key)
        │
        │  WebSocket → settings-sync-daemon (in helix-ubuntu container)
        ▼
~/.config/zed/settings.json
        │
        ▼
Zed  ──spawns──▶  goose acp  ──ACP/stdio──▶  Goose agent
                       │
                       └──▶ LLM provider (via env: GOOSE_PROVIDER, GOOSE_MODEL,
                                         OPENAI_BASE_URL, OPENAI_API_KEY, …)
```

This mirrors the Qwen path exactly. The only difference is the binary
(`goose acp` vs `qwen --experimental-acp`) and the env-var names.

## Key Decisions

### D1: Pre-install `goose` in `Dockerfile.ubuntu-helix`, not at runtime

Goose's recommended install is `curl … | bash`, which fetches a binary.
Doing that at session start would add a download to every cold start and
make sessions fail when GitHub is unreachable. Bake the binary into the
image, pinned via `GOOSE_VERSION`, the same way we pin every other
upstream version in the Dockerfile.

### D2: Add a new runtime constant `goose_code` — don't overload existing runtimes

`CodeAgentRuntime` is a string enum in `api/pkg/types/task_management.go`.
The settings-sync-daemon switches on it. Adding `goose_code` keeps the
existing four runtimes untouched and is symmetric with the prior pattern.

### D3: Use Zed's `type: "custom"` agent_server entry — not `type: "registry"`

`claude-acp` uses `type: "registry"` because it's a real entry in Zed's
extension registry. Goose isn't (yet). The Qwen entry uses
`type: "custom"`, which is the right precedent for a CLI-on-disk.

### D4: Forward LLM config via `GOOSE_PROVIDER` + `GOOSE_MODEL` env vars

Per Goose docs, env vars override the user's Goose config file. That's
exactly the right hook for Helix — we already have the user's chosen
provider/model in `CodeAgentConfig`, and we never want a session to use
the random provider Goose was last configured with.

| Helix `APIType` | `GOOSE_PROVIDER` | API-key env var       |
|-----------------|------------------|-----------------------|
| `openai`        | `openai`         | `OPENAI_API_KEY`      |
| `anthropic`     | `anthropic`      | `ANTHROPIC_API_KEY`   |
| `google`        | `google`         | `GOOGLE_API_KEY`      |

If `BaseURL` is set (Helix proxy mode), also export `OPENAI_BASE_URL` /
`ANTHROPIC_BASE_URL` etc., applying `rewriteLocalhostURL` as Qwen and
Claude Code already do.

### D5: Don't touch `Dockerfile.sway-helix` in this PR

Sway is experimental (`HELIX_EXPERIMENTAL_DESKTOPS` gate, see
`helix/CLAUDE.md`). The user is leaning toward deleting it entirely.
Installing Goose into Sway now would be wasted work if the next task
deletes it. Recommendation: open a follow-up task to either delete
`Dockerfile.sway-helix` + `desktop/sway-config/` + the experimental-desktop
gate in `sandbox/04-start-dockerd.sh`, or mirror the Goose install at
that time.

### D6: Reuse Zed's `context_servers` — don't duplicate MCP wiring

Goose automatically picks up MCP servers declared in Zed's
`context_servers` block. Helix already populates this in
settings-sync-daemon (chrome-devtools, Kodit, github, drone-ci). Goose
sessions will see all of these for free; no extra plumbing needed.

## Files Touched

| File                                                              | Change                                               |
|-------------------------------------------------------------------|------------------------------------------------------|
| `Dockerfile.ubuntu-helix`                                         | Install pinned Goose CLI; add telemetry-off config   |
| `api/pkg/types/task_management.go`                                | Add `CodeAgentRuntimeGooseCode = "goose_code"`       |
| `api/cmd/settings-sync-daemon/main.go`                            | New `case "goose_code":` in `generateAgentServerConfig` |
| `frontend/src/types.ts`                                           | Add `'goose_code'` to runtime union                  |
| `frontend/src/api/api.ts` (generated)                             | Regen via `./stack update_openapi`                   |
| `frontend/src/contexts/apps.tsx`                                  | Add `'goose_code': 'Goose'` to display-name map      |
| `frontend/src/pages/Onboarding.tsx`, `ProjectSettings.tsx`        | Surface "Goose" as a selectable runtime option       |

## Risks & Mitigations

- **Goose release URLs are version-pinned.** Using `GOOSE_VERSION` per
  upstream's CI/CD recommendation avoids the "stable" tag breaking when a
  release drops the binary asset.
- **TLS in the container.** The Dockerfile already sets
  `NODE_TLS_REJECT_UNAUTHORIZED=0` and `git config http.sslVerify false`
  for enterprise / self-signed cert environments. Goose talks to GitHub
  releases at install time and to the LLM provider at runtime — both work
  through the existing trust config; no extra env vars expected.
- **Goose's "configure on first run" prompt.** `CONFIGURE=false` on the
  install script skips the interactive setup. Provider config comes from
  env vars at session time, not from a baked-in `~/.config/goose/config.yaml`.
- **Frontend type churn.** Multiple files hard-code the runtime union as
  a string literal; the implementer must grep for all of them (see Files
  Touched), not just edit `types.ts`.
