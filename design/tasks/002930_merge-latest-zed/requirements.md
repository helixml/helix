# Requirements: Merge Latest Zed Upstream Into Helix Fork

## Context

Today is **2026-08-24**. This is the next cycle in the recurring
`zed-industries/zed` → Helix fork upstream-merge series.

**Read `helix-specs/design/tasks/002701_merge-latest-zed/` (all three files)
before starting.** This spec records only the delta and the decisions specific
to this window.

### Critical finding from the pre-work review

The brief asks for three prior specs to be reviewed
(`spt_01m06y1ycx8z4agagq347chg8d` 2026-08-17,
`spt_01kzmxaa523mc3dr3a2keknvax` 2026-08-10,
`spt_01kxyty7684hnq9xky7j92pbvk`). Measured reality in `helix-specs`:

- The **newest merge spec in the repo is `002701_merge-latest-zed` (2026-08-10)**.
  There is **no 2026-08-17 merge spec** in `helix-specs/design/tasks/` — the
  newest specs of any kind after 2026-08-10 are `002871` (2026-08-17,
  unrelated), `002901` (2026-08-19) and `002903` (2026-08-20). If the
  2026-08-17 spec exists it was never pushed as design docs. See Open Questions.
- **002701 was never executed.** Proof: fork `main` still pins
  `rust-toolchain.toml` `channel = "1.95.0"`, still has 13 files referencing
  `git_ui::`, and the newest upstream merge in the fork's first-parent history
  is `PR #73 feature/merge-upstream-zed-2026-07-29`.

**Consequence: this window is 002701's window plus two more weeks.** Every work
item in 002701 (the `git_ui` → `git_ui_core` split, the rustc bump, the
`title_bar/Cargo.toml` conflict) is still outstanding, and the commit count has
grown from 128 to 331. 002701's design.md remains directly applicable and
should be treated as the primary working document alongside this one.

### Measured baseline (2026-08-24, real numbers from `/home/retro/work/zed`)

```
fence (merge-base)   b9256fa8f018bf03eb2e420120163746f8298d83  2026-07-29
                     "Stop the npm cache from growing without bound (#61750)"
upstream HEAD        7f2a2c3c3ee2f23f28772dee7661fb98d3910990  2026-08-24
                     "gpui: Support Windows Restart Manager shutdown (#62987)"
fork HEAD (main)     7357327f2df809279daeceec10e453edf77710bb  2026-08-20
                     "Merge pull request #91 from helixml/fix/release-refused-turn-lane"
commits to merge     331     (git log --oneline origin/main..upstream/main | wc -l)
fork-only commits    349
window               26 days
ACP                  2.0.0 → 2.0.0    ** NO BUMP **  (derive 2.0.0, schema 1.5.0, both sides identical)
rust-toolchain       1.95.0 → 1.97.1  ** carried over from 002701, still the main risk **
zed crate version    1.15.0 → 1.18.0
textual conflicts    5 files (measured via `git merge-tree --write-tree`)
new upstream crates  git_ui_core, gpui_apple, tabular_data_preview
removed upstream     csv_preview
```

Re-measure before starting — upstream moves daily — but the shape will hold.

### The five textual conflicts (measured, not predicted)

| File | Nature |
|---|---|
| `Cargo.lock` | Routine. Take upstream, regenerate via the build. |
| `crates/title_bar/Cargo.toml` | Fork `external_websocket_sync` optional dep + `git_ui` vs upstream `git_ui_core`. |
| `crates/agent_ui/Cargo.toml` | Fork `time`/`time_format`/`tokio` block vs upstream removing `time` and `git_ui`, adding `git_ui_core`. |
| `crates/zed/Cargo.toml` | Fork `tokio`/`ztracing`/`tracing` block vs upstream dependency reshuffle + new `inspector` feature + version bump. |
| `crates/zed/src/main.rs` | Helix `build_application(args.headless)` vs upstream `build_application().with_restart_arguments(restart_arguments)`. |

All five are small. The real work is the same three things as 002701:
the `git_ui_core` migration, the rustc bump, and the auto-merged-file audit.

### Answers to the brief's open questions (measured during planning)

