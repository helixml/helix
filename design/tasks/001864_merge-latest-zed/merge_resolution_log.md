# Merge Resolution Log

Every conflict resolution decision during the upstream merge, for review.

**Merge:** `upstream/main` (commit `62bd61a679`) into `feature/001864-merge-latest-zed` (based on `helixml/zed` main at `1e07aea47a`)

**Total conflicts:** 35 files

---

## Category 1: Accept Upstream (no Helix changes in these files)

These files have no Helix-specific modifications. Resolved by accepting upstream version (`git checkout --theirs`).

| # | File | Rationale |
|---|------|-----------|
| 1 | `.github/workflows/assign-reviewers.yml` | No Helix changes, CI not used |
| 2 | `.github/workflows/autofix_pr.yml` | No Helix changes, CI not used |
| 3 | `.github/workflows/bump_patch_version.yml` | No Helix changes, CI not used |
| 4 | `.github/workflows/cherry_pick.yml` | No Helix changes, CI not used |
| 5 | `.github/workflows/extension_auto_bump.yml` | No Helix changes, CI not used |
| 6 | `.github/workflows/extension_bump.yml` | No Helix changes, CI not used |
| 7 | `.github/workflows/extension_tests.yml` | No Helix changes, CI not used |
| 8 | `.github/workflows/extension_workflow_rollout.yml` | No Helix changes, CI not used |
| 9 | `.github/workflows/pr_labeler.yml` | No Helix changes, CI not used |
| 10 | `.github/workflows/release.yml` | No Helix changes, CI not used |
| 11 | `.github/workflows/run_tests.yml` | No Helix changes, CI not used |
| 12 | `.github/workflows/run_agent_evals.yml` | Deleted upstream, accepted deletion (`git rm`) |
| 13 | `assets/keymaps/default-linux.json` | No Helix changes |
| 14 | `assets/keymaps/default-macos.json` | No Helix changes |
| 15 | `assets/keymaps/default-windows.json` | No Helix changes |
| 16 | `assets/keymaps/vim.json` | No Helix changes |
| 17 | `assets/settings/default.json` | No Helix changes |
| 18 | `crates/editor/src/edit_prediction_tests.rs` | No Helix changes |
| 19 | `crates/editor/src/editor.rs` | No Helix changes |
| 20 | `crates/editor/src/tasks.rs` | No Helix changes |
| 21 | `crates/agent/src/tools/context_server_registry.rs` | No Helix changes |
| 22 | `crates/agent_servers/src/acp.rs` | No Helix changes |
| 23 | `crates/workspace/src/multi_workspace.rs` | No Helix changes |
| 24 | `crates/zed/src/main.rs` | No Helix changes |

## Category 2: Manual merge (both sides have meaningful changes)

### 25. `crates/feature_flags/src/flags.rs`

**Upstream change:** PR #54206 revamped feature flag system — added `type Value = PresenceFlag;` associated type to every `FeatureFlag` impl.

**Helix change:** `AcpBetaFeatureFlag::enabled_for_all()` returns `true` (enables ACP beta features unconditionally in Helix builds).

**Resolution:** Accepted upstream's new `type Value` lines everywhere, preserved Helix's `enabled_for_all() -> true` override on `AcpBetaFeatureFlag`.

**Risk:** Low — changes are orthogonal (new associated type vs. method override).

### 26. `crates/recent_projects/src/dev_container_suggest.rs`

**Upstream change:** Added `let cli_auto_open = workspace.open_in_dev_container();` line.

**Helix change:** Early return at top of `suggest_on_worktree_updated` checking `RemoteSettings::get_global(cx).suggest_dev_container`.

**Resolution:** Kept Helix early return first, then upstream's `cli_auto_open` line after it.

**Risk:** Low — Helix's early return exits before the new upstream code runs (correct behavior: if setting disabled, skip entirely).

## Category 3: Pending resolution

| # | File | Status |
|---|------|--------|
| 27 | `Cargo.lock` | PENDING |
| 28 | `crates/acp_thread/src/acp_thread.rs` | PENDING — critical Helix file (Critical Fixes #6, #8, #9) |
| 29 | `crates/agent_ui/Cargo.toml` | PENDING — Helix feature flag propagation |
| 30 | `crates/agent_ui/src/agent_panel.rs` | PENDING — most complex Helix file |
| 31 | `crates/agent_ui/src/conversation_view.rs` | PENDING — HeadlessConnection, from_existing_thread |
| 32 | `crates/title_bar/Cargo.toml` | PENDING — Helix feature flag propagation |
| 33 | `crates/title_bar/src/title_bar.rs` | PENDING — WebSocket status indicator |
| 34 | `crates/workspace/src/workspace.rs` | PENDING — agent follow focus guard |
| 35 | `crates/zed/Cargo.toml` | PENDING — Helix feature flag definition |
