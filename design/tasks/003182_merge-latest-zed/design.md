# Design: Merge Latest Zed Upstream Into Helix Fork

## Overview

Absorb **616** upstream commits (`b9256fa8f0` → `7960b2a7c9`, a 47-day window)
into `helixml/zed` `main` with a **true merge commit**, preserving all 357
fork-only commits and the whole Helix surface.

This spec is the **delta** on top of
`helix-specs/design/tasks/003081_merge-latest-zed/design.md`. That document's
architecture — round-based merging, `git merge-tree` reconnaissance, the
auto-merge audit method, the `git_ui_core` symbol map — is unchanged and is not
repeated here. Read it first.

Three things are genuinely new this window:

1. **ACP `2.0.0 → 2.1.0`** — the first protocol bump in five windows.
2. **The fork's agent-questions / elicitation relay landed on 2026-09-13** —
   inside `crates/agent_servers/src/acp.rs`, the one non-mechanical conflict.
3. **`crates/acp_thread/src/acp_thread.rs` grew +1263 upstream** (was ±16),
   escalating it from a confirming grep to a real audit target.

## Strategy

### Round-based merge (unchanged from prior windows)

616 commits is too large to resolve in one shot with confidence. Split at
upstream merge commits into ~4 rounds of roughly 150 commits each, exactly as
the 2026-07-29 merge did (4 rounds, documented in `portingguide.md:743`). Each
round: merge → resolve → `cargo check` → commit → note in the guide. A broken
round is then bisectable to ~150 commits rather than 616.

The final state must still be a single lineage where `git log` shows fork `main`
containing `upstream/main`.

### Merge, not rebase

Non-negotiable. Rebasing 357 fork-only commits across 616 upstream commits
replays every historical conflict. `git merge upstream/main` replays each
conflict once.

### Branch and push model

```
feature/003182-merge-latest-zed   cut from fork main (7c315e2021)
  └── round 1..4 merge commits + fixup commits
  └── pushed to origin (the gitea mirror of helixml/zed)
```

`main` is never force-pushed. `origin/helix-fork` (dead since 2026-02-07) is not
touched. No agent-initiated PRs.

## Key Decisions

### D1 — `acp.rs`: take upstream's lifecycle, re-host every fork item on top

This is the design decision that dominates the merge.

Upstream and the fork independently rewrote overlapping parts of the same file:

