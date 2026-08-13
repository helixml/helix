# opencode as a Helix code agent runtime

Date: 2026-08-13
Status: implemented (Phase 1 + 1b) and exercised end-to-end in a live sandbox.
The remaining version-override checks require a release newer than 1.18.18.

## Summary

opencode ships a first-class ACP agent (`opencode acp`) that speaks the same
JSON-RPC/stdio protocol Zed already uses for claude / codex / gemini / goose /
qwen. I ran it locally against a real model and it works end-to-end. Adding a
`opencode` runtime is the **same shape of change as goose_code** (commit
`55c6616c2`), minus the Rust build stage — opencode ships prebuilt binaries.

Estimated effort: ~1 day for Phase 1 (parity with goose), no Zed changes needed.

## What "opencode v2" actually is

The name is doing a lot of work in the press coverage. Facts as of 2026-08-13:

| Thing | Reality |
|---|---|
| npm `opencode-ai@latest` | **1.18.18** (2026-08-13). There is no published `2.x`. |
| npm `opencode-ai@beta` | `0.0.0-beta-202608110357` — this *is* "v2", published as a `0.0.0-beta-<timestamp>` line out of `anomalyco/opencode-beta`. |
| Repo | Moved from `sst/opencode` → **`anomalyco/opencode`** (196k stars, default branch `dev`). |
| v2 content | Bun→Node runtime migration (memory), Tauri→Electron desktop, API redesign, parallel sessions/tabs. It is a **runtime/desktop rewrite, not an agent-protocol rewrite**. |

**The part that matters for us:** I ran the identical ACP probe against both
`1.18.18` and the v2 beta. Same `protocolVersion: 1`, byte-identical
`agentCapabilities`, same `opencode acp` entrypoint, same config format, same
full turn result. **v2 does not change our integration surface.** Pin the
stable tag, revisit when v2 goes GA.

## Verification performed

All of the below was run locally on this machine (binary
`opencode-linux-x64@1.18.18` and `@beta`, driven by a hand-written ~90-line ACP
client at `~/opencode-test/acp-prompt.mjs`).

1. **ACP handshake** — `initialize` returns:
   ```json
   {"protocolVersion":1,
    "agentCapabilities":{"loadSession":true,
      "mcpCapabilities":{"http":true,"sse":true},
      "promptCapabilities":{"embeddedContext":true,"image":true},
      "sessionCapabilities":{"close":{},"fork":{},"list":{},"resume":{}}},
    "authMethods":[{"id":"opencode-login",...}],
    "agentInfo":{"name":"OpenCode","version":"1.18.18"}}
   ```
   `loadSession: true` + `sessionCapabilities.resume` means thread resume works,
   which our spec-task flow depends on.

2. **Full agentic turn** — `session/new` → `session/prompt` → `tool_call(read)`
   → `agent_message_chunk` → `stopReason: end_turn` with a `usage` block
   (`inputTokens`/`outputTokens`/`cachedReadTokens`). Also verified write:
   the agent created `PROOF.txt` and read it back. This is the same event
   stream `external_websocket_sync` already consumes.

3. **Helix-proxy routing** — configured a custom provider
   `{"npm":"@ai-sdk/openai-compatible","options":{"baseURL":"…/v1","apiKey":"{env:…}"}}`
   pointed at the local dev stack's `http://localhost:8080/v1`. opencode
   registered the model as `helix/anthropic/claude-haiku-4-5-20251001` — nested
   slashes in the model id are fine. The request reached Helix and failed only
   on the dev stack's billing check (`failed to check balance: … org_id not
   specified`), i.e. a local wallet/org issue, not an opencode issue. I then
   re-ran the same `@ai-sdk/openai-compatible` code path against a live
   OpenAI-compatible endpoint and got a complete turn.

4. **stdio MCP passthrough** — passed a hand-written stdio MCP server in
   `session/new.mcpServers`; opencode spawned it, listed the tool, called it, and
   reported `ECHO:integration-ok`. This is how Helix injects Kodit et al. via
   Zed's `context_servers`. (`mcpCapabilities` only advertises `http`/`sse`
   because stdio is the ACP baseline, not an optional capability.)

5. **Inline config via env** — `OPENCODE_CONFIG_CONTENT='{...}'` is honoured.
   We can pass the whole provider/model/permission config as one env var on the
   `agent_servers` entry; no config file writing needed (unlike goose).

6. **Provider gating** — `"enabled_providers":["helix"]` collapses the model
   picker to only our model. Without it opencode auto-registers providers from
   ambient env (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY` are already set in our
   containers) **and** shows its free "OpenCode Zen" models. This gate is
   mandatory, otherwise users can escape Helix's proxy and billing.

