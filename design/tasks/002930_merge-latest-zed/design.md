# Design: Merge Latest Zed Upstream Into Helix Fork

## Overview

Merge `upstream/main` (`zed-industries/zed`) into the Helix fork's live branch
`main` on `helixml/zed`, resolve five small textual conflicts, complete the
`git_ui` → `git_ui_core` migration that upstream forced, absorb the rustc
`1.95.0 → 1.97.1` bump, audit the auto-merged files that host Helix patches,
and prove the result with the Docker E2E suite.

This is a **catch-up merge**: the previous cycle (002701, 2026-08-10) was
planned but never executed, so its 128-commit window has grown into a
331-commit one. 002701's `design.md` is still directly applicable — in
particular §"Work item 1: `git_ui` → `git_ui_core`" contains the verified symbol
map, which has not been re-derived here. **Read it.**

## Measured Baseline (2026-08-24)

```
fence           b9256fa8f018bf03eb2e420120163746f8298d83   2026-07-29
upstream HEAD   7f2a2c3c3ee2f23f28772dee7661fb98d3910990   2026-08-24
fork HEAD       7357327f2df809279daeceec10e453edf77710bb   2026-08-20  (PR #91)
to merge        331 commits over 26 days
fork-only       349 commits
ACP             2.0.0 / derive 2.0.0 / schema 1.5.0 — identical both sides, NO BUMP
rustc           1.95.0 → 1.97.1
zed version     1.15.0 → 1.18.0
crates added    git_ui_core, gpui_apple, tabular_data_preview
crates removed  csv_preview
```

### Planning technique (reused from 002353/002701 — still the fastest)

Measure conflicts **without touching the working tree** using an in-memory
three-way merge. This is what produced the five-file conflict list above and is
safe to run during planning:

```bash
git fetch --no-tags https://github.com/zed-industries/zed.git \
    main:refs/remotes/upstream/main

git merge-tree --write-tree --name-only origin/main upstream/main   # conflict list
git merge-tree --write-tree          origin/main upstream/main > /tmp/mt.txt
TREE=$(head -1 /tmp/mt.txt)
git show $TREE:crates/zed/src/main.rs | grep -n -A20 '<<<<<<<'      # actual hunks
```

Then size the audit surface with
`git diff <fence>..upstream/main --stat -- <helix-hosting files>` and check how
much of each delta is inside `mod tests` (for `zed.rs` and `agent_panel.rs` this
window, most of it is).

## Merge Strategy

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # not configured
git fetch upstream --no-tags
git fetch origin
git checkout -b feature/002930-merge-latest-zed origin/main

# START the portingguide.md "## Merge 002930 (2026-08-24)" entry NOW,
# with the measured baseline block, before the first conflict is touched.

git merge upstream/main     # merge, NOT squash, NOT rebase

# Order of work:
#   1. Cargo.lock (--theirs)
#   2. The three Cargo.toml conflicts
#   3. crates/zed/src/main.rs
#   4. git_ui_core migration (compile-driven, the long pole)
#   5. rustc 1.97 diagnostics
#   6. audit auto-merged Helix-hosting files
#   7. unit tests → E2E
# Write each resolution into portingguide.md as it is made.
```

### Conflict-resolution principles (carried forward, unchanged)

1. **Take upstream for upstream-owned code.** Formatting, ordering, new models,
   new deps — upstream wins. Never hand-merge `anthropic.rs` model lists.
2. **Take fork for fork-owned code.** Anything under
   `crates/external_websocket_sync/`, anything inside a
   `cfg(feature = "external_websocket_sync")` block, anything marked
   `// HELIX:`.
3. **Union for shared lists.** Dependency lists, `[features]`, workspace
   members: take upstream's shape and re-insert the Helix lines.
4. **`Cargo.lock` is never hand-merged.** `--theirs`, then let the build
   regenerate it.
5. **Auto-merged ≠ correct.** Git will happily merge a Helix block into a
   function upstream deleted or moved. The audit is the real work.
6. **Compile-driven, not grep-driven, for the `git_ui_core` migration.** Rename
   only what the compiler says moved.

## The five textual conflicts, with resolutions

