# Requirements: Merge Latest Zed Upstream Into Helix Fork

## Context

Today is **2026-09-14**. This is the next cycle in the recurring
`zed-industries/zed` → Helix fork upstream-merge series.

**Read these first, in this order:**

- `helix-specs/design/tasks/003081_merge-latest-zed/` (2026-09-07) — the previous
  spec and the single most authoritative source. Its conflict analysis,
  `git_ui_core` symbol map, ACP session-lifecycle plan, auto-merge audit list and
  Open Questions are **all still valid**. This spec records the **delta** plus
  fresh measurement; do not re-derive what 003081 already documents.
- `003012_merge-latest-zed/` (2026-08-31), `002930_merge-latest-zed/` (2026-08-24)
  and `002701_merge-latest-zed/` (2026-08-10) — earlier windows of the same
  unexecuted merge. 002701's `design.md` holds the original
  `git_ui` → `git_ui_core` symbol map.
- `/home/retro/work/zed/portingguide.md` — the living porting guide (the real
  one; see Open Question 4).

### Critical finding — this is the FIFTH unexecuted window

**003081 was never executed**, exactly as 003012, 002930 and 002701 before it.
Measured today on `/home/retro/work/zed`:

| Evidence | Value | Means |
|---|---|---|
| merge-base with upstream | `b9256fa8f0` (2026-07-29) | **the fence has not moved in five specs** |
| `rust-toolchain.toml` channel | `1.95.0` | rustc bump never happened |
| `crates/git_ui_core` | does not exist | migration never happened |
| `crates/csv_preview`, `panel`, `rich_text`, `supermaven` | still present | crate churn never absorbed |
| `crates/zed/Cargo.toml` version | `1.15.0` | last upstream absorb was 2026-07-29 |
| newest `## Merge` in `portingguide.md` | `2026-07-29` (line 743) | no merge has landed since |

Backlog growth across five planning cycles: **128 → 331 → 434 → 540 → 616 commits.**

### What DID change since 003081 — the fork is alive again

Unlike the previous three windows, the fork side moved. **Six new fork commits**
landed between 2026-09-07 and 2026-09-13, and they matter:

| Commit | Date | What |
|---|---|---|
| `7c315e2021` | 2026-09-13 | Merge PR **#96** `feat/codex-elicitations` (fork HEAD) |
| `bed34e22ce` | 2026-09-13 | `feat(acp): normalize Codex elicitation answers` |
| `baff1b4a4a` | 2026-09-13 | Merge PR **#95** `feat/agent-elicitations` |
| `7469142ffc` | 2026-09-13 | `feat(acp): relay agent questions to Helix` |
| `a7a1a2dfb1` | 2026-09-12 | Merge PR **#93** `feat/subagent-activity-sync` |
| `16c6b99b8f` | 2026-09-12 | `feat(sync): expose Codex subagent activity` |

**ACP Elicitations have landed on `main`.** 003081 Open Question 1 (asked three
times across three specs) is now answered by events: the feature is merged, not
stranded on `feature/002731-agent-questions`. `thread_service.rs` carries **57
elicitation references** and handles `AcpThreadEvent::ElicitationRequested` /
`ElicitationResponded`. `sandbox-versions.txt` in `/home/retro/work/helix/` is
already at `ZED_COMMIT=7c315e2021…` — i.e. the fork side is current.

**But the elicitation work landed in the worst possible place for this merge:**
`crates/agent_servers/src/acp.rs`, which is the one non-mechanical conflict. See
"The acp.rs conflict got worse" below.

### Measured baseline (2026-09-14, from `/home/retro/work/zed`)

```
fence (merge-base)   b9256fa8f018bf03eb2e420120163746f8298d83  2026-07-29
                     "Stop the npm cache from growing without bound (#61750)"
upstream HEAD        7960b2a7c9568e90fbe0727332149e5b2a5fd57a  2026-09-12
                     "gpui: Add Canvas font fallback for web text (#64110)"
fork HEAD (main)     7c315e2021232258fa3781d0105610e2a2be290a  2026-09-13
commits to merge     616     (git rev-list --count main..upstream/main)
fork-only commits    357     (was 351 at 003081)
window               47 days
ACP                  2.0.0 -> 2.1.0   ** FIRST BUMP IN FIVE WINDOWS **
                     schema 1.5.0 -> 1.7.0, derive 2.0.0 -> 2.1.0
rust-toolchain       1.95.0 -> 1.98.1  (003081 measured 1.97.1)
zed crate version    1.15.0 -> 1.21.0  (003081 measured 1.20.0)
textual conflicts    8 files (git merge-tree --write-tree; tree bab80a8e4c)
new upstream crates  git_ui_core, gpui_apple, tabular_data_preview,
                     call_hierarchy, language_detection, lsp_command_selector
removed upstream     csv_preview, rich_text, supermaven, supermaven_api, panel
```

Re-measure before starting — upstream moves daily — but the shape will hold.

### The eight textual conflicts (measured with `git merge-tree`)

003081 measured seven. **`crates/recent_projects/src/dev_container_suggest.rs`
is new this window.**