7. **Headless auto-approve** — `"permission":"allow"` (global form) eliminates
   `session/request_permission` round-trips. Verified: with it, read/write/list
   ran with zero prompts; without it, a read outside cwd raised a permission
   request. This is opencode's equivalent of qwen's `--yolo`.

8. **Offline/air-gapped** — opencode lazily npm-installs into
   `$XDG_CONFIG_HOME/opencode/node_modules` (~63MB) for plugins. I re-ran with a
   **fresh XDG config and `npm_config_registry=http://127.0.0.1:1`** (unreachable)
   and the `@ai-sdk/openai-compatible` provider still worked — the SDKs are
   bundled in the binary and the plugin bootstrap degrades gracefully. Air-gap
   risk is low, but we should still set `autoupdate: false`.

9. **Per-session isolation** — honours `XDG_CONFIG_HOME` / `XDG_DATA_HOME`.
   Session state lives in `$XDG_DATA_HOME/opencode/opencode.db` (SQLite). Same
   pattern as the per-session `XDG_CONFIG_HOME=/home/retro/.config/helix-goose`
   we already use for goose.

## Why no Zed changes are needed

- `thread_service.rs:zed_agent_server_id()` only special-cases `"claude"` →
  `claude-acp` and `"codex"` → `codex-acp`; **everything else passes through**.
  So `agent_name: "opencode"` resolves to the `agent_servers.opencode` entry.
- `ExternalAgent::Custom { name }` → `CustomAgentServer::new(AgentId(name))` is
  fully generic (`crates/external_websocket_sync/src/types.rs:45`).
- `"opencode"` is already in Zed's `KNOWN_TERMINAL_AGENT_COMMANDS`
  (`agent_panel.rs:127`) and in `EXTENSION_TO_REGISTRY_IDS`
  (`agent_server_store.rs:199`) — it's a real ACP-registry agent
  (`cdn.agentclientprotocol.com/registry/v1/latest/registry.json`).

**Bake the binary, don't use `"type":"registry"`.** The registry path does a
runtime HTTPS fetch of the index plus a GitHub-release download — wrong for
air-gapped/proxied enterprise installs, and it re-downloads per container. Use
`"type":"custom"` + a baked binary, exactly like goose/qwen.

## Implementation plan

### Phase 1 — runtime parity with goose (~1 day)

1. **`sandbox-versions.txt`**: add `OPENCODE_VERSION=1.18.18`.

2. **`Dockerfile.ubuntu-helix`**: add an `ARG OPENCODE_VERSION` and a small
   fetch stage — no compilation, unlike the `goose-build` Rust stage:
   ```dockerfile
   ARG OPENCODE_VERSION=1.18.18
   # linux-x64 / linux-arm64 from github.com/anomalyco/opencode/releases,
   # sha256 pinned from the ACP registry manifest.
   ```
   Install to `/usr/local/bin/opencode`. Must handle both `TARGETARCH` values
   (the registry lists `linux-x86_64` and `linux-aarch64`).

3. **`api/pkg/types/task_management.go`**: add
   `CodeAgentRuntimeOpenCode CodeAgentRuntime = "opencode"`, and an
   `ZedAgentName()` case returning `"opencode"`. Leave
   `ValidateCodeAgentModelCompatibility` permissive — opencode is
   provider-agnostic like qwen/goose.

4. **`api/pkg/server/zed_config_handlers.go`** →
   `buildCodeAgentConfigFromAssistant`: add a `case types.CodeAgentRuntimeOpenCode`
   mirroring the qwen branch:
   ```go
   baseURL   = helixURL + "/v1"
   apiType   = "openai"
   agentName = "opencode"
   model     = fmt.Sprintf("%s/%s", providerName, modelName)
   ```
   Anthropic models route through the OpenAI-compatible proxy, same as qwen —
   one code path, no per-provider branching.