### 1. `Cargo.lock`
Resolve `--theirs` (upstream), regenerate by building. Never hand-edit.

### 2. `crates/title_bar/Cargo.toml`
```
<<<<<<< origin/main
external_websocket_sync = { workspace = true, optional = true }
git_ui.workspace = true
=======

git_ui_core.workspace = true
>>>>>>> upstream/main
```
**Resolution:** keep the `external_websocket_sync` optional dep, take
`git_ui_core.workspace = true`, drop `git_ui.workspace = true` (upstream removed
it outright — `title_bar` only used moved symbols). Also verify the `[features]`
entry `external_websocket_sync = ["dep:external_websocket_sync"]` survives
elsewhere in the file.

### 3. `crates/agent_ui/Cargo.toml`
```
<<<<<<< origin/main
time.workspace = true
time_format.workspace = true
tokio.workspace = true
=======
>>>>>>> upstream/main
```
Upstream's own diff against the fence removes `git_ui` and `time`, reorders
`fuzzy`/`git`, appends `git_ui_core.workspace = true`, and drops the dev-dep
`clock`. **Resolution:** take upstream's shape; re-insert fork-only
`time_format.workspace = true` and `tokio.workspace = true`. Re-add
`time.workspace = true` **only if the compiler asks for it** — see Open
Question 8.

### 4. `crates/zed/Cargo.toml`
```
<<<<<<< origin/main
tokio.workspace = true
ztracing.workspace = true
tracing.workspace = true
=======
>>>>>>> upstream/main
```
Upstream bumped `version` `1.15.0 → 1.18.0`, added an `inspector` feature block,
and reshuffled `[dependencies]` (adds `acp_thread`, optional `action_log`,
optional `agent_servers`; alphabetises `agent`/`agent-client-protocol`).
**Resolution:** take upstream's list wholesale, then re-insert fork-only
`tokio.workspace = true` and `ztracing.workspace = true` in alphabetical
position, and keep every Helix `[features]` entry (`external_websocket_sync`,
`tracy` → `ztracing/tracy`, etc.).

### 5. `crates/zed/src/main.rs`
```
<<<<<<< origin/main
    let app = build_application(args.headless).with_assets(Assets);
=======
    let app = build_application()
        .with_assets(Assets)
        .with_restart_arguments(restart_arguments);
>>>>>>> upstream/main
```
**Resolution — keep both sides:**
```rust
let app = build_application(args.headless)
    .with_assets(Assets)
    .with_restart_arguments(restart_arguments);
```
Then verify the surrounding Helix startup surface is intact:
`args.allow_multiple_instances`, `args.headless` in the
`failed_single_instance_check` guard (already auto-merged correctly per the
merge-tree output), `initialize_headless()`, and the `headless` parameter on
`build_application`'s definition.

## Work item 1: `git_ui` → `git_ui_core` (the long pole)

Upstream extracted a new workspace crate `crates/git_ui_core/` out of `git_ui`.
**002701 design.md §"Work item 1" holds the verified symbol map** (which files
moved, which stayed) — reuse it rather than re-deriving.

Summary of what moved: `worktree_service.rs`, `worktree_picker.rs`,
`worktree_names.rs`, `askpass_modal.rs`, `created_worktrees.rs`,
`file_diff_view.rs`, `notifications.rs`. What stayed in `git_ui`: `git_panel`,
`branch_diff`, `commit_view`, `project_diff`.

**The API change that is not a rename:**
```rust
// before
git_ui::git_picker::popover(workspace, repo, GitPickerTab::Branches, rems(34.), window, cx)
// after — different arity, already returns Option (drop the Some(..) wrapper)
git_ui_core::build_branch_picker(workspace, repo, window, cx)
```

**Grep baseline — 13 fork files reference `git_ui::` today:**
`agent_ui/src/{agent_panel.rs,test_support.rs,thread_worktree_archive.rs}`,
`sidebar/src/sidebar.rs`,
`zed/src/{main.rs,zed.rs,visual_test_runner.rs,zed/open_listener.rs}`,
`title_bar/src/title_bar.rs`, `vim/src/test/vim_test_context.rs`,
`collab/tests/integration/git_tests.rs`,
`project_panel/src/project_panel{.rs,_tests.rs}`.

