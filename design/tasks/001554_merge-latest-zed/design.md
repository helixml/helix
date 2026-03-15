# Design: Merge Latest Zed Upstream

## Strategy

Do a **git merge** (not rebase) of `upstream/main` into a feature branch. Merging preserves history and makes conflict resolution incremental — it's the same approach used for previous upstream syncs (`upstream-merge-2026-02-24`, `upstream-merge-2026-02-26`).

## Upstream Source

- Upstream repo: `https://github.com/zed-industries/zed` (add as `upstream` remote if not present)
- Last merged upstream state: commit `ddc071d503` (2026-02-26)
- Branch strategy: `upstream-merge-YYYY-MM-DD` → PR → merge to `main`

## High-Risk Files (Conflict-Prone)

These upstream files contain Helix modifications and will likely conflict. Resolve each carefully using the `portingguide.md` rebase checklist:

| File | Helix change | Risk |
|------|-------------|------|
| `crates/agent/src/agent.rs` | `load_session()` entity lifetime fix (Critical Fix #1) | High |
| `crates/agent_ui/src/agent_panel.rs` | callback setup, `from_existing_thread`, onboarding dismissal, `acp_history_store()` | High |
| `crates/agent_ui/src/acp/thread_view.rs` | `HeadlessConnection`, `UserCreatedThread`, `ThreadTitleChanged`, THREAD_REGISTRY, history refresh | High |
| `crates/acp_thread/src/acp_thread.rs` | `content_only()` method (Critical Fix #3) | Medium |
| `crates/feature_flags/src/flags.rs` | `AcpBetaFeatureFlag::enabled_for_all()` returns `true` | Low |
| `crates/extensions_ui/src/extensions_ui.rs` | Agent keyword/upsell removal | Low |
| `crates/recent_projects/src/dev_container_suggest.rs` | `suggest_dev_container` early return | Low |
| `crates/http_client_tls/src/http_client_tls.rs` | `NoCertVerifier` for insecure TLS | Low |
| `crates/reqwest_client/src/reqwest_client.rs` | Insecure TLS support | Low |
| `crates/agent_settings/src/agent_settings.rs` | `show_onboarding`, `auto_open_panel` settings | Low |
| `crates/zed/src/zed.rs` | WebSocket sync init (cfg-gated) | Low |
| `crates/title_bar/` | Connection status indicator | Low |
| `Cargo.toml` (workspace) | `external_websocket_sync` member + dependency | Low |

## `from_existing_thread()` Compatibility Check

The most fragile integration point: `thread_view.rs::from_existing_thread()` constructs a `ConnectedServerState` that must match upstream's struct fields. After merge, verify `ConnectedServerState` still has `active_id`, `threads` HashMap, and `conversation` Entity — upstream may rename or restructure this.

## Porting Guide Updates

During conflict resolution, document **every new upstream change** that affected a Helix-modified file:
- New fields added to structs we extend
- Method signature changes in functions we call
- Trait changes for interfaces we implement (`AgentConnection`, etc.)
- New feature flags or settings we should be aware of

Add these to the **Rebase Checklist** section of `portingguide.md` and update the commit history table.

## E2E Test Pipeline

The E2E test (`Dockerfile.ci`) is the final validation gate. It:
1. Builds Go test server with real Helix server code
2. Builds the Zed binary (from pre-built binary in CI)
3. Runs 7-phase test against a real LLM (Anthropic API)

If CI passes the `zed-e2e-test` Drone step, the merge is complete. The `sandbox-versions.txt` in the helix repo then needs updating to the new commit hash so helix CI uses the new Zed build.

## Patterns Found in Codebase

- All Helix additions are `#[cfg(feature = "external_websocket_sync")]` gated — conflicts are isolated
- Previous merges used `git merge upstream/main` approach (not rebase)
- CI uses Drone with a dedicated `zed-e2e-test` step that requires `ANTHROPIC_API_KEY` secret
- The Go test server in `e2e-test/helix-ws-test-server/` imports real Helix API code via `replace` directive in `go.mod` — the CI Dockerfile patches this path
- Unit tests: `cargo test -p external_websocket_sync` (no Postgres required)
- Build check: `cargo check --package zed --features external_websocket_sync`
