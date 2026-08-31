# Requirements: Merge Latest Zed Upstream Into Helix Fork

## Context

Today is **2026-08-31**. This is the next cycle in the recurring
`zed-industries/zed` → Helix fork upstream-merge series.

**Read these first — they are the primary working documents:**

- `helix-specs/design/tasks/002930_merge-latest-zed/` (2026-08-24) — the most
  recent merge spec. Its five-conflict table, `git_ui_core` migration plan,
  rustc-bump plan, constraint audit and open questions all still apply.
- `helix-specs/design/tasks/002701_merge-latest-zed/` (2026-08-10) — its
  design.md holds the full `git_ui` → `git_ui_core` symbol map, which is still
  the authoritative reference for that work item.
- `/home/retro/work/zed/portingguide.md` — the living porting guide.

This spec records only the **delta** since 002930 plus fresh measurement.

### Critical finding from the pre-work review — third unexecuted window

**Neither 002701 nor 002930 was ever executed.** Measured on
`/home/retro/work/zed` `origin/main` today:

| Evidence | Value | Means |
|---|---|---|
| `rust-toolchain.toml` `channel` | `1.95.0` | rustc bump never happened |
| `crates/git_ui_core` | does not exist | `git_ui_core` migration never happened |
| `crates/csv_preview` | still present | upstream crate churn never absorbed |
| files referencing `git_ui::` | 13 | unchanged since 002701's baseline |
| `crates/zed/Cargo.toml` `version` | `1.15.0` | last upstream absorb was 2026-07-29 |
| newest `## Merge` in `portingguide.md` | `2026-07-29` (line 743) | no merge landed since |
| merge-base with upstream | `b9256fa8f0` (2026-07-29) | the fence has not moved |

**Consequence: this is 002701's window plus three more weeks.** The backlog has
grown 128 → 331 → **434** commits across three planning cycles. Every work item
in 002701 and 002930 is still outstanding. This spec supersedes both by
measurement, not by content — their design docs remain directly applicable.

The only thing that changed on the fork side since 002930 is **PR #92**
(`codex/deterministic-headless-smoke`), which moved fork HEAD from
`7357327f2d` to `1e0be14e6c`.

### Measured baseline (2026-08-31, real numbers from `/home/retro/work/zed`)

```
fence (merge-base)   b9256fa8f018bf03eb2e420120163746f8298d83  2026-07-29
                     "Stop the npm cache from growing without bound (#61750)"
upstream HEAD        399258feeaf90ad8a3a208c99221ee87b6452f38  2026-08-29
                     "Reduce LLVM IR bloat in closure funnels (#63426)"
fork HEAD (main)     1e0be14e6cf6db374a26af2f56cfb1419ae6e12b  2026-08-27
                     "Merge pull request #92 from helixml/codex/deterministic-headless-smoke"
commits to merge     434     (git log --oneline origin/main..upstream/main | wc -l)
fork-only commits    351
window               33 days
ACP                  2.0.0 -> 2.0.0   ** NO BUMP ** (derive 2.0.0, schema 1.5.0, identical both sides)
rust-toolchain       1.95.0 -> 1.97.1  ** carried from 002701/002930, still the main risk **
zed crate version    1.15.0 -> 1.19.0  (002930 measured 1.18.0)
textual conflicts    6 files (git merge-tree --write-tree, resulting tree 86a57a88aa)
new upstream crates  git_ui_core, gpui_apple, tabular_data_preview,
                     call_hierarchy, language_detection
removed upstream     csv_preview, rich_text, supermaven, supermaven_api
```

Re-measure before starting — upstream moves daily — but the shape will hold.

### The six textual conflicts (measured, not predicted)

002930 measured five. `crates/http_client_tls/Cargo.toml` is **new** this window.

| File | Nature |
|---|---|
| `Cargo.lock` | Routine. Take upstream, regenerate via the build. |
| `crates/http_client_tls/Cargo.toml` | **NEW.** Fork added `rustls-pki-types = "1"`; upstream added `log.workspace = true` and `webpki-roots.workspace = true`. Union — keep all three. |
| `crates/title_bar/Cargo.toml` | Fork `external_websocket_sync` optional dep + `[features]` entry vs upstream `git_ui` → `git_ui_core`, drops `notifications`, drops the `cfg(windows)` `windows` dep, dev-dep reshuffle. |
| `crates/agent_ui/Cargo.toml` | Fork `time_format`/`tokio`/`external_websocket_sync_dep` block vs upstream removing `git_ui` and `time`, reordering `fuzzy`/`git`, adding `git_ui_core`, pruning five dev-deps. |
| `crates/zed/Cargo.toml` | Fork `tokio`/`external_websocket_sync` + `[features]` entry vs upstream version bump to `1.19.0`, new `inspector` feature, dependency reshuffle (`acp_thread`, `action_log` optional, `agent_servers` optional). |
| `crates/zed/src/main.rs` | Helix `build_application(args.headless)` vs upstream `build_application().with_restart_arguments(restart_arguments)`. |

