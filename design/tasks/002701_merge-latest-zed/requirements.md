# Requirements: Merge Latest Zed Upstream Into Helix Fork

## Context

Today is **2026-08-10**. This is the next cycle in the recurring
`zed-industries/zed` → Helix fork upstream-merge series. The immediately prior
cycle is `helix-specs/design/tasks/002353_merge-latest-zed/` — **read all three
of its files before starting**; this spec only records the *delta* and the
decisions specific to this window.

Good news up front: **this is the smallest, cleanest window in the series.**
The prior merge (002353) landed on 2026-07-29 and closed the 709-commit debt.
Because it landed, this window is only 12 days wide.

### Measured baseline (2026-08-10, real numbers from the sandbox clone)

```
fence (merge-base)   b9256fa8f0  2026-07-29  "Stop the npm cache from growing without bound (#61750)"
upstream HEAD        1271f8b0e8  2026-08-10  "Bump rustc to 1.97 (#62395)"
fork HEAD (main)     77d466a455              "Merge pull request #77 from helixml/fix/e2e-phase9-bounded-prompt"
commits to merge     128         (git log --oneline origin/main..upstream/main | wc -l)
fork-only commits    323 (232 non-merge)
window               12 days
diffstat             414 files, +23638 / -9385
ACP                  2.0.0 → 2.0.0            ** NO BUMP ** (schema 1.5.0, derive 2.0.0 — all unchanged)
rust-toolchain       1.95.0 → 1.97.1          ** THE NEW RISK **
textual conflicts    1 file, 1 hunk           (crates/title_bar/Cargo.toml)
```

Re-measure before starting — upstream moves daily — but the shape will hold.

### What actually makes this window risky

Textual conflicts are near-zero. The work is three things:

1. **`git_ui` → `git_ui_core` crate split (upstream).** A new workspace crate
   `crates/git_ui_core/` was extracted from `git_ui`, taking
   `worktree_service.rs`, `worktree_picker.rs`, `worktree_names.rs`,
   `askpass_modal.rs`, `created_worktrees.rs`, `file_diff_view.rs`,
   `notifications.rs`. Upstream also replaced
   `git_ui::git_picker::popover(..)` with `git_ui_core::build_branch_picker(..)`
   (different arity — no `GitPickerTab`, no width arg). **13 fork files still
   say `git_ui::`.** This is the sole textual conflict *and* the dominant
   compile-error source.

2. **rustc `1.95.0 → 1.97.1`.** Two minor compiler versions in one hop. New
   lints/inference. Upstream pre-emptively changed `rems_from_px(12.)` →
   `rems_from_px(12_f32)` in ~8 places, which strongly implies an inference
   ambiguity the new compiler surfaces. Fork-only code with unsuffixed float
   literals into the same APIs will need the same treatment. The Docker builder
   picks the toolchain up from `rust-toolchain.toml`, so the bump lands
   automatically — but the first build will be a cold-cache full rebuild.

3. **The usual "auto-merged ≠ correct" audit** — smaller than usual, because
   the four highest-risk Helix files have **zero upstream changes** this window
   (see below).

### Files that did NOT change upstream this window (measured — big relief)

`git diff b9256fa8f0..upstream/main --stat` is **empty** for all of:
- `crates/agent_ui/src/conversation_view.rs`  (`from_existing_thread`,
  THREAD_REGISTRY, thread-load lock, `send_agent_ready`)
