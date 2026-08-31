# Design: Merge Latest Zed Upstream Into Helix Fork

## Companion documents

This is a **delta design**. Two prior designs remain directly applicable and
should be open alongside it:

- `002701_merge-latest-zed/design.md` — the authoritative `git_ui` →
  `git_ui_core` symbol map and the rustc-bump analysis. Written against the
  *same* upstream split; still accurate because neither has landed.
- `002930_merge-latest-zed/design.md` — the merge-strategy and audit-ordering
  rationale, and the constraint-supersession analysis.
- `/home/retro/work/zed/portingguide.md` — the living record of every prior
  merge's conflicts and the 11 Critical Fixes.

## 1. Situation

Three planning cycles (002701, 002930, 003012) have now planned the same merge.
Measured evidence that **none of the first two executed**: `rust-toolchain.toml`
is still `1.95.0`, `crates/git_ui_core` does not exist, `crates/csv_preview` is
still present, 13 files still reference `git_ui::`, and the newest
`## Merge` section in `portingguide.md` is `2026-07-29`. The merge-base with
upstream has not moved off `b9256fa8f0` (2026-07-29).

Backlog growth: **128 → 331 → 434 commits**. The window is now 33 days.

The design consequence is that this spec plans **one merge covering all three
windows**, not an incremental catch-up. The structural work items (crate split,
toolchain bump) are one-time costs that do not grow with the window; the audit
surface does, and it has grown a lot.

## 2. Merge strategy

**A single true merge commit, `git merge upstream/main`, onto a feature branch
cut from fork `main`.** Not a rebase, not a squash.

Rationale (unchanged from prior windows, restated because it is load-bearing):

- The fork carries **351 fork-only commits** across PRs #73–#92. A rebase would
  rewrite all of them and force every open feature branch — including
  `feature/002731-agent-questions` — to be recreated.
- A merge commit gives the next window a clean fence: `git merge-base` moves to
  upstream HEAD, so the next `origin/main..upstream/main` count is exact.
- Squashing would destroy upstream's bisectable history, which is the main
  reason merges are cheap to audit at all.

Branch: `feature/003012-merge-latest-zed`, cut from `origin/main`
(`1e0be14e6c`), pushed to `origin` (the gitea mirror of `helixml/zed`). `main` is
never force-pushed; `origin/helix-fork` (dead at 2026-02-07) is never touched.

### Sequencing decision — merge into `main`, not into the feature branch

Per Open Question 1 (assumption (a)): merge upstream into `main` first, gate on
the **17** phases that exist on `main`, and let
`feature/002731-agent-questions` rebase afterwards.

This is safe because of a measured fact:
`crates/external_websocket_sync/**` **does not exist upstream at all** — it is a
fork-only crate. It therefore cannot produce a merge conflict, and the entire
elicitation surface (30 + 106 + 44 + 10 references across
`external_websocket_sync.rs`, `thread_service.rs`, `types.rs`,
`websocket_sync.rs`) lives inside it. The feature branch's rebase after this
merge touches only files upstream never modified, plus the Go e2e server.

The only cross-boundary risk is `crates/acp_thread/src/acp_thread.rs`, which
hosts the `Elicitation` types the feature branch consumes. Planning verified
those types are **upstream's own and byte-identical on both sides**
(`AgentThreadEntry::Elicitation(ElicitationEntryId)`, `struct Elicitation`,
`ElicitationStatus`, `ElicitationStoreEvent`) — the file auto-merges with a
±16 upstream delta that does not touch that block.

## 3. Conflict resolution plan — six files

`git merge-tree --write-tree origin/main upstream/main` produces tree
`86a57a88aa` with exactly six conflicted paths. All are small; none is
structural.

### 3.1 `crates/http_client_tls/Cargo.toml` — NEW this window

The one conflict 002930 did not see. Both sides appended to the same
`[dependencies]` block:

```
fork:     rustls-pki-types = "1"
upstream: log.workspace = true
          webpki-roots.workspace = true
```

