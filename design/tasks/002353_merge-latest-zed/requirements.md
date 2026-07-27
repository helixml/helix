# Requirements: Merge Latest Zed Upstream Into Helix Fork

## Context

Today is **2026-07-27**. This is the next cycle in the recurring
`zed-industries/zed` → Helix fork upstream-merge series.

**The baseline was measured at planning time** (the working clone is populated,
unlike the 002265 planning round). The numbers below are real, not estimates —
but re-measure before starting, because upstream moves daily.

```
fence (merge-base)   e45e42af6e  2026-06-18  "agent_ui: Use the thread title for agent notifications (#59377)"
upstream HEAD        50da8c40dc  2026-07-26  "Avoid unnecessary git scan when the agent creates a checkpoint (#61271)"
fork HEAD (main)     13e8c2aaa8              "Merge pull request #71 from helixml/fix/zed-e2e-model-readiness"
commits to merge     709          (git log --oneline main..upstream/main | wc -l)
fork-only commits    294 (213 non-merge)
window               ~38 days
ACP crate            0.14.0  →  2.0.0     (+ new transitive agent-client-protocol-schema 1.5.0)
merge diffstat       1263 files, +155150 / -40934
textual conflicts    9 files, 10 hunks (enumerated in design.md)
```

### The important finding: 002224 / 002251 / 002265 never landed

The last upstream merge actually on fork `main` is **002100-extension
(2026-06-18)** — that is the fence. The three specs planned since
(`002224`, `002251`, `002265`) were written but their merges were never merged
into fork `main`. Only `origin/feature/002224-merge-latest-zed` exists as a
branch; there is no 002251/002265 branch at all.

**Consequences:**
- This window is the largest in the series (709 commits vs 002251's 409).
- It carries the **full ACP `0.14.0 → 2.0.0`** bump — i.e. *both* the
  `0.14 → 1.x` bump that 002251 planned for and the further `1.x → 2.0` bump.
- The 002224/002251/002265 specs are **still-unspent playbooks**, not history.
  Read them; their ACP-bump guidance applies to this merge.
- `portingguide.md`'s newest entry is `## Merge 002100-extension (2026-06-18)`,
  consistent with the fence. The guide is not stale — it is accurate.

Precedent specs to read first (in this order):
- `helix-specs/design/tasks/002265_merge-latest-zed/{requirements,design,tasks}.md`
  — most recent, most refined playbook.
- `helix-specs/design/tasks/002251_merge-latest-zed/…` — the ACP-bump-focused one.
- `helix-specs/design/tasks/002224_merge-latest-zed/…` — plus its
  `pull_request_zed.md` / `pull_request_helix.md` templates.

## Repository Layout (sandbox reality — measured)

- **Working repo**: `/home/retro/work/zed/`, branch **`main`** (not `helix-fork`).
  - `origin` = `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
    (in-cluster gitea mirror of `helixml/zed`).
  - `upstream` = `https://github.com/zed-industries/zed.git` — **was not
    configured**; added during planning. Verify it is present, read-only.
- **Living porting guide**: `/home/retro/work/zed/portingguide.md` (1109 lines).
  This is the file to update continuously. The task brief's path
  `design/2026-02-07-zed-fork-rebase-to-upstream.md` lives in the **helix** repo
  and is the historical one-time-port narrative with no per-merge sections —
  see Open Questions.
- **Helix platform repo**: `/home/retro/work/helix/` —
  `sandbox-versions.txt` currently `ZED_COMMIT=12eed911a7b2a98db4ef1e404e11ae95c1626787`.
- **Toolchain**: there is **no local `cargo`/`rustc`**. `docker` and `go` are
  present. All Rust building/testing must go through
  `cd /home/retro/work/helix && ./stack build-zed dev` (Docker) or the E2E
  Docker images.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb 38 days and 709 commits of upstream
> work — including the ACP 2.0 protocol bump — while preserving every Helix
> customisation, so the fork stops accumulating merge debt and the next window
> is small again.

### 2. Helix User
> As a Helix user, I want upstream's newest fixes, elicitation/message-queue
> work and performance improvements, without losing WebSocket sync, incremental
> streaming, headless mode, Codex/Claude ACP routing, or any Critical Fix.

### 3. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` updated **as
> each conflict is resolved** with a dated `## Merge 002353 (2026-07-27)` entry
> documenting every conflict, every ACP repair site, and every retired patch —
> because this window proves that an unmerged window compounds badly.

## Acceptance Criteria

### Merge Completeness
- [ ] Re-measure the fence / commit count / upstream HEAD before starting; record
      the measured values in `portingguide.md`
- [ ] `git merge upstream/main` — **merge, not squash, not rebase**
- [ ] Merge branch contains all upstream commits through the recorded upstream
      HEAD; any skipped commit explicitly justified in `portingguide.md`
- [ ] All 294 fork-only commits preserved verbatim (PRs #62–#71 in particular:
      crash-terminal-frame #65, unify-agent-send #66, Codex-ACP #67,
      ACP turn usage #68, stale-thread recovery #69/#70, e2e model readiness #71)
- [ ] `git log` confirms fork branch is ahead of and contains `upstream/main`

### ACP `0.14.0 → 2.0.0` (the dominant work item)
- [ ] `Cargo.toml` takes upstream's `agent-client-protocol = { version = "=2.0.0",
      features = ["unstable"] }` pin — do **not** keep `=0.14.0`
- [ ] `Cargo.lock` resolved `--theirs` and regenerated by the build; new
      transitive `agent-client-protocol-schema 1.5.0` present
- [ ] All ACP consumer crates compile: `acp_thread`, `acp_tools`, `agent`,
      `agent_servers`, `agent_ui`, `external_websocket_sync`, `sidebar`,
      `remote_server`, `eval_cli`, `zed`
- [ ] Non-exhaustive struct literals converted to builders (upstream's new
      `client_capabilities_for_agent()` in `agent_servers/src/acp.rs` is the
      reference shape); `ErrorCode` match arms checked
- [ ] `AcpThreadEvent::Stopped(StopReason)` remains a tuple variant everywhere,
      **including `#[cfg(test)]` code** (`grep -nE "AcpThreadEvent::Stopped\b([^(]|$)"`
      must return 0)