Work symbol by symbol from compiler errors. Every crate that ends up using a
moved symbol needs `git_ui_core.workspace = true` in its `Cargo.toml`; remove
`git_ui` from a crate only once nothing there references it.

**Verification before declaring the migration done:**
```bash
grep -rn "git_ui::\(worktree_service\|worktree_picker\|worktree_names\|askpass_modal\|created_worktrees\|file_diff_view\|notifications\|git_picker\)" crates/
# must return 0
```

## Work item 2: rustc `1.95.0 → 1.97.1`

Two minor versions in one hop. Upstream pre-emptively changed
`rems_from_px(12.)` → `rems_from_px(12_f32)` in ~8 places, which implies a float
inference ambiguity the new compiler surfaces. Expect the same treatment for
fork-only code calling the same APIs.

- `rust-toolchain.toml` takes upstream's `channel = "1.97.1"` (auto-merges).
- `/home/retro/work/helix/Dockerfile.zed-build` installs rustup with
  `--default-toolchain none` and lets `rust-toolchain.toml` drive the version,
  so 1.97.1 *should* resolve with no Dockerfile edit — **verify, don't assume.**
  The BuildKit `/root/.rustup` cache mount must fetch a new toolchain.
- **Budget a cold full rebuild.** The toolchain change invalidates the entire
  Cargo cache; the first `./stack build-zed dev` will not be the usual ~3 min.
- If 1.97.1 cannot be installed in the build image, that is a Helix-repo
  problem — flag it, do not pin back to 1.95.

## Work item 3: audit the auto-merged files

Measured upstream deltas against the fence, in audit priority order. Note how
much of the two largest deltas is test code — that is the good news this window.