| Brief question | Measured answer |
|---|---|
| How many new upstream commits? | **331** |
| Has the ACP crate version bumped? | **No.** `agent-client-protocol =2.0.0`, derive 2.0.0, schema 1.5.0 — identical on both sides. No builder-pattern sweep needed this window. |
| Have high-conflict files been restructured upstream? | `extensions_ui.rs` −662 (bulk moved to `components/extension_card.rs`), `zed.rs` +382 (**~90% inside `mod tests`**), `anthropic.rs` +330 (new models), `agent_panel.rs` +118 (**only 2 non-test hunks**), `thread_view.rs` ±68, `agent/src/agent.rs` +43 (one hunk in `NativeAgentConnection` — Fix #1's host), `acp.rs` ±33, `title_bar.rs` ±16. `crates/acp_thread/src/connection.rs` and `crates/external_websocket_sync/**` are **unchanged upstream**. |
| Does the `Elicitation` variant in `AgentThreadEntry` conflict with upstream? | **No — the variant is upstream's own.** `AgentThreadEntry::Elicitation(ElicitationEntryId)` already existed at the fence (2026-07-29) and is byte-identical on fork `main`, `upstream/main`, and the feature branch. No enum conflict. |
| Is upstream's session-list/resume UI now a replacement for `from_existing_thread`? | No. `acp_thread/src/connection.rs` is unchanged upstream this window; nothing forces the question. Keep `from_existing_thread` as-is. |
| New lessons in helix-specs since 2026-08-17? | None — no merge spec was written after 002701. |
| Were 002701's open questions answered in the interim? | **No.** All nine remain open and are re-raised below. |

### ACP Elicitations / Phase 18 — important correction to the brief

The brief states elicitation sync events and e2e Phase 18 "have been built and
merged onto helix-fork". **Measured: they have not been merged.**

- `crates/external_websocket_sync/src/**` on fork `main` contains **zero**
  occurrences of `elicitation`.
- `e2e-test/helix-ws-test-server/main.go` on fork `main` enumerates
  **17 phases**. There is no Phase 18.
- The work lives on `origin/feature/002731-agent-questions`, which is
  **7 commits ahead of and 14 commits behind `origin/main`** (tip
  `859325b38f test(e2e): widen the per-round timeout for the added phase`).
  That branch adds `elicitation` references to
  `external_websocket_sync.rs` (16), `thread_service.rs` (72), `types.rs` (32),
  `websocket_sync.rs` (9), and Phase 18 to the e2e server.

**Therefore Phase 18 cannot be a gate for this merge as things stand.** The
merge gate is **all 17 phases on `main`**. See Open Question 1 for the decision
the user needs to make.

## Repository Layout (sandbox reality — measured)

- **Working repo**: `/home/retro/work/zed/`, branch **`main`** (not `helix-fork`).
  - `origin` = `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
    (in-cluster gitea mirror of `helixml/zed`). `origin/main` == local `main`.
  - `origin/helix-fork` **exists but is dead** — its tip is
    `746a9c4fb4` dated **2026-02-07**. Do not use it. The live branch is `main`.
  - `upstream` = `https://github.com/zed-industries/zed.git` — **not
    configured**; add it. Confirmed reachable from the sandbox during planning.
- **Living porting guide**: `/home/retro/work/zed/portingguide.md` (1325 lines).
  Newest merge section is `## Merge 2026-07-29 (upstream catch-up, 764 commits)`
  at **line 744**. The brief's path
  `design/2026-02-07-zed-fork-rebase-to-upstream.md` **does not exist in the zed
  repo on any live branch** — it is the historical one-time-port narrative in
  the **helix** repo. See Open Question 3.
- **Helix platform repo**: `/home/retro/work/helix/` —
  `sandbox-versions.txt` line 1 is
  `ZED_COMMIT=71a2940881e37fff3ca099cb49ae15ce4b996f9a`, which is **already
  stale** (it is the second parent of fork HEAD `7357327f2d`, not HEAD itself).
- **Toolchain**: **no local `cargo`/`rustc`.** Only `docker` and `go`. All Rust
  building/testing goes through
  `cd /home/retro/work/helix && ./stack build-zed dev` or the E2E Docker images.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the 331 upstream commits and adopt
> upstream's `git_ui_core` crate split and the rustc 1.97.1 bump, so the fork
> stops accumulating the merge debt that 002701 was supposed to clear and never
> returns to a 700-commit backlog.

### 2. Helix User
> As a Helix user, I want upstream's newest fixes and performance work without
> losing WebSocket sync, incremental streaming, headless mode, Codex/Claude ACP
> routing, or any of the 11 Critical Fixes.

### 3. Agent-Questions Feature Owner
> As the owner of `feature/002731-agent-questions`, I want the merge to land in
> a way that leaves my 7 unmerged commits rebasable without a second painful
> conflict resolution — the `external_websocket_sync` crate is untouched
> upstream, so this should be achievable.

### 4. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` updated **as
> each conflict is resolved** with a dated `## Merge 002930 (2026-08-24)` entry
> documenting the `git_ui_core` migration and the rustc bump — those are
> one-time structural changes whose resolution pattern will not be obvious from
> the diff alone.

## Acceptance Criteria

### Merge Completeness
- [ ] `upstream` remote added (`https://github.com/zed-industries/zed.git`),
      fetched, treated as read-only
- [ ] Re-measure fence / commit count / upstream HEAD before starting; record
      the measured values in `portingguide.md`
- [ ] `git merge upstream/main` — **merge, not squash, not rebase**
- [ ] Merge contains all upstream commits through the recorded upstream HEAD;
      any skipped commit explicitly justified in `portingguide.md`
- [ ] All 349 fork-only commits preserved verbatim (PRs #73–#91)
- [ ] `git log` confirms the fork branch is ahead of and contains `upstream/main`
- [ ] No `<<<<<<<` / `>>>>>>>` markers anywhere in the tree

### The five textual conflicts
- [ ] `crates/title_bar/Cargo.toml` — keep
      `external_websocket_sync = { workspace = true, optional = true }` and the
      `[features]` `external_websocket_sync = ["dep:external_websocket_sync"]`
      entry; take upstream's `git_ui_core.workspace = true`; **drop**
      `git_ui.workspace = true` (upstream removed it outright)
- [ ] `crates/agent_ui/Cargo.toml` — take upstream's shape (removes `git_ui`,
      removes `time`, reorders `fuzzy`/`git`, adds `git_ui_core`, drops dev-dep
      `clock`) and keep fork-only `time_format` and `tokio`. Restore `time` only
      if fork code still needs it — compile-driven, not guessed
- [ ] `crates/zed/Cargo.toml` — take upstream's version `1.18.0`, the new
      `inspector` feature block, and the reshuffled dependency list
      (`acp_thread`, `action_log` optional, `agent_servers` optional, …); keep
      fork-only `tokio` and `ztracing`, and keep the Helix `[features]` entries
- [ ] `crates/zed/src/main.rs` — resolve to
      `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`,
      i.e. keep the Helix `headless` argument **and** take upstream's
      `with_restart_arguments`
- [ ] `Cargo.lock` — resolve `--theirs` (upstream), then regenerate by building

### `git_ui` → `git_ui_core` migration (the dominant work item)
Follow **002701 design.md §"Work item 1"** — the symbol map there is still
accurate; it was written against the same upstream split.
- [ ] Every fork-only `git_ui::` reference to a **moved** symbol re-pointed at
      `git_ui_core::`. Grep baseline — 13 files reference `git_ui::` today:
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
- [ ] Symbols that stayed in `git_ui` (`git_panel`, `branch_diff`,
      `commit_view`, `project_diff`) are **not** blindly renamed — the migration
      is compile-driven, symbol by symbol
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
- [ ] Cold-cache full rebuild budgeted for — the toolchain change invalidates
      the entire Cargo cache

### New / removed upstream crates
- [ ] `crates/git_ui_core`, `crates/gpui_apple`, `crates/tabular_data_preview`
      present as workspace members; `crates/csv_preview` removed
- [ ] Workspace `Cargo.toml` `[workspace] members` and `[workspace.dependencies]`
      keep **all** Helix members (`external_websocket_sync`, `sidebar`, …) while
      absorbing upstream's additions/removals

### Auto-merge audit (auto-merged ≠ correct)
Priority order for this window, based on measured upstream deltas:
- [ ] **P1 `crates/extensions_ui/src/extensions_ui.rs` (−662)** — bulk moved to
      `components/extension_card.rs`. The 3× `// HELIX: External agent` markers
      must still be present **and in a live code path**; follow the code and
      re-apply in the new file if upstream moved their host, recording the move
      in the porting guide
- [ ] **P2 `crates/agent/src/agent.rs` (+43)** — one new hunk inside
      `NativeAgentConnection`. This is Fix #1's host file (it was untouched in
      002701's window; it has changed since). Verify `pending_sessions` /
      `wait_for_tools_ready` intact
- [ ] **P3 `crates/zed/src/zed.rs` (+382)** — note ~90% of the delta is inside
      `mod tests`. Non-test hunks: new action registrations,
      `open_new_ssh_project_from_project` signature, `notify_settings_errors`,
      `initialize_panels`. Verify `initialize_agent_panel` and the WebSocket
      init inside it survive
- [ ] **P4 `crates/agent_ui/src/agent_panel.rs` (+118)** — only **two**
      non-test hunks (`SiblingThreadHost` at ~4792 and an `AgentPanel` block at
      ~6051, the `git_ui_core` + `IconButton` rework). Helix cfg blocks should
      pass cleanly; confirm rather than repair
- [ ] **P5 `crates/agent_ui/src/conversation_view/thread_view.rs` (±68)** —
      `restrict_scroll_to_axis()`, `pause_following_tail()` on compaction,
      `rems_from_px(_f32)`. Verify Helix `current_model_id()` fallback
- [ ] **P6 `crates/anthropic/src/anthropic.rs` (+330)** — new models. Take
      upstream ordering wholesale (brief constraint #2)
- [ ] **P7 `crates/agent_servers/src/acp.rs` (±33)** — verify
      `SessionCreationGuard` / `session_creation_chain` (PR #50) intact
- [ ] **P8 `crates/zed/src/main.rs`** — `--headless`,
      `--allow-multiple-instances`, `initialize_headless()`,
      `build_application(headless)` all intact around upstream's new
      `restart_arguments` flow
- [ ] Confirming grep only (unchanged upstream — do not spend audit budget):
      `crates/acp_thread/src/connection.rs`,
      `crates/external_websocket_sync/**`,
      `crates/agent_ui/src/conversation_view.rs` (±2),
      `crates/reqwest_client/src/reqwest_client.rs`,
      `crates/agent/src/tools/grep_tool.rs`,
      `crates/language_models/src/provider/open_ai.rs`

### ACP — no bump this window
- [ ] Confirm `Cargo.toml` still pins
      `agent-client-protocol = { version = "=2.0.0", features = ["unstable"] }`
      and `Cargo.lock` resolves `agent-client-protocol 2.0.0`,
      `agent-client-protocol-derive 2.0.0`,
      `agent-client-protocol-schema 1.5.0` — **both sides are already
      identical, so any change here is a mistake**
- [ ] No builder-pattern sweep is expected. If one becomes necessary, it means
      the pin moved — stop and record why
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/`
      returns 0 (including `#[cfg(test)]` code)

### Critical Fix Preservation (`portingguide.md` §"Critical Fixes")
All 11 must survive. Only Fix #1's host file changed upstream this window, so
this is largely a confirming grep pass:
- [ ] Fix #1: `NativeAgent` clone / `pending_sessions` shared-task in
      `load_session()` — **audit properly, `agent.rs` changed (+43)**
- [ ] Fix #2: no duplicate WebSocket sends from `conversation_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (`stopped_emitted_for_task`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal-completion path
- [ ] Fix #11: entity-identity guard in `agent_panel.rs` `load_agent_thread`

### Helix-Specific Surface (brief constraints 1–13, re-verified)
- [ ] `crates/external_websocket_sync/` intact — 10 source files:
      `external_websocket_sync.rs`, `mcp.rs`, `mock_helix_server.rs`,
      `protocol_test.rs`, `server.rs`, `sync.rs`, `sync_settings/`,
      `thread_service.rs`, `types.rs`, `websocket_sync.rs`
- [ ] All custom code stays wrapped in `cfg(feature = "external_websocket_sync")`;
      compilation passes **with and without** the feature (brief #4)
- [ ] `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query
      callback, `acp_history_store()` present in `agent_panel.rs`
- [ ] `ThreadDisplayNotification` handler still calls
      `OnboardingUpsell::set_dismissed(true, cx)` **and** initialises
      `NativeAgentSessionList` (brief #9, #10)
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` (brief #7). Upstream
      removed `ProjectPanelUndoRedoFeatureFlag` / `AutoWatchFeatureFlag` from
      `flags.rs` (−27) — confirm `AcpBetaFeatureFlag` survived that pruning
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) stays in
      `cfg(not(feature = "external_websocket_sync"))` (brief #11)
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved so
      `message_added` streams incrementally (brief #6, #13). `App::subscribe`
      (no window) is the correct pattern for WebSocket forwarding;
      `subscribe_in` is silently dropped without a window context
- [ ] **Constraints #8 and #12 are SUPERSEDED — verify the cfg gates, not the
      JSON.** See "Constraint Audit" below
- [ ] `title_bar`'s `external_websocket_sync` dep stays `optional = true`;
      `render_restricted_mode` and the sign-in cfg gate intact; workspace
      `rust-embed` keeps `debug-embed`; `wait_for_tools_ready` uses
      `cx.background_executor().timer()` (no `smol::Timer`)
- [ ] 3× `// HELIX: External agent` markers in the extensions UI
- [ ] `grep_tool.rs` `truncate_long_lines()` / `MAX_LINE_CHARS = 500` intact
- [ ] `reqwest_client.rs` / `http_client_tls.rs` insecure-TLS support intact
      (`ZED_HTTP_INSECURE_TLS`)
- [ ] `dev_container_suggest.rs` early return; migration banner `Hidden`;
      trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches stay exhaustive
- [ ] Fix 1b cfg-gated draft-suppression `return;` is still the FIRST statement
      of its `BaseView::Uninitialized` branch

#### Constraint Audit: #8 and #12 are superseded (carried forward from 002701)

The brief requires `trust_all_worktrees: true` and `show_sign_in: false` in
`assets/settings/default.json`. They are **not** there and should **not** be
restored. Both patches were re-implemented as feature-gated code, which removes
merge surface from a hot 2500-line JSON file:

- **Trust** → `crates/project/src/trusted_worktrees.rs`:
  `if cfg!(feature = "external_websocket_sync") { … }` auto-trusts every
  worktree; backed by `title_bar.rs` `render_restricted_mode()` returning `None`
  under the same gate.
- **Sign-in** → `crates/title_bar/src/title_bar.rs`:
  `&& !cfg!(feature = "external_websocket_sync")` suppresses the Sign In button.

Audit the two code gates instead, and record the supersession in the porting
guide so future briefs stop carrying stale constraints.

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] Compiles **both** with and without `external_websocket_sync` (brief #4)
- [ ] `cargo test -p external_websocket_sync` — full pass (all unit/mock tests)
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6 invariant)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] **E2E Docker test — HARD GATE.** Run via
      `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` (preferred
      wrapper over the brief's raw `docker build`/`docker run`, same image).
      `go mod tidy` in `e2e-test/helix-ws-test-server/` first. Never use
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
- [ ] New `## Merge 002930 (2026-08-24)` section inserted **above**
      `## Merge 2026-07-29 (upstream catch-up, 764 commits)` (line ~744)
- [ ] Window summary: measured commit count, date window, fence SHA, upstream
      HEAD SHA, ACP unchanged, rustc `1.95.0 → 1.97.1`, zed `1.15.0 → 1.18.0`
- [ ] `### Conflicts and Resolutions` — all five conflicted files, hunk by hunk
- [ ] `### git_ui → git_ui_core migration` — the full symbol map (moved vs
      stayed), every migrated call site, and the
      `popover` → `build_branch_picker` signature change. **The single most
      valuable thing to write down this window**
- [ ] `### rustc 1.95 → 1.97` — the bump, whether the Docker builder needed
      changes, and every fork-only diagnostic fixed
- [ ] `### Crate churn` — `git_ui_core` / `gpui_apple` / `tabular_data_preview`
      added, `csv_preview` removed
- [ ] `### Retired / superseded Helix patches` — including the constraint
      #8/#12 supersession note (JSON defaults → cfg gates)
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Commit-history table extended; stale Rebase-Checklist entries corrected
- [ ] A note recording that **002701 was planned but never executed**, so the
      next engineer understands why this window is 331 commits and not ~200

### Process
- [ ] Feature branch `feature/002930-merge-latest-zed` cut from fork `main`
- [ ] Branch pushed to `origin` (mirrors `helixml/zed`)
- [ ] `sandbox-versions.txt` `ZED_COMMIT` bumped from the stale
      `71a2940881e37fff3ca099cb49ae15ce4b996f9a` to the new merge HEAD, on a
      `feature/002930-merge-latest-zed` branch in `/home/retro/work/helix/`, pushed
- [ ] `pull_request_zed.md` + `pull_request_helix.md` written into this task dir
- [ ] `main` **not** force-pushed; `origin/helix-fork` left untouched; no
      agent-initiated PRs (the Helix UI opens them)
- [ ] Re-fetch `upstream/main` and `origin/main` before declaring done; absorb
      any out-of-band fork pushes; run an extension round if upstream advanced
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
- Sandbox resource defaults (`spt_01m0evm3dpanc1sfktywbxhes4`) — confirmed
  no Zed-side surface; nothing in the 331 upstream commits touches it

## Open Questions

1. **Phase 18 / elicitations are NOT on `main` — how should this be sequenced?**
   The brief treats Phase 18 as a mandatory gate, but the elicitation sync
   events and Phase 18 live only on `origin/feature/002731-agent-questions`
   (7 ahead / 14 behind `main`). Three options:
   (a) **merge upstream into `main` first, gate on 17 phases, and let the
   feature branch rebase afterwards** — least risk, and safe because
   `crates/external_websocket_sync/**` is *unchanged upstream* this window, so
   the rebase should be near-trivial;
   (b) land `feature/002731-agent-questions` into `main` first, then merge
   upstream and gate on 18 phases;
   (c) merge upstream into the feature branch directly.
   **Assumption: (a).** Please confirm — this is the single most consequential
   decision in the spec.

2. **Was there ever a 2026-08-17 merge spec (`spt_01m06y1ycx8z4agagq347chg8d`)?**
   It is not in `helix-specs`. Planning proceeded from 002701 (2026-08-10) plus
   fresh measurement. **Assumption: no additional lessons were recorded on
   2026-08-17.** If that spec has content not in the repo, please paste it.

3. **Porting-guide path (unanswered in 002265, 002353, 002701 — fourth ask).**
   The brief names `design/2026-02-07-zed-fork-rebase-to-upstream.md`, which
   exists only in the **helix** repo as a historical narrative with no
   `## Merge NNN` sections. Every merge since has updated
   `/home/retro/work/zed/portingguide.md`. **Assumption: `zed/portingguide.md`
   is the target.** Confirm, or say if both should be touched — and ideally
   correct the path in future briefs.

4. **Repo/branch layout (third ask).** The brief says
   `/prod/home/luke/pm/zed-upstream` on branch `helix-fork` with remotes
   `helix`/`origin`. Sandbox reality is `/home/retro/work/zed` on `main`;
   `origin/helix-fork` is dead at 2026-02-07. **Assumption: work on `main` in
   the sandbox layout.** Confirm.

5. **`--locked` in CI (brief constraint #5) — stale, third ask.** There is no
   `.drone.yml` in the zed repo. Helix's `.drone.yml` and
   `Dockerfile.zed-build` run
   `cargo build --features external_websocket_sync` with **no** `--locked`.
   **Assumption: the constraint is obsolete; verify CI green and do not add
   `--locked` without instruction.** A yes/no would let us delete it permanently.

6. **rustc 1.97.1 availability in the build image (carried from 002701).**
   `Dockerfile.zed-build` installs rustup with `--default-toolchain none` and
   lets `rust-toolchain.toml` drive the version, so 1.97.1 *should* resolve
   automatically — but the BuildKit `/root/.rustup` cache mount must fetch a
   fresh toolchain and the base image's glibc/linker must support it.
   **Assumption: no Dockerfile change needed.** If the download fails or the
   base image is too old this becomes a Helix-repo change; flag immediately
   rather than pinning back to 1.95.

7. **`git_ui_core` alongside `git_ui`.** Some fork crates may need both.
   **Assumption: add `git_ui_core` alongside `git_ui` where both are needed;
   remove `git_ui` only when nothing in that crate references it.** For
   `title_bar` and `agent_ui`, upstream removed `git_ui` outright — take that.

8. **`crates/agent_ui/Cargo.toml` — is fork code still using `time`?** Upstream
   deleted `time.workspace = true`; the fork's conflict side keeps `time`,
   `time_format` and `tokio`. **Assumption: keep `time_format` and `tokio`
   unconditionally, and re-add `time` only if the compiler demands it.**

9. **E2E credentials.** The gate needs a working `ANTHROPIC_API_KEY` in the
   environment. Assumed available to the implementation agent — flag
   immediately if not, because the merge cannot be declared complete without it.

10. **Feature-off compilation gate (carried from 002701).** With no local cargo,
    the cleanest way to `cargo check -p zed` *without* `external_websocket_sync`
    is a one-off Docker run in the build image. Is there a `./stack` target for
    a feature-off build? **Assumption: none; use a one-off Docker invocation.**