5. **`api/cmd/settings-sync-daemon/main.go`** → `generateAgentServerConfig`:
   add `case "opencode"` returning
   ```go
   map[string]interface{}{"opencode": map[string]interface{}{
       "name": "opencode", "type": "custom",
       "command": "opencode", "args": []string{"acp"},
       "env": env,
   }}
   ```
   where `env` carries:
   | var | value |
   |---|---|
   | `OPENCODE_CONFIG_CONTENT` | the JSON below, marshalled |
   | `HELIX_API_KEY` | `d.userAPIKey` (referenced as `{env:HELIX_API_KEY}`) |
   | `XDG_CONFIG_HOME` | `/home/retro/.config/helix-opencode` |
   | `XDG_DATA_HOME` | `/home/retro/work/.opencode-state` (persists across restarts, like `QWEN_DATA_DIR`) |
   | `OPENCODE_DISABLE_AUTOUPDATE` | belt-and-braces alongside the config flag |

   The config content (built in Go with a struct, not `map[string]interface{}`,
   per our Go rules):
   ```json
   {
     "model": "helix/<provider>/<model>",
     "small_model": "helix/<provider>/<model>",
     "permission": "allow",
     "autoupdate": false,
     "enabled_providers": ["helix"],
     "provider": {
       "helix": {
         "npm": "@ai-sdk/openai-compatible",
         "name": "Helix",
         "options": { "baseURL": "<rewritten base URL>", "apiKey": "{env:HELIX_API_KEY}" },
         "models": {
           "<provider>/<model>": {
             "name": "<model>",
             "tool_call": true,
             "limit": { "context": <MaxTokens>, "output": <MaxOutputTokens> },
             "options": { "reasoningEffort": "<ReasoningEffort>" }
           }
         }
       }
     }
   }
   ```
   Reuse `d.rewriteLocalhostURL()` for the base URL (dev-mode container
   networking) and `normalizeCodeAgentReasoningEffort` for the effort value.
   `MaxTokens`/`MaxOutputTokens` already come from `applyAdvertisedModelLimits`,
   so opencode's context bar will be accurate — omit the `limit` block when
   they're 0 rather than writing zeros.

6. **Frontend** (mirrors `815572058`):
   - `frontend/src/types.ts` — add `'opencode'` to the runtime union.
   - `frontend/src/contexts/apps.tsx`, `components/app/AppSettings.tsx`,
     `components/agent/CodingAgentForm.tsx` — add the selector option.
     `Onboarding.tsx` / `ProjectSettings.tsx` inherit it via `CodingAgentForm`.
   - `components/agent/AgentHarness.tsx` — add
     `opencode: { label: 'opencode', color: '' }` plus
     `assets/harness/opencode.svg` (official mark is published at
     `cdn.agentclientprotocol.com/registry/v1/latest/opencode.svg`). Per our UI
     rules this is the canonical asset — do not substitute a Lucide glyph.
   - `components/tasks/SpecTaskModelPicker.tsx` — expose the active harness mark
     and name in the model control tooltip; Task Details groups the long
     harness variant, model/reasoning, and compute controls into labeled rows.
   - `components/helix-org/BotRuntimeForm.tsx` — same option for org bots.
   - `./stack update_openapi` afterwards.

7. **Tests**: extend `api/pkg/types/task_management_test.go` and
   `api/cmd/settings-sync-daemon/main_test.go` with an opencode case (the
   daemon test should assert the marshalled `OPENCODE_CONFIG_CONTENT` contains
   `enabled_providers` and `permission: allow` — those are the two settings
   whose loss is a billing/UX escape).

### Phase 1b — admin-settable version override

Goal: the baked binary is the floor, and an admin can pin a **newer** version
from the dashboard without an image rebuild. Verified mechanics:

- The release tarball is 60MB compressed / 184MB extracted, and is a **single
  self-contained binary** with no runtime deps.
- `curl` → `sha256sum -c` → `tar xzf` → `./opencode --version` works, and the
  extracted binary serves a full ACP turn from an arbitrary path — so
  `agent_servers.opencode.command` can point at a cache path instead of
  `/usr/local/bin/opencode`.
- Both the archive URL and its **sha256 are published in the ACP registry
  manifest** (`cdn.agentclientprotocol.com/registry/v1/latest/registry.json`),
  per platform. We don't have to invent a URL scheme or trust an unverified
  download.

**Resolve on the API, install in the container, gate on success.**

