# Design: Auto-Approve Qwen Code Tool Permission Prompts in Zed

## Architecture recap

```
Helix API ──WebSocket sync──▶ Zed IDE (ACP client) ──ACP──▶ Qwen Code (ACP agent)
  settings-sync-daemon writes Zed settings.json (agent_servers.qwen, tool_permissions)
```

- Helix configures qwen as a custom `agent_servers` entry with args
  `--yolo --experimental-acp --no-telemetry --include-directories /home/retro/work`
  and `default_mode: "yolo"`
  (`api/cmd/settings-sync-daemon/main.go:165-193`).
- Helix also sets `agent.always_allow_tool_actions: true`
  (`api/pkg/external-agent/zed_config.go:217`) which the daemon maps to
  `agent.tool_permissions.default = "allow"`
  (`injectAgentToolPermissions`, `main.go:1583-1595`).
- When qwen wants to run a tool, it (sometimes) sends ACP
  `session/request_permission`; Zed decides whether to prompt.
- **Helix does not proxy or auto-answer `request_permission`** — the websocket
  sync layer has no permission handling. So the decision is entirely Zed's.

## Root cause (verified in both trees)

### Fact 1 — Zed always prompts external agents (the real bug)
Zed has two separate permission paths and the "always allow" setting is wired
into only one of them:

- **External-ACP path (qwen):** `handle_request_permission`
  (`zed/crates/agent_servers/src/acp.rs`, ~line 4027) unconditionally calls
  `thread.request_tool_call_authorization(...)`
  (`zed/crates/acp_thread/src/acp_thread.rs`, ~line 2656), which sets the tool
  call to `WaitingForConfirmation`, emits `ToolAuthorizationRequested`, and
  parks on a oneshot channel that is only resolved by a **user clicking** a
  button (`AllowOnce` / `AllowAlways` / `RejectOnce` in
  `crates/agent_ui/src/acp/thread_view.rs`). **No `AgentSettings` /
  `tool_permissions` read anywhere in this path.**
- **Native-agent path:** `run_authorization_loop`
  (`zed/crates/agent/src/thread.rs`, ~line 5762) short-circuits via
  `decide_permission_from_settings(...)` /
  `crates/agent/src/tool_permissions.rs` — returning `Allow` **before** any UI
  prompt. Its callers are exclusively `crates/agent/src/tools/*` (native tools).

⇒ For qwen, `always_allow_tool_actions` / `tool_permissions.default = "allow"`
is a no-op; Zed always renders the dialog. In a headless sandbox that = stall.

### Fact 2 — Qwen still emits request_permission under `--yolo`
`--yolo` parses into YOLO (`qwen-code/.../config/config.ts:776-778`) and
persists per-session (`acpAgent.ts:390-423`). It is **not** overridden by the
folder-trust check in a clean sandbox (`security.folderTrust.enabled` defaults
`false`, so `trustedFolder` is always `true`; `config.ts:786-796` never fires).
But `Session.ts` still reaches `this.client.requestPermission(...)` (~line 990)
when:
- The runtime mode is flipped off YOLO after `new_session` via
  `session/set_mode` or `set_session_config_option(mode)`
  (`Session.ts:638-650`, `acpAgent.ts:287-333`) — the likely cause of the
  *edit-tool* "Allow all edits / Reject" prompt in the screenshots.
- `ask_user_question` is called — intentionally never YOLO-exempt
  (`Session.ts:848-853`), a permanent stall vector.

The edit/write tools default to `'ask'` (`edit.ts:276`, `write-file.ts:105`);
YOLO force-allows them only while the mode is genuinely YOLO at call time.

## Decision: fix on the Zed side (auto-approve external-agent permissions)

**Make Zed's external-ACP `handle_request_permission` honour the agent's
`tool_permissions` / `always_allow_tool_actions` setting and auto-respond,
mirroring the native agent's `run_authorization_loop` short-circuit.**

### Why this over the alternatives
- **Realises existing intent.** Helix already declares `always_allow_tool_actions:
  true`; today it silently does nothing for external agents. This makes it work.
- **Root cause, not symptom.** It removes the stall regardless of qwen's internal
  mode, so it is immune to the mode-flip (fact 2a) and to any future tool whose
  default is `'ask'`.
- **General.** Applies to every custom ACP agent (qwen, goose, codex, …), not a
  qwen-only patch.
- **Single well-scoped change** in the crate that owns the bug.

### Alternatives considered (rejected)
- **Qwen-side "lock YOLO" flag** (ignore post-startup mode downgrades when
  `--yolo` set): fixes edits but not `ask_user_question`, fights a symptom, and
  is qwen-specific. Keep as a possible follow-up only if fact 2a is confirmed and
  we want defence-in-depth.
- **Helix proxies/auto-answers `request_permission`** over the websocket sync:
  Helix currently has no permission channel; adding one duplicates what Zed
  already models and is more code than the Zed patch.

## Implementation sketch (Zed)

In `zed/crates/agent_servers/src/acp.rs`, `handle_request_permission`:

1. Resolve the effective agent tool-permission setting (reuse
   `AgentSettings::get_global(cx)` / the same `decide_permission_from_settings`
   helper the native agent uses in `crates/agent/src/thread.rs` +
   `tool_permissions.rs`). Pass the tool name/kind from `args.tool_call`.
2. If the decision is **Allow**: pick an option from `args.options` — prefer an
   `AllowAlways`-kind option, else the first `AllowOnce`-kind option — and
   `responder.respond(RequestPermissionResponse::new(selected.into()))`
   immediately, **without** calling `request_tool_call_authorization` (no UI).
   Still upsert the tool call into the thread view so it renders as
   auto-approved (for transparency), but do not block.
3. If **Deny**: respond with a reject/cancel outcome.
4. If **Confirm** (or no definitive setting): fall through to the existing
   interactive `request_tool_call_authorization` path — unchanged behaviour.

Keep the option-selection logic tolerant: qwen's options come from
`toPermissionOptions` and include allow-once / allow-always / reject kinds
(ACP `PermissionOption.kind`). Match on `kind`, not on brittle string ids.

### Build / deploy
- Zed is a Rust change → rebuild the pinned binary: `./stack build-zed release`
  (release is required on ARM), then `./stack build-ubuntu` to bake it into the
  desktop image, then **bump `sandbox-versions.txt` `ZED_COMMIT`** to the new
  commit (follow the ordering rule in CLAUDE.md: commit Zed locally → copy hash →
  open Helix PR with bumped hash → push Zed branch → merge Zed → merge Helix).
- No Helix API change is required; the daemon already writes the "allow" setting.

## Implementation Notes (discovered during implementation)

### Cannot reuse the native `decide_permission_from_settings` helper
The original design proposed reusing
`agent::tool_permissions::decide_permission_from_settings`. **This is
impossible: it would create a dependency cycle.** `crates/agent/Cargo.toml`
already depends on `agent_servers` (line 24), so `agent_servers` cannot depend
on `agent`.

Resolution: read the setting from **`crates/agent_settings`** instead
(`AgentSettings::get_global(cx).tool_permissions.default`). `agent_settings`
depends on neither `agent` nor `agent_servers`, so adding it to
`agent_servers` is cycle-free. `ToolPermissionMode` (`Allow` / `Deny` /
`Confirm`, defined in `crates/settings_content/src/agent.rs:844`) is re-exported
via the `settings` crate, which `agent_servers` already depends on.

### Only the global `default` applies to external agents (resolves Open Question 4)
Per-tool `tool_permissions` rules are keyed by **Zed's native tool names**
(`edit_file`, `terminal`, …). An external ACP `request_permission` carries no
stable tool *name* — `args.tool_call` has only `tool_call_id`, `title`, `kind`,
`locations` and `raw_input`. Mapping an arbitrary agent's tool titles onto
Zed's native rule keys would be guesswork, so **only the global
`tool_permissions.default` is honoured** for external agents. This is also the
behaviour the native path falls back to when a tool has no rules entry, so it
is consistent. Note the hardcoded security rules (e.g. `rm -rf /` blocking) are
likewise native-tool-keyed and do not apply here.

### Auto-approval reuses the existing thread machinery
Rather than bypassing `request_tool_call_authorization` entirely, the handler
calls it and then immediately calls `authorize_tool_call(...)` with the chosen
option **inside the same `cx.update` closure**. No frame renders between the
two, so the user never sees a prompt, while the tool call still appears in the
thread view with the correct `InProgress`/`Rejected` status via the existing
status-transition logic (`acp_thread.rs:2695+`). This is the same mechanism
Zed's own e2e tests use to simulate a click, so it is a well-trodden path.

## Verification Results (live, inner Helix)

Reproduced the original failure scenario end-to-end and confirmed it is fixed.

**Setup:** registered `test@helix.ml` in the inner Helix at `localhost:8080`,
created org `testorg` + project `testproj` with the **Qwen Code** runtime
(agent "Opus 4 in Qwen Code"), then started a spec task via
`helix spectask start`.

**Environment confirmed correct before judging the result:**
- Sandbox container ran image `helix-ubuntu:ae55ac` — the image built from this
  branch's Zed binary (`docker inspect` matched
  `sandbox-images/helix-ubuntu.version`).
