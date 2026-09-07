# Requirements: Merge Latest Zed Upstream Into Helix Fork

## Context

Today is **2026-09-07**. This is the next cycle in the recurring
`zed-industries/zed` → Helix fork upstream-merge series.

**Read these first:**

- `helix-specs/design/tasks/003012_merge-latest-zed/` (2026-08-31) — the previous
  spec. Its conflict analysis, `git_ui_core` symbol map, rustc-bump plan and
  audit strategy are all still valid; this spec records the **delta** plus fresh
  measurement.
- `helix-specs/design/tasks/002930_merge-latest-zed/` (2026-08-24) and
  `002701_merge-latest-zed/` (2026-08-10) — earlier windows of the same
  unexecuted merge. 002701's design.md holds the original `git_ui` → `git_ui_core`
  symbol map.
- `/home/retro/work/zed/portingguide.md` — the living porting guide.

### Critical finding — this is the FOURTH unexecuted window

**003012 was never executed**, exactly as 002930 and 002701 before it. Measured
today on `/home/retro/work/zed`:

| Evidence | Value | Means |
|---|---|---|
| `003012/tasks.md` checkboxes ticked | **0 of 82** | spec committed 2026-08-31, never worked |
| `rust-toolchain.toml` channel | `1.95.0` | rustc bump never happened |
| `crates/git_ui_core` | does not exist | migration never happened |
| `crates/csv_preview`, `crates/rich_text`, `crates/supermaven` | still present | crate churn never absorbed |
| `crates/zed/Cargo.toml` version | `1.15.0` | last upstream absorb was 2026-07-29 |
| newest `## Merge` in `portingguide.md` | `2026-07-29` (line 743) | no merge landed since |
| merge-base with upstream | `b9256fa8f0` (2026-07-29) | the fence has not moved |
| fork `main` HEAD | `1e0be14e6c` (2026-08-27) | unchanged since 003012 was written |

Backlog growth across four planning cycles: **128 → 331 → 434 → 540 commits.**
Nothing at all changed on the fork side in the last week — not one commit. Every
work item in 002701, 002930 and 003012 is still outstanding. This spec supersedes
them by measurement, not by content; their design docs remain directly applicable.

### Measured baseline (2026-09-07, from `/home/retro/work/zed`)

```
fence (merge-base)   b9256fa8f018bf03eb2e420120163746f8298d83  2026-07-29
                     "Stop the npm cache from growing without bound (#61750)"
upstream HEAD        1870e269ad88802147f2baec3086abb67d17260a  2026-09-07
                     "grammars: Highlight C23 `constexpr` keyword (#63833)"
fork HEAD (main)     1e0be14e6cf6db374a26af2f56cfb1419ae6e12b  2026-08-27
commits to merge     540     (git rev-list --count origin/main..upstream/main)
fork-only commits    351
window               40 days
ACP                  2.0.0 -> 2.0.0   ** NO BUMP ** (4th consecutive window)
rust-toolchain       1.95.0 -> 1.97.1
zed crate version    1.15.0 -> 1.20.0  (003012 measured 1.19.0)
textual conflicts    7 files (git merge-tree --write-tree; tree 1238fa2924)
new upstream crates  git_ui_core, gpui_apple, tabular_data_preview,
                     call_hierarchy, language_detection
removed upstream     csv_preview, rich_text, supermaven, supermaven_api, panel
```

Re-measure before starting — upstream moves daily — but the shape will hold.

### The seven textual conflicts (measured with `git merge-tree`)

003012 measured six. **`crates/agent_servers/src/acp.rs` is new this window and
is the only conflict that is not mechanical.**