**Resolution: union.** Keep all three. This is a pure adjacency conflict with no
semantic overlap. Pair it with the P8 audit of
`crates/http_client_tls/src/http_client_tls.rs` (±11 upstream) — upstream's
`webpki-roots` addition and the fork's `rustls-pki-types` insecure-TLS support
must coexist at the source level too.

### 3.2 `crates/title_bar/Cargo.toml`

Fork adds an `external_websocket_sync` optional dep and `[features]` entry;
upstream does `git_ui` → `git_ui_core`, drops `notifications`, drops the whole
`[target.'cfg(windows)'.dependencies]` block, and reshuffles dev-deps
(`recent_projects` test-support in; `notifications`, `release_channel` out).

**Resolution:** take upstream's entire shape, then re-insert the two fork lines:
`external_websocket_sync = { workspace = true, optional = true }` in
`[dependencies]` and `external_websocket_sync = ["dep:external_websocket_sync"]`
in `[features]`. `git_ui.workspace = true` is **dropped**, not renamed — upstream
removed it outright, so `title_bar` needs only `git_ui_core`.

### 3.3 `crates/agent_ui/Cargo.toml`

Fork adds `time_format`, `tokio`, the `external_websocket_sync` `[features]`
entry and the `external_websocket_sync_dep` path dependency (aliased via
`package = "external_websocket_sync"`). Upstream drops `git_ui` and `time`,
reorders `fuzzy`/`git`, adds `git_ui_core`, and prunes five dev-deps (`clock`,
`node_runtime`, `remote_server`, `tree-sitter-md`, `unindent`).

**Resolution:** take upstream's shape; re-insert the four fork lines. Then let
the compiler decide about `time` and the pruned dev-deps — re-add only what
fails to build. Do not guess; a speculative re-add silently re-creates the same
conflict next window.

### 3.4 `crates/zed/Cargo.toml`

Upstream bumps `version` `1.15.0` → `1.19.0`, adds an `inspector` feature block,
and reshuffles dependencies (`acp_thread` added, `action_log` and `agent_servers`
made `optional`). Fork adds `tokio`, `ztracing`, the `external_websocket_sync`
optional dep and its `[features]` entry.

**Resolution:** take upstream wholesale, re-insert the Helix lines. Note the
Helix `external_websocket_sync` feature fans out to
`agent_ui/external_websocket_sync`, `title_bar/external_websocket_sync`,
`agent_servers/external_websocket_sync`, `project/external_websocket_sync` —
`agent_servers` becoming `optional` upstream means that feature edge now needs
`dep:agent_servers` semantics checked. If cargo complains about an optional
dependency referenced by a feature, that is the cause.

### 3.5 `crates/zed/src/main.rs`

The only source conflict. Fork changed `build_application()` →
`build_application(headless: bool)` and threads `args.headless` through;
upstream added `.with_restart_arguments(restart_arguments)`.

**Resolution:**

```rust
let app = build_application(args.headless)
    .with_assets(Assets)
    .with_restart_arguments(restart_arguments);
```

Both error-path call sites keep `build_application(false)`. Separately verify
that `|| args.allow_multiple_instances || args.headless` survives upstream's
rework of the single-instance check block — upstream touched the surrounding
region (`@@ -226,22 +226,12 @@`, `@@ -268,10 +258,15 @@`), so this is an
audit point even though it is not a textual conflict.

### 3.6 `Cargo.lock`

Resolve `--theirs` (upstream), then regenerate by building. Never hand-edit.

## 4. Work item 1 — `git_ui` → `git_ui_core` migration

The dominant work item, carried unexecuted through two windows. **Use 002701
design.md §"Work item 1" as the symbol map** — it was written against this exact
upstream split and remains accurate.

Approach: **compile-driven, symbol by symbol.** Upstream split `git_ui` into
`git_ui_core` (reusable pickers/primitives) and `git_ui` (panels/views). Some
symbols moved; some stayed. A blanket `s/git_ui::/git_ui_core::/` is wrong and
will produce a large, hard-to-review breakage.

Grep baseline — **13 files** reference `git_ui::` on the fork today:

```
crates/agent_ui/src/agent_panel.rs
crates/agent_ui/src/test_support.rs
crates/agent_ui/src/thread_worktree_archive.rs
crates/sidebar/src/sidebar.rs
crates/zed/src/main.rs
crates/zed/src/zed.rs
crates/zed/src/visual_test_runner.rs
crates/zed/src/zed/open_listener.rs
crates/title_bar/src/title_bar.rs
crates/vim/src/test/vim_test_context.rs
crates/collab/tests/integration/git_tests.rs
crates/project_panel/src/project_panel.rs
crates/project_panel/src/project_panel_tests.rs
```

The one signature change that is not a rename:

```rust
// before
git_ui::git_picker::popover(workspace, repo, GitPickerTab::Branches, rems(34.), window, cx)
// after — different arity AND already returns Option, so the Some(..) wrapper is dropped
git_ui_core::build_branch_picker(workspace, repo, window, cx)
```

Symbols that **stayed** in `git_ui`: `git_panel`, `branch_diff`, `commit_view`,
`project_diff`. Crates needing both gain `git_ui_core.workspace = true`
alongside `git_ui`; `git_ui` is removed from a manifest only when nothing in
that crate references it any more (`title_bar` and `agent_ui` do reach that
state, because upstream removed `git_ui` from both).

Write the full before/after symbol map into `portingguide.md` as it is
discovered. **This is the single highest-value artefact of this window** — it is
a one-time structural change whose resolution is not recoverable from the diff.

## 5. Work item 2 — rustc `1.95.0 → 1.97.1`

Take upstream's `rust-toolchain.toml`. The risk is not the language change; it
is the build environment:

- `/home/retro/work/helix/Dockerfile.zed-build` installs rustup with
  `--default-toolchain none` and lets `rust-toolchain.toml` drive the version, so
  1.97.1 *should* resolve with no Dockerfile edit. **Verify, do not assume** —
  the BuildKit `/root/.rustup` cache mount must fetch a fresh toolchain, and the
  base image's glibc/linker must support it.
- The toolchain change **invalidates the entire Cargo cache**. Budget for a
  cold full rebuild, and do not interpret the first long build as a hang.
- New-compiler diagnostics in **fork-only** code get fixed following upstream's
  own remedy where one exists (the known example: `rems_from_px(12.)` →
  `rems_from_px(12_f32)`).

If the toolchain download fails or the base image is too old, that becomes a
Helix-repo change — flag it rather than pinning back to 1.95, which would
re-create this exact debt for a fourth window.

## 6. Work item 3 — crate churn

Larger than 002930 measured. Added upstream: `git_ui_core`, `gpui_apple`,
`tabular_data_preview`, **`call_hierarchy`**, **`language_detection`**. Removed
upstream: `csv_preview`, **`rich_text`**, **`supermaven`**, **`supermaven_api`**.

Planning verified the removals are clean: a workspace-wide grep of
`crates/*/Cargo.toml` finds `rich_text`, `supermaven` and `supermaven_api`
referenced **only by their own manifests** — no fork crate depends on them.
Take upstream's removal wholesale.

The workspace `Cargo.toml` `[workspace] members` and `[workspace.dependencies]`
must absorb upstream's additions/removals **while keeping every Helix member**
(`external_websocket_sync`, `sidebar`, …). That file auto-merges; review the
result rather than trusting it.

## 7. Work item 4 — the auto-merge audit

**Auto-merged is not the same as correct.** Git merges by line adjacency; it has
no notion of whether a Helix `cfg` block still sits in a live code path after
upstream moved the function around it.

The critical design input this window: **002930's audit priorities are stale.**
Upstream deltas re-measured against the same fence are far larger, and four files
002930 classified "confirming grep only" have changed materially.