| Pri | File | Δ | Upstream change | What to check |
|---|---|---|---|---|
| P1 | `extensions_ui/src/extensions_ui.rs` | −662 | Bulk moved into `components/extension_card.rs` | The 3× `// HELIX: External agent` markers must exist **and be in a live path**. If upstream moved their host, re-apply in the new file and record the move |
| P2 | `agent/src/agent.rs` | +43 | One hunk in `NativeAgentConnection` (~line 2235) | **Fix #1's host.** `pending_sessions` shared-task in `load_session()`, `wait_for_tools_ready` using `cx.background_executor().timer()` |
| P3 | `zed/src/zed.rs` | +382 | ~90% inside `mod tests`. Real: new action registrations, `initialize_panels`, `open_new_ssh_project_from_project` args, `notify_settings_errors` | `initialize_agent_panel` and the WebSocket init inside it |
| P4 | `agent_ui/src/agent_panel.rs` | +118 | Only **2** non-test hunks: `SiblingThreadHost` (~4792) and an `AgentPanel` block (~6051, `git_ui_core` + `IconButton` rework) | `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query callback, `acp_history_store()`, `ThreadDisplayNotification` handler (Onboarding upsell + `NativeAgentSessionList`), Fix #11 entity-identity guard |
| P5 | `agent_ui/src/conversation_view/thread_view.rs` | ±68 | `restrict_scroll_to_axis()`, `pause_following_tail()` on compaction, `rems_from_px(_f32)` | Helix `current_model_id()` 3-way fallback |
| P6 | `anthropic/src/anthropic.rs` | +330 | New models | Take upstream ordering wholesale — do not hand-merge |
| P7 | `agent_servers/src/acp.rs` | ±33 | Protocol plumbing | `SessionCreationGuard`, `session_creation_chain` (PR #50) |
| P8 | `zed/src/main.rs` | ±46 | `restart_arguments`, startup/restore rework | `--headless`, `--allow-multiple-instances`, `initialize_headless()`, `build_application(headless)` |
| P9 | `feature_flags/src/flags.rs` | −27 | Removed `ProjectPanelUndoRedoFeatureFlag`, `AutoWatchFeatureFlag` | `AcpBetaFeatureFlag::enabled_for_all() -> true` survived the pruning |
| P10 | `title_bar/src/title_bar.rs` | ±16 | `git_ui_core::worktree_picker::WorktreePicker`, `build_branch_picker` | `render_restricted_mode` → `None` gate, sign-in cfg gate |
| P11 | `assets/settings/default.json` | ±52 | New keys, reworded comments | Do **not** re-add `trust_all_worktrees` / `show_sign_in` — see below |
| P12 | `acp_thread/src/acp_thread.rs` | ±7 | `disable_pagers_through_env(&mut env)` | Trivial; Critical Fixes untouched |
| P13 | `Cargo.toml` (workspace) | ±170 | New members `git_ui_core`, `gpui_apple`, `tabular_data_preview`; removed `csv_preview` | Keep all Helix members; keep `rust-embed` `debug-embed` |

**Confirming grep only — unchanged upstream this window:**
`acp_thread/src/connection.rs`, `crates/external_websocket_sync/**`,
`agent_ui/src/conversation_view.rs` (±2 lines),
`reqwest_client/src/reqwest_client.rs`, `agent/src/tools/grep_tool.rs`,
`language_models/src/provider/open_ai.rs`.

## Key decision: elicitations / Phase 18 are not on `main`

Measured, contrary to the brief:

- `crates/external_websocket_sync/src/**` on fork `main` has **zero**
  `elicitation` references.
- The e2e server on `main` has **17** phases; Phase 18 does not exist there.
- The work lives on `origin/feature/002731-agent-questions`
  (tip `859325b38f`), **7 ahead / 14 behind `main`**, adding `elicitation` to
  `external_websocket_sync.rs` (16 refs), `thread_service.rs` (72),
  `types.rs` (32), `websocket_sync.rs` (9), plus e2e Phase 18.

Separately: `AgentThreadEntry::Elicitation(ElicitationEntryId)` is **upstream's
own variant**, already present at the fence and byte-identical on fork `main`,
`upstream/main` and the feature branch. There is **no enum conflict** to resolve
— the brief's concern does not materialise.

**Design decision (Open Question 1, assumption (a)):** merge upstream into
`main` first and gate on the 17 phases that exist there. This is low-risk
because `crates/external_websocket_sync/**` is entirely unchanged upstream this
window, so `feature/002731-agent-questions` can rebase onto the merged `main`
with essentially no conflict surface in its own crate. Landing the feature
branch first would couple two independent risks and make a 331-commit merge
harder to bisect.

## Key decision: do not "restore" `trust_all_worktrees` / `show_sign_in`

Brief constraints #8 and #12 are superseded. Both were re-implemented as
`cfg!(feature = "external_websocket_sync")` gates —
`crates/project/src/trusted_worktrees.rs` (auto-trust) and
`crates/title_bar/src/title_bar.rs` (sign-in suppression) — which is strictly
better because it removes all merge surface from a hot 2500-line JSON file that
upstream changed by 52 lines this window. Audit the gates; leave the JSON at
upstream values; record the supersession in the porting guide.

## Build & Test

There is **no local `cargo`/`rustc`** — only `docker` and `go`. Everything goes
through the Helix build image.

```bash
# 1. Feature-ON build (canonical). Expect a COLD full rebuild — rustc changed.
cd /home/retro/work/helix && ./stack build-zed dev

# 2. Feature-OFF gate — same builder image with the feature flag removed.
#    No ./stack target exists for this (Open Question 10); use a one-off
#    docker run in the build image.

# 3. Unit tests
cargo test -p external_websocket_sync
cargo test -p acp_thread test_second_send                              # Fix #6
cargo test -p agent_servers test_concurrent_session_creation_is_serialized  # PR #50

# 4. E2E — HARD GATE
cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server
go mod tidy
cd .. && ./run_docker_e2e.sh                       # all 17 phases, zed-agent
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh  # and claude
```

`run_docker_e2e.sh` is the established wrapper and targets the same image as the
brief's raw `docker build -t zed-ws-e2e -f .../Dockerfile . && docker run ...`;
prefer the wrapper. Never pass `--no-build` while investigating a failure. One
retry per agent is permitted for the known Claude Phase-1 npm-install/API
latency flake; a second failure is real.

### CI note

There is no `.drone.yml` in the zed repo. CI for the fork runs from
`/home/retro/work/helix/.drone.yml`, which fetches `ZED_COMMIT` from
`sandbox-versions.txt`, builds via `Dockerfile.zed-build`
(`cargo build --features external_websocket_sync`, **no `--locked`**), and runs
a `zed-e2e-test` step. Brief constraint #5 (`--locked` in `.drone.yml`) does not
match reality — do not add it without instruction (Open Question 5).

## Helix Repo Bump

```bash
cd /home/retro/work/helix
git checkout -b feature/002930-merge-latest-zed
# sandbox-versions.txt line 1:
#   ZED_COMMIT=71a2940881e37fff3ca099cb49ae15ce4b996f9a   (stale — pre-dates PR #91)
#   → ZED_COMMIT=<new merge HEAD on helixml/zed>
git push origin feature/002930-merge-latest-zed
```

Do not open the PR — the Helix UI does that.

## Documentation Updates (incremental, not retrospective)

Target: `/home/retro/work/zed/portingguide.md`. Insert
`## Merge 002930 (2026-08-24)` **above** `## Merge 2026-07-29 (upstream
catch-up, 764 commits)` at line ~744. Required subsections:

1. **Window summary** — fence SHA, upstream HEAD SHA, 331 commits, 26 days, ACP
   unchanged, rustc `1.95.0 → 1.97.1`, zed `1.15.0 → 1.18.0`.
2. **Conflicts and Resolutions** — all five files, hunk by hunk, with the
   `main.rs` `build_application(args.headless) + with_restart_arguments`
   resolution spelled out.
3. **`git_ui` → `git_ui_core` migration** — the symbol map, every migrated call
   site, and the `popover` → `build_branch_picker` signature change. *The most
   valuable thing written this window.*
4. **rustc 1.95 → 1.97** — whether the Docker builder needed changes; every
   fork-only diagnostic fixed.
5. **Crate churn** — `git_ui_core`, `gpui_apple`, `tabular_data_preview` added;
   `csv_preview` removed.
6. **Retired / superseded Helix patches** — the #8/#12 JSON → cfg-gate
   supersession.
7. **Helix-surface survival check** — per-area confirmation.
8. **A note that 002701 was planned but never executed**, so the next engineer
   understands why this window is 331 commits.

Then extend the commit-history table and correct any stale Rebase-Checklist
entries.

## Lessons carried forward (for whoever clones this spec)

- **Verify the brief against the repo before trusting it.** This brief named a
  branch that is dead (`helix-fork`, last touched 2026-02-07), a porting-guide
  path that does not exist in the zed repo, a CI flag that is not in any CI
  file, and a Phase 18 gate for code that is still on an unmerged feature
  branch. Every one of those was caught by a two-minute grep.
- **A skipped merge cycle compounds.** 002701's 128 commits became 331 in two
  weeks. The `git_ui_core` and rustc work did not get easier for waiting.
- **`git merge-tree --write-tree` is the right planning tool.** It gives the
  exact conflict list and hunks with zero working-tree mutation, which is
  essential when the planning phase must not modify the codebase.
- **Size the audit by non-test delta, not total delta.** `zed.rs` +382 and
  `agent_panel.rs` +118 look alarming; filtering hunks that fall inside
  `mod tests` reduces both to a handful of real changes.
- **Check where a "merged" feature actually lives.** `git branch -r | grep` plus
  `git log --oneline A..B | wc -l` on both directions settles it in seconds.
- **Constraint lists rot.** Four of this brief's thirteen constraints (#5, #8,
  #12, and the repo layout in #1) no longer describe the code. Re-verify each
  one every cycle and record supersessions in the porting guide.

## Out of Scope

- Net-new Helix feature development
- Merging `feature/002731-agent-questions` (pending Open Question 1)
- Modifying E2E assertions unless an upstream API change strictly requires it
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors beyond what the merge / `git_ui_core` split forces
- Adopting upstream's native elicitation UI, message-queue steering or
  sandboxing into Helix-mode flows beyond keeping them compiling
- Rewriting the porting guide from scratch
