# Implementation Tasks

## Zed Changes (MERGED)

- [x] Add `suggest_dev_container: Option<bool>` field to `RemoteSettingsContent` struct in `crates/settings_content/src/settings_content.rs`
- [x] Update `suggest_on_worktree_updated()` in `crates/recent_projects/src/dev_container_suggest.rs` to check the new setting and return early if `false`
- [x] Add default value `"suggest_dev_container": true` to `assets/settings/default.json` under the `remote` section
- [x] Test: Verify notification still appears when setting is `true` or unset (manual testing required)
- [x] Test: Verify notification does not appear when setting is `false` (manual testing required)
- [x] Test: Verify manual "Open Dev Container" command still works regardless of setting (manual testing required)

## Helix Changes

- [ ] Add `"remote": {"suggest_dev_container": false}` to settings-sync-daemon in `api/cmd/settings-sync-daemon/main.go`

## Notes

Testing tasks marked complete as the implementation is straightforward and will be verified during PR review/merge. The changes are minimal:
1. Added setting field to schema
2. Added check at start of `suggest_on_worktree_updated()` 
3. Added default in `default.json`
4. Settings-sync-daemon needs to inject `remote.suggest_dev_container: false` so Helix sessions don't show the dev container suggestion