- Live Zed connection: `sessions.config->>'zed_thread_id'` was a non-empty UUID.
- Live Zed settings inside the container:
  `agent.tool_permissions = {"default": "allow"}` and qwen args
  `["--yolo", "--experimental-acp", "--no-telemetry", "--include-directories", "/home/retro/work"]`.

**Result:** the task ran to `spec_review` with **no permission prompt**. Qwen
wrote `requirements.md`, `design.md` and `tasks.md` — the exact operation that
stalled in the bug report screenshots ("Writing to …/requirements.md" →
"Awaiting Confirmation") — and committed them (`helix-specs @ fc65046`). The
Zed thread ends with "The design is ready for review ✅" and the notification
feed shows "Agent finished working". Screenshots in `screenshots/`.

**Unit tests:** 4/4 pass (`cargo test -p agent_servers --lib`), covering the
option-preference order and the no-matching-kind fallthrough.

**Zed WebSocket-sync e2e (`run_docker_e2e.sh`, `E2E_AGENTS="zed-agent,claude"`):**
all **17 zed-agent phases PASSED**. The **claude round failed at Phase 1** with
`Events received: 0, Threads: 0` — the claude-acp agent never established a
session at all.

That failure is **pre-existing in this environment, not caused by this change** —
verified empirically, not assumed: I checked out unmodified `origin/main`
(`06e9ce8059`), rebuilt the Zed binary, and re-ran the e2e with
`E2E_AGENTS="claude"`. It times out in exactly the same place on the baseline.
(Consistent with this, the two most recent commits on `main` are
`fix/zed-e2e-model-readiness` and `fix/e2e-phase16-deferred-message`, and
`CLAUDE.md` documents that the local Anthropic proxy may reject the e2e's default
model.) Mechanically it also could not be this change: with zero threads created,
no tool call and therefore no `request_permission` ever occurs, so the modified
handler is never entered.

**Negative case (confirm → still prompts) — NOT separately exercised live.** The
`ToolPermissionMode::Confirm` arm returns `None`, which leaves the original
interactive code path unchanged (no `authorize_tool_call` call), so behaviour is
unchanged by construction rather than by measurement. Reproducing it live would
require defeating the settings-sync daemon, which rewrites
`tool_permissions.default` to `allow` on every sync.

## Testing plan

- **Live e2e in inner Helix (mandatory).** Register/onboard at `localhost:8080`,
  create a spec-task with the **qwen_code** runtime, and confirm an edit/write
  runs with **no** "Awaiting Confirmation" prompt and the task progresses. This
  touches session/thread lifecycle + a live Zed, so seeded DB rows are not a
  substitute (see CLAUDE.md "Live external-agent testing is mandatory").
- **Zed WebSocket-sync e2e** (`run_docker_e2e.sh`) if the permission path is
  exercised there — run it, don't assume.
- **Negative test.** With `tool_permissions.default` set to `ask`, confirm the
  interactive dialog still appears (no regression to Zed's native agent or to
  users who want prompts).
- **Log check.** Optionally instrument qwen `config.setApprovalMode` once to see
  whether Zed flips the mode after `new_session` (validates fact 2a); revert
  before commit.

## Learnings / gotchas for future agents

- Zed has **two** permission systems. `agent.tool_permissions` /
  `always_allow_tool_actions` only govern the **native** agent
  (`crates/agent/src/thread.rs` `run_authorization_loop`,
  `crates/agent/src/tool_permissions.rs`). External ACP agents
  (`crates/agent_servers/src/acp.rs`) have their **own** permission path that
  ignores those settings. Don't assume a setting that works for the built-in
  agent works for qwen/goose/codex.
- Helix already sets everything it can (`--yolo`, `default_mode: yolo`,
  `always_allow_tool_actions`) — the gap is purely in Zed's external-agent
  handler. Don't add more Helix-side config expecting it to help.
- Zed's `default_mode` → `session/set_mode` is sent only if the agent advertises
  a mode whose `id` exactly matches the configured value (e.g. `yolo`) in its
  `new_session` `available_modes`; otherwise it's silently skipped with a warning
  (`crates/agent_servers/src/acp.rs` ~1666). Setting `default_mode` does **not**
  suppress Zed's confirmation dialog.
- In qwen, `ask_user_question` is deliberately never YOLO-bypassed
  (`Session.ts:848-853`) — a genuine autonomous-agent hazard independent of this
  fix.
- Line numbers above are approximate (the Zed working tree drifts from the
  pinned commit); search by function name.
