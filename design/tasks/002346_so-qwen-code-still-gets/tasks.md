# Implementation Tasks: Auto-Approve Qwen Code Tool Permission Prompts in Zed

## Zed change (primary fix)

- [x] In `zed/crates/agent_servers/src/acp.rs`, locate `handle_request_permission` (the external-ACP permission handler) — found at line 4027.
- [x] Investigate reusing the native helper — **NOT possible**, see design.md "Implementation Notes": `crates/agent` depends on `crates/agent_servers`, so importing `agent::tool_permissions` would be a dependency cycle. Also external ACP tool calls carry no stable tool *name*, so per-tool rules can't be keyed. Decision: use the **global `tool_permissions.default`** only, read from `AgentSettings` (`crates/agent_settings`, no cycle).
- [ ] Add `agent_settings` to `crates/agent_servers/Cargo.toml` dependencies.
- [ ] In `handle_request_permission`, read `AgentSettings::get_global(cx).tool_permissions.default`.
- [ ] When **Allow**: select an option from `args.options` (prefer `AllowAlways` kind, else `AllowOnce`), register the tool call then immediately authorize it in the same `update` closure so no interactive prompt is ever rendered.
- [ ] When **Deny**: select a reject option (prefer `RejectOnce`) and authorize with it; if none offered, respond `Cancelled`.
- [ ] When **Confirm** (the default): fall through to the existing interactive path unchanged.
- [ ] Match permission options by ACP `PermissionOption.kind`, not by string id.

## Verification

- [ ] Add/adjust a Zed unit test (or e2e) covering: setting=allow → auto-approved, no UI; setting=ask → interactive dialog still shown.
- [ ] Build the Zed binary: `./stack build-zed release`.
- [ ] Bake into desktop image: `./stack build-ubuntu`.
- [ ] Live e2e in inner Helix (`localhost:8080`): create a **qwen_code** spec-task, confirm an edit/write runs with no "Awaiting Confirmation" prompt and the task progresses autonomously (capture screenshot/logs).
- [ ] Run the Zed WebSocket-sync e2e (`run_docker_e2e.sh`) if it exercises the permission path.
- [ ] Negative test: with `tool_permissions.default = ask`, confirm the interactive dialog still appears (no regression).

## Release wiring

- [ ] Commit the Zed change locally; copy the commit hash (`git rev-parse HEAD`).
- [ ] Update `helix/sandbox-versions.txt` `ZED_COMMIT` to the new hash.
- [ ] Open the Helix PR (with bumped hash) BEFORE pushing the Zed branch (per CLAUDE.md ordering).
- [ ] Push the Zed branch, open its PR (`gh pr create --repo helixml/zed`).
- [ ] Merge Zed PR, then merge Helix PR.

## Optional follow-up (only if confirmed needed)

- [ ] Instrument qwen `config.setApprovalMode` to log the modeId Zed sends after `new_session` — confirm/deny the post-startup YOLO→default flip (root-cause fact 2a). Revert before commit.
- [ ] Decide separately whether `ask_user_question` should be auto-answered in headless/automation contexts (out of scope here).