| File | Nature |
|---|---|
| `crates/agent_servers/src/acp.rs` | **NEW and the headline work item.** Upstream replaced the ref-counted session-close model with an `observe_release`-driven one. 3 hunks. See below. |
| `Cargo.lock` | Routine. Take upstream, regenerate via the build. |
| `crates/http_client_tls/Cargo.toml` | Adjacency only: fork `rustls-pki-types = "1"` vs upstream `webpki-roots.workspace = true`. Union — keep both. |
| `crates/title_bar/Cargo.toml` | Fork `external_websocket_sync` optional dep + `git_ui.workspace` vs upstream `git_ui_core.workspace`. Keep the fork dep, drop `git_ui`, take `git_ui_core`. |
| `crates/agent_ui/Cargo.toml` | Fork `time`/`time_format`/`tokio` block vs upstream deleting `time`. Keep `time_format` + `tokio`; re-add `time` only if the compiler demands it. |
| `crates/zed/Cargo.toml` | Fork `tokio`/`ztracing`/`tracing` block vs upstream removal. Keep the fork lines; take upstream's version `1.20.0` and dependency reshuffle from the auto-merged region. |
| `crates/zed/src/main.rs` | One hunk: Helix `build_application(args.headless).with_assets(Assets)` vs upstream `build_application().with_assets(Assets).with_restart_arguments(restart_arguments)`. Union both. |

### The new `acp.rs` conflict — upstream changed the ACP session lifecycle

Upstream commit series in this window replaced explicit reference counting with
GPUI entity-release observation:

- `PendingAcpSession.ref_count` and `AcpSession.ref_count` **removed**;
  `AcpSession` gains `_release_subscription: Subscription`.
- New `AcpConnection::register_session(...)` installs `cx.observe_release(thread, …)`
  which removes the session and fires `CloseSessionRequest` when the last handle
  to the `AcpThread` entity drops; gated on new `agent_supports_session_close()`.
- `AcpConnection`'s `supports_close_session` / `close_session` overrides are
  **gone** — the trait defaults in `crates/acp_thread/src/connection.rs`
  (unchanged upstream) now apply.
- Unrelated but in the same file: the ACP IO future moved from
  `cx.background_spawn` to `background_executor().spawn_dedicated(...)` (macOS
  512 KiB GCD worker stacks overflow in dev builds). This part auto-merges.

The fork side of the same region carries:

- `force_close_session` (Helix PR #63), declared on the trait in
  `crates/acp_thread/src/connection.rs:151` and **called from
  `crates/external_websocket_sync/src/thread_service.rs:2674`** — a live Helix
  code path, not dead code.
- Fork extensions to `close_session` that decrement `pending.ref_count` /
  `session.ref_count` — fields that **no longer exist** after the merge.
- `session_creation_chain` / `SessionCreationGuard` / `acquire_session_creation_slot`
  (PR #50) — untouched by upstream, auto-merges.
- The test `test_concurrent_session_creation_is_serialized`, which conflicts
  textually with upstream's new `release_dropped_entities` test helper (union
  resolve; keep both).

### Answers to the brief's open questions (measured during planning)

| Brief question | Measured answer |
|---|---|
| How many new upstream commits? | **540** (`origin/main..upstream/main`). The brief's `helix-fork..origin/main` form does not apply — see Repository Layout. |
| Outcome of the 2026-08-31 specs? | **Neither ran.** 003012 has 0 of 82 tasks ticked and no follow-up commits; the fork is at the same commit it was on 2026-08-27. |
| Has the ACP crate version bumped? | **No.** `agent-client-protocol = "=2.0.0"` pinned identically on both sides; `Cargo.lock` resolves `2.0.0` / derive `2.0.0` / schema `1.5.0` on both. **No builder sweep needed** (fourth window). |
| Have high-conflict files been restructured upstream? | Yes — see the audit table below. `agent.rs` doubled (+363 → **+732**) and `zed.rs` grew (+816 → **+1096**) since 003012 measured them a week ago. |
| Have the ACP Elicitations PRs landed on the fork? | **No.** `crates/external_websocket_sync/src/**` on `origin/main` contains **zero** occurrences of `elicitation`; the e2e server enumerates **17 phases, no Phase 18**. The work still sits only on `origin/feature/002731-agent-questions` (tip `859325b38f`, 2026-08-12, 7 ahead / 16 behind `main` — no movement in three weeks). |
| Do elicitation types conflict with upstream additions? | **No.** `AgentThreadEntry::Elicitation`, `Elicitation`, `ElicitationStatus`, `ElicitationStoreEvent` are **upstream's own types**, present on both sides; `acp_thread.rs` auto-merges (±16). |
| New lessons in helix-specs since 2026-08-31? | **None.** 003012 is still the newest merge spec and nothing has been appended to it. |
| Do the in-flight Helix items conflict? | **No.** No commit has touched `api/pkg/server/websocket_external_agent_sync.go` since 2026-08-28, and all four listed items are Helix-repo Go/TS work with no Zed-side surface. |

### Upstream delta on the audited files (`git diff --stat b9256fa8f0 upstream/main`)

| File | 003012 measured | **003081 measured** | Why it matters |
|---|---|---|---|
| `crates/zed/src/zed.rs` | +816 | **+1096** | `initialize_agent_panel` (still present upstream) + WebSocket init host |
| `crates/agent/src/agent.rs` | +363 | **+732** | Fix #1 host (`pending_sessions`, `wait_for_tools_ready`) |
| `crates/agent_ui/src/agent_panel.rs` | +549 | **+585** | Fix #11, `from_existing_thread`, `ThreadDisplayNotification` |
| `crates/extensions_ui/src/extensions_ui.rs` | −662 | **−662** | 3× `// HELIX: External agent` markers, bulk moved to `components/extension_card.rs` |
| `crates/anthropic/src/anthropic.rs` | +502 | **+561** | new models — take upstream ordering wholesale |
| `crates/agent_servers/src/acp.rs` | ±33 | **±336, now conflicting** | session lifecycle rewrite + `SessionCreationGuard` |
| `crates/agent_ui/src/conversation_view.rs` | ±125 | **±134** | Fix #2 host (no duplicate WebSocket sends) |
| `crates/title_bar/src/title_bar.rs` | +108 | **+110** | sign-in suppression + `render_restricted_mode` |
| `crates/agent_ui/src/conversation_view/thread_view.rs` | ±88 | **±88** | Helix `current_model_id()` fallback |
| `crates/reqwest_client/src/reqwest_client.rs` | ±73 | **±73** | `ZED_HTTP_INSECURE_TLS` host |
| `crates/feature_flags/src/flags.rs` | −27 | **−27** | `AcpBetaFeatureFlag` survives upstream's pruning |
| `crates/language_models/src/provider/open_ai.rs` | ±18 | **±23** | light confirming read |
| `crates/acp_thread/src/acp_thread.rs` | ±16 | **±16** | elicitation block — confirm untouched |
| `crates/agent/src/tools/grep_tool.rs` | ±12 | **±12** | `truncate_long_lines()` / `MAX_LINE_CHARS` |
| `crates/http_client_tls/src/http_client_tls.rs` | ±11 | **±11** | insecure-TLS host |
| `crates/acp_thread/src/connection.rs` | unchanged | **unchanged** | confirming grep only (fork adds `force_close_session` here) |
| `crates/agent_ui/src/acp/**`, `crates/project/src/trusted_worktrees.rs` | unchanged | **unchanged** | confirming grep only |
| `crates/external_websocket_sync/**` | fork-only | **fork-only** | does not exist upstream; cannot conflict |

## Repository Layout (sandbox reality — measured)

- **Working repo**: `/home/retro/work/zed/`, branch **`main`** (not `helix-fork`).
  - `origin` = `http://helix-api.internal:18080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
    (in-cluster gitea mirror of `helixml/zed`). `origin/main` == local `main`.
  - `origin/helix-fork` exists but is **dead** (tip `746a9c4fb4`, 2026-02-07,
    4750 commits behind `main`). Do not use it.
  - `upstream` = `https://github.com/zed-industries/zed.git` — was **not**
    configured; added and fetched during this planning pass, confirmed reachable.
    Read-only.
- **Living porting guide**: `/home/retro/work/zed/portingguide.md` (123 KB).
  Newest merge section is `## Merge 2026-07-29 (upstream catch-up, 764 commits)`
  at line 743. The brief's path `design/2026-02-07-zed-fork-rebase-to-upstream.md`
  exists only in `/home/retro/work/helix/` as the historical one-time-port
  narrative with no `## Merge NNN` sections (Open Question 3).
- **Helix platform repo**: `/home/retro/work/helix/` —
  `ZED_COMMIT=6f9300a70db9126b5f03deeb883c19adc21d545b`, still the stale
  second-parent-of-HEAD value 003012 recorded.
- **Toolchain**: **no local `cargo`/`rustc`** — only `docker` and `go`. All Rust
  work goes through `cd /home/retro/work/helix && ./stack build-zed dev` or the
  e2e Docker images.
- **`ANTHROPIC_API_KEY` is NOT set** in the planning environment (Open Question 9).

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the 540 upstream commits, adapt the
> fork's session-close code to upstream's new release-observer lifecycle, and
> adopt `git_ui_core` and rustc 1.97.1, so the fork stops accumulating debt that
> four consecutive specs failed to clear.

### 2. Helix User
> As a Helix user, I want upstream's newest fixes without losing WebSocket sync,
> incremental streaming, headless mode, ACP routing, or any of the 11 Critical
> Fixes.

### 3. Agent-Questions Feature Owner
> As the owner of `feature/002731-agent-questions`, I want the merge to leave my
> 7 unmerged commits rebasable — `crates/external_websocket_sync/**` is fork-only
> and untouched upstream, so this should be near-trivial.

### 4. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` updated **as
> each conflict is resolved**, with a dated `## Merge 003081 (2026-09-07)` entry
> covering the ACP session-lifecycle adaptation, the `git_ui_core` migration and
> the rustc bump.

## Acceptance Criteria

### Merge completeness
- [ ] `upstream` remote present, fetched, treated as read-only
- [ ] Fence / commit count / upstream HEAD re-measured before starting and
      recorded in `portingguide.md`
- [ ] `git merge upstream/main` — **merge, not squash, not rebase**
- [ ] All 351 fork-only commits preserved; any skipped upstream commit justified
      in the porting guide
- [ ] `git log` confirms the fork branch contains and is ahead of `upstream/main`
- [ ] No `<<<<<<<` / `>>>>>>>` markers anywhere in the tree

### Conflict resolutions
- [ ] **`crates/agent_servers/src/acp.rs`** — take upstream's `observe_release` /
      `register_session` lifecycle wholesale; **delete** the fork's `ref_count`
      bookkeeping in `close_session` (the fields are gone); **keep
      `force_close_session`**, re-expressed against the new model (remove the
      `sessions` / `pending_sessions` entry, then send `CloseSessionRequest`),
      because `thread_service.rs:2674` calls it; keep `session_creation_chain` /
      `SessionCreationGuard` / `acquire_session_creation_slot` (PR #50) intact;
      union-resolve the test hunk so both
      `test_concurrent_session_creation_is_serialized` and upstream's
      `release_dropped_entities` helper survive
- [ ] `crates/acp_thread/src/connection.rs` — fork's `force_close_session` trait
      method retained (upstream left this file byte-unchanged)
- [ ] `crates/http_client_tls/Cargo.toml` — union: keep `rustls-pki-types = "1"`
      **and** upstream's `webpki-roots.workspace = true` / `log.workspace = true`
- [ ] `crates/title_bar/Cargo.toml` — keep
      `external_websocket_sync = { workspace = true, optional = true }` and its
      `[features]` entry; drop `git_ui`, take `git_ui_core`
- [ ] `crates/agent_ui/Cargo.toml` — keep `time_format` and `tokio` and the
      `external_websocket_sync` feature/dep; take upstream's removal of `time`,
      re-adding it only if the compiler demands it
- [ ] `crates/zed/Cargo.toml` — keep `tokio`, `ztracing`, `tracing`,
      `external_websocket_sync` optional dep and Helix `[features]`; take
      upstream's version `1.20.0` and dependency reshuffle
- [ ] `crates/zed/src/main.rs` — resolve to
      `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`;
      other `build_application(false)` call sites keep their argument;
      `args.allow_multiple_instances || args.headless` survives
- [ ] `Cargo.lock` — resolve `--theirs`, then regenerate by building

### `git_ui` → `git_ui_core` migration
Measured symbol split (upstream `git_ui_core/src/git_ui_core.rs` vs
`git_ui/src/git_ui.rs`):

- **Moved to `git_ui_core`**: `worktree_service`, `created_worktrees`,
  `worktree_picker`, `worktree_names`, `notifications`, `askpass_modal`,
  `file_diff_view`, plus free functions `build_branch_picker`,
  `set_branch_picker_builder`, `open_file_history`, `set_file_history_opener`
- **Stayed in `git_ui`**: `init`, `git_panel`, `git_picker`, `git_graph`,
  `project_diff`, `solo_diff_view`, `staged_diff`, `unstaged_diff`,
  `branch_diff`, `commit_view`

- [ ] All 13 fork files referencing `git_ui::` re-pointed **symbol by symbol**
      (compile-driven, never a blanket rename):
      `agent_ui/src/{agent_panel.rs,test_support.rs,thread_worktree_archive.rs}`,
      `sidebar/src/sidebar.rs`, `title_bar/src/title_bar.rs`,
      `zed/src/{main.rs,zed.rs,visual_test_runner.rs,zed/open_listener.rs}`,
      `project_panel/src/{project_panel.rs,project_panel_tests.rs}`,
      `vim/src/test/vim_test_context.rs`,
      `collab/tests/integration/git_tests.rs`
- [ ] `git_ui::git_picker::popover(workspace, repo, GitPickerTab::Branches, rems(34.), window, cx)`
      migrated to `git_ui_core::build_branch_picker(workspace, repo, window, cx)` —
      different arity **and** return type (already `Option`, drop the `Some(..)`)
- [ ] `git_ui::init(cx)` stays as-is (`git_ui_core` exposes no `init`)
- [ ] `git_ui_core.workspace = true` added to each crate that needs it; `git_ui`
      removed from a manifest only when nothing in that crate still uses it

### rustc `1.95.0 → 1.97.1`
- [ ] `rust-toolchain.toml` takes upstream's `channel = "1.97.1"`
- [ ] `/home/retro/work/helix/Dockerfile.zed-build` resolves and installs 1.97.1
      from `rust-toolchain.toml` with no manual pin edit — **verify**, do not assume
- [ ] New-compiler diagnostics in fork-only code fixed, following upstream's own
      remedy where one exists
- [ ] Cold-cache full rebuild budgeted for

### Crate churn
- [ ] Added and building: `git_ui_core`, `gpui_apple`, `tabular_data_preview`,
      `call_hierarchy`, `language_detection`
- [ ] Removed and gone: `csv_preview`, `rich_text`, `supermaven`,
      `supermaven_api`, **`panel`** (new this window — `crates/git_ui/Cargo.toml`
      currently has `panel.workspace = true`; upstream's `git_ui` no longer does,
      so take upstream's manifest, then grep for any remaining `panel::` use)
- [ ] `crates/zed/Cargo.toml` `csv_preview.workspace = true` dropped with the crate
- [ ] Workspace `[workspace] members` keeps every Helix member
      (`external_websocket_sync`, `sidebar`, …) while absorbing upstream's
      additions, removals and alphabetical reordering

### Auto-merge audit (auto-merged ≠ correct)
- [ ] **P1 `crates/agent/src/agent.rs` (+732)** — largest delta this window. Fix
      #1: `NativeAgent` clone / `pending_sessions` shared-task in `load_session()`
      (fork uses `get_mut`, upstream `get`) and `wait_for_tools_ready` intact
- [ ] **P2 `crates/zed/src/zed.rs` (+1096)** — `initialize_agent_panel` still
      exists upstream; verify it and the WebSocket init inside it survive, plus
      new action registrations / `initialize_panels`
- [ ] **P3 `crates/agent_ui/src/agent_panel.rs` (+585)** — `send_agent_ready`,
      `wait_for_websocket_connected`, UI-state-query callback, `acp_history_store()`,
      `from_existing_thread`, `ThreadDisplayNotification` handler, Fix #11
- [ ] **P4 `crates/agent_ui/src/conversation_view.rs` (±134)** — Fix #2, no
      duplicate WebSocket sends; note it calls `supports_close_session()` /
      `close_session()` at lines ~791 and must still compile against upstream's
      trait defaults now that `AcpConnection` no longer overrides them
- [ ] **P5 `crates/title_bar/src/title_bar.rs` (+110)** — `render_restricted_mode()`
      returns `None` under the feature gate; `&& !cfg!(feature = "external_websocket_sync")`
      sign-in suppression survives
- [ ] **P6 `crates/extensions_ui/src/extensions_ui.rs` (−662)** — the 3×
      `// HELIX: External agent` markers must be present **and in a live code
      path**; re-apply in `components/extension_card.rs` if upstream moved the
      host, and record the move
- [ ] **P7 `crates/reqwest_client/src/reqwest_client.rs` (±73)** —
      `ZED_HTTP_INSECURE_TLS` intact
- [ ] **P8 `crates/agent_ui/src/conversation_view/thread_view.rs` (±88)** — Helix
      `current_model_id()` fallback
- [ ] **P9 `crates/anthropic/src/anthropic.rs` (+561)** — take upstream ordering
      wholesale
- [ ] **P10 `crates/http_client_tls/src/http_client_tls.rs` (±11)** — fork's
      `rustls-pki-types` usage compiles alongside upstream's `webpki-roots`
- [ ] **P11 `crates/agent/src/tools/grep_tool.rs` (±12)** — `truncate_long_lines()`
      / `MAX_LINE_CHARS = 500`
- [ ] **P12 `crates/feature_flags/src/flags.rs` (−27)** — `AcpBetaFeatureFlag`
      survives and keeps `enabled_for_all() -> true`
- [ ] **P13 `crates/zed/src/main.rs`** — `--headless`, `--allow-multiple-instances`,
      `initialize_headless()` intact around upstream's `restart_arguments` flow
- [ ] **P14 `crates/language_models/src/provider/open_ai.rs` (±23)** — light read
- [ ] Confirming grep only (verified byte-unchanged upstream):
      `crates/acp_thread/src/connection.rs`, `crates/agent_ui/src/acp/**`,
      `crates/project/src/trusted_worktrees.rs`, `crates/external_websocket_sync/**`

### ACP — no bump (fourth window)
- [ ] `agent-client-protocol = { version = "=2.0.0", features = ["unstable"] }`
      unchanged in `Cargo.toml`; `Cargo.lock` resolves `2.0.0` / derive `2.0.0` /
      schema `1.5.0`. Both sides are already identical — **any change here is a
      mistake**
- [ ] No builder-pattern sweep expected. If one becomes necessary the pin moved —
      stop and record why
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` returns 0

### Elicitations
- [ ] `AgentThreadEntry::Elicitation`, `Elicitation`, `ElicitationStatus`,
      `ElicitationStoreEvent` present and unchanged in `acp_thread.rs` after the
      merge (upstream's own types)
- [ ] `feature/002731-agent-questions` left untouched by this merge; a note in
      the porting guide records that it must be rebased onto the merge result

### Critical Fix preservation (`portingguide.md` §"Critical Fixes")
All 11 must survive. Three fix hosts changed substantially (`agent.rs` +732,
`agent_panel.rs` +585, `conversation_view.rs` ±134), so this is a real audit.
- [ ] Fix #1 `pending_sessions` shared-task — audit properly
- [ ] Fix #2 no duplicate WebSocket sends — audit properly
- [ ] Fix #3 `content_only()` strips `## Assistant` heading
- [ ] Fix #4 `notify_thread_display()` for follow-ups to non-visible threads
- [ ] Fix #5 stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6 every `send()` emits exactly one `Stopped` (`stopped_emitted_for_task`)
- [ ] Fix #7 `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8 `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9 `stopped_emitted_for_task` guard on the normal-completion path
- [ ] Fix #11 entity-identity guard in `agent_panel.rs` `load_agent_thread`
- [ ] PR #63 `force_close_session` still reachable from `thread_service.rs`

### Helix-specific surface (brief constraints 1–13, re-verified)
- [ ] `crates/external_websocket_sync/` intact (10 source files)
- [ ] Compiles **with and without** the `external_websocket_sync` feature
- [ ] `ThreadDisplayNotification` handler still calls
      `OnboardingUpsell::set_dismissed(true, cx)` and initialises
      `NativeAgentSessionList`
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true`
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) stays under
      `cfg(not(feature = "external_websocket_sync"))`
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved so
      `message_added` streams incrementally (`subscribe_in` is silently dropped
      without a window context)
- [ ] **Constraints #8 and #12 remain SUPERSEDED** — `trust_all_worktrees` and
      `show_sign_in` are cfg gates in `crates/project/src/trusted_worktrees.rs`
      and `crates/title_bar/src/title_bar.rs`, **not** entries in
      `assets/settings/default.json`. Audit the gates, not the JSON, and do not
      restore the JSON keys
- [ ] `title_bar`'s `external_websocket_sync` dep stays `optional = true`;
      workspace `rust-embed` keeps `debug-embed`; `wait_for_tools_ready` uses
      `cx.background_executor().timer()`
- [ ] `dev_container_suggest.rs` early return; migration banner `Hidden`;
      trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches stay exhaustive; Fix 1b
      cfg-gated draft-suppression `return;` is still the FIRST statement of its
      `BaseView::Uninitialized` branch

### Build & test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds, zero errors
- [ ] Feature-off `cargo check -p zed` via a one-off Docker run in the build image
- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] `cargo test -p agent_servers` session-lifecycle tests pass against upstream's
      new release-observer model (the tests around `close_session` changed upstream)
- [ ] **E2E Docker test — HARD GATE.** Run
      `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` (preferred over
      the brief's raw `docker build`/`docker run`; same image, handles wiring).
      Run `go mod tidy` in `e2e-test/helix-ws-test-server/` first. Never use
      `--no-build` while investigating a failure
- [ ] All **17** phases green for `zed-agent`, and for `claude` via
      `E2E_AGENTS="zed-agent,claude"`. Phase 18 does not exist on `main` — see
      Open Question 1
- [ ] One retry permitted per agent for the known Claude Phase-1 npm-install
      flake; a second failure is a real failure
- [ ] helixml/zed CI green (drone `build-zed`, `zed-e2e-image`,
      `zed-e2e-headless-smoke`, `zed-e2e-protocol-lifecycle`,
      `zed-e2e-live-claude-latest` in `/home/retro/work/helix/.drone.yml`)

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] New `## Merge 003081 (2026-09-07)` section inserted **above**
      `## Merge 2026-07-29 …` (line ~743)
- [ ] Window summary: 540 commits, 40-day window, fence and upstream HEAD SHAs,
      ACP unchanged, rustc `1.95.0 → 1.97.1`, zed `1.15.0 → 1.20.0`
- [ ] `### ACP session lifecycle: ref counting → observe_release` — the highest
      value section this window; record how `force_close_session` was re-expressed
- [ ] `### Conflicts and Resolutions` — all seven files, hunk by hunk
- [ ] `### git_ui → git_ui_core migration` — the full moved/stayed symbol map
- [ ] `### rustc 1.95 → 1.97` — bump, Docker-builder outcome, fork diagnostics
- [ ] `### Crate churn` — five added, five removed (incl. `panel`)
- [ ] `### Retired / superseded Helix patches` — the constraint #8/#12 supersession
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Commit-history table extended; stale Rebase-Checklist entries corrected
- [ ] A note that **002701, 002930 and 003012 were all planned and never
      executed** — why this window is 540 commits and not ~100

### Process
- [ ] Feature branch `feature/003081-merge-latest-zed` cut from fork `main`
- [ ] Branch pushed to `origin` (mirrors `helixml/zed`); `main` not force-pushed;
      `origin/helix-fork` left untouched; no agent-initiated PRs
- [ ] `sandbox-versions.txt` `ZED_COMMIT` bumped from the stale
      `6f9300a70db9126b5f03deeb883c19adc21d545b` to the merge HEAD, on a
      `feature/003081-merge-latest-zed` branch in `/home/retro/work/helix/`
- [ ] `pull_request_zed.md` + `pull_request_helix.md` written into this task dir
- [ ] Re-fetch `upstream/main` and `origin/main` before declaring done; run an
      extension round if upstream advanced materially mid-work

## Out of Scope

- Net-new Helix feature development
- Merging `feature/002731-agent-questions` into `main` (unless Open Question 1
  says otherwise)
- Modifying e2e assertions unless an upstream API change strictly requires it
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors beyond what the merge, the `git_ui_core` split and the ACP
  session-lifecycle change force
- Adopting upstream's new UX (native elicitation UI, message-queue steering,
  sandboxing) into Helix-mode flows beyond keeping them compiling
- Rewriting the porting guide from scratch — amend and extend in place

## Open Questions

1. **Phase 18 / elicitations are still not on `main` — how should this be
   sequenced? (third ask.)** The brief again makes Phase 18 a mandatory gate, but
   the elicitation events and Phase 18 exist only on
   `origin/feature/002731-agent-questions` (7 ahead / 16 behind, no movement
   since 2026-08-12). Options: (a) merge upstream into `main`, gate on 17 phases,
   rebase the feature branch afterwards; (b) land the feature branch first, then
   merge and gate on 18; (c) merge upstream into the feature branch.
   **Assumption: (a)** — `external_websocket_sync` is fork-only so the later
   rebase is near-trivial, and landing an unmerged branch first adds risk to an
   already large window. Please confirm.

2. **Four consecutive merge specs have gone unexecuted (escalation, not a
   technical question).** 002701, 002930 and 003012 all planned this same merge;
   none ran, and the fork has not received a single commit since 2026-08-27. The
   backlog has grown 128 → 331 → 434 → 540. If there is a blocker upstream of
   planning (capacity, credentials, build-image breakage), naming it is worth
   more than a fifth spec. **Assumption: no known blocker; this window executes.**

3. **`force_close_session` under upstream's new lifecycle.** Upstream deleted
   `AcpConnection`'s ref-counted close in favour of `observe_release`. The fork's
   PR #63 `force_close_session` is still called from `thread_service.rs:2674`.
   **Assumption: keep it, re-expressed as "drop the bookkeeping entry, then send
   `CloseSessionRequest`", and drop the fork's ref-count arithmetic.** If instead
   the release-observer makes it entirely redundant, say so — deleting it and its
   trait declaration would be cleaner but changes Helix thread-teardown timing.

4. **Porting-guide path (sixth ask — 002265, 002353, 002701, 002930, 003012).**
   The brief names `design/2026-02-07-zed-fork-rebase-to-upstream.md`, which
   exists only in the **helix** repo as a historical narrative with no
   `## Merge NNN` sections. **Assumption: `zed/portingguide.md` is the target.**
   Confirm, or say if both should be touched — and ideally correct the brief.

5. **Repo/branch layout (fifth ask).** The brief says
   `/prod/home/luke/pm/zed-upstream` on branch `helix-fork` with remotes
   `helix`/`origin`. Sandbox reality is `/home/retro/work/zed` on `main`;
   `origin/helix-fork` is dead at 2026-02-07, 4750 commits behind.
   **Assumption: work on `main` in the sandbox layout.**

6. **`--locked` in CI (brief constraint #5) — stale, fifth ask.** There is no
   `.drone.yml` in the zed repo; Helix's `Dockerfile.zed-build` runs
   `cargo build --features external_websocket_sync` with **no** `--locked`.
   **Assumption: the constraint is obsolete; verify CI green and do not add
   `--locked` without instruction.** A yes/no would let us delete it permanently.

7. **rustc 1.97.1 in the build image (carried).** `Dockerfile.zed-build` installs
   rustup with `--default-toolchain none` and lets `rust-toolchain.toml` drive
   the version, so 1.97.1 *should* resolve automatically — but the BuildKit
   `/root/.rustup` cache mount must fetch a fresh toolchain and the base image
   must support it. **Assumption: no Dockerfile change needed.** Flag immediately
   if not; do not pin back to 1.95.

8. **`crates/panel` removal (new this window).** Upstream deleted it;
   `crates/git_ui/Cargo.toml` on the fork still has `panel.workspace = true`,
   inherited from base. **Assumption: take upstream's manifest, drop the crate,
   and fix any residual `panel::` use.** If a Helix crate turns out to depend on
   it the compiler will say so — flag rather than resurrecting the crate.

9. **E2E credentials — measured absent.** `ANTHROPIC_API_KEY` is not set in the
   planning sandbox. The E2E hard gate cannot run without it. **Assumption: the
   implementation agent's environment provides it.** If not, flag immediately —
   the merge cannot be declared complete.

10. **Feature-off compilation gate (carried).** With no local cargo, the cleanest
    way to `cargo check -p zed` *without* `external_websocket_sync` is a one-off
    Docker run in the build image. Is there a `./stack` target for a feature-off
    build? **Assumption: none; use a one-off Docker invocation.**
