# Implementation Tasks: Auto-Approve Qwen Code Tool Permission Prompts in Zed

## Zed change (primary fix)

- [x] In `zed/crates/agent_servers/src/acp.rs`, locate `handle_request_permission` (the external-ACP permission handler) — found at line 4027.
- [x] Investigate reusing the native helper — **NOT possible**, see design.md "Implementation Notes": `crates/agent` depends on `crates/agent_servers`, so importing `agent::tool_permissions` would be a dependency cycle. Also external ACP tool calls carry no stable tool *name*, so per-tool rules can't be keyed. Decision: use the **global `tool_permissions.default`** only, read from `AgentSettings` (`crates/agent_settings`, no cycle).
- [x] Add `agent_settings` to `crates/agent_servers/Cargo.toml` dependencies.
- [x] In `handle_request_permission`, read `AgentSettings::get_global(cx).tool_permissions.default`.
- [x] When **Allow**: select an option from `args.options` (prefer `AllowAlways` kind, else `AllowOnce`), register the tool call then immediately authorize it in the same `update` closure so no interactive prompt is ever rendered.
- [x] When **Deny**: select a reject option (prefer `RejectOnce`) and authorize with it; if none offered, respond `Cancelled`.
- [x] When **Confirm** (the default): fall through to the existing interactive path unchanged.
- [x] Match permission options by ACP `PermissionOption.kind`, not by string id.

## Verification

- [x] Add Zed unit tests for the option-selection logic (prefers AllowAlways, falls back to AllowOnce, prefers RejectOnce when denying, returns None when no matching kind). 4/4 pass.
- [x] Build the Zed binary (`./stack build-zed dev`) — compiles clean, no `agent_servers` warnings.
- [x] Bake into desktop image: `./stack build-ubuntu` (VERSION `ae55ac`).
- [x] Live e2e in inner Helix: qwen_code spec task ran to `spec_review` with no permission prompt; wrote+committed all 3 spec docs. Verified container image `ae55ac`, live `zed_thread_id`, and `tool_permissions.default=allow` in the container. Screenshots captured.
- [~] Run the Zed WebSocket-sync e2e (`run_docker_e2e.sh`) to check for regressions on the shared external-ACP path (Claude Code is also an external ACP agent).
- [x] Negative case: `Confirm` arm returns `None`, leaving the interactive path unchanged by construction. NOT exercised live — the settings daemon rewrites the setting to `allow` on every sync. Documented in design.md.

## Release wiring

- [x] Commit the Zed change locally; hash `4d248e320eeab82a8dd3d86d8c83c01f92b712b7`.
- [x] Update `helix/sandbox-versions.txt` `ZED_COMMIT` to the new hash (resolved a conflict with main, which had pinned the base commit my work descends from).
- [ ] Open the Helix PR (with bumped hash) BEFORE pushing the Zed branch (per CLAUDE.md ordering).
- [ ] Push the Zed branch, open its PR (`gh pr create --repo helixml/zed`).
- [ ] Merge Zed PR, then merge Helix PR.

## Optional follow-up (only if confirmed needed)

- [ ] Instrument qwen `config.setApprovalMode` to log the modeId Zed sends after `new_session` — confirm/deny the post-startup YOLO→default flip (root-cause fact 2a). Revert before commit.
- [ ] Decide separately whether `ask_user_question` should be auto-answered in headless/automation contexts (out of scope here).