- [ ] Every ACP-forced repair site listed in `portingguide.md`
- [ ] Escalate to the user rather than guess if any ACP change has unclear
      lifecycle/semantics

### Conflict Resolution
- [ ] All 9 conflicting files resolved per the table in `design.md`
- [ ] **Auto-merged ≠ correct**: the manual audit list in `design.md` §"Audit
      auto-merged files" is walked file by file. `acp_thread.rs` (+3957 lines),
      `conversation_view.rs` (+2213), `conversation_view/thread_view.rs` (+3604),
      `agent_panel.rs` (+1257), `zed.rs` (+784), `agent.rs` (+593) all auto-merge
      textually and all carry Helix code
- [ ] Dead `crates/agent_ui/src/acp.rs` + `crates/agent_ui/src/acp/` tree
      (not declared in `agent_ui.rs`, deleted upstream by #50201) removed, with a
      porting-guide note — or explicitly kept with a stated reason

### Critical Fix Preservation (`portingguide.md` §"Critical Fixes")
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

### Helix-Specific Surface (brief constraints 1–13)
- [ ] `crates/external_websocket_sync/` intact (10 source files incl.
      `thread_service.rs`, `sync.rs`, `mcp.rs`, `mock_helix_server.rs`)
- [ ] `send_agent_ready`, `wait_for_websocket_connected`, UI-state-query callback,
      `acp_history_store()` present in `agent_panel.rs`
- [ ] `ThreadDisplayNotification` handler still calls
      `OnboardingUpsell::set_dismissed(true, cx)` **and** initialises
      `NativeAgentSessionList` (brief #9, #10)
- [ ] Fix 1b cfg-gated draft-suppression `return;` is still the FIRST statement of
      its `BaseView::Uninitialized` branch (re-locate — `agent_panel.rs` churns hard)
- [ ] `from_existing_thread()` field set matches its `::new` sibling on
      `ConversationView` (thread-load lock, `register_thread`,
      `ensure_thread_subscription`, `send_agent_ready` all preserved)
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` (brief #7)
- [ ] `trust_all_worktrees: true`, `show_sign_in: false`, branding/onboarding
      settings intact in `assets/settings/default.json` (brief #8, #12)
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) stays in
      `cfg(not(feature = "external_websocket_sync"))` (brief #11)
- [ ] Windowless `cx.subscribe()` in `thread_service.rs` preserved so
      `message_added` streams incrementally, not only at completion (brief #6, #13)
- [ ] `--allow-multiple-instances`, `--headless`, `initialize_headless()`,
      `build_application(headless)` in `crates/zed/src/main.rs`
- [ ] `title_bar`'s `external_websocket_sync` dep is `optional = true` +
      `render_restricted_mode`; workspace `rust-embed` keeps `debug-embed`;
      `wait_for_tools_ready` uses `cx.background_executor().timer()` (no `smol::Timer`)
- [ ] 3× `// HELIX: External agent` markers in `extensions_ui.rs`
- [ ] `grep_tool.rs` `truncate_long_lines()` / `MAX_LINE_CHARS = 500` intact
- [ ] `config_options.rs` `current_model_value()` re-based onto upstream's renamed
      `first_config_option_id_matching`
- [ ] `reqwest_client.rs` / `http_client_tls.rs` insecure-TLS support intact
      (`ZED_HTTP_INSECURE_TLS`), merged with upstream's new keepalive settings
- [ ] `dev_container_suggest.rs` early return; migration banner `Hidden`;
      trial-end upsell early return
- [ ] `BaseView` / `ContextServerStatus` (and any new ACP status enum) matches
      stay exhaustive

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] Compiles **both** with and without `external_websocket_sync` (brief #4).
      Since there is no local cargo, run the feature-off gate through the same
      Docker builder with the feature flag removed, or via
      `docker run … cargo check -p zed` in the build image
- [ ] `cargo test -p external_websocket_sync` — full pass
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6 invariant)
- [ ] `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` (PR #50)
- [ ] **E2E Docker test — HARD GATE.** All phases green for `zed-agent`, and for
      `claude` via `E2E_AGENTS="zed-agent,claude"`. `go mod tidy` in
      `e2e-test/helix-ws-test-server/` first. Never use `--no-build` when
      investigating failures
- [ ] One retry permitted per agent for the known Claude Phase-1 npm-install /
      single API-latency flake; a second failure is a real failure

#### E2E phases (measured — **16**, not 4)

`helix-ws-test-server/main.go` enumerates 16 phases. The brief's four named
phases map to Phases 1–4; all 16 must pass:

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
| 9 | Rapid 3-turn cancel |
| 10 | User-created thread injection + work session |
| 11 | Spectask routing (`FindConnectedSessionForSpecTask` picks most recent) |
| 12 | Reconnect (kill Zed, reconnect, deliver to existing thread) |
| 13 | Helix-initiated cancel → `turn_cancelled` status `cancelled` |
| 14 | Cancel no-op (bogus `request_id`) → status `noop` |
| 15 | Streaming patches arrive incrementally (`message_added` cadence) |
| 16 | Speculative-draft `user_created_thread` regression |

Runner: `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`. The brief's
raw `docker build -t zed-ws-e2e -f …/Dockerfile . && docker run …` targets the
same image; `run_docker_e2e.sh` is the established wrapper and is preferred.

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] New `## Merge 002353 (2026-07-27)` section at the top of the merge-history
      list (i.e. above `## Merge 002100-extension (2026-06-18)` at line ~670)
- [ ] Window summary: measured commit count, date window, fence SHA, upstream
      HEAD SHA, ACP before/after
- [ ] `### Conflicts and Resolutions` — one subsection per manual conflict
- [ ] `### ACP 0.14 → 2.0 — repaired sites` — one bullet per fixed call site
- [ ] `### Retired Helix patches` — with justification, or explicit "none"
- [ ] `### Helix-surface survival check` — per-area confirmation
- [ ] Commit-history table extended; stale Rebase-Checklist entries corrected
      (e.g. items that still say `thread_view.rs` where the live file is
      `conversation_view.rs`); new entries only for genuinely new gotchas
- [ ] A note recording that 002224/002251/002265 never landed, so future
      engineers don't mis-read the series

### Process
- [ ] Feature branch `feature/002353-merge-latest-zed` from current fork `main`
- [ ] Branch pushed to `origin` (mirrors `helixml/zed`)
- [ ] `sandbox-versions.txt` `ZED_COMMIT` bumped to the new merge HEAD on a
      `feature/002353-merge-latest-zed` branch in `/home/retro/work/helix/`, pushed
- [ ] `pull_request_zed.md` + `pull_request_helix.md` written into this task dir
- [ ] `main` **not** force-pushed; no agent-initiated PRs (the Helix UI opens them)
- [ ] Re-fetch `upstream/main` and `origin/main` before declaring done; absorb
      any out-of-band fork pushes; run an extension round if upstream advanced
      materially mid-work
- [ ] helixml/zed CI pipeline green

## Out of Scope
- Net-new Helix feature development
- Modifying E2E assertions unless an upstream ACP API change strictly requires it
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix crates beyond what the merge / ACP bump forces
- Adopting upstream's new elicitation UI, message-queue/steering UX, or
  sandboxing UX into Helix-mode flows beyond keeping them compiling
- Rewriting the porting guide from scratch — amend and extend in place

## Open Questions

1. **Porting-guide path (repeat of 002265's unanswered question).** The brief
   names `design/2026-02-07-zed-fork-rebase-to-upstream.md`, which exists only in
   the **helix** repo and is the 638-line historical one-time-port narrative with
   no `## Merge NNN` sections. Every merge since has updated
   `/home/retro/work/zed/portingguide.md`. **Assumption: `zed/portingguide.md` is
   the target.** Confirm, or say if both should be touched.

2. **Repo/branch layout (repeat).** The brief says
   `/prod/home/luke/pm/zed-upstream` on branch `helix-fork` with remotes
   `helix`/`origin`. Sandbox reality is `/home/retro/work/zed` on `main` with
   `origin` (gitea mirror) + `upstream`. **Assumption: work in the sandbox
   layout.** Confirm.

3. **`--locked` in CI (brief constraint #5) appears stale.** There is **no
   `.drone.yml` in the zed repo**. The Helix repo's `.drone.yml` clones zed and
   delegates to `Dockerfile.zed-build`, whose invocation is
   `cargo build [--release] --features external_websocket_sync` — **no `--locked`**.
   Should `--locked` be (re-)added to `Dockerfile.zed-build`, or is the
   constraint obsolete? **Assumption: obsolete; verify the CI build succeeds and
   do not add `--locked` without instruction.**

4. **Why did 002224/002251/002265 not land?** If those branches were abandoned
   for a known reason (e.g. an unresolved ACP problem), that reason is likely to
   recur here. Is there context we should have?

5. **Upstream's new message-queue/steering feature vs Helix's prompt queue.**
   Upstream added `conversation_view/message_queue.rs` (#59310, "Refactor the
   queue feature and add steering ability"). Helix has its own prompt-queue
   semantics exercised by E2E Phases 8 and 9 (busy-defer, interrupt).
   **Assumption: keep Helix semantics; adopt upstream's module only as far as
   compilation requires.** Confirm — if upstream's steering should replace the
   Helix path, that is a separate task.

6. **Upstream's session list / resume UI vs `from_existing_thread`.** Upstream
   now has `supports_resume_session` / `supports_session_history` /
   `AgentSessionList` on the `AgentConnection` trait. **Assumption: keep
   `from_existing_thread` as-is this window;** replacing it with upstream's
   native path is a follow-up, not merge work. Confirm.

7. **Two upstream-modified GitHub workflows the fork deleted**
   (`hotfix-review-monitor.yml`, `stale-pr-reminder.yml`, modify/delete
   conflicts). **Assumption: keep the fork's deletion** (`git rm`) since Helix
   does not run GitHub Actions. Confirm, or say to take upstream's version.

8. **E2E credentials.** The gate needs a working `ANTHROPIC_API_KEY` in the
   environment. Assumed available to the implementation agent — flag immediately
   if not, because the merge cannot be declared complete without it.

9. **Feature-off compilation gate.** With no local cargo, the cleanest way to
   check `cargo check -p zed` *without* `external_websocket_sync` is a one-off
   Docker run in the build image. Is there an existing `./stack` target for the
   feature-off build that we should use instead? **Assumption: none; use a
   one-off Docker invocation.**
