# Design: Merge Latest Zed Upstream Into Helix Fork

## Companion documents

- `003012_merge-latest-zed/design.md` (2026-08-31) — still accurate for the
  `git_ui_core` migration, rustc bump, crate churn and audit strategy. Not
  superseded in content, only in measurement.
- `002701_merge-latest-zed/design.md` (2026-08-10) — the original `git_ui` →
  `git_ui_core` symbol map.
- `/home/retro/work/zed/portingguide.md` — the living guide; §"Critical Fixes"
  is the authoritative list of what must survive.

## 1. Situation

540 upstream commits over 40 days sit between fence `b9256fa8f0` (2026-07-29) and
`upstream/main` `1870e269ad` (2026-09-07). The fork has not moved since
2026-08-27. Three previous specs planned this same merge and none ran, so the
window is four times larger than one cycle's worth of drift.

`git merge-tree --write-tree origin/main upstream/main` yields tree
`1238fa2924` with **7 conflicted files**. Six are mechanical (manifests, one
line in `main.rs`, `Cargo.lock`). The seventh —
`crates/agent_servers/src/acp.rs` — is a semantic conflict and is where the
thinking goes.

## 2. Merge strategy

**True merge commit**, `git merge upstream/main` on a
`feature/003081-merge-latest-zed` branch cut from fork `main`.

- Rebase is rejected: it rewrites 351 fork-only commits and invalidates every
  open feature branch, including `feature/002731-agent-questions`.
- Squash is rejected: it destroys upstream bisectability, which is exactly what
  makes an audit of a 540-commit window tractable.

**Sequencing: merge into `main`; rebase `feature/002731-agent-questions`
afterwards.** `crates/external_websocket_sync/**` does not exist upstream and
therefore cannot conflict with it; the branch's 7 commits touch only that crate
and the Go e2e server, so the post-merge rebase is near-trivial. The e2e gate for
this merge is therefore the **17 phases that exist on `main`**, not the brief's
18 (Open Question 1).

## 3. The ACP session-lifecycle conflict (the one real design decision)

### What upstream did

| Before (fence) | After (upstream/main) |
|---|---|
| `PendingAcpSession { task, ref_count }` | `PendingAcpSession { task }` |
| `AcpSession { …, ref_count: usize }` | `AcpSession { …, _release_subscription: Subscription }` |
| `AcpConnection::close_session` decrements a count, closes at zero | `AcpConnection::register_session()` installs `cx.observe_release(thread, …)`; when the last handle to the `AcpThread` entity drops, the session is removed and `CloseSessionRequest` is sent |
| `AcpConnection::supports_close_session()` override | gone — the `AgentConnection` trait defaults in `crates/acp_thread/src/connection.rs` (byte-unchanged upstream) now apply; internally gated on the new `agent_supports_session_close()` |
| ACP IO future on `cx.background_spawn` | `background_executor().spawn_dedicated(…)` — dev builds need ~0.5 MiB of stack per inbound dispatch, more than macOS GCD's 512 KiB |

Net effect: session teardown becomes a consequence of GPUI entity lifetime rather
than an explicit call.

### What the fork has in the same region