| File | 002930 | **003012** | Classification change |
|---|---|---|---|
| `zed/src/zed.rs` | +382 | **+816 (32 hunks)** | P3 → **P1** |
| `agent_ui/src/agent_panel.rs` | +118 | **+549 (24 hunks)** | P4 → **P2** |
| `agent/src/agent.rs` | +43 | **+363** | P2 → **P3** |
| `agent_ui/src/conversation_view.rs` | ±2 | **±125** | grep-only → **P4** |
| `title_bar/src/title_bar.rs` | ±16 | **+108** | unlisted → **P5** |
| `reqwest_client/src/reqwest_client.rs` | unchanged | **±73** | grep-only → **P7** |
| `http_client_tls/src/http_client_tls.rs` | unchanged | **±11** | unlisted → **P8** |
| `agent/src/tools/grep_tool.rs` | unchanged | **±12** | grep-only → **P12** |
| `language_models/.../open_ai.rs` | unchanged | **±18** | grep-only → **P14** |

Files **verified byte-unchanged upstream** and therefore genuinely grep-only:
`crates/acp_thread/src/connection.rs`, `crates/agent_ui/src/acp/**`,
`crates/project/src/trusted_worktrees.rs`, and all of
`crates/external_websocket_sync/**` (fork-only, cannot conflict).

Audit method per file: for each Helix marker (`cfg(feature = ...)` block,
`// HELIX:` comment, named function from the Critical Fixes list), confirm it is
(a) still present and (b) still reachable — upstream moving its host function or
extracting it into a new module is the failure mode, not deletion. The
`extensions_ui.rs` −662 is the canonical example: the bulk moved to
`components/extension_card.rs`, so the 3× `// HELIX: External agent` markers may
need re-applying in the new file even though the merge reported no conflict.

## 8. ACP — no change, third window running

`agent-client-protocol = { version = "=2.0.0", features = ["unstable"] }` in
**both** `Cargo.toml`s. `Cargo.lock` on **both** sides resolves
`agent-client-protocol 2.0.0`, `agent-client-protocol-derive 2.0.0`,
`agent-client-protocol-schema 1.5.0`.

**No builder-pattern sweep is expected.** The non-exhaustive-struct discipline
from prior windows (`ErrorCode` variants, struct literals → builders) is dormant
this window. If a builder sweep becomes necessary, the pin has moved — stop and
record why in the porting guide before proceeding.

## 9. Build & test strategy under sandbox constraints

**There is no local `cargo` or `rustc`.** Only `docker` and `go`. All Rust work
goes through:

- `cd /home/retro/work/helix && ./stack build-zed dev` — the primary build gate
- a one-off Docker run in the build image for the **feature-off** `cargo check -p zed`
  (no `./stack` target exists for this; see Open Question 10)
- `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` for the E2E gate —
  **preferred over the brief's raw `docker build` / `docker run`**; it is a
  wrapper over the same image and handles wiring. Run `go mod tidy` in
  `e2e-test/helix-ws-test-server/` first. Never pass `--no-build` while
  investigating a failure

Gate order (cheapest signal first):

1. Merge + resolve six conflicts → tree clean, no conflict markers
2. `./stack build-zed dev` → compile with the feature
3. Feature-off Docker `cargo check -p zed` → compile without the feature
4. `cargo test -p external_websocket_sync` (all unit/mock tests)
5. `cargo test -p acp_thread test_second_send` (Fix #6)
6. `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
7. **E2E hard gate**: all 17 phases green for `zed-agent`, then for `claude` via
   `E2E_AGENTS="zed-agent,claude"`. One retry per agent is permitted for the
   known Claude Phase-1 npm-install / API-latency flake; a second failure is real
8. helixml/zed CI green (drone `build-zed` + `zed-e2e-test` in Helix's
   `.drone.yml`)

`ANTHROPIC_API_KEY` was measured **absent** in the planning environment. Step 7
cannot run without it — check for it early rather than after the rebuild.

## 10. Porting guide — written continuously

Target: `/home/retro/work/zed/portingguide.md` (1324 lines). Insert
`## Merge 003012 (2026-08-31)` **above** `## Merge 2026-07-29 (upstream
catch-up, 764 commits)` at line ~743, matching the existing reverse-chronological
layout. Amend and extend in place; never rewrite.

Write each subsection **as the work happens**, not at the end. The ordering below
matches the order the work will occur in:

1. Window summary — fence SHA, upstream HEAD SHA, 434 commits, 33-day window,
   ACP unchanged, rustc `1.95.0 → 1.97.1`, zed `1.15.0 → 1.19.0`
2. `### Conflicts and Resolutions` — all six files, hunk by hunk
3. `### git_ui → git_ui_core migration` — the full symbol map (highest value)
4. `### rustc 1.95 → 1.97` — bump, Docker-builder outcome, fork-only diagnostics
5. `### Crate churn` — five added, four removed
6. `### Retired / superseded Helix patches` — the constraint #8/#12 supersession
   (JSON defaults → cfg gates), so future briefs stop carrying stale constraints