| Concern | Upstream (new) | Fork (new, PRs #93/#95/#96) |
|---|---|---|
| Session teardown | `register_session()` + `cx.observe_release()` + `agent_supports_session_close()`; `ref_count` fields deleted | `force_close_session()` (PR #63) built on `ref_count` |
| Session creation | `pending_sessions.borrow().get()` (no ref bump) | `session_creation_chain` / `SessionCreationGuard` (PR #50) |
| `session/update` handler | typed `handle_session_notification` | `RawSessionNotification` + `handle_raw_session_notification` — relays unknown/Codex-subagent update variants that strict deserialization would drop |
| `session/request_permission` | typed ACP request/response | `RawRequestPermissionRequest/Response` with a top-level `answers` field — the agent-questions wire format |
| Client capabilities | `parameterizedModelPicker` meta | + Codex-only `jetbrains.air.capabilities: ["nativeSubagentSessions"]` |
| Spawn logging | `log::debug!` / `log::trace!` | `[ACP_SPAWN]` structured info logs |

**Decision: upstream's lifecycle wins; every fork item is re-hosted on top of it.**

Rationale:
- Upstream's `observe_release` model is strictly better — it cannot leak a
  session when a caller forgets to decrement, which is exactly the class of bug
  the fork's `ref_count` arithmetic was patching around.
- The fork's items are *additive* — extra handlers, extra meta keys, extra
  logging. None of them depend on `ref_count` except `force_close_session`, and
  that one is re-expressible.
- Keeping the fork's ref counting would mean carrying a divergent lifecycle
  forever, re-conflicting every window.

**`force_close_session` re-expression:**

```
old: decrement session.ref_count; if it hits 0, remove + send CloseSessionRequest
new: unconditionally remove the sessions/pending_sessions entry (which drops the
     _release_subscription along with it), then send CloseSessionRequest
```

The semantics the caller at `thread_service.rs:3691` needs are unchanged: "tear
this session down now, regardless of who else holds a handle." That is precisely
what removing the map entry + sending the request does. The dropped
`_release_subscription` means the observer will not double-fire.

### D2 — ACP 2.1.0: builder sweep, but expect it to be cheap

ACP structs are `#[non_exhaustive]`, so a version bump turns every struct literal
into a compile error. Historically this has been the most tedious part of a
merge.

**This window it should be cheap**, because the fork-only ACP construction sites
already use the builder form. Measured in
`crates/external_websocket_sync/src/thread_service.rs`:

```rust
acp::CreateElicitationRequest::new(acp::ElicitationFormMode::new(...))
acp::CreateElicitationResponse::new(acp::ElicitationAction::Accept(
    acp::ElicitationAcceptAction::new().content(...)))
```

The sweep is therefore a **verification** pass, not a rewrite: compile, fix what
the compiler names, and confirm `ErrorCode` variants still exist.

**The one real risk is D3.**

### D3 — The `JsonRpc*` derive macros are the ACP-bump risk, not the structs

The fork's raw-relay layer uses three derive macros from
`agent_client_protocol`:

```rust
#[derive(Debug, Clone, Serialize, Deserialize, JsonRpcNotification)]
#[notification(method = "session/update")]
struct RawSessionNotification { … }

#[derive(Debug, Clone, Serialize, Deserialize, JsonRpcRequest)]
#[request(method = "session/request_permission", response = RawRequestPermissionResponse)]
struct RawRequestPermissionRequest { … }
```

Upstream's post-merge `acp.rs` **stops importing** `JsonRpcNotification` and
`JsonRpcRequest`. That is because upstream stopped *using* them, not necessarily
because 2.1.0 removed them — but nothing in the upstream tree proves they
survived the bump.

**Decision: check this first, before any other ACP work.** It is a five-minute
check (`cargo check -p agent_servers` after taking the 2.1.0 pin) with a large
blast radius. If the macros are gone, the raw relay must be re-expressed by hand
against `Client`'s handler registration API, and that is a headline work item
that should be surfaced to the user immediately rather than absorbed silently.

### D4 — `acp_thread.rs` is now an audit target, not a grep

003081 treated this file as "confirming grep only" on a ±16 delta. It is now
**+1263 −17**. Inspection shows the bulk is new tests
(`test_idle_sleep_prevention_*`, `request_test_form_elicitation`) plus URL-mode
elicitation helpers, and the types the fork depends on are byte-identical on both
sides today:

- `AgentThreadEntry::Elicitation(ElicitationEntryId)` — same
- `ElicitationStatus::{Pending{respond_tx}, Accepted, Declined, Canceled, Completed}` — same
- `ElicitationStoreEvent::{ElicitationRequested, ElicitationResponded, ElicitationUpdated}` — same
- `AcpThread::elicitation()` / `elicitation_mut()` — same signatures

**But** upstream added this wiring:

```rust
AcpThreadEvent::ElicitationRequested(_) | AcpThreadEvent::ElicitationResponded(_)
    => this.update_idle_sleep_prevention(cx),
```

The fork subscribes to those same events in `thread_service.rs:1825/1834`.
**Decision: audit that the two subscriptions coexist** — GPUI emits to all
subscribers, so they should, but this is exactly the kind of "auto-merged and
compiles, therefore assumed correct" case the audit exists to catch. Confirm the
elicitation e2e path still relays by running
`cargo test -p external_websocket_sync`.

### D5 — `dev_container_suggest.rs`: fork guard on top, upstream body underneath

New conflict this window, and mechanical. Upstream hoisted the CLI auto-open
branch to the top of `suggest_on_worktree_updated` and deleted the trailing
`cli_auto_open` block; the fork inserted a settings guard at the top of the same
function.

```rust
pub fn suggest_on_worktree_updated(…) {
    // HELIX: keep this FIRST
    if !RemoteSettings::get_global(cx).suggest_dev_container {
        return;
    }
    // upstream body verbatim from here
    if workspace.open_in_dev_container() {
        open_dev_container_from_cli(workspace, window, cx);
        return;
    }
    …
}
```

The fork's `use settings::Settings;` and `use crate::RemoteSettings;` imports
stay. All fork references to the deleted `cli_auto_open` / `has_configs` locals
go away with upstream's body.

### D6 — Gate on 17 e2e phases; Phase 18 is a separate spec

The brief mandates "Phase 18: elicitation round-trip". Measured: the e2e
directory contains **zero** occurrences of "elicitation", and `main.go`
documents phases 1–15 plus queue phases 16–17. Phase 18 has never existed on any
branch.

What *has* changed since 003081 asked this question: the feature under test
landed. So the gap is now real coverage debt rather than a sequencing question.

**Decision: gate this merge on the 17 phases that exist plus
`cargo test -p external_websocket_sync`** (which does exercise the elicitation
relay — `thread_service.rs:4250+` covers schema parsing, form-mode requests,
response content and the qwen meta path). Writing Phase 18 is Go work in
`helix-ws-test-server/main.go` and belongs in its own spec; doing it inside a
616-commit merge means a failing gate can't be attributed to either change.

Surfaced as Open Question 1 — if the user wants Phase 18 in scope, it is
additive work on top of this plan, not a substitute for any of it.

### D7 — Stale carry-forward constraints: audit the gate, not the artifact

Three of the brief's 13 carry-forward constraints are measurably stale. Prior
specs already flagged #8 and #12; #10 is newly confirmed dead.

| Brief constraint | Measured reality | Action |
|---|---|---|
| #8 `trust_all_worktrees: true` in `default.json` | Fork has `false` at line 2522, **byte-identical to upstream**. Enforced by a cfg gate in `crates/project/src/trusted_worktrees.rs` | Audit the gate. **Do not** edit the JSON |
| #12 `show_sign_in: false` in `default.json` | Fork has `true` at line 568, **byte-identical to upstream**. Enforced by `&& !cfg!(feature = "external_websocket_sync")` in `title_bar.rs:375` | Audit the gate. **Do not** edit the JSON |
| #10 `NativeAgentSessionList` initialised in `ThreadDisplayNotification` | The symbol **does not exist anywhere in the tree** | Drop the constraint; record the retirement |
| #5 `--locked` in `.drone.yml` | Zed repo has no `.drone.yml`; Helix's `Dockerfile.zed-build:108` has no `--locked` | Do not add it. Verify CI green instead |

A merge agent that "fixes" the JSON to match the brief would inject a real
behavioural regression. The porting guide must record these retirements so the
next window stops re-litigating them.

## Architecture Notes for Future Merges

Learnings worth carrying, beyond this window:

- **`git merge-tree --write-tree main upstream/main` is the cheapest possible
  reconnaissance.** It computes the full merge without touching the working tree
  and prints exactly the conflicting paths. Run it *before* planning. Costs
  seconds; replaced hours of guessing in every recent window.
- **Conflict count is a poor proxy for difficulty.** Seven of this window's eight
  conflicts are manifest/adjacency noise. One file — `acp.rs` — is the whole job.
  Measure per-file deltas (`git diff --numstat <fence>..upstream/main -- <path>`)
  and rank by *semantic* overlap, not by conflict marker count.
- **Auto-merged ≠ correct is the recurring lesson.** Every large delta on a file
  that hosts a Helix patch (`zed.rs` +972, `agent_panel.rs` +679, `agent.rs`
  +585) merged *textually* clean and still needs a human read. The audit list in
  requirements.md exists because a previous window lost a Helix patch this way.
- **Fork feature work landing in a known high-conflict file between plan and
  execution is the failure mode to watch.** 003081 planned `acp.rs` against a
  fork state that no longer exists. Re-run `merge-tree` and re-read the fork side
  of every conflicted file at the start of execution, however recent the spec.
- **The fence not moving across five specs is the real finding.** Reconnaissance
  is not the bottleneck — the last four specs were all accurate. Something
  downstream of planning is. Worth naming before spec six.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| ACP 2.1.0 dropped the `JsonRpc*` derive macros | Low | Check first (D3); if hit, surface immediately as a scope change |
| `acp.rs` resolution loses the agent-questions relay | Medium | Explicit per-item checklist in requirements.md; `qwen_permission_answers_serialize_at_the_response_top_level` test is the tripwire |
| Auto-merged Helix patch silently lost in a +900-line file | Medium | 16-item P1–P16 audit; grep for each named symbol |
| rustc two-version jump (1.95 → 1.98) breaks the build image | Low | `--default-toolchain none` should resolve it; verify early, do not pin back |
| 616 commits is unbisectable if something breaks | Medium | 4-round split at upstream merge commits |
| E2E cannot run (no `ANTHROPIC_API_KEY`) | Medium | `run_docker_e2e.sh` sources `../helix/.env`; flag immediately if absent — merge cannot be declared complete |
| Upstream advances materially mid-work | High (47-day windows) | Re-fetch and run an extension round before declaring done |

## Files Touched (summary)

- `/home/retro/work/zed/` — the merge itself, on `feature/003182-merge-latest-zed`
- `/home/retro/work/zed/portingguide.md` — new `## Merge 003182 (2026-09-14)`
  section inserted above `## Merge 2026-07-29` (line ~743), written incrementally
- `/home/retro/work/helix/sandbox-versions.txt` — `ZED_COMMIT` bumped to the
  post-merge SHA, on `feature/003182-merge-latest-zed`
- `helix-specs/design/tasks/003182_merge-latest-zed/pull_request_zed.md` and
  `pull_request_helix.md` — PR bodies for the human to open