1. `session_creation_chain` + `SessionCreationGuard` + `acquire_session_creation_slot`
   (PR #50) — serializes `new_session`/`load_session`/`resume_session` so
   `claude-agent-acp` cannot spawn two child SDK processes that race to
   `npx`-install the same MCP servers. **Untouched by upstream; auto-merges.**
2. Fork edits inside `close_session` that decrement `pending.ref_count` and
   `session.ref_count`. **These fields no longer exist.**
3. `force_close_session` (PR #63) — declared on the trait at
   `crates/acp_thread/src/connection.rs:151`, implemented in `acp.rs`, and
   **called from `crates/external_websocket_sync/src/thread_service.rs:2674`**.
   Live Helix code, not dead weight.
4. `test_concurrent_session_creation_is_serialized`, textually adjacent to
   upstream's new `release_dropped_entities` test helper.

### Resolution

**Take upstream's lifecycle wholesale; keep only what Helix actually calls.**

- Delete the fork's ref-count arithmetic — it is bookkeeping for a mechanism that
  no longer exists, and preserving it would mean re-adding fields upstream
  deliberately removed.
- Keep `force_close_session`, re-expressed against the new model: remove the
  entry from `pending_sessions` and `sessions` (which also drops the release
  subscription), then send `CloseSessionRequest`. Same observable behaviour Helix
  depends on — "tear this session down now, regardless of who still holds a
  handle" — with none of the removed state.
- Leave `supports_close_session` / `close_session` unimplemented on
  `AcpConnection` (trait defaults apply), matching upstream. Verify the call
  sites in `crates/agent_ui/src/conversation_view.rs:791` and the `agent.rs`
  wrappers still behave: they check `supports_close_session()` first, so the
  default `false` degrades to a no-op rather than an error.
- Union-resolve the test hunk: keep both the fork's serialization test and
  upstream's `release_dropped_entities` helper.

Alternative rejected: re-introducing `ref_count` alongside `_release_subscription`.
Two competing lifetime mechanisms in one struct is exactly the kind of debt that
makes the *next* window worse, and it would fight upstream's tests.

## 4. The six mechanical conflicts

| File | Resolution |
|---|---|
| `crates/http_client_tls/Cargo.toml` | Union. Fork's `rustls-pki-types = "1"` and upstream's `webpki-roots.workspace = true` / `log.workspace = true` are adjacent lines with no semantic overlap; both are needed. |
| `crates/title_bar/Cargo.toml` | Keep `external_websocket_sync = { workspace = true, optional = true }` and its `[features]` entry; drop `git_ui.workspace = true`; take `git_ui_core.workspace = true`. |
| `crates/agent_ui/Cargo.toml` | Keep `time_format` + `tokio` (fork-only); take upstream's deletion of `time`. Re-add `time` only if the compiler demands it — compile-driven, not guessed. |
| `crates/zed/Cargo.toml` | Keep `tokio`, `ztracing`, `tracing` and the Helix `[features]`/optional-dep entries; take upstream's `version = "1.20.0"` and dependency reshuffle from the auto-merged region. |
| `crates/zed/src/main.rs` | One hunk. Resolve to `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)` — Helix's argument plus upstream's builder call. |
| `Cargo.lock` | `--theirs`, then regenerate by building. Never hand-edit. |

## 5. `git_ui` → `git_ui_core` migration

Upstream split a UI-free core out of `git_ui`. Measured from
`upstream/main:crates/git_ui_core/src/git_ui_core.rs` and `…/git_ui/src/git_ui.rs`:

- **Moved to `git_ui_core`**: `worktree_service`, `created_worktrees`,
  `worktree_picker`, `worktree_names`, `notifications`, `askpass_modal`,
  `file_diff_view`; free functions `build_branch_picker`,
  `set_branch_picker_builder`, `open_file_history`, `set_file_history_opener`.
- **Stayed in `git_ui`**: `init`, `git_panel`, `git_picker`, `git_graph`,
  `project_diff`, `solo_diff_view`, `staged_diff`, `unstaged_diff`,
  `branch_diff`, `commit_view`. `git_ui_core` has **no** `init`, so
  `git_ui::init(cx)` in `zed.rs` stays exactly as it is.

Thirteen fork files reference `git_ui::`. By frequency the fork's usage is
dominated by moved symbols (`worktree_service::*` ×8, `created_worktrees::*` ×6,
`worktree_picker::WorktreePicker`) plus stayed symbols (`git_graph::*` ×5,
`git_panel::GitPanel`, `project_diff::*`, `staged_diff`, `unstaged_diff`,
`solo_diff_view`). **A blanket sed rename is wrong** — it would break the panels
that stayed. Migrate symbol by symbol, driven by the compiler.

The one signature change: `git_ui::git_picker::popover(workspace, repo,
GitPickerTab::Branches, rems(34.), window, cx)` becomes
`git_ui_core::build_branch_picker(workspace, repo, window, cx)` — fewer
arguments, and it already returns `Option`, so the `Some(..)` wrapper at the call
site is dropped.

Add `git_ui_core.workspace = true` wherever needed; remove `git_ui` from a
manifest only when nothing in that crate still references it (for `title_bar` and
`agent_ui`, upstream removed it outright — follow that).

## 6. rustc 1.95.0 → 1.97.1 and crate churn

`rust-toolchain.toml` takes upstream's `channel = "1.97.1"`.
`/home/retro/work/helix/Dockerfile.zed-build` installs rustup with
`--default-toolchain none` and lets `rust-toolchain.toml` drive the version, so
the bump *should* need no Dockerfile change — but the BuildKit `/root/.rustup`
cache mount has to fetch a new toolchain, so **verify rather than assume**, and
budget for a cold full rebuild since the toolchain change invalidates the Cargo
cache. Fix fork-only diagnostics using upstream's own remedy where one exists.

Crates added: `git_ui_core`, `gpui_apple`, `tabular_data_preview`,
`call_hierarchy`, `language_detection`. Removed: `csv_preview`, `rich_text`,
`supermaven`, `supermaven_api`, and — **new this window** — `panel`.
`crates/git_ui/Cargo.toml` on the fork still carries `panel.workspace = true`;
upstream's `git_ui` no longer does, so taking upstream's manifest resolves it.
`crates/zed/Cargo.toml` carries `csv_preview.workspace = true`, which goes with
the crate. After the merge, grep the workspace for any surviving `panel::`,
`rich_text::`, `supermaven` reference before declaring the removals clean.

## 7. The auto-merge audit

Auto-merged ≠ correct. Audit priority is re-derived from
`git diff --stat b9256fa8f0 upstream/main`, not inherited: `agent.rs` doubled to
**+732** and `zed.rs` grew to **+1096** in the single week since 003012 measured
them, which reorders the priority list.

P1 `agent/src/agent.rs` (+732) · P2 `zed/src/zed.rs` (+1096) ·
P3 `agent_ui/src/agent_panel.rs` (+585) · P4 `agent_ui/src/conversation_view.rs`
(±134) · P5 `title_bar/src/title_bar.rs` (+110) · P6
`extensions_ui/src/extensions_ui.rs` (−662) · P7 `reqwest_client` (±73) ·
P8 `conversation_view/thread_view.rs` (±88) · P9 `anthropic.rs` (+561) ·
P10 `http_client_tls.rs` (±11) · P11 `grep_tool.rs` (±12) · P12 `flags.rs` (−27) ·
P13 `zed/src/main.rs` · P14 `open_ai.rs` (±23).

Two specifics worth calling out:

- `extensions_ui.rs` (−662) is the canonical "auto-merged but wrong" case:
  upstream moved the bulk into `components/extension_card.rs`, so the 3×
  `// HELIX: External agent` markers can survive textually while landing in dead
  code. Follow the code, re-apply in the new file, and record the move.
- `conversation_view.rs` calls `supports_close_session()` / `close_session()`;
  after §3 those resolve to trait defaults. Confirm the guard-then-call pattern
  still behaves.

Confirming grep only — verified byte-unchanged upstream this window:
`acp_thread/src/connection.rs`, `agent_ui/src/acp/**`,
`project/src/trusted_worktrees.rs`, and `external_websocket_sync/**` (fork-only,
cannot conflict).

## 8. ACP protocol — no change, fourth window running

`agent-client-protocol = { version = "=2.0.0", features = ["unstable"] }` on both
sides; `Cargo.lock` resolves `2.0.0` / derive `2.0.0` / schema `1.5.0` on both.
**No builder-pattern sweep.** The non-exhaustive-struct discipline from earlier
windows is dormant. If a sweep becomes necessary, the pin moved — stop and record
why before proceeding. Note this is orthogonal to §3: the *protocol* is
unchanged; what moved is Zed's internal session bookkeeping.

## 9. Build and test strategy under sandbox constraints

There is **no local `cargo` or `rustc`** — only `docker` and `go`. Gate order,
cheapest signal first:

1. Merge + resolve seven conflicts → clean tree, no conflict markers
2. `cd /home/retro/work/helix && ./stack build-zed dev` → compile with the feature
3. One-off Docker `cargo check -p zed` **without** `external_websocket_sync`
4. `cargo test -p external_websocket_sync` (all unit/mock tests)
5. `cargo test -p acp_thread test_second_send` (Fix #6)
6. `cargo test -p agent_servers` — the serialization test (PR #50) **and**
   upstream's reworked session-lifecycle tests
7. **E2E hard gate**: `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`,
   after `go mod tidy` in `e2e-test/helix-ws-test-server/`. All 17 phases green
   for `zed-agent`, then for `claude` via `E2E_AGENTS="zed-agent,claude"`. One
   retry per agent for the known Claude Phase-1 npm-install flake; a second
   failure is real. Never `--no-build` while investigating.
8. helixml/zed CI green (`build-zed`, `zed-e2e-image`, `zed-e2e-headless-smoke`,
   `zed-e2e-protocol-lifecycle`, `zed-e2e-live-claude-latest` in Helix's
   `.drone.yml`)

`ANTHROPIC_API_KEY` was measured **absent** in the planning environment. Step 7
cannot run without it — check for it *before* the rebuild, not after.

## 10. Porting guide — written continuously

Target `/home/retro/work/zed/portingguide.md`. Insert
`## Merge 003081 (2026-09-07)` **above** `## Merge 2026-07-29 (upstream catch-up,
764 commits)` at line ~743, matching the reverse-chronological layout. Amend in
place; never rewrite.

Write each subsection as the work happens, in this order:

1. Window summary — fence SHA, upstream HEAD SHA, 540 commits, 40 days, ACP
   unchanged, rustc `1.95.0 → 1.97.1`, zed `1.15.0 → 1.20.0`
2. `### ACP session lifecycle: ref counting → observe_release` — **the highest
   value section this window**; how `force_close_session` was re-expressed and
   why the ref-count arithmetic was dropped
3. `### Conflicts and Resolutions` — all seven files, hunk by hunk
4. `### git_ui → git_ui_core migration` — the moved/stayed symbol map
5. `### rustc 1.95 → 1.97` — bump, Docker-builder outcome, fork diagnostics
6. `### Crate churn` — five added, five removed (incl. `panel`)
7. `### Retired / superseded Helix patches` — the #8/#12 supersession
8. `### Helix-surface survival check`
9. Commit-history table extended; stale Rebase-Checklist entries corrected
10. A note that **002701, 002930 and 003012 were planned and never executed**

## 11. Key decisions and rationale

| Decision | Alternative rejected | Why |
|---|---|---|
| True merge commit | Rebase / squash | 351 fork-only commits would be rewritten; squash destroys the bisectability that makes a 540-commit audit tractable |
| Take upstream's `observe_release` lifecycle; keep only `force_close_session` | Re-add `ref_count` beside `_release_subscription` | Two lifetime mechanisms in one struct is compounding debt and fights upstream's own tests |
| Keep `force_close_session` | Delete it as redundant | `thread_service.rs:2674` calls it; deleting changes Helix thread-teardown timing (Open Question 3) |
| Merge into `main`, rebase `002731-agent-questions` after | Land the feature branch first | `external_websocket_sync` is fork-only and cannot conflict; landing an unmerged branch first adds risk to an already large window |
| Compile-driven `git_ui_core` migration | Blanket rename | Only some symbols moved; a blanket rename breaks the panels that stayed |
| Take upstream's shape in every manifest, re-insert Helix lines | Line-by-line merge | Upstream reorders and prunes aggressively; taking its shape shrinks the diff *next* window too |
| Gate on 17 e2e phases | Gate on 18 per the brief | Phase 18 measurably does not exist on `main` |
| Re-derive audit priorities from a fresh `--stat` | Reuse 003012's ordering | `agent.rs` doubled in one week; inherited priorities skip real targets |

## 12. Learnings for future merge windows

- **Measure, do not inherit.** A week-old delta table is already wrong:
  `agent.rs` went +363 → +732 and a brand-new semantic conflict appeared in
  `acp.rs`.
- **`git merge-tree --write-tree origin/main upstream/main`** gives the exact
  conflict list without touching the working tree, and `git merge-file` on the
  three stage blobs shows the actual hunks. That pair is the right first
  measurement in any merge-planning pass and takes seconds.
- **Check whether the last spec actually ran** before writing the next one. Cheap
  proofs: ticked boxes in its `tasks.md`, `rust-toolchain.toml` channel,
  existence of the crate the last window was to add, newest `## Merge` heading in
  `portingguide.md`, whether `git merge-base` moved.
- **A fork-only crate cannot conflict.** `external_websocket_sync` does not exist
  upstream, which answers every "will sync survive the merge" question by
  construction and drives the sequencing decision.
- **Grep the *callers* before deleting fork code that no longer compiles.**
  `force_close_session` looks like dead ref-count scaffolding until you find the
  one call in `thread_service.rs`.
- **Auto-merged ≠ correct** — `extensions_ui.rs` −662 is the canonical example.
- **`ZED_COMMIT` is chronically stale by one parent**, pointing at a PR branch
  tip rather than the merge commit. Bump it to the merge HEAD and expect the same
  pattern next window.