7. `### Helix-surface survival check` — per-area confirmation
8. Commit-history table extended; stale Rebase-Checklist entries corrected
9. **A note that 002701 and 002930 were planned but never executed** — this is
   why the window is 434 commits and not ~100. Recording it is how the pattern
   gets escalated rather than repeated a fourth time

The brief's path `design/2026-02-07-zed-fork-rebase-to-upstream.md` exists only
in `/home/retro/work/helix/` and is the historical one-time-port narrative with
no `## Merge NNN` sections. It is not the target (Open Question 3, fifth ask).

## 11. Key decisions and rationale

| Decision | Alternative rejected | Why |
|---|---|---|
| True merge commit | Rebase | 351 fork-only commits would be rewritten; every open feature branch would need recreating |
| True merge commit | Squash | Destroys upstream bisectability, which is what makes the audit tractable |
| Merge into `main`, rebase `002731-agent-questions` after | Land the feature branch first | `external_websocket_sync` is fork-only and cannot conflict with upstream; the post-merge rebase is near-trivial. Landing an unmerged feature branch first adds risk to an already large window |
| Compile-driven `git_ui_core` migration | Blanket sed rename | Only some symbols moved; a blanket rename breaks `git_ui` panels and produces an unreviewable diff |
| Take upstream shape in every Cargo.toml, re-insert Helix lines | Merge line-by-line | Upstream reorders and prunes aggressively; taking its shape minimises the diff *next* window too |
| Union resolve for `http_client_tls/Cargo.toml` | Pick a side | Pure adjacency conflict, no semantic overlap — both sides' deps are needed |
| Re-derive audit priorities from fresh `--stat` | Reuse 002930's P1–P8 | Four files 002930 called grep-only have changed materially; reusing its ordering would skip real audit targets |
| `run_docker_e2e.sh` over raw docker commands | The brief's `docker build`/`docker run` | Same image, handles wiring; the brief's form omits the Go server prep |
| Gate on 17 phases | Gate on 18 per the brief | Phase 18 measurably does not exist on `main` (only on `feature/002731-agent-questions`) |

## 12. Learnings recorded for future merge windows

- **Measure, do not inherit.** Prior specs' delta tables go stale in a week.
  `git diff --stat <fence> upstream/main -- <hot files>` takes seconds and
  reclassified four files this window.
- **`git merge-tree --write-tree --name-only origin/main upstream/main`** gives
  the exact conflict list without touching the working tree — the right first
  measurement in any merge planning pass.
- **Check whether the last spec actually ran** before writing the next one. Cheap
  proofs: `rust-toolchain.toml` channel, existence of the crate the last window
  was supposed to add, newest `## Merge` heading in `portingguide.md`, and
  whether `git merge-base` moved.
- **A fork-only crate cannot conflict.** `external_websocket_sync` does not exist
  upstream, so every question about "will the sync crate survive the merge" is
  answered by construction. That fact drives the sequencing decision.
- **Auto-merged ≠ correct** is the recurring failure mode; the `extensions_ui.rs`
  −662 move is the canonical example of a Helix marker surviving textually but
  landing in dead code.
- **`ZED_COMMIT` in `sandbox-versions.txt` is chronically stale by one parent** —
  it tends to point at the PR branch tip rather than the merge commit. Bump it to
  the merge HEAD, and check the same pattern next window.