- `crates/agent_servers/src/acp.rs`  (`SessionCreationGuard`,
  `session_creation_chain`, PR #50)
- `crates/agent/src/agent.rs`  (Fix #1 `pending_sessions`, `wait_for_tools_ready`)
- `crates/acp_thread/src/connection.rs`  (the `AgentConnection` trait)
- `crates/external_websocket_sync/**`  (entirely fork-owned)
- `crates/anthropic/src/anthropic.rs`
- `crates/reqwest_client/src/reqwest_client.rs`
- `crates/agent_ui/src/config_options.rs`
- `crates/language_models/src/provider/open_ai.rs`
- `crates/agent/src/tools/grep_tool.rs`

Do not spend audit budget on these beyond a confirming grep.

### Files that DID change upstream, with measured deltas

| File | Δ | Upstream change | Helix exposure |
|---|---|---|---|
| `crates/extensions_ui/src/extensions_ui.rs` | −662 | Large refactor: bulk moved into `components/extension_card.rs` (+744) | **3× `// HELIX: External agent` markers live here.** Highest audit priority |
| `crates/zed/src/zed.rs` | +141 | New action registrations, `open_new_ssh_project_from_project` args, big new test block | `initialize_agent_panel` + WebSocket init |
| `crates/agent_ui/src/agent_panel.rs` | ±35 | 9× `git_ui::` → `git_ui_core::`; full-screen `IconButton` id/`toggle_state` rework | Heavy Helix cfg blocks throughout |
| `crates/agent_ui/src/conversation_view/thread_view.rs` | ±68 | `restrict_scroll_to_axis()` builder; `pause_following_tail()` on compaction; `rems_from_px(_f32)` | Helix `current_model_id()` 3-way fallback |
| `crates/zed/src/main.rs` | ±27 | `restore_task` → `.shared()`, `first_window_rx` oneshot, `credentials_provider` arg (#61346) | `--headless`, `--allow-multiple-instances`, `initialize_headless()`, `build_application(headless)` |
| `assets/settings/default.json` | ±17 | New keys: `git_gutter_width`, `git.diff_base`, `terminal.starts_open`; decoration comments reworded | See Constraint Audit below |
| `crates/feature_flags/src/flags.rs` | −27 | Removed `ProjectPanelUndoRedoFeatureFlag`, `AutoWatchFeatureFlag` | **`AcpBetaFeatureFlag` untouched — survives** |
| `crates/title_bar/src/title_bar.rs` | ±8 | `git_ui_core::worktree_picker::WorktreePicker`; `build_branch_picker` | `render_restricted_mode`, sign-in gate |
| `crates/acp_thread/src/acp_thread.rs` | ±5 | `PAGER=""` → `disable_pagers_through_env(&mut env)` | Trivial; Critical Fixes untouched |
| `Cargo.toml` (workspace) | +4/−1 | `crates/git_ui_core` member + dep; `ctor 1.0.6→1.0.12`; `windows-registry 0.6.0` | Keep Helix members + `rust-embed` `debug-embed` |

## Repository Layout (sandbox reality — measured, unchanged from 002353)

- **Working repo**: `/home/retro/work/zed/`, branch **`main`** (not `helix-fork`).
  - `origin` = `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
    (in-cluster gitea mirror of `helixml/zed`). `origin/main` == local `main`
    == `77d466a455`.
  - `upstream` = `https://github.com/zed-industries/zed.git` — **was not
    configured**; added during planning. Verify present and read-only.
- **Living porting guide**: `/home/retro/work/zed/portingguide.md` (~1230 lines,
  newest section `## Merge 2026-07-29 (upstream catch-up, 764 commits)` at
  line 712). This is the file to update continuously. The brief's path
  `design/2026-02-07-zed-fork-rebase-to-upstream.md` lives in the **helix** repo
  and is the historical one-time-port narrative — see Open Questions.
- **Helix platform repo**: `/home/retro/work/helix/` —
  `sandbox-versions.txt` currently `ZED_COMMIT=77d466a4550ebd1184901f3c6ed5816d3632ab0a`.
- **Toolchain**: **no local `cargo`/`rustc`.** Only `docker` and `go`. All Rust
  building/testing goes through `cd /home/retro/work/helix && ./stack build-zed dev`
  or the E2E Docker images.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the 128 upstream commits and adopt
> upstream's `git_ui_core` crate split and rustc 1.97 bump, so the fork stays
> one small window behind and the 709-commit backlog of 002353 never recurs.

### 2. Helix User
> As a Helix user, I want upstream's newest fixes and performance work without
> losing WebSocket sync, incremental streaming, headless mode, Codex/Claude ACP
> routing, or any of the 11 Critical Fixes.

### 3. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` updated **as
> each conflict is resolved** with a dated `## Merge 002701 (2026-08-10)` entry
> documenting the `git_ui_core` migration and the rustc bump — because those
> are one-time structural changes whose resolution pattern will not be obvious
> from the diff alone.

## Acceptance Criteria

### Merge Completeness
- [ ] Re-measure fence / commit count / upstream HEAD before starting; record
      the measured values in `portingguide.md`
- [ ] `git merge upstream/main` — **merge, not squash, not rebase**
- [ ] Merge branch contains all upstream commits through the recorded upstream
      HEAD; any skipped commit explicitly justified in `portingguide.md`
- [ ] All 323 fork-only commits preserved verbatim (PRs #73–#77 in particular:
      2026-07-29 upstream catch-up #73, ACP silence-watchdog #74, silent-wedge
      watchdog #75, sandbox bubblewrap-on-PATH #76, e2e phase-9 bounded
      prompt #77)
- [ ] `git log` confirms fork branch is ahead of and contains `upstream/main`

### The single textual conflict
- [ ] `crates/title_bar/Cargo.toml` resolved: take upstream's
      `git_ui_core.workspace = true` (replacing `git_ui.workspace = true`)
      **and** keep both Helix lines — the `[features]`
      `external_websocket_sync = ["dep:external_websocket_sync"]` entry and the
      `external_websocket_sync = { workspace = true, optional = true }`
      dependency

### `git_ui` → `git_ui_core` migration (the dominant work item)
- [ ] Every fork-only `git_ui::` reference to a moved symbol re-pointed at
      `git_ui_core::`. Grep baseline — 13 files currently reference `git_ui::`:
      `agent_ui/src/{agent_panel.rs,test_support.rs,thread_worktree_archive.rs}`,
      `sidebar/src/sidebar.rs`,
      `zed/src/{main.rs,zed.rs,visual_test_runner.rs,zed/open_listener.rs}`,
      `title_bar/src/title_bar.rs`, `vim/src/test/vim_test_context.rs`,
      `collab/tests/integration/git_tests.rs`,
      `project_panel/src/project_panel{.rs,_tests.rs}`
- [ ] `git_ui::git_picker::popover(workspace, repo, GitPickerTab::Branches, rems(34.), window, cx)`
      call sites migrated to `git_ui_core::build_branch_picker(workspace, repo, window, cx)`
      — note the **different arity and return type** (already `Option`, so the
      `Some(..)` wrapper is dropped)
- [ ] Any fork crate whose `Cargo.toml` depends on `git_ui` purely for moved
      symbols gains `git_ui_core.workspace = true`
- [ ] Symbols that stayed in `git_ui` (e.g. `git_panel`, `branch_diff`,
      `commit_view`, `project_diff`) are **not** blindly renamed —
      migration is compile-driven, symbol by symbol

### rustc `1.95.0 → 1.97.1`
- [ ] `rust-toolchain.toml` takes upstream's `channel = "1.97.1"`
- [ ] The Docker builder (`/home/retro/work/helix/Dockerfile.zed-build`)
      resolves and installs 1.97.1 from `rust-toolchain.toml` with no manual
      pin edit required — **verify**, do not assume; the rustup cache mount may
      need to fetch a new toolchain
- [ ] New-compiler diagnostics in **fork-only** code fixed, following upstream's
      own remedy where one exists (e.g. `rems_from_px(12.)` →
      `rems_from_px(12_f32)`)
- [ ] Cold-cache full rebuild budgeted for — the toolchain change invalidates
      the entire Cargo cache

### Conflict Resolution / auto-merge audit
- [ ] **Auto-merged ≠ correct.** Walk the audit list in `design.md`
      §"Audit auto-merged files". Priority order this window:
      `extensions_ui.rs` (−662, refactored out from under 3 HELIX markers),
      `zed.rs` (+141), `main.rs` (±27), `agent_panel.rs` (±35),
      `conversation_view/thread_view.rs` (±68)
- [ ] The 3× `// HELIX: External agent` markers in `extensions_ui.rs` are still
      present **and still in a live code path** — if upstream moved their host
      code into `components/extension_card.rs`, follow the code and re-apply
      there, recording the move in the porting guide
- [ ] No `<<<<<<<` / `>>>>>>>` markers remain anywhere in the tree

### ACP — no bump this window
- [ ] Confirm `Cargo.toml` still pins
      `agent-client-protocol = { version = "=2.0.0", features = ["unstable"] }`
      and `Cargo.lock` still resolves `agent-client-protocol 2.0.0`,
      `agent-client-protocol-derive 2.0.0`, `agent-client-protocol-schema 1.5.0`
      — **both sides are already identical, so any change here is a mistake**
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/`
      returns 0 (including `#[cfg(test)]` code)
- [ ] `Cargo.lock` conflicts (if any appear) resolved `--theirs`, then
      regenerated by the build

### Critical Fix Preservation (`portingguide.md` §"Critical Fixes")
All 11 must survive. Upstream did not touch their host files this window, so
this should be a confirming grep pass, not repair work:
- [ ] Fix #1: `NativeAgent` clone / `pending_sessions` shared-task in `load_session()`
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
- [ ] `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query
      callback, `acp_history_store()` present in `agent_panel.rs`
- [ ] `ThreadDisplayNotification` handler still calls
      `OnboardingUpsell::set_dismissed(true, cx)` **and** initialises
      `NativeAgentSessionList` (brief #9, #10)
- [ ] Fix 1b cfg-gated draft-suppression `return;` is still the FIRST statement
      of its `BaseView::Uninitialized` branch
- [ ] `from_existing_thread()` field set matches its `::new` sibling on
      `ConversationView` (file unchanged upstream — expect a clean pass)
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` (brief #7) — confirmed
      present at `crates/feature_flags/src/flags.rs:22-34` and untouched by
      upstream's flag removals
- [ ] **Constraints #8 and #12 are SUPERSEDED — see Constraint Audit below.**
      Verify the *replacement* code gates, not the JSON values
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) stays in
      `cfg(not(feature = "external_websocket_sync"))` (brief #11)
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved so
      `message_added` streams incrementally, not only at completion (brief #6, #13)
- [ ] `--allow-multiple-instances`, `--headless`, `initialize_headless()`,
      `build_application(headless)` in `crates/zed/src/main.rs` — **audit
      carefully**, upstream reworked the surrounding startup/restore flow
- [ ] `title_bar`'s `external_websocket_sync` dep stays `optional = true`;
      `render_restricted_mode` and the sign-in cfg gate intact; workspace
      `rust-embed` keeps `debug-embed`; `wait_for_tools_ready` uses
      `cx.background_executor().timer()` (no `smol::Timer`)
- [ ] 3× `// HELIX: External agent` markers in `extensions_ui.rs`
- [ ] `grep_tool.rs` `truncate_long_lines()` / `MAX_LINE_CHARS = 500` intact
- [ ] `reqwest_client.rs` / `http_client_tls.rs` insecure-TLS support intact
      (`ZED_HTTP_INSECURE_TLS`)
- [ ] `dev_container_suggest.rs` early return; migration banner `Hidden`;
      trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches stay exhaustive

#### Constraint Audit: #8 and #12 are superseded (measured finding)

The brief says `trust_all_worktrees: true` and `show_sign_in: false` must remain
in `assets/settings/default.json`. **On fork `main` today they are
`trust_all_worktrees: false` (line 2522) and `show_sign_in: true` (line 568) —
i.e. upstream's values.** This is *not* a regression. Both patches were
re-implemented as feature-gated code, which is strictly better because it
removes all merge surface from a hot 2500-line JSON file:

- **Trust** → `crates/project/src/trusted_worktrees.rs:469-474`:
  `if cfg!(feature = "external_websocket_sync") { … }` auto-trusts every
  worktree. Backed by `title_bar.rs:695-701` `render_restricted_mode()`
  returning `None` under the same gate.
- **Sign-in** → `crates/title_bar/src/title_bar.rs:379`:
  `&& !cfg!(feature = "external_websocket_sync")` suppresses the Sign In button.

**Therefore: do NOT "restore" the JSON values.** Audit the two code gates
instead, and record the supersession in the porting guide so the next brief
stops carrying stale constraints. Flagged in Open Questions for confirmation.

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] Compiles **both** with and without `external_websocket_sync` (brief #4)
- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6 invariant)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] **E2E Docker test — HARD GATE.** All phases green for `zed-agent`, and
      for `claude` via `E2E_AGENTS="zed-agent,claude"`. `go mod tidy` in
      `e2e-test/helix-ws-test-server/` first. Never use `--no-build` when
      investigating failures
- [ ] One retry permitted per agent for the known Claude Phase-1 npm-install /
      API-latency flake; a second failure is a real failure

#### E2E phases — **17**, not 4 and no longer 16

The brief names 4 phases; 002353 measured 16. `helix-ws-test-server/main.go`
now enumerates **17** — Phase 17 (interrupt) was added by fork PRs #74–#77.
The brief's four named phases map to Phases 1–4; all 17 must pass.

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

Runner: `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`. The brief's
raw `docker build -t zed-ws-e2e -f …/Dockerfile . && docker run …` targets the
same image; `run_docker_e2e.sh` is the established wrapper and is preferred.

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] New `## Merge 002701 (2026-08-10)` section inserted at the top of the
      merge-history list, i.e. above `## Merge 2026-07-29 (upstream catch-up,
      764 commits)` at line ~712
- [ ] Window summary: measured commit count, date window, fence SHA, upstream
      HEAD SHA, ACP unchanged, rustc `1.95.0 → 1.97.1`
- [ ] `### Conflicts and Resolutions` — the `title_bar/Cargo.toml` hunk
- [ ] `### git_ui → git_ui_core migration` — the full symbol map (which symbols
      moved, which stayed), every migrated call site, and the
      `popover` → `build_branch_picker` signature change. **This is the single
      most valuable thing to write down this window**
- [ ] `### rustc 1.95 → 1.97` — the toolchain bump, whether the Docker builder
      needed changes, and every fork-only diagnostic fixed
- [ ] `### Retired / superseded Helix patches` — including the constraint
      #8/#12 supersession note (JSON defaults → cfg gates)
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Commit-history table extended; stale Rebase-Checklist entries corrected

### Process
- [ ] Feature branch `feature/002701-merge-latest-zed` from current fork `main`
- [ ] Branch pushed to `origin` (mirrors `helixml/zed`)
- [ ] `sandbox-versions.txt` `ZED_COMMIT` bumped from
      `77d466a4550ebd1184901f3c6ed5816d3632ab0a` to the new merge HEAD, on a
      `feature/002701-merge-latest-zed` branch in `/home/retro/work/helix/`, pushed
- [ ] `pull_request_zed.md` + `pull_request_helix.md` written into this task dir
- [ ] `main` **not** force-pushed; no agent-initiated PRs (the Helix UI opens them)
- [ ] Re-fetch `upstream/main` and `origin/main` before declaring done; absorb
      any out-of-band fork pushes; run an extension round if upstream advanced
      materially mid-work
- [ ] helixml/zed CI pipeline green

## Out of Scope
- Net-new Helix feature development
- Modifying E2E assertions unless an upstream API change strictly requires it
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix crates beyond what the merge / `git_ui_core` split forces
- Adopting upstream's new UX (elicitation, message-queue steering, sandboxing)
  into Helix-mode flows beyond keeping them compiling
- Rewriting the porting guide from scratch — amend and extend in place

## Open Questions

1. **Constraint #8/#12 supersession — please confirm.** The brief still requires
   `trust_all_worktrees: true` and `show_sign_in: false` in
   `assets/settings/default.json`. They are not there; both were re-implemented
   as `cfg!(feature = "external_websocket_sync")` gates
   (`trusted_worktrees.rs:474`, `title_bar.rs:379`). **Assumption: the cfg-gate
   implementation is the intended one and the JSON constraints are obsolete —
   audit the gates and leave the JSON at upstream values.** Confirm, and
   ideally drop #8/#12 from future briefs.

2. **Porting-guide path (repeat of 002265 / 002353's unanswered question).** The
   brief names `design/2026-02-07-zed-fork-rebase-to-upstream.md`, which exists
   only in the **helix** repo and is a 638-line historical narrative with no
   `## Merge NNN` sections. Every merge since has updated
   `/home/retro/work/zed/portingguide.md`. **Assumption: `zed/portingguide.md`
   is the target.** Confirm, or say if both should be touched.

3. **Repo/branch layout (repeat).** The brief says
   `/prod/home/luke/pm/zed-upstream` on branch `helix-fork` with remotes
   `helix`/`origin`. Sandbox reality is `/home/retro/work/zed` on `main` with
   `origin` (gitea mirror) + `upstream`. **Assumption: work in the sandbox
   layout.** Confirm.

4. **`--locked` in CI (brief constraint #5) — still appears stale (repeat).**
   There is no `.drone.yml` in the zed repo; Helix's `Dockerfile.zed-build`
   runs `cargo build [--release] --features external_websocket_sync` with no
   `--locked`. **Assumption: obsolete; verify CI green and do not add
   `--locked` without instruction.** 002353 asked this and it went unanswered —
   a yes/no would let us delete the constraint.

5. **rustc 1.97.1 availability in the build image.** `Dockerfile.zed-build`
   installs rustup with `--default-toolchain none` and lets
   `rust-toolchain.toml` drive the version, so 1.97.1 *should* resolve
   automatically. But the BuildKit `/root/.rustup` cache mount will need to
   fetch a fresh toolchain, and the base image's glibc/linker must support it.
   **Assumption: no Dockerfile change needed.** If the toolchain download fails
   or the base image is too old, this becomes a Helix-repo change and should be
   flagged immediately rather than worked around by pinning back to 1.95.

6. **`git_ui_core` and the `git_ui` dependency.** Some fork crates may end up
   needing *both* `git_ui` and `git_ui_core`. **Assumption: add `git_ui_core`
   alongside `git_ui` where both are needed; only remove `git_ui` from a
   `Cargo.toml` when nothing in that crate references it any more.** For
   `title_bar` specifically, upstream removed `git_ui` outright — take that.

7. **Upstream's session list / resume UI vs `from_existing_thread` (repeat).**
   `acp_thread/src/connection.rs` did **not** change this window, so nothing
   forces the question. **Assumption: keep `from_existing_thread` as-is;**
   replacing it with upstream's native path remains a separate follow-up task.

8. **E2E credentials.** The gate needs a working `ANTHROPIC_API_KEY` in the
   environment. Assumed available to the implementation agent — flag
   immediately if not, because the merge cannot be declared complete without it.

9. **Feature-off compilation gate (repeat).** With no local cargo, the cleanest
   way to check `cargo check -p zed` *without* `external_websocket_sync` is a
   one-off Docker run in the build image. Is there an existing `./stack` target
   for the feature-off build? **Assumption: none; use a one-off Docker
   invocation.**
