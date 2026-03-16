# Design: Merge Latest Zed Upstream into Fork

## Repository Setup

**Fork repo:** `/home/retro/work/zed/`
**Origin remote:** `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
**Upstream:** `https://github.com/zed-industries/zed` (must be added as a remote named `upstream`)

The fork tracks upstream via a merge strategy (not rebase). The previous merge commit is in the git history. Add `upstream` remote and merge `upstream/main` into `origin/main`.

## Merge Strategy: Merge (not Rebase)

Use `git merge upstream/main` rather than rebase. Rationale:
- The fork has 20+ Helix-specific commits interleaved after the last upstream base commit.
- Rebase would require replaying every commit one by one, surfacing conflicts at each.
- Merge creates a single conflict-resolution commit, which is simpler and matches the December 2025 merge approach.
- The previous merge docs (`helix/design/2025-12-09-zed-upstream-merge.md`) confirm merge was used last time.

## High-Risk Files (Expect Conflicts)

Based on the porting guide and previous merge experience, these files are most likely to conflict:

| File | Risk | Why |
|------|------|-----|
| `crates/agent_ui/src/agent_panel.rs` | HIGH | Largest fork changes; upstream frequently modifies agent panel |
| `crates/agent_ui/src/acp/thread_view.rs` | HIGH | HeadlessConnection, event handling, from_existing_thread() |
| `crates/agent_servers/src/acp.rs` | HIGH | load_session(), session persistence; ACP protocol evolves upstream |
| `crates/agent/src/agent.rs` | MEDIUM | load_session() entity lifetime fix (Critical Fix #1) |
| `crates/acp_thread/src/acp_thread.rs` | MEDIUM | content_only() method (Critical Fix #3) |
| `crates/anthropic/src/anthropic.rs` | MEDIUM | Model definitions change with new Claude releases |
| `Cargo.toml` (workspace) | LOW | external_websocket_sync member entry must be preserved |
| `crates/zed/Cargo.toml` | LOW | external_websocket_sync feature flag |
| `crates/feature_flags/src/flags.rs` | LOW | AcpBetaFeatureFlag always-on override |

Files in `crates/external_websocket_sync/` are entirely Helix-specific and will not conflict.

## Conflict Resolution Priorities

When resolving conflicts, the rule is: **upstream wins by default, except for `#[cfg(feature = "external_websocket_sync")]` blocks, which must be preserved verbatim.**

For files where upstream has restructured a function that contains a fork-specific fix, the fix must be re-applied to the new upstream structure. Consult the "Critical Fixes" section of `portingguide.md` for the exact intent of each fix.

Key things to check after resolving every conflict in a high-risk file:
1. `agent.rs` `load_session()` — `let agent = self.0.clone()` line present (Critical Fix #1).
2. `thread_view.rs` — no `MessageAdded`/`MessageCompleted` sends in event handlers (Critical Fix #2).
3. `acp_thread.rs` — `content_only()` method on `AssistantMessage` still exists (Critical Fix #3).
4. `thread_service.rs` — `notify_thread_display()` called before follow-up to non-visible thread (Critical Fix #4).

## `from_existing_thread()` Compatibility Check

`thread_view.rs::from_existing_thread()` constructs a `ConnectedServerState` using specific field names (`active_id`, `threads` HashMap, `conversation` Entity). If upstream has changed the `ConnectedServerState` struct, this constructor needs updating. Always verify the struct fields match after resolving conflicts in `thread_view.rs`.

## Test Plan

Run tests in this order (fail fast):

1. `cargo check --package zed --features external_websocket_sync`
   → Catches compile errors before wasting time on test runs.

2. `cargo test -p external_websocket_sync`
   → Unit tests + protocol integration tests (fast, no LLM).

3. Docker E2E test:
   ```bash
   docker build -t zed-ws-e2e -f crates/external_websocket_sync/e2e-test/Dockerfile .
   docker run --rm -e ANTHROPIC_API_KEY=... -e TEST_TIMEOUT=240 zed-ws-e2e
   ```
   → Full 8-phase protocol test with a real LLM. This is the hard sign-off gate.

The E2E test phases as of the last commit (`8b033a4`):
1. Agent ready → phase 1
2. New thread via chat_message
3. Follow-up on existing thread
4. New thread (second)
5. Follow-up to non-visible thread (Thread A while Thread B displayed)
6. User input sync (Zed → Helix)
7. Query UI state verification
8. Mid-stream interrupt / Stopped emission

## Porting Guide Update

After the merge is complete, update `portingguide.md`:
- Add any newly conflicted upstream files to the "Modified Upstream Files" section.
- Add any new critical fixes discovered.
- Update the "Rebase Checklist" with new items if needed.
- Update the "Commit History" table with the merge commit hash and description.
- Update the E2E test phase list if phases were added/changed.

## Codebase Patterns Learned

- All Helix changes use `#[cfg(feature = "external_websocket_sync")]` gates — search for these when verifying fixes survived the merge.
- The E2E test Go server (`e2e-test/helix-ws-test-server/main.go`) imports real Helix production code and uses an in-memory store — it does not need updating for upstream merges.
- The fork's `.dockerignore` is intentionally simplified vs upstream's — preserve the fork version.
- `feature_flags/src/flags.rs` has `enabled_for_all()` returning `true` — upstream's version returns `false`. Always keep fork's version.
- Build command is `cargo build --features external_websocket_sync -p zed`, NOT `cargo build -p zed`.
- Use `./script/clippy` (not `cargo clippy`) per CLAUDE.md.