| File | Nature |
|---|---|
| `crates/agent_servers/src/acp.rs` | **The headline work item, and materially harder than 003081 predicted.** See below. |
| `crates/recent_projects/src/dev_container_suggest.rs` | **NEW.** Upstream restructured `suggest_on_worktree_updated`: the `cli_auto_open` branch moved to an early `open_dev_container_from_cli(...)` call. Fork adds a `RemoteSettings::get_global(cx).suggest_dev_container` early return at the **top** of the same function. Resolution: keep the fork's guard as the first statement, take upstream's restructured body wholesale, drop the fork's now-dead references to `cli_auto_open`/`has_configs`. |
| `Cargo.lock` | Routine. Take upstream (`--theirs`), regenerate via the build. |
| `crates/http_client_tls/Cargo.toml` | Adjacency only: fork `rustls-pki-types = "1"` vs upstream `webpki-roots.workspace`/`log.workspace`. Union — keep both. |
| `crates/title_bar/Cargo.toml` | Fork `external_websocket_sync` optional dep + `git_ui.workspace` vs upstream `git_ui_core.workspace`. Keep the fork dep, drop `git_ui`, take `git_ui_core`. |
| `crates/agent_ui/Cargo.toml` | Fork `time`/`time_format`/`tokio` block vs upstream deleting `time`. Keep `time_format` + `tokio`; re-add `time` only if the compiler demands it. |
| `crates/zed/Cargo.toml` | Fork `tokio`/`ztracing`/`tracing` + `external_websocket_sync` block vs upstream removal. Keep the fork lines; take upstream's version `1.21.0` and dependency reshuffle. |
| `crates/zed/src/main.rs` | One hunk: Helix `build_application(args.headless).with_assets(Assets)` vs upstream `build_application().with_assets(Assets).with_restart_arguments(restart_arguments)`. Union both. |

### The `acp.rs` conflict got worse — two independent rewrites collided

003081 characterised this file as "upstream replaced ref counting with
`observe_release`, 3 hunks". That is still true, but **the fork has since added a
large new surface to the exact same file**. Upstream delta on `acp.rs` is now
**+174 −209**; the fork side adds roughly 400 lines of its own.