1. **`api/pkg/types/system_settings.go`** — add `OpenCodeVersion string` to
   `SystemSettings`, `*string` to `SystemSettingsRequest`, and the plain field
   to `SystemSettingsResponse` (it's not sensitive). Empty = use the baked
   version. GORM AutoMigrate handles the column.

2. **Validate at write time, in the admin UI** — this is the fail-fast point
   where a human can act on the error, and it keeps the container path simple.
   The `PUT` handler must:
   - reject anything not matching `^\d+\.\d+\.\d+$` (the value ends up in a
     download URL — no path or URL injection),
   - reject a version **older than or equal to** the baked floor (this is an
     upgrade lever, not a downgrade lever — downgrades belong in the image),
   - resolve the version against the ACP registry / GitHub release and reject
     it if no `linux-x86_64` + `linux-aarch64` artifact with a sha256 exists.

3. **Ship the resolved artifact, not just the number.** Extend
   `types.CodeAgentConfig` with an `OpenCodeBinary *AgentBinarySpec` carrying
   `{Version, URL, SHA256}` for the container's arch, filled in
   `buildCodeAgentConfigFromAssistant`. Two reasons: the container never has to
   know the URL scheme, and air-gapped operators can later point the resolver
   at an internal mirror in one place. Cache the registry lookup server-side —
   don't hit the CDN per session.

4. **`settings-sync-daemon`** installs before emitting `agent_servers`:
   - Read the baked version from `/opt/helix/opencode.version` (written at image
     build time, mirroring the existing `sandbox-images/*.version` convention) —
     cheaper and more reliable than spawning `--version`.
   - If `cfg.OpenCodeBinary == nil` or its version equals the baked one, use
     `command: "opencode"` and stop. This is the default path; no network.
   - Otherwise download to `<cache>/opencode-<version>/opencode`, verify the
     sha256, `chmod +x`, then point `command` at that absolute path. Download to
     a temp file and `os.Rename` into place so a torn download can't be executed
     and two containers racing on a shared cache can't half-read each other's
     file.
   - **On failure: log the error and `return nil`** (no `agent_servers` emitted),
     exactly like the existing `claude_code` branch does when credentials aren't
     ready yet. The daemon re-polls, so a transient network blip self-heals; a
     persistent failure leaves the session visibly agent-less instead of
     silently running a version the admin didn't pin. Do **not** fall back to
     the baked binary — that is precisely the silent-skip our Go rules forbid,
     and it would leave admins believing a rollout landed when it didn't.

5. **Cache across sessions.** A per-container cache means every session pays a
   60MB download. Add a host-level bind mount in
   `hydra_executor.go:buildMounts` — `/data/agent-cache` →
   `/opt/helix/agent-cache` — so all containers on a sandbox host share one
   copy. Version-keyed subdirectories make it append-only and safe to GC by age
   (`workspace_gc.go` is the obvious place to sweep it).

6. **Frontend** — one text field in `components/dashboard/SystemSettingsTable.tsx`
   ("opencode version override", helper text showing the baked floor and that
   blank means "use bundled"). Validation errors from the API surface inline.

**Deliberately not doing:** opencode's own `autoupdate` / `opencode upgrade`.
It would mutate the binary mid-session, outside our version pinning, with no
sha verification we control. `autoupdate: false` stays in the config.

### Phase 2 — parity extras (optional, follow-up PR)

- **Slash commands.** opencode emits `available_commands_update` over ACP just
  like goose. Its `command` config block + `.opencode/command/*.md` files are
  the natural home for a "project commands" feature analogous to goose recipes.
  Not needed for launch.
- **Subagents.** `agent` / `subagent_depth` config could expose opencode's
  subagent fleet. Deliberately out of scope for Phase 1.
- **Subscription credentials.** opencode has its own OAuth (`opencode-login`
  auth method, `opencode providers login`). We could map
  `credential_type=subscription` onto it later, the same way claude_code and
  codex_cli do. Phase 1 is API-key-through-the-Helix-proxy only.

## Risks

| Risk | Mitigation |
|---|---|
| Ambient `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` in the container let opencode auto-register direct providers, bypassing Helix billing | `enabled_providers: ["helix"]` — verified to collapse the picker to our single model |
| opencode's free "OpenCode Zen" models appear as selectable options | same gate |
| Runtime npm install for plugins hits the network | Verified to degrade gracefully with an unreachable registry; also set `autoupdate: false` |
| v2/beta churn changes the ACP surface | Verified identical today; pin `OPENCODE_VERSION`, re-run the probe when bumping |
| Auto-update silently swapping the binary in a session | `autoupdate: false` + baked binary owned by root |
| Admin pins a version that can't be downloaded (air-gapped, bad tag) | Validated at save time against the registry; at runtime a failed install blocks `agent_servers` rather than silently reverting to the baked binary |
| Version string reaching a download URL unsanitised | `^\d+\.\d+\.\d+$` on write + sha256 from the registry manifest |
| 60MB download per session | Host-shared `/data/agent-cache` mount, version-keyed |

## Testing checklist (before merge)

- [x] `./stack build-ubuntu` — confirm `opencode --version` in the image
- [x] Start a **spec task** with `code_agent_runtime=opencode` (a bare
      `zed_external` chat session won't do — no repo means Zed never connects)
- [x] Confirm `config->>'zed_thread_id'` is non-empty (live Zed)
- [x] Send a message, confirm streaming + tool calls land in the Helix chat
- [x] Confirm token usage is recorded (opencode reports `usage` per turn)
- [ ] Exercise the remaining lifecycle seam: clear thread → send another
      message. Stop → resume → send and a second turn were verified.
- [ ] Confirm the model picker shows exactly one Helix-routed model

## What was verified during implementation

Automated, runs in CI:

- `pkg/opencode` — 8 tests: semver validation (including `v` prefixes, path
  traversal and URL-injection attempts), both-architectures resolution, partial
  release rejection, missing-digest rejection, 404 handling, response caching,
  and concurrent cache-miss deduplication.
- `cmd/settings-sync-daemon` — 13 tests: baked binary by default, the
  `enabled_providers` gate, `permission: allow` + `autoupdate: false`,
  provider-qualified model id, omitted unknown limits, reasoning effort,
  install-and-verify, digest-mismatch rejection (with no partial file left
  behind), no-agent-server-on-failure, retry throttling, pin-equals-baked
  skipping the download, cache reuse, and unexpected archive-file rejection.
- `pkg/store` — the system-settings update test verifies that
  `opencode_version` is persisted and survives partial updates.
- `pkg/server` — the runtime-selection table gains an opencode row.

Manual, run once during development:

- The Dockerfile install block was built as a standalone image: sha256 verified,
  `opencode --version` printed 1.18.18, `/opt/helix/opencode.version` written,
  the cache dir created retro-writable, and the binary root-owned so the agent
  user cannot overwrite its own runtime. The arm64 digest was verified by
  download.
- `./stack build-ubuntu` produced image `227cc1`. A live OpenCode spec task
  (`spt_01kzx26k7nnwwrpjpxjvh4a2fb`) started Zed, spawned
  `/usr/local/bin/opencode acp`, created thread
  `ses_005ce3035ffeqQAxiNg8XyYQey`, streamed a shell tool call, returned
  `OPENCODE_READY`, and recorded a 10,755-token usage event. A second turn on
  the same thread also completed. This run caught and fixed the build-time
  `opencode --version` smoke check creating root-owned state under
  `/home/retro/.local`; the smoke check now uses an isolated temporary home.
- The same live task was checked through the production task UI: hovering the
  chat composer model control showed the OpenCode mark and `opencode`, while
  Task Details showed labeled Harness, Model, and Compute rows. The running
  container also reported PID 2542 as `/usr/local/bin/opencode acp`, with the
  matching command and `acp` argument in Zed settings. The focused component
  suite (9 tests) and the production frontend build passed.
- `cmd/settings-sync-daemon/opencode_live_test.go` (build tag `livetest`) feeds
  the daemon's generated config to a **real `opencode acp` process** and asserts
  the ACP handshake succeeds and `session/new` offers exactly one model. It
  passed against opencode 1.18.18 with a live OpenAI-compatible endpoint. It is
  tag-gated because it needs a binary and a reachable model endpoint.

## What is NOT verified

- **Clear-thread lifecycle.** Stop → resume → first turn and a second turn on
  the same live thread passed, but clear thread → next turn is still unchecked.
- **The version override has never been exercised against a real newer
  release.** 1.18.18 is the newest published version, so `ValidateVersion`
  rejects every real value today; the install path is covered only by tests
  using a synthetic tarball and an httptest server. The first real bump is the
  first true test.
- **The shared host cache mount is untested at runtime.** `/data/agent-cache`
  is created by the bind mount; no sandbox host has actually been through it,
  and nothing GCs it yet (see Phase 2).
- [ ] Version override: blank setting → session uses the baked binary, no network
- [ ] Version override: set a newer version → new session runs it (check
      `agent_servers.opencode.command` points at the cache path and the agent
      reports the new version in its ACP `agentInfo`)
- [ ] Version override: set a version whose download 404s → session comes up
      with no opencode agent and a clear daemon log, **not** a silent downgrade
- [ ] Second session on the same host reuses the cached binary (no re-download)