All six are small. The real work is unchanged from 002701/002930: the
`git_ui_core` migration, the rustc bump, and the auto-merged-file audit —
except the audit surface has grown substantially (see below).

### Answers to the brief's open questions (measured during planning)

| Brief question | Measured answer |
|---|---|
| How many new upstream commits? | **434** (`origin/main..upstream/main`). Note the brief's `helix-fork..origin/main` form does not apply — see Repository Layout. |
| Has the ACP crate version bumped? | **No.** `agent-client-protocol = "=2.0.0"` in both `Cargo.toml`s; `Cargo.lock` resolves `agent-client-protocol 2.0.0`, `-derive 2.0.0`, `-schema 1.5.0` on **both** sides. **No builder-pattern sweep needed.** Third window running with no bump. |
| Have high-conflict files been restructured upstream? | Yes, and **much more than 002930 measured**. See the audit table below. |
| Does the `Elicitation` variant in `AgentThreadEntry` conflict with upstream? | **No — the variant is upstream's own.** `AgentThreadEntry::Elicitation(ElicitationEntryId)`, `struct Elicitation`, `ElicitationStatus`, `ElicitationStoreEvent` exist byte-identically on fork `main` (line 407) and `upstream/main` (line 398); the offset differs only by fork insertions earlier in the file. `acp_thread.rs` auto-merges (±16). |
| Is upstream's session-list/resume UI a replacement for `from_existing_thread`? | **No.** `crates/acp_thread/src/connection.rs` and `crates/agent_ui/src/acp/` are **byte-unchanged upstream** this window. Nothing forces the question. Keep `from_existing_thread`. |
| New lessons in helix-specs since 2026-08-24? | **None.** 002930 is still the newest merge spec; nothing was appended to it, and no design doc has been added since. |
| Were 002930's open questions answered in the interim? | **No.** All ten remain open and are re-raised below (several are now on their fourth or fifth ask). |

### Upstream delta on the audited files — grew sharply since 002930

Measured `git diff --stat b9256fa8f0 upstream/main`:

| File | 002930 measured | **003012 measured** | Why it matters |
|---|---|---|---|
| `crates/zed/src/zed.rs` | +382 | **+816 (32 hunks)** | `initialize_agent_panel` + WebSocket init host |
| `crates/agent_ui/src/agent_panel.rs` | +118 | **+549 (24 hunks)** | Fix #11, `from_existing_thread`, `ThreadDisplayNotification` |
| `crates/anthropic/src/anthropic.rs` | +330 | **+502** | new models — take upstream ordering wholesale |
| `crates/agent/src/agent.rs` | +43 | **+363** | Fix #1 host (`pending_sessions`, `wait_for_tools_ready`) |
| `crates/agent_ui/src/conversation_view.rs` | ±2 | **±125** | Fix #2 host (no duplicate WebSocket sends) |
| `crates/title_bar/src/title_bar.rs` | ±16 | **+108** | sign-in suppression + `render_restricted_mode` (constraint #12 host) |
| `crates/agent_ui/src/conversation_view/thread_view.rs` | ±68 | **±88** | Helix `current_model_id()` fallback |
| `crates/reqwest_client/src/reqwest_client.rs` | unchanged | **±73 (3 non-test hunks + new `mod tests`)** | `ZED_HTTP_INSECURE_TLS` host |
| `crates/extensions_ui/src/extensions_ui.rs` | −662 | **−662** | 3× `// HELIX: External agent` markers |
| `crates/agent_servers/src/acp.rs` | ±33 | **±33** | `SessionCreationGuard` (PR #50) |
| `crates/feature_flags/src/flags.rs` | −27 | **−27** | `AcpBetaFeatureFlag` — **verified survives upstream's pruning** |
| `crates/agent/src/tools/grep_tool.rs` | unchanged | **±12** | `truncate_long_lines()` / `MAX_LINE_CHARS` |
| `crates/http_client_tls/src/http_client_tls.rs` | unchanged | **±11** | insecure-TLS host |
| `crates/language_models/src/provider/open_ai.rs` | unchanged | **±18** | — |
| `crates/acp_thread/src/connection.rs` | unchanged | **unchanged** | confirming grep only |
| `crates/project/src/trusted_worktrees.rs` | — | **unchanged** | constraint #8 host — confirming grep only |
| `crates/agent_ui/src/acp/**` | — | **unchanged** | confirming grep only |
| `crates/external_websocket_sync/**` | fork-only | **fork-only** | cannot conflict; auto-merges clean |

**The four files that were "unchanged upstream, confirming grep only" in 002930
are now changed (`reqwest_client.rs`, `grep_tool.rs`, `http_client_tls.rs`,
`open_ai.rs`) and must be properly audited, not grepped.**

### ACP Elicitations / Phase 18 — correction to the brief (re-confirmed)

The brief states elicitation sync events and e2e Phase 18 have landed on the
fork. **Measured today: they have not**, exactly as 002930 reported.

- `crates/external_websocket_sync/src/**` on `origin/main` contains **zero**
  occurrences of `elicitation`.
- `e2e-test/helix-ws-test-server/main.go` on `origin/main` enumerates
  **17 phases**. There is no Phase 18.
- The work lives on `origin/feature/002731-agent-questions`, tip
  `859325b38f` (2026-08-12), now **7 commits ahead of and 16 behind
  `origin/main`** (was 7/14 at 002930 — it drifted two further commits behind).
  That branch adds `elicitation` references to `external_websocket_sync.rs`
  (30), `thread_service.rs` (106), `types.rs` (44), `websocket_sync.rs` (10),
  and Phase 18 + 44 elicitation references to the e2e server.

**Therefore Phase 18 cannot be a gate for this merge as things stand.** The
merge gate is **all 17 phases on `main`**. See Open Question 1 — this is now the
second consecutive spec asking the same question.

## Repository Layout (sandbox reality — measured)

- **Working repo**: `/home/retro/work/zed/`, branch **`main`** (not `helix-fork`).
  - `origin` = `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
    (in-cluster gitea mirror of `helixml/zed`). `origin/main` == local `main`.
  - `origin/helix-fork` exists but is **dead** (tip `746a9c4fb4`, 2026-02-07).
    Do not use it. The live branch is `main`.
  - `upstream` = `https://github.com/zed-industries/zed.git` — was not
    configured; **added and fetched during planning**, confirmed reachable.
    Treat as read-only.
- **Living porting guide**: `/home/retro/work/zed/portingguide.md` (1324 lines).
  Newest merge section is `## Merge 2026-07-29 (upstream catch-up, 764 commits)`
  at **line 743**. The brief's path
  `design/2026-02-07-zed-fork-rebase-to-upstream.md` exists only in
  `/home/retro/work/helix/` as the historical one-time-port narrative with no
  `## Merge NNN` sections. See Open Question 3.
- **Helix platform repo**: `/home/retro/work/helix/` — `sandbox-versions.txt`
  line 1 is `ZED_COMMIT=6f9300a70db9126b5f03deeb883c19adc21d545b`, which is
  **already stale**: it is the second parent of fork HEAD `1e0be14e6c`, not HEAD
  itself. The same stale-by-one-parent pattern 002930 recorded.
- **Toolchain**: **no local `cargo`/`rustc`.** Only `docker` and `go`. All Rust
  building/testing goes through `cd /home/retro/work/helix && ./stack build-zed dev`
  or the E2E Docker images.
- **`ANTHROPIC_API_KEY` is NOT set in the planning environment.** See Open
  Question 9 — the E2E gate cannot run without it.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the 434 upstream commits and adopt
> upstream's `git_ui_core` crate split and the rustc 1.97.1 bump, so the fork
> stops accumulating merge debt that three consecutive specs failed to clear and
> never returns to a 700-commit backlog.

### 2. Helix User
> As a Helix user, I want upstream's newest fixes and performance work without
> losing WebSocket sync, incremental streaming, headless mode, Codex/Claude ACP
> routing, or any of the 11 Critical Fixes.

### 3. Agent-Questions Feature Owner
> As the owner of `feature/002731-agent-questions`, I want the merge to land in a
> way that leaves my 7 unmerged commits rebasable without a second painful
> conflict resolution — `crates/external_websocket_sync/**` is fork-only and
> untouched upstream, so this should be achievable.

### 4. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` updated **as
> each conflict is resolved**, with a dated `## Merge 003012 (2026-08-31)` entry
> documenting the `git_ui_core` migration, the rustc bump and the crate churn —
> one-time structural changes whose resolution pattern is not obvious from the
> diff alone.

## Acceptance Criteria

### Merge Completeness
- [ ] `upstream` remote present (`https://github.com/zed-industries/zed.git`),
      fetched, treated as read-only
- [ ] Re-measure fence / commit count / upstream HEAD before starting; record
      the measured values in `portingguide.md`
- [ ] `git merge upstream/main` — **merge, not squash, not rebase**
- [ ] Merge contains all upstream commits through the recorded upstream HEAD;
      any skipped commit explicitly justified in `portingguide.md`
- [ ] All 351 fork-only commits preserved verbatim (PRs #73–#92)
- [ ] `git log` confirms the fork branch is ahead of and contains `upstream/main`
- [ ] No `<<<<<<<` / `>>>>>>>` markers anywhere in the tree

### The six textual conflicts
- [ ] `crates/http_client_tls/Cargo.toml` — **union resolve**: keep fork's
      `rustls-pki-types = "1"` **and** take upstream's `log.workspace = true`
      and `webpki-roots.workspace = true`
- [ ] `crates/title_bar/Cargo.toml` — keep
      `external_websocket_sync = { workspace = true, optional = true }` and the
      `[features]` `external_websocket_sync = ["dep:external_websocket_sync"]`
      entry; take upstream's `git_ui_core.workspace = true` and **drop**
      `git_ui.workspace = true`; take upstream's removal of `notifications`, the
      `[target.'cfg(windows)'.dependencies]` block, and the dev-dep reshuffle
      (`recent_projects` test-support in, `notifications`/`release_channel` out)
- [ ] `crates/agent_ui/Cargo.toml` — take upstream's shape (drops `git_ui`,
      drops `time`, reorders `fuzzy`/`git`, adds `git_ui_core`, prunes dev-deps
      `clock`, `node_runtime`, `remote_server`, `tree-sitter-md`, `unindent`);
      keep fork-only `time_format`, `tokio`, the `external_websocket_sync`
      `[features]` entry and the `external_websocket_sync_dep` path dependency.
      Restore `time` / any pruned dev-dep only if the compiler demands it —
      compile-driven, not guessed
- [ ] `crates/zed/Cargo.toml` — take upstream's version `1.19.0`, the new
      `inspector` feature block, and the reshuffled dependency list
      (`acp_thread` added, `action_log` optional, `agent_servers` optional, …);
      keep fork-only `tokio`, `ztracing`, `external_websocket_sync` optional dep
      and the Helix `[features]` entries
- [ ] `crates/zed/src/main.rs` — resolve to
      `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`,
      keeping the Helix `headless` argument **and** taking upstream's
      `with_restart_arguments`. Both `build_application(false)` call sites in the
      error path keep their argument. `args.allow_multiple_instances ||
      args.headless` in the single-instance check must survive upstream's
      rework of that block
- [ ] `Cargo.lock` — resolve `--theirs` (upstream), then regenerate by building

### `git_ui` → `git_ui_core` migration (the dominant work item)
Follow **002701 design.md §"Work item 1"** — the symbol map there is still
accurate; it was written against this same upstream split.
- [ ] Every fork-only `git_ui::` reference to a **moved** symbol re-pointed at
      `git_ui_core::`. Grep baseline — **13 files** reference `git_ui::` today:
      `agent_ui/src/{agent_panel.rs,test_support.rs,thread_worktree_archive.rs}`,
      `sidebar/src/sidebar.rs`,
      `zed/src/{main.rs,zed.rs,visual_test_runner.rs,zed/open_listener.rs}`,
      `title_bar/src/title_bar.rs`, `vim/src/test/vim_test_context.rs`,
      `collab/tests/integration/git_tests.rs`,
      `project_panel/src/project_panel{.rs,_tests.rs}`
- [ ] `git_ui::git_picker::popover(workspace, repo, GitPickerTab::Branches, rems(34.), window, cx)`
      call sites migrated to
      `git_ui_core::build_branch_picker(workspace, repo, window, cx)` — different
      arity **and** return type (already `Option`, so the `Some(..)` wrapper is
      dropped)
- [ ] Symbols that stayed in `git_ui` (`git_panel`, `branch_diff`, `commit_view`,
      `project_diff`) are **not** blindly renamed — compile-driven, symbol by symbol
- [ ] Any fork crate depending on `git_ui` purely for moved symbols gains
      `git_ui_core.workspace = true`; `git_ui` removed from a `Cargo.toml` only
      when nothing in that crate references it any more

### rustc `1.95.0 → 1.97.1`
- [ ] `rust-toolchain.toml` takes upstream's `channel = "1.97.1"`
- [ ] `/home/retro/work/helix/Dockerfile.zed-build` resolves and installs 1.97.1
      from `rust-toolchain.toml` with no manual pin edit — **verify**, do not
      assume; the BuildKit `/root/.rustup` cache mount must fetch a new toolchain
- [ ] New-compiler diagnostics in **fork-only** code fixed, following upstream's
      own remedy where one exists (e.g. `rems_from_px(12.)` → `rems_from_px(12_f32)`)
- [ ] Cold-cache full rebuild budgeted for — the toolchain change invalidates the
      entire Cargo cache

### New / removed upstream crates (larger churn than 002930 measured)
- [ ] Added present as workspace members: `crates/git_ui_core`,
      `crates/gpui_apple`, `crates/tabular_data_preview`,
      **`crates/call_hierarchy`**, **`crates/language_detection`**
- [ ] Removed and gone: `crates/csv_preview`, **`crates/rich_text`**,
      **`crates/supermaven`**, **`crates/supermaven_api`**
- [ ] Verified during planning: **no fork crate depends on `rich_text`,
      `supermaven` or `supermaven_api`** (only their own manifests reference
      them), so the removals are clean. Re-confirm after the merge with a
      workspace-wide grep before declaring done
- [ ] Workspace `Cargo.toml` `[workspace] members` and `[workspace.dependencies]`
      keep **all** Helix members (`external_websocket_sync`, `sidebar`, …) while
      absorbing upstream's additions and removals

### Auto-merge audit (auto-merged ≠ correct)
Priority order reflects the **re-measured** upstream deltas, which are far larger
than 002930 assumed. The four "confirming grep only" files from 002930 are now
real audit targets.
- [ ] **P1 `crates/zed/src/zed.rs` (+816, 32 hunks)** — much larger than the
      +382 002930 measured. Verify `initialize_agent_panel` and the WebSocket
      init inside it survive; check new action registrations and
      `initialize_panels`
- [ ] **P2 `crates/agent_ui/src/agent_panel.rs` (+549, 24 hunks)** — was +118.
      Verify `send_agent_ready`, `wait_for_websocket_connected`, the
      UI-state-query callback, `acp_history_store()`, `from_existing_thread`,
      the `ThreadDisplayNotification` handler and Fix #11's entity-identity guard
- [ ] **P3 `crates/agent/src/agent.rs` (+363)** — was +43. Fix #1's host. Verify
      `NativeAgent` clone / `pending_sessions` shared-task in `load_session()`
      and `wait_for_tools_ready` intact
- [ ] **P4 `crates/agent_ui/src/conversation_view.rs` (±125)** — was ±2, so
      002930's "confirming grep only" classification is stale. Fix #2's host:
      verify no duplicate WebSocket sends
- [ ] **P5 `crates/title_bar/src/title_bar.rs` (+108)** — was ±16. Constraint
      #12's host. Verify `render_restricted_mode()` returns `None` under the
      feature gate and the `&& !cfg!(feature = "external_websocket_sync")`
      sign-in suppression survives
- [ ] **P6 `crates/extensions_ui/src/extensions_ui.rs` (−662)** — bulk moved to
      `components/extension_card.rs`. The 3× `// HELIX: External agent` markers
      must still be present **and in a live code path**; follow the code and
      re-apply in the new file if upstream moved their host, recording the move
      in the porting guide
- [ ] **P7 `crates/reqwest_client/src/reqwest_client.rs` (±73)** — was
      unchanged. 3 non-test hunks plus a new `mod tests`. Verify
      `ZED_HTTP_INSECURE_TLS` insecure-TLS support intact
- [ ] **P8 `crates/http_client_tls/src/http_client_tls.rs` (±11)** — was
      unchanged. Paired with the new Cargo.toml conflict; verify the fork's
      `rustls-pki-types` usage still compiles alongside upstream's
      `webpki-roots` addition
- [ ] **P9 `crates/agent_ui/src/conversation_view/thread_view.rs` (±88)** —
      `restrict_scroll_to_axis()`, `pause_following_tail()` on compaction,
      `rems_from_px(_f32)`. Verify the Helix `current_model_id()` fallback
- [ ] **P10 `crates/anthropic/src/anthropic.rs` (+502)** — new models. Take
      upstream ordering wholesale (brief constraint #2)
- [ ] **P11 `crates/agent_servers/src/acp.rs` (±33)** — verify
      `SessionCreationGuard` / `session_creation_chain` (PR #50) intact
- [ ] **P12 `crates/agent/src/tools/grep_tool.rs` (±12)** — was unchanged.
      Verify `truncate_long_lines()` / `MAX_LINE_CHARS = 500` intact
- [ ] **P13 `crates/zed/src/main.rs`** — `--headless`,
      `--allow-multiple-instances`, `initialize_headless()`,
      `build_application(headless)` all intact around upstream's new
      `restart_arguments` flow
- [ ] **P14 `crates/language_models/src/provider/open_ai.rs` (±18)** — was
      unchanged; light confirming read
- [ ] Confirming grep only (**verified byte-unchanged upstream** — do not spend
      audit budget): `crates/acp_thread/src/connection.rs`,
      `crates/agent_ui/src/acp/**`, `crates/project/src/trusted_worktrees.rs`,
      `crates/external_websocket_sync/**` (fork-only)

### ACP — no bump this window (third window running)
- [ ] Confirm `Cargo.toml` still pins
      `agent-client-protocol = { version = "=2.0.0", features = ["unstable"] }`
      and `Cargo.lock` resolves `agent-client-protocol 2.0.0`,
      `agent-client-protocol-derive 2.0.0`,
      `agent-client-protocol-schema 1.5.0` — **both sides are already identical,
      so any change here is a mistake**
- [ ] No builder-pattern sweep is expected. If one becomes necessary it means the
      pin moved — stop and record why in the porting guide
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/`
      returns 0 (including `#[cfg(test)]` code)

### Elicitation types — no conflict expected
- [ ] `AgentThreadEntry::Elicitation(ElicitationEntryId)`, `struct Elicitation`,
      `ElicitationStatus`, `ElicitationStoreEvent` present and unchanged in
      `crates/acp_thread/src/acp_thread.rs` after the merge (they are upstream's
      own types; verified identical on both sides during planning)
- [ ] `crates/acp_thread/src/acp_thread.rs` auto-merge (±16) reviewed to confirm
      the elicitation block is untouched

### Critical Fix Preservation (`portingguide.md` §"Critical Fixes")
All 11 must survive. Unlike 002930's window, **three fix-host files changed
substantially** (`agent.rs` +363, `conversation_view.rs` ±125,
`agent_panel.rs` +549), so this is a real audit, not a grep pass.
- [ ] Fix #1: `NativeAgent` clone / `pending_sessions` shared-task in
      `load_session()` — **audit properly, `agent.rs` +363**
- [ ] Fix #2: no duplicate WebSocket sends from `conversation_view.rs` —
      **audit properly, ±125**
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (`stopped_emitted_for_task`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal-completion path
- [ ] Fix #11: entity-identity guard in `agent_panel.rs` `load_agent_thread` —
      **audit properly, +549**

### Helix-Specific Surface (brief constraints 1–13, re-verified)
- [ ] `crates/external_websocket_sync/` intact — 10 source files:
      `external_websocket_sync.rs`, `mcp.rs`, `mock_helix_server.rs`,
      `protocol_test.rs`, `server.rs`, `sync.rs`, `sync_settings/`,
      `thread_service.rs`, `types.rs`, `websocket_sync.rs`
- [ ] All custom code stays wrapped in `cfg(feature = "external_websocket_sync")`;
      compilation passes **with and without** the feature (brief #4)
- [ ] `ThreadDisplayNotification` handler still calls
      `OnboardingUpsell::set_dismissed(true, cx)` **and** initialises
      `NativeAgentSessionList` (brief #9, #10)
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` (brief #7). **Verified
      during planning: upstream's `flags.rs` (−27) pruning removed
      `ProjectPanelUndoRedoFeatureFlag` / `AutoWatchFeatureFlag` but
      `AcpBetaFeatureFlag` survives on both sides** — confirm the fork's
      `enabled_for_all()` override is still attached after the auto-merge
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) stays in
      `cfg(not(feature = "external_websocket_sync"))` (brief #11)
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved so
      `message_added` streams incrementally (brief #6, #13). `App::subscribe`
      (no window) is the correct pattern for WebSocket forwarding; `subscribe_in`
      is silently dropped without a window context
- [ ] **Constraints #8 and #12 are SUPERSEDED — verify the cfg gates, not the
      JSON.** See "Constraint Audit" below
- [ ] `title_bar`'s `external_websocket_sync` dep stays `optional = true`;
      workspace `rust-embed` keeps `debug-embed`; `wait_for_tools_ready` uses
      `cx.background_executor().timer()` (no `smol::Timer`)
- [ ] 3× `// HELIX: External agent` markers in the extensions UI
- [ ] `grep_tool.rs` `truncate_long_lines()` / `MAX_LINE_CHARS = 500` intact
- [ ] `reqwest_client.rs` / `http_client_tls.rs` insecure-TLS support intact
      (`ZED_HTTP_INSECURE_TLS`)
- [ ] `dev_container_suggest.rs` early return; migration banner `Hidden`;
      trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches stay exhaustive
- [ ] Fix 1b cfg-gated draft-suppression `return;` is still the FIRST statement of
      its `BaseView::Uninitialized` branch

#### Constraint Audit: #8 and #12 are superseded (carried from 002701/002930)

The brief requires `trust_all_worktrees: true` and `show_sign_in: false` in
`assets/settings/default.json`. They are **not** there and should **not** be
restored. Both patches were re-implemented as feature-gated code, which removes
merge surface from a hot 2500-line JSON file:

- **Trust** → `crates/project/src/trusted_worktrees.rs`:
  `if cfg!(feature = "external_websocket_sync") { … }` auto-trusts every
  worktree; backed by `title_bar.rs` `render_restricted_mode()` returning `None`
  under the same gate. **`trusted_worktrees.rs` is byte-unchanged upstream this
  window** — confirming grep suffices.
- **Sign-in** → `crates/title_bar/src/title_bar.rs`:
  `&& !cfg!(feature = "external_websocket_sync")` suppresses the Sign In button.
  **`title_bar.rs` is +108 upstream this window** — audit this one properly.

Audit the two code gates instead of the JSON, and record the supersession in the
porting guide so future briefs stop carrying stale constraints.

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] Compiles **both** with and without `external_websocket_sync` (brief #4)
- [ ] `cargo test -p external_websocket_sync` — full pass (all unit/mock tests)
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6 invariant)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] **E2E Docker test — HARD GATE.** Run via
      `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` (preferred
      wrapper over the brief's raw `docker build` / `docker run`, same image).
      Run `go mod tidy` in `e2e-test/helix-ws-test-server/` first. Never use
      `--no-build` when investigating failures
- [ ] All **17** phases green for `zed-agent`, and for `claude` via
      `E2E_AGENTS="zed-agent,claude"`
- [ ] One retry permitted per agent for the known Claude Phase-1 npm-install /
      API-latency flake; a second failure is a real failure

#### E2E phases on `main` — **17** (Phase 18 is not on `main`)

| # | Phase |
|---|---|
| 1 | Basic thread creation — `agent_ready` → `chat_message` → `thread_created` → `message_completed` |
| 2 | Follow-up on existing thread — `entry_count` increases |
| 3 | New second thread (context exhaustion) and switch |
| 4 | Follow-up to non-visible Thread A while Thread B active (entity-released regression) |
| 5 | Simulate user input (Zed → Helix direction) |
| 6 | Query UI state — correct `thread_id`, `entry_count`, `active_view` |
| 7 | `open_thread` command then `chat_message` |
| 8 | Mid-stream interrupt (prompt-queue busy-defer) |
| 9 | Rapid 3-turn cancel (bounded prompt — PR #77) |
| 10 | User-created thread injection + work session |
| 11 | Spectask routing (`FindConnectedSessionForSpecTask` picks most recent) |
| 12 | Reconnect (kill Zed, reconnect, deliver to existing thread) |
| 13 | Helix-initiated cancel → `turn_cancelled` status `cancelled` |
| 14 | Cancel no-op (bogus `request_id`) → status `noop` |
| 15 | Streaming patches arrive incrementally (`message_added` cadence) |
| 16 | Speculative-draft `user_created_thread` regression |
| 17 | Interrupt: `interrupt=true` enqueued while busy cancels and delivers |

**Phase 18 (elicitation) exists only on `origin/feature/002731-agent-questions`.**
It becomes a gate only if Open Question 1 is answered "merge that branch first".

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] New `## Merge 003012 (2026-08-31)` section inserted **above**
      `## Merge 2026-07-29 (upstream catch-up, 764 commits)` (line ~743)
- [ ] Window summary: measured commit count, date window, fence SHA, upstream
      HEAD SHA, ACP unchanged, rustc `1.95.0 → 1.97.1`, zed `1.15.0 → 1.19.0`
- [ ] `### Conflicts and Resolutions` — all six conflicted files, hunk by hunk,
      including the new `http_client_tls/Cargo.toml` union resolve
- [ ] `### git_ui → git_ui_core migration` — the full symbol map (moved vs
      stayed), every migrated call site, and the
      `popover` → `build_branch_picker` signature change. **The single most
      valuable thing to write down this window**
- [ ] `### rustc 1.95 → 1.97` — the bump, whether the Docker builder needed
      changes, and every fork-only diagnostic fixed
- [ ] `### Crate churn` — `git_ui_core` / `gpui_apple` / `tabular_data_preview` /
      `call_hierarchy` / `language_detection` added; `csv_preview` / `rich_text` /
      `supermaven` / `supermaven_api` removed
- [ ] `### Retired / superseded Helix patches` — including the constraint #8/#12
      supersession note (JSON defaults → cfg gates)
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Commit-history table extended; stale Rebase-Checklist entries corrected
- [ ] A note recording that **002701 and 002930 were both planned but never
      executed**, so the next engineer understands why this window is 434 commits
      and not ~100 — and so the pattern gets escalated rather than repeated

### Process
- [ ] Feature branch `feature/003012-merge-latest-zed` cut from fork `main`
- [ ] Branch pushed to `origin` (mirrors `helixml/zed`)
- [ ] `sandbox-versions.txt` `ZED_COMMIT` bumped from the stale
      `6f9300a70db9126b5f03deeb883c19adc21d545b` to the new merge HEAD, on a
      `feature/003012-merge-latest-zed` branch in `/home/retro/work/helix/`, pushed
- [ ] `pull_request_zed.md` + `pull_request_helix.md` written into this task dir
- [ ] `main` **not** force-pushed; `origin/helix-fork` left untouched; no
      agent-initiated PRs (the Helix UI opens them)
- [ ] Re-fetch `upstream/main` and `origin/main` before declaring done; absorb any
      out-of-band fork pushes; run an extension round if upstream advanced
      materially mid-work
- [ ] helixml/zed CI pipeline green (drone `build-zed` + `zed-e2e-test` steps in
      `/home/retro/work/helix/.drone.yml`)

## Out of Scope

- Net-new Helix feature development
- Merging `feature/002731-agent-questions` into `main` (unless Open Question 1
  says otherwise)
- Modifying E2E assertions unless an upstream API change strictly requires it
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix crates beyond what the merge / `git_ui_core` split forces
- Adopting upstream's new UX (native elicitation UI, message-queue steering,
  sandboxing) into Helix-mode flows beyond keeping them compiling
- Rewriting the porting guide from scratch — amend and extend in place
- Sandbox resource defaults (`spt_01m0evm3dpanc1sfktywbxhes4`) — re-confirmed:
  no Zed-side surface; nothing in the 434 upstream commits touches it
- Live verification of agent questions (`spt_01kzt7qza6mkrrfdch2yes0g6q`) —
  re-confirmed: nothing from that work has landed on fork `main`; the only fork
  commit since 002930 is PR #92 (deterministic headless smoke)

## Open Questions

1. **Phase 18 / elicitations are STILL not on `main` — how should this be
   sequenced? (second ask)** The brief again treats Phase 18 as a mandatory gate,
   but the elicitation sync events and Phase 18 live only on
   `origin/feature/002731-agent-questions` (7 ahead / 16 behind `main`, tip
   2026-08-12, no movement since 002930). Three options:
   (a) **merge upstream into `main` first, gate on 17 phases, and let the feature
   branch rebase afterwards** — least risk, and safe because
   `crates/external_websocket_sync/**` is fork-only and cannot conflict with
   upstream, so the rebase should be near-trivial;
   (b) land `feature/002731-agent-questions` into `main` first, then merge
   upstream and gate on 18 phases;
   (c) merge upstream into the feature branch directly.
   **Assumption: (a).** Please confirm — this is the single most consequential
   decision in the spec, and it was left unanswered last window.

2. **Why have three consecutive merge specs gone unexecuted? (escalation, not a
   technical question)** 002701 (2026-08-10) and 002930 (2026-08-24) both planned
   this exact merge and neither ran. The backlog has grown 128 → 331 → 434. If
   there is a blocker upstream of planning (capacity, credentials, build-image
   breakage), naming it now is more valuable than a fourth spec.
   **Assumption: no known blocker; this window will be executed.**

3. **Porting-guide path (fifth ask — 002265, 002353, 002701, 002930).** The brief
   names `design/2026-02-07-zed-fork-rebase-to-upstream.md`, which exists only in
   the **helix** repo as a historical narrative with no `## Merge NNN` sections.
   Every merge since has updated `/home/retro/work/zed/portingguide.md`.
   **Assumption: `zed/portingguide.md` is the target.** Confirm, or say if both
   should be touched — and ideally correct the path in future briefs.

4. **Repo/branch layout (fourth ask).** The brief says
   `/prod/home/luke/pm/zed-upstream` on branch `helix-fork` with remotes
   `helix`/`origin`. Sandbox reality is `/home/retro/work/zed` on `main`;
   `origin/helix-fork` is dead at 2026-02-07. **Assumption: work on `main` in the
   sandbox layout.** Confirm.

5. **`--locked` in CI (brief constraint #5) — stale, fourth ask.** There is no
   `.drone.yml` in the zed repo. Helix's `.drone.yml` and `Dockerfile.zed-build`
   run `cargo build --features external_websocket_sync` with **no** `--locked`.
   **Assumption: the constraint is obsolete; verify CI green and do not add
   `--locked` without instruction.** A yes/no would let us delete it permanently.

6. **rustc 1.97.1 availability in the build image (carried from 002701/002930).**
   `Dockerfile.zed-build` installs rustup with `--default-toolchain none` and lets
   `rust-toolchain.toml` drive the version, so 1.97.1 *should* resolve
   automatically — but the BuildKit `/root/.rustup` cache mount must fetch a fresh
   toolchain and the base image's glibc/linker must support it. **Assumption: no
   Dockerfile change needed.** If the download fails or the base image is too old
   this becomes a Helix-repo change; flag immediately rather than pinning back
   to 1.95.

7. **`git_ui_core` alongside `git_ui`.** Some fork crates may need both.
   **Assumption: add `git_ui_core` alongside `git_ui` where both are needed;
   remove `git_ui` only when nothing in that crate references it.** For
   `title_bar` and `agent_ui`, upstream removed `git_ui` outright — take that.

8. **`crates/agent_ui/Cargo.toml` — is fork code still using `time` and the five
   pruned dev-deps?** Upstream deleted `time.workspace = true` and dev-deps
   `clock`, `node_runtime`, `remote_server`, `tree-sitter-md`, `unindent`. The
   fork's conflict side keeps `time`, `time_format` and `tokio`. **Assumption:
   keep `time_format` and `tokio` unconditionally, and re-add `time` / any
   dev-dep only if the compiler demands it.**

9. **E2E credentials — now measured as absent.** `ANTHROPIC_API_KEY` is **not
   set** in the planning sandbox environment. The E2E hard gate cannot run
   without it. **Assumption: the implementation agent's environment provides
   it.** If not, flag immediately — the merge cannot be declared complete.

10. **Feature-off compilation gate (carried from 002701/002930).** With no local
    cargo, the cleanest way to `cargo check -p zed` *without*
    `external_websocket_sync` is a one-off Docker run in the build image. Is there
    a `./stack` target for a feature-off build? **Assumption: none; use a one-off
    Docker invocation.**

11. **Upstream removed `rich_text`, `supermaven` and `supermaven_api`.** Planning
    verified no fork crate manifest depends on them, so the removal looks clean.
    **Assumption: take upstream's removal wholesale.** If any Helix-only source
    file `use`s these crates without a manifest entry (a workspace-inherited
    dependency would show up), the compiler will catch it — but flag it in the
    porting guide rather than re-adding the crates.