**Upstream side (unchanged from 003081's analysis):**
- `PendingAcpSession.ref_count` and `AcpSession.ref_count` **removed**;
  `AcpSession` gains `_release_subscription: Subscription`.
- New `AcpConnection::register_session(...)` installs
  `cx.observe_release(thread, …)` which removes the session and fires
  `CloseSessionRequest` when the last handle drops; gated on new
  `agent_supports_session_close()` (reads `agent_capabilities.session_capabilities.close`).
- `AcpConnection`'s `supports_close_session` / `close_session` overrides are
  **gone**; the trait defaults in `crates/acp_thread/src/connection.rs`
  (byte-unchanged upstream) now apply.
- ACP IO future moved from `cx.background_spawn` to
  `background_executor().spawn_dedicated(...)`. Auto-merges.

**Fork side — the part 003081 could not have seen (PRs #93/#95/#96):**
- `RawSessionNotification` (`#[derive(JsonRpcNotification)]`, method
  `session/update`) replacing upstream's typed `handle_session_notification`
  registration with `handle_raw_session_notification` — this is how Codex
  subagent activity and unknown update variants are relayed to Helix without
  being dropped by strict deserialization.
- `RawRequestPermissionRequest` / `RawRequestPermissionResponse`
  (`#[derive(JsonRpcRequest)]` / `JsonRpcResponse`, method
  `session/request_permission`) carrying the extra top-level `answers` field —
  the agent-questions relay wire format. There is a fork unit test,
  `qwen_permission_answers_serialize_at_the_response_top_level`.
- `feature_flags::{AcpBetaFeatureFlag, FeatureFlagAppExt}` import.
- A Codex-only `jetbrains` / `nativeSubagentSessions` capability injected into
  `client_capabilities_for_agent`'s `Meta`.
- `[ACP_SPAWN]` diagnostic logging replacing upstream's `log::debug!`/`log::trace!`.
- Pre-existing from 003081's window: `session_creation_chain`,
  `SessionCreationGuard`, `acquire_session_creation_slot` (PR #50);
  `force_close_session` (PR #63), still called from
  `crates/external_websocket_sync/src/thread_service.rs:3691`.

All of these live in the **conflicted region or adjacent to it**. Resolution
must take upstream's lifecycle wholesale while re-hosting every one of the fork
items on top of it.

### Answers to the brief's open questions (measured during planning)

| Brief question | Measured answer |
|---|---|
| How many new upstream commits? | **616** (`main..upstream/main`). The brief's `helix-fork..origin/main` form does not apply — see Repository Layout. |
| What commit is the fork at; did 003081 complete? | Fork `main` = `7c315e2021` (2026-09-13). **003081 did not run** — the fence is still `b9256fa8f0`. The fork commits since 003081 are feature work (elicitations, subagent activity), not the merge. |
| Has the ACP crate version bumped? | **YES — `2.0.0` → `2.1.0`, the first bump in five windows** (schema `1.5.0` → `1.7.0`). Take upstream's pin. Risk is lower than it looks: fork-only ACP construction sites in `external_websocket_sync` already use the builder form (`ElicitationFormMode::new(...)`, `CreateElicitationResponse::new(...)`, `ElicitationAcceptAction::new().content(...)`). A struct-literal sweep is still required — see Acceptance Criteria. |
| Have the ACP Elicitations PRs landed on the fork? | **YES.** Zed PRs #95/#96 merged 2026-09-13; `thread_service.rs` has 57 elicitation references. The brief's claim that Zed PR #89 / Helix PR #3051 were "closed" is superseded — the work landed under different PR numbers. |
| Do elicitation event types conflict with upstream additions? | **No.** `AgentThreadEntry::Elicitation`, `Elicitation`, `ElicitationEntryId`, `ElicitationStatus` (`Pending{respond_tx}`/`Accepted`/`Declined`/`Canceled`/`Completed`) and `ElicitationStoreEvent` are byte-identical on both sides; `AcpThread::elicitation()` / `elicitation_mut()` unchanged. `ElicitationUrlMode`, `ElicitationSessionScope`, `complete_url_elicitation`, `request_elicitation_with_id` already exist on both sides. |
| Have high-conflict files been restructured upstream? | Yes — see the audit table. **`crates/acp_thread/src/acp_thread.rs` jumped from ±16 to +1263 −17** and is now a real audit item, not a confirming grep. `zed.rs` +972, `agent_panel.rs` +679, `agent.rs` +585. |
| What does the porting guide record from the last completed merge? | The newest section is `## Merge 2026-07-29 (upstream catch-up, 764 commits)` at line 743. Key learnings carried forward: resolve `Cargo.lock` `--theirs` and regenerate; audit auto-merged Helix hosts rather than trusting the merge; repair signature drift in a separate follow-up commit; run the merge in rounds when the window is large. |
| Do the in-flight Helix items conflict? | **No.** `Fix False "Agent Did Not Respond" Stamps`, `API-Queued Agent Messages in Prompt Queue` (Helix PR #3153) and `Shifted Characters From Mobile Keyboards` are all Helix-repo Go/TS work touching `api/pkg/server/websocket_external_agent_sync.go`. That file does not exist in the Zed repo and cannot conflict with upstream Zed. |
| New lessons in helix-specs since 003081? | **None.** 003081 is still the newest merge spec and nothing has been appended to it. |

### Upstream delta on the audited files (`git diff --numstat b9256fa8f0 upstream/main`)

| File | 003081 measured | **003182 measured** | Why it matters |
|---|---|---|---|
| `crates/acp_thread/src/acp_thread.rs` | ±16 | **+1263 −17** | **Escalated from grep to audit.** Mostly new idle-sleep-prevention tests + elicitation test helpers, but it hosts the elicitation types the fork now depends on |
| `crates/zed/src/zed.rs` | +1096 | **+972 −136** | `initialize_agent_panel` + WebSocket init host |
| `crates/agent_ui/src/agent_panel.rs` | +585 | **+679 −43** | Fix #11, `from_existing_thread`, `ThreadDisplayNotification` |
| `crates/agent/src/agent.rs` | +732 | **+585 −151** | Fix #1 host (`pending_sessions`, `wait_for_tools_ready`) |
| `crates/anthropic/src/anthropic.rs` | +561 | **+495 −73** | new models — take upstream ordering wholesale |
| `crates/extensions_ui/src/extensions_ui.rs` | −662 | **+68 −608** | 3× `// HELIX: External agent` markers, bulk moved to `components/extension_card.rs` |
| `crates/agent_servers/src/acp.rs` | ±336, conflicting | **+174 −209, conflicting** | session lifecycle rewrite vs. fork's raw-notification relay |
| `crates/agent_ui/src/conversation_view.rs` | ±134 | **+149 −42** | Fix #2 host (no duplicate WebSocket sends) |
| `crates/agent_ui/src/conversation_view/thread_view.rs` | ±88 | **+97 −63** | Helix `current_model_id()` fallback |
| `crates/title_bar/src/title_bar.rs` | +110 | **+85 −25** | sign-in suppression + `render_restricted_mode` |
| `crates/reqwest_client/src/reqwest_client.rs` | ±73 | **+66 −7** | `ZED_HTTP_INSECURE_TLS` host |
| `assets/settings/default.json` | not listed | **+150 −23** | auto-merges; confirm no Helix key lost |
| `crates/feature_flags/src/flags.rs` | −27 | **+0 −27** | `AcpBetaFeatureFlag` survives upstream's pruning |
| `crates/language_models/src/provider/open_ai.rs` | ±23 | **+18 −5** | light confirming read |
| `crates/http_client_tls/src/http_client_tls.rs` | ±11 | **+10 −1** | insecure-TLS host |
| `crates/agent/src/tools/grep_tool.rs` | ±12 | **+7 −5** | `truncate_long_lines()` / `MAX_LINE_CHARS` |
| `crates/acp_thread/src/connection.rs` | unchanged | **byte-unchanged** | confirming grep (fork adds `force_close_session` here) |
| `crates/project/src/trusted_worktrees.rs` | unchanged | **byte-unchanged** | confirming grep |
| `crates/external_websocket_sync/**` | fork-only | **fork-only** | does not exist upstream; cannot conflict |

## Repository Layout (sandbox reality — measured)

Unchanged from 003081. Repeated here because the brief still describes a
different machine.

- **Working repo**: `/home/retro/work/zed/`, branch **`main`** (not `helix-fork`).
  - `origin` = `http://helix-api.internal:18080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
    (in-cluster gitea mirror of `helixml/zed`). `origin/main` == local `main`.
  - `origin/helix-fork` exists but is **dead** (tip `746a9c4fb4`, 2026-02-07).
    Do not use it.
  - `upstream` = `https://github.com/zed-industries/zed.git` — was **not**
    configured; added and fetched during this planning pass, confirmed reachable
    and read-only.
- **Living porting guide**: `/home/retro/work/zed/portingguide.md` (1324 lines).
  The brief's path `design/2026-02-07-zed-fork-rebase-to-upstream.md` exists only
  in `/home/retro/work/helix/` (638 lines) as the historical one-time-port
  narrative from 2026-02-07/08, with no `## Merge NNN` sections. See Open
  Question 4.
- **Helix platform repo**: `/home/retro/work/helix/` —
  `sandbox-versions.txt` has `ZED_COMMIT=7c315e2021…`, i.e. **already current**
  with fork `main` (it was stale in 003081). It still needs bumping to the
  post-merge SHA.
- **Toolchain**: **no local `cargo`/`rustc`** — only `docker` and `go`. All Rust
  work goes through `cd /home/retro/work/helix && ./stack build-zed dev` or the
  e2e Docker images.
- **`ANTHROPIC_API_KEY` is NOT set** in the planning environment.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the 616 upstream commits, reconcile
> the fork's brand-new raw-notification/agent-questions layer with upstream's new
> `observe_release` session lifecycle, and adopt ACP 2.1.0, `git_ui_core` and
> rustc 1.98.1 — so the fork stops accumulating debt that five consecutive specs
> failed to clear.

### 2. Helix User
> As a Helix user, I want upstream's newest fixes without losing WebSocket sync,
> incremental streaming, headless mode, ACP routing, agent questions/elicitations,
> Codex subagent activity, or any of the 11 Critical Fixes.

### 3. Elicitations Feature Owner
> As the owner of the just-landed elicitation relay (PRs #93/#95/#96), I want my
> `RawSessionNotification` / `RawRequestPermissionResponse` types, the Codex
> `jetbrains` capability meta and the `answers`-at-top-level wire format to
> survive the merge intact and still round-trip against ACP 2.1.0.

### 4. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` updated **as
> each conflict is resolved**, with a dated `## Merge 003182 (2026-09-14)` entry
> covering the ACP 2.1.0 bump, the session-lifecycle adaptation, the
> raw-notification re-hosting, the `git_ui_core` migration and the rustc bump.

## Acceptance Criteria

### Merge completeness
- [ ] `upstream` remote present, fetched, treated as read-only
- [ ] Fence / commit count / upstream HEAD re-measured before starting and
      recorded in `portingguide.md`
- [ ] `git merge upstream/main` — **merge, not squash, not rebase**
- [ ] All 357 fork-only commits preserved; any skipped upstream commit justified
      in the porting guide
- [ ] `git log` confirms the fork branch contains and is ahead of `upstream/main`
- [ ] No `<<<<<<<` / `>>>>>>>` markers anywhere in the tree

### Conflict resolutions
- [ ] **`crates/agent_servers/src/acp.rs`** — take upstream's `register_session`
      / `observe_release` / `agent_supports_session_close()` lifecycle wholesale;
      **delete** the fork's `ref_count` bookkeeping (the fields are gone); then
      re-host **every** fork item on top of it:
  - [ ] `RawSessionNotification` + `handle_raw_session_notification` registration
        kept in place of upstream's typed `handle_session_notification`
  - [ ] `RawRequestPermissionRequest` / `RawRequestPermissionResponse` with the
        top-level `answers` field kept; the
        `qwen_permission_answers_serialize_at_the_response_top_level` test passes
  - [ ] Codex `jetbrains` / `nativeSubagentSessions` meta kept in
        `client_capabilities_for_agent`
  - [ ] `feature_flags::{AcpBetaFeatureFlag, FeatureFlagAppExt}` import retained
        and still used
  - [ ] `session_creation_chain` / `SessionCreationGuard` /
        `acquire_session_creation_slot` (PR #50) intact
  - [ ] `force_close_session` (PR #63) **kept**, re-expressed against the new
        model (remove the `sessions`/`pending_sessions` entry, then send
        `CloseSessionRequest`) — `thread_service.rs:3691` calls it
  - [ ] `[ACP_SPAWN]` diagnostic logging retained (fork deliberately replaced
        upstream's `log::debug!`/`log::trace!`)
  - [ ] Union-resolve the test region so both
        `test_concurrent_session_creation_is_serialized` and upstream's
        `release_dropped_entities` helper survive
- [ ] **`crates/recent_projects/src/dev_container_suggest.rs`** — fork's
      `if !RemoteSettings::get_global(cx).suggest_dev_container { return; }` stays
      as the **first statement** of `suggest_on_worktree_updated`; upstream's
      restructured body (early `open_dev_container_from_cli(...)`, simplified
      `find_configs_in_snapshot(worktree).is_empty()` check) taken wholesale;
      `use settings::Settings;` and `use crate::RemoteSettings;` imports kept
- [ ] `crates/acp_thread/src/connection.rs` — fork's `force_close_session` trait
      method retained (upstream left this file byte-unchanged)
- [ ] `crates/http_client_tls/Cargo.toml` — union: keep `rustls-pki-types = "1"`
      **and** upstream's `webpki-roots.workspace` / `log.workspace`
- [ ] `crates/title_bar/Cargo.toml` — keep
      `external_websocket_sync = { workspace = true, optional = true }` and its
      `[features]` entry; drop `git_ui`, take `git_ui_core`
- [ ] `crates/agent_ui/Cargo.toml` — keep `time_format`, `tokio` and the
      `external_websocket_sync` feature/dep; take upstream's removal of `time`,
      re-adding only if the compiler demands it
- [ ] `crates/zed/Cargo.toml` — keep `tokio`, `ztracing`, `tracing`,
      `external_websocket_sync` optional dep and Helix `[features]`; take
      upstream's version `1.21.0` and dependency reshuffle
- [ ] `crates/zed/src/main.rs` — resolve to
      `build_application(args.headless).with_assets(Assets).with_restart_arguments(restart_arguments)`;
      other `build_application(false)` call sites keep their argument;
      `args.allow_multiple_instances || args.headless` survives
- [ ] `Cargo.lock` — resolve `--theirs`, then regenerate by building

### ACP `2.0.0 → 2.1.0` (first bump in five windows)
- [ ] `Cargo.toml` takes upstream's
      `agent-client-protocol = { version = "=2.1.0", features = ["unstable"] }`
- [ ] `Cargo.lock` resolves `agent-client-protocol 2.1.0`, `-derive 2.1.0`,
      `-schema 1.7.0`
- [ ] Struct-literal sweep across **fork-only** ACP construction sites — ACP uses
      `#[non_exhaustive]` structs, so any remaining `acp::Foo { .. }` literal is a
      hard error. Known sites (all already builder-form; re-verify, do not assume):
      `crates/external_websocket_sync/src/thread_service.rs` — `CreateElicitationRequest`,
      `ElicitationFormMode`, `ElicitationSchema`, `ElicitationContentValue`,
      `ElicitationAcceptAction`, `CreateElicitationResponse`, `ElicitationAction`,
      `PlanEntryStatus`, `Meta`, `SessionId`, `ToolCallId`, `RequestId`,
      `PermissionOptionKind`
- [ ] `crates/agent_servers/src/acp.rs` fork types still derive cleanly against
      ACP 2.1.0's `JsonRpcNotification` / `JsonRpcRequest` / `JsonRpcResponse`
      macros (upstream dropped these imports; **verify the macros are still
      exported by 2.1.0** — if they were removed, the raw-relay layer must be
      re-expressed and this becomes a blocking finding, recorded in the guide)
- [ ] `ErrorCode` variants used by the fork still exist in 2.1.0
- [ ] `grep -rnE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` returns 0

### `git_ui` → `git_ui_core` migration
003081's symbol map is confirmed still accurate against upstream HEAD:

- **Moved to `git_ui_core`**: `worktree_service`, `created_worktrees`,
  `worktree_picker`, `worktree_names`, `notifications`, `askpass_modal`,
  `file_diff_view`, plus free functions `build_branch_picker`,
  `set_branch_picker_builder`, `open_file_history`, `set_file_history_opener`
- **Stayed in `git_ui`**: `init`, `git_panel`, `git_picker`, `branch_picker`,
  `git_graph`, `project_diff`, `solo_diff_view`, `staged_diff`, `unstaged_diff`,
  `branch_diff`, `commit_view`, `clone`, `multi_diff_view`, `picker_prompt`,
  `repository_selector`, `stash_picker`, `text_diff_view`, `git_status_icon`

- [ ] All **13** fork files referencing `git_ui::` re-pointed **symbol by symbol**
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

### rustc `1.95.0 → 1.98.1`
- [ ] `rust-toolchain.toml` takes upstream's `channel = "1.98.1"`
      (003081 planned for 1.97.1 — upstream has moved on)
- [ ] `/home/retro/work/helix/Dockerfile.zed-build` resolves and installs 1.98.1
      from `rust-toolchain.toml` with no manual pin edit — it runs
      `rustup … --default-toolchain none` so this **should** be automatic.
      **Verify, do not assume**; the BuildKit `/root/.rustup` cache mount must
      fetch a fresh toolchain
- [ ] New-compiler diagnostics in fork-only code fixed, following upstream's own
      remedy where one exists
- [ ] Cold-cache full rebuild budgeted for (two toolchain versions skipped)

### Crate churn
- [ ] Added and building: `git_ui_core`, `gpui_apple`, `tabular_data_preview`,
      `call_hierarchy`, `language_detection`, **`lsp_command_selector`** (new
      this window)
- [ ] Removed and gone: `csv_preview`, `rich_text`, `supermaven`,
      `supermaven_api`, `panel` — note `crates/git_ui/Cargo.toml:47` on the fork
      still has `panel.workspace = true` and upstream's manifest does not; take
      upstream's, then grep for residual `panel::` use
- [ ] `crates/zed/Cargo.toml` `csv_preview.workspace = true` dropped with the crate
- [ ] Workspace `[workspace] members` keeps every Helix member
      (`external_websocket_sync`, `sidebar`, …) while absorbing upstream's
      additions, removals and alphabetical reordering

### Auto-merge audit (auto-merged ≠ correct)
Priorities re-ordered by this window's measured deltas.

- [ ] **P1 `crates/zed/src/zed.rs` (+972 −136)** — `initialize_agent_panel` still
      exists upstream; verify it and the WebSocket init inside it survive, plus
      new action registrations / `initialize_panels`
- [ ] **P2 `crates/agent_ui/src/agent_panel.rs` (+679 −43)** — `send_agent_ready`,
      `wait_for_websocket_connected`, UI-state-query callback, `acp_history_store()`,
      `from_existing_thread`, `ThreadDisplayNotification` handler, Fix #11
- [ ] **P3 `crates/agent/src/agent.rs` (+585 −151)** — Fix #1: `NativeAgent`
      clone / `pending_sessions` shared-task in `load_session()` (fork uses
      `get_mut`, upstream `get`) and `wait_for_tools_ready` intact
- [ ] **P4 `crates/acp_thread/src/acp_thread.rs` (+1263 −17)** — **newly
      escalated.** Confirm the elicitation types are byte-identical post-merge
      (they were pre-merge); confirm the new `update_idle_sleep_prevention`
      wiring on `ElicitationRequested`/`ElicitationResponded` does not double-fire
      against the fork's own subscribers; confirm `AcpThreadEvent` variant set is
      a superset of what `thread_service.rs` matches on
- [ ] **P5 `crates/anthropic/src/anthropic.rs` (+495 −73)** — take upstream
      ordering wholesale
- [ ] **P6 `crates/extensions_ui/src/extensions_ui.rs` (+68 −608)** — the 3×
      `// HELIX: External agent` markers must be present **and in a live code
      path**; re-apply in `components/extension_card.rs` if upstream moved the
      host, and record the move
- [ ] **P7 `crates/agent_ui/src/conversation_view.rs` (+149 −42)** — Fix #2, no
      duplicate WebSocket sends; it calls `supports_close_session()` /
      `close_session()` and must still compile against upstream's trait defaults
      now that `AcpConnection` no longer overrides them
- [ ] **P8 `crates/agent_ui/src/conversation_view/thread_view.rs` (+97 −63)** —
      Helix `current_model_id()` fallback
- [ ] **P9 `crates/title_bar/src/title_bar.rs` (+85 −25)** — `render_restricted_mode()`
      returns `None` under the feature gate; `&& !cfg!(feature = "external_websocket_sync")`
      sign-in suppression survives
- [ ] **P10 `crates/reqwest_client/src/reqwest_client.rs` (+66 −7)** —
      `ZED_HTTP_INSECURE_TLS` intact
- [ ] **P11 `assets/settings/default.json` (+150 −23)** — auto-merges; confirm no
      Helix-set key was silently dropped, and confirm upstream's new keys parse
- [ ] **P12 `crates/feature_flags/src/flags.rs` (+0 −27)** — `AcpBetaFeatureFlag`
      survives and keeps `enabled_for_all() -> true` (currently `flags.rs:30-32`)
- [ ] **P13 `crates/zed/src/main.rs`** — `--headless`, `--allow-multiple-instances`,
      `initialize_headless()` intact around upstream's `restart_arguments` flow
- [ ] **P14 `crates/agent/src/tools/grep_tool.rs` (+7 −5)** — `truncate_long_lines()`
      / `MAX_LINE_CHARS = 500`
- [ ] **P15 `crates/language_models/src/provider/open_ai.rs` (+18 −5)** — light read
- [ ] **P16 `crates/http_client_tls/src/http_client_tls.rs` (+10 −1)** — fork's
      `rustls-pki-types` usage compiles alongside upstream's `webpki-roots`
- [ ] Confirming grep only (verified byte-unchanged upstream):
      `crates/acp_thread/src/connection.rs`,
      `crates/project/src/trusted_worktrees.rs`,
      `crates/external_websocket_sync/**`

### Critical Fix preservation (`portingguide.md` §"Critical Fixes")
All 11 must survive. Three fix hosts changed substantially, so this is a real audit.
- [ ] Fix #1 `pending_sessions` shared-task
- [ ] Fix #2 no duplicate WebSocket sends
- [ ] Fix #3 `content_only()` strips `## Assistant` heading
- [ ] Fix #4 `notify_thread_display()` for follow-ups to non-visible threads
- [ ] Fix #5 stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6 every `send()` emits exactly one `Stopped` (`stopped_emitted_for_task`)
- [ ] Fix #7 `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8 `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9 `stopped_emitted_for_task` guard on the normal-completion path
- [ ] Fix #11 entity-identity guard in `agent_panel.rs` `load_agent_thread`
- [ ] PR #63 `force_close_session` still reachable from `thread_service.rs`
- [ ] (Fix #10 remains RETIRED — do not resurrect)

### Helix-specific surface (brief constraints 1–13, re-verified today)
- [ ] `crates/external_websocket_sync/` intact (10 source files, 77 test
      attributes, 57 elicitation references in `thread_service.rs`)
- [ ] Compiles **with and without** the `external_websocket_sync` feature
- [ ] `ThreadDisplayNotification` handler still calls
      `OnboardingUpsell::set_dismissed(true, cx)`
      (`agent_panel.rs:1644`; also `:496` and `:6548`)
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true`
- [ ] Built-in agent hiding stays under `cfg(not(feature = "external_websocket_sync"))`
      (2 sites measured today)
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved so
      `message_added` streams incrementally (`subscribe_in` is silently dropped
      without a window context)
- [ ] Elicitation relay intact: `AcpThreadEvent::ElicitationRequested` →
      `register_elicitation_question`, `ElicitationResponded` →
      `resolve_elicitation_question`, `elicitation_questions()` /
      `elicitation_response_content()` / `qwen_questions_from_meta()`
- [ ] Codex subagent-activity relay (PR #93) intact
- [ ] **Constraints #8 and #12 remain SUPERSEDED.** Measured today:
      `assets/settings/default.json` has `"show_sign_in": true` (line 568) and
      `"trust_all_worktrees": false` (line 2522) — **byte-identical to upstream**.
      These are enforced by cfg gates in `crates/project/src/trusted_worktrees.rs`
      and `crates/title_bar/src/title_bar.rs`, **not** by the JSON. Audit the
      gates; do **not** "restore" the JSON keys
- [ ] **Constraint #10 remains SUPERSEDED.** `NativeAgentSessionList` does not
      exist anywhere in the tree. Do not try to preserve it
- [ ] `title_bar`'s `external_websocket_sync` dep stays `optional = true`;
      workspace `rust-embed` keeps `debug-embed`; `wait_for_tools_ready` uses
      `cx.background_executor().timer()`
- [ ] Migration banner `Hidden`; trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` matches stay exhaustive; Fix 1b
      cfg-gated draft-suppression `return;` is still the FIRST statement of its
      `BaseView::Uninitialized` branch

### Build & test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds, zero errors
- [ ] Feature-off `cargo check -p zed` via a one-off Docker run in the build image
- [ ] `cargo test -p external_websocket_sync` — full pass (includes the
      elicitation unit tests at `thread_service.rs:4250+`)
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] `cargo test -p agent_servers qwen_permission_answers_serialize_at_the_response_top_level`
      (agent-questions wire format, new this window)
- [ ] `cargo test -p agent_servers` session-lifecycle tests pass against upstream's
      new release-observer model
- [ ] **E2E Docker test — HARD GATE.** Run
      `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` (preferred over
      the brief's raw `docker build`/`docker run`; same image, handles credential
      and host wiring from `../helix/.env`). Run `go mod tidy` in
      `e2e-test/helix-ws-test-server/` first. Never use `--no-build` while
      investigating a failure
- [ ] All **17** existing phases green for `zed-agent`, and for `claude` via
      `E2E_AGENTS="zed-agent,claude"`. **Phase 18 does not exist** — measured:
      `main.go` documents phases 1–15 plus queue phases 16–17, and the e2e
      directory contains **zero** occurrences of "elicitation". See Open Question 1
- [ ] One retry permitted per agent for the known Claude Phase-1 npm-install
      flake; a second failure is a real failure
- [ ] helixml/zed CI green — drone steps `build-zed`, `zed-e2e-image`,
      `zed-e2e-headless-smoke`, `zed-e2e-headless-plan`,
      `zed-e2e-protocol-lifecycle`, `zed-e2e-live-claude-latest` in
      `/home/retro/work/helix/.drone.yml`

### Documentation (hard gate — written incrementally)
- [ ] `/home/retro/work/zed/portingguide.md` updated **as each conflict is
      resolved**, not at the end
- [ ] New `## Merge 003182 (2026-09-14)` section inserted **above**
      `## Merge 2026-07-29 …` (line ~743)
- [ ] Window summary: 616 commits, 47-day window, fence `b9256fa8f0` and upstream
      HEAD SHAs, ACP `2.0.0 → 2.1.0`, rustc `1.95.0 → 1.98.1`, zed `1.15.0 → 1.21.0`
- [ ] `### ACP 2.0.0 → 2.1.0` — what broke, what did not, the struct-literal sweep
      result, and whether the `JsonRpc*` derive macros survived
- [ ] `### ACP session lifecycle: ref counting → observe_release` — the highest
      value section this window; record how `force_close_session` was re-expressed
- [ ] `### Re-hosting the agent-questions relay` — how
      `RawSessionNotification` / `RawRequestPermissionResponse` / the Codex
      `jetbrains` meta were carried onto upstream's new handler set
- [ ] `### Conflicts and Resolutions` — all eight files, hunk by hunk
- [ ] `### git_ui → git_ui_core migration` — the full moved/stayed symbol map
- [ ] `### rustc 1.95 → 1.98` — bump, Docker-builder outcome, fork diagnostics
- [ ] `### Crate churn` — six added (incl. `lsp_command_selector`), five removed
- [ ] `### Retired / superseded Helix patches` — constraints #8, #10 and #12
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Commit-history table extended; stale Rebase-Checklist entries corrected
- [ ] A note that **002701, 002930, 003012 and 003081 were all planned and never
      executed** — why this window is 616 commits and not ~100

### Process
- [ ] Feature branch `feature/003182-merge-latest-zed` cut from fork `main`
- [ ] Branch pushed to `origin` (mirrors `helixml/zed`); `main` not force-pushed;
      `origin/helix-fork` left untouched; no agent-initiated PRs
- [ ] `sandbox-versions.txt` `ZED_COMMIT` bumped from `7c315e2021…` to the merge
      HEAD, on a `feature/003182-merge-latest-zed` branch in `/home/retro/work/helix/`
- [ ] `pull_request_zed.md` + `pull_request_helix.md` written into this task dir
- [ ] Re-fetch `upstream/main` and `origin/main` before declaring done; run an
      extension round if upstream advanced materially mid-work

## Out of Scope

- Net-new Helix feature development
- Modifying e2e assertions unless an upstream API change strictly requires it
- **Authoring e2e Phase 18** — see Open Question 1; it is a feature-test addition,
  not merge work, and doing it inside a 616-commit merge conflates two risks
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors beyond what the merge, ACP 2.1.0, the `git_ui_core` split and the
  session-lifecycle change force
- Adopting upstream's new UX (native elicitation UI, message-queue steering,
  sandboxing) into Helix-mode flows beyond keeping them compiling
- Rewriting the porting guide from scratch — amend and extend in place

## Open Questions

1. **Phase 18 is a mandatory gate in the brief, but it does not exist — and now
   the feature it would test *has* landed.** This is the fourth ask, and the
   situation has changed: elicitations merged to fork `main` on 2026-09-13 (PRs
   #95/#96), but **no e2e phase covers them**. Measured: `main.go` documents
   phases 1–15 plus queue phases 16–17; `grep -ri elicitation e2e-test/` returns
   nothing. So the gate cannot be satisfied as written.
   **Assumption: gate this merge on the 17 phases that exist, plus
   `cargo test -p external_websocket_sync` (which does cover the elicitation
   relay at `thread_service.rs:4250+`), and raise Phase 18 as a separate
   follow-up spec.** If you want Phase 18 written as part of this task, say so —
   it is maybe a day of Go work in `helix-ws-test-server/main.go` and should be
   costed on top of the merge, not inside it.

2. **Five consecutive merge specs have gone unexecuted (escalation, not a
   technical question).** 002701, 002930, 003012 and 003081 all planned this same
   merge; none ran. The backlog has grown 128 → 331 → 434 → 540 → **616**. The
   fork *is* receiving feature work (6 commits last week), so the blocker is not
   capacity in general — it is specific to the merge. If there is a known
   blocker (build-image breakage, missing E2E credentials, risk appetite),
   naming it is worth more than a sixth spec.
   **Assumption: no known blocker; this window executes.**

3. **Does ACP 2.1.0 still export the `JsonRpcNotification` / `JsonRpcRequest`
   derive macros?** The fork's brand-new agent-questions relay
   (`RawSessionNotification`, `RawRequestPermissionRequest/Response`) depends on
   them, and upstream's post-merge `acp.rs` no longer imports them — so nothing
   upstream proves they survived the 2.0 → 2.1 bump.
   **Assumption: they survived and the derives compile unchanged.** If not, this
   becomes the largest single work item in the merge and should be flagged
   immediately rather than worked around.

4. **`force_close_session` under upstream's new lifecycle** (carried from 003081,
   unanswered). Upstream deleted `AcpConnection`'s ref-counted close in favour of
   `observe_release`. The fork's PR #63 `force_close_session` is still called from
   `thread_service.rs:3691`.
   **Assumption: keep it, re-expressed as "drop the bookkeeping entry, then send
   `CloseSessionRequest`", and drop the fork's ref-count arithmetic.** If the
   release-observer makes it entirely redundant, say so — deleting it and its
   trait declaration would be cleaner but changes Helix thread-teardown timing.

5. **Porting-guide path (seventh ask — 002265, 002353, 002701, 002930, 003012,
   003081).** The brief names `design/2026-02-07-zed-fork-rebase-to-upstream.md`.
   That file exists only in the **helix** repo (638 lines) as a historical
   2026-02-07/08 narrative with no `## Merge NNN` sections; the living guide with
   every prior merge record is `/home/retro/work/zed/portingguide.md` (1324 lines).
   **Assumption: `zed/portingguide.md` is the target.** Please confirm, and
   ideally correct the brief template so the eighth spec does not ask again.

6. **Repo/branch layout (sixth ask).** The brief says
   `/prod/home/luke/pm/zed-upstream` on branch `helix-fork` with remotes
   `helix`/`origin`. Sandbox reality is `/home/retro/work/zed` on `main`;
   `origin/helix-fork` is dead at 2026-02-07.
   **Assumption: work on `main` in the sandbox layout.**

7. **`--locked` in CI (brief constraint #5) — stale, sixth ask.** Measured again
   today: there is no `.drone.yml` in the zed repo, and Helix's
   `Dockerfile.zed-build:108` runs
   `cargo build --features external_websocket_sync` with **no** `--locked`.
   **Assumption: the constraint is obsolete; verify CI green and do not add
   `--locked` without instruction.** A yes/no would let us delete it permanently.

8. **rustc 1.98.1 in the build image.** `Dockerfile.zed-build` installs rustup
   with `--default-toolchain none` and lets `rust-toolchain.toml` drive the
   version, so 1.98.1 *should* resolve automatically — but the BuildKit
   `/root/.rustup` cache mount must fetch a fresh toolchain and the Ubuntu 25
   base image must support it. This is a two-version jump (1.95 → 1.98).
   **Assumption: no Dockerfile change needed.** Flag immediately if not; do not
   pin back to 1.95.

9. **`crates/panel` removal (carried, unanswered).** Upstream deleted it;
   `crates/git_ui/Cargo.toml:47` on the fork still has `panel.workspace = true`.
   **Assumption: take upstream's manifest, drop the crate, fix residual `panel::`
   use.** If a Helix crate depends on it, flag rather than resurrecting.

10. **E2E credentials — measured absent.** `ANTHROPIC_API_KEY` is not set in the
    planning sandbox. `run_docker_e2e.sh` sources `../helix/.env` and
    `.env.usercreds`, so the implementing agent may still be fine.
    **Assumption: the implementation environment provides it.** If not, flag
    immediately — the merge cannot be declared complete.

11. **Feature-off compilation gate (carried).** With no local cargo, the cleanest
    way to `cargo check -p zed` *without* `external_websocket_sync` is a one-off
    Docker run in the build image. Is there a `./stack` target for a feature-off
    build? **Assumption: none; use a one-off Docker invocation.**
