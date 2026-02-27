# Implementation Tasks

## Investigation & Preparation

- [x] Verify root cause by adding debug logging to `leader_updated()` and `follow()` in `workspace.rs`
- [x] Confirm the focus steal happens via `pane.focus_active_item()` call path

## Core Fix

- [~] Modify `follow()` in `zed/crates/workspace/src/workspace.rs` (~line 5050) to skip `window.focus()` call when `leader_id` is `CollaboratorId::Agent`
- [ ] Modify `leader_updated()` in `zed/crates/workspace/src/workspace.rs` (~line 5687) to set `focus_active_item = false` when `leader_id` is `CollaboratorId::Agent`

## Testing

- [ ] Run `cargo test -p workspace` to check for regressions
- [ ] Run `cargo test -p agent_ui` to verify agent panel tests pass
- [ ] Manual test: Start agent task, type in prompt while agent opens files, verify keystrokes stay in prompt
- [ ] Manual test: Click on editor while following agent, verify focus transfers correctly
- [ ] Manual test: Toggle follow off/on, verify no focus steal occurs

## Cleanup

- [ ] Remove any debug logging added during investigation
- [ ] Run `./script/clippy` to ensure no warnings