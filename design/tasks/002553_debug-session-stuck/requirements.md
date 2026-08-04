# Requirements: Fix Stale request_id Dropping message_completed and Wedging Sessions

## Background

On meta.helix.ml, session `ses_01kz5zgx69c2r68vy1bbeqpbr8` showed "running" for ~4m48s
after the agent had visibly finished. The completion event **was delivered and Helix
deliberately discarded it** because it carried a `request_id` belonging to an interaction
that had been interrupted 23 minutes earlier.

The chain (already root-caused by the reporter, verified against source during planning):

1. Zed emitted `message_completed` for thread `7f88913e…` tagged
   `request_id=int_01kz6akbywh1b4avvbc3zq0h53` — a turn interrupted at 12:05:29Z. The turn
   actually in flight was `int_01kz6ammh8caqx7s654gvfwbbp`.
2. `handleMessageCompleted` found `requestToInteractionMapping["int_01kz6akby…"] == ""`
   (the consumed sentinel), set `mappingConsumed = true` and `return nil` at
   `api/pkg/server/websocket_external_agent_sync.go:2718-2724`.
3. The DB fallback directly below (~L2734), which scans for the most recent `waiting`
   interaction and would have settled it correctly, is **unreachable** for this case —
   the early return fires first. The ordering is the bug.
4. `int_01kz6ammh…` stayed `state=waiting` with a complete 182,510-char final report;
   `sessions.config->>'external_agent_status'` stayed `running`.
5. The prompt queue's busy gate (`prompt_history_handlers.go:375-390`) logged
   "Session is busy (interaction waiting)" every ~2s forever, holding a queued prompt
   behind a `message_completed` that had already been received and thrown away.
6. Nothing times this out. `auto_wake_stuck_interactions.go` explicitly leaves *connected*
   sessions alone, and `desktopResumeReapStaleThreshold` only reaps when there is **no**
   live WebSocket. There is no recovery path for "agent connected + completion dropped".
7. It cleared only as a side effect of a user-forced interrupt.

The guard fired 3× on that session that day (11:09:09Z, 12:05:00Z, 12:28:23Z). The first
two were masked because a follow-up prompt triggered the `[TRANSITION] Auto-completed`
rescue path. It only becomes user-visible when the agent finishes and nobody sends
anything else — i.e. **exactly at end of task**.

### Do not just widen the guard

`git log -S mappingConsumed` gives one commit, `2186abcda` (2026-04-16), whose own message
describes *this same wedge* being fixed by replacing timing-based dedup with the
state-based `mappingConsumed` sentinel. And `design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md`
documents why the sentinel must **not** simply be removed: Zed's wrapper buffers events
that aren't direct ACP responses and flushes them later tagged with the *last* `request_id`
it saw. Rebinding a consumed sentinel to the current waiting interaction previously caused
mid-turn interactions to be marked Complete prematurely.

So both naive directions are already known-broken: keep the sentinel and genuine
completions are dropped; drop the sentinel and stale replays prematurely complete live
turns. **Fix the identity problem, do not re-tune the heuristic.**

---

## User Stories

### US-1 — A finished turn always finishes
**As a** user of a spec-task agent session,
**I want** the session to leave "running" as soon as the agent's turn ends,
**so that** I am not left staring at a spinner over an answer that is already complete.

**Acceptance criteria**
- [ ] A `message_completed` for a thread that has a `waiting` interaction is **never**
      silently discarded, regardless of the `request_id` it carries.
- [ ] After the completion lands, the interaction is `complete`, the session's
      `external_agent_status` is no longer `running`, and the frontend receives the update.
- [ ] The 182k-char-style case (real content already accumulated on the waiting
      interaction) settles onto **that** interaction, not a new or unrelated one.

### US-2 — Turn identity is authoritative, not a hint from the agent
**As a** Helix maintainer,
**I want** turn identity resolved from state Helix owns,
**so that** a stale echoed id from the agent cannot mis-route or drop a completion.

**Acceptance criteria**
- [ ] A single resolver decides which interaction an inbound thread event belongs to, and
      both `message_added` and `message_completed` use it.
- [ ] Today's asymmetry is gone: `message_added` recovers to the waiting interaction while
      `message_completed` does not (evidenced by the same burst logging
      "No interaction to route assistant message_added … content dropped" *and* still
      accumulating all 182,510 chars). The two must not be able to disagree again.
- [ ] `request_id` is used to disambiguate, never as the sole gate for dropping an event.

### US-3 — Genuine duplicates are still suppressed
**As a** Helix maintainer,
**I want** the real duplicate-suppression behaviour retained,
**so that** we do not regress `2186abcda` or the 2026-04-28 premature-completion bug.

**Acceptance criteria**
- [ ] A second copy of a completion already applied to interaction X is suppressed.
- [ ] A wrapper-replayed `message_completed` carrying a stale id does **not** complete a
      mid-stream interaction that has not actually finished its turn.
- [ ] The distinction is made on "have I already applied a completion to this turn?",
      not on "have I seen this request_id?".

### US-4 — Zed stops echoing a dead turn's id
**As a** Helix maintainer,
**I want** Zed to tag events with the request_id of the turn actually running,
**so that** the stale id is not generated in the first place.

**Acceptance criteria**
- [ ] After an interrupt→resend cycle, Zed's `message_completed` and `message_added`
      carry the **current** turn's `request_id`.
- [ ] Verified against the three-rapid-interrupts repro, reading `Zed.log` in the container.
- [ ] Ships via the full cross-repo flow (see design doc): commit in Zed, bump
      `ZED_COMMIT` in `sandbox-versions.txt`, **open the Helix PR before pushing the Zed
      branch**, merge Zed first, then Helix.

### US-5 — A real backstop, not a blind timer
**As a** user,
**I want** a wedged turn to self-heal even if the id fix misses a case,
**so that** "agent connected, turn finished, no completion applied" is bounded.

**Acceptance criteria**
- [ ] Helix can *ask* a connected agent whether a turn is running, rather than guessing
      from elapsed time. Zed already answers `noop` for a request_id with no live turn —
      that is the signal.
- [ ] A long-running tool call is **never** completed by the backstop. Proven by test, not
      by argument.
- [ ] The backstop covers the gap left by `auto_wake_stuck_interactions.go` (declines
      connected sessions) and `desktopResumeReapStaleThreshold` (only fires with no live
      WebSocket).

### US-6 — The state is visible, not silent
**As a** user,
**I want** to see when Helix thinks a turn is running but the agent says it is not,
**so that** I do not burn five minutes trusting a wrong spinner.

**Acceptance criteria**
- [ ] A `waiting` interaction whose agent reports no turn running surfaces in the UI
      and/or as a WARN-level log with session + interaction + request_id.

---

## Non-Goals

- Re-deriving the meta.helix.ml diagnosis. It is captured above; **verify, then fix**.
- Any action against meta.helix.ml. All work happens in the sandbox's inner Helix at
  `http://localhost:8080`. Do not touch, restart or attach to anything on meta.
- Rewriting the ACP/Zed sync protocol wholesale.

---

## Verification Requirements

Per CLAUDE.md — no confidence you did not earn.

- [ ] **Repro first, before any code change.** Create a spec task (a bare
      `agent_type=zed_external` chat session never connects an agent — a spec task is
      required). Send a prompt → interrupt → send another → interrupt → send a third, in
      quick succession, mirroring 12:05:00 → 12:05:29 → 12:05:42. Let the third run to
      completion and **send nothing afterwards**. Confirm via `psql` and the API log:
      `message_completed` bearing the first turn's `request_id`, the consumed-mapping
      warning at L2722, and an interaction stuck in `waiting`.
- [ ] The same repro goes green after the fix: turn completes, queued prompt is delivered.
- [ ] **Test the next operation, not just the state change** — after the completion lands,
      send another message and confirm it is delivered *and answered*.
- [ ] A long tool call is not prematurely completed by the backstop — demonstrated.
- [ ] New cases in `api/pkg/server/websocket_external_agent_sync_test.go` (a testify
      `WebSocketSyncSuite`, ~46 handler-path methods): "completion with a stale request_id
      while another interaction is waiting" and "genuine duplicate completion is still
      suppressed".
- [ ] **If the Zed WebSocket sync is touched at all, the dockerized e2e is mandatory**
      (`crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`). "Compiles" and
      "logically follows the pattern" are explicitly not acceptable — that rule exists
      because e2e runs have caught real ordering and interrupt races.
- [ ] Write-up at `design/2026-08-04-message-completed-stale-request-id-wedge.md`,
      including why `2186abcda`'s approach did not hold.
- [ ] PR opened against `helixml/helix`; full URL in the summary.

---

## Open Questions

1. **Zed root cause — confirm the planning hypothesis.** Reading
   `crates/external_websocket_sync/src/thread_service.rs` during planning surfaced a
   concrete mechanism the original report did not name (detailed in design.md): both the
   assistant-side rotation (L961-972) and the `Stopped` fallback (L1151-1169) key off
   `last_completed_request_id`, which is only written when a `message_completed` is
   actually emitted. An **interrupted** turn emits `turn_cancelled`, not
   `message_completed`, so that variable is never updated and the rotation freezes on the
   first turn's id indefinitely. The user-message rotation at L946-957 that would
   otherwise save it is unreachable for Helix-sent prompts, because
   `is_external_originated_entry` early-returns at **L901**, before it. This explains the
   23-minute staleness exactly. **Please confirm this is the intended fix target** before
   implementation commits to it — it is a strong hypothesis from source reading, not yet
   confirmed against a live repro.
2. **Resolver shape.** design.md argues for "thread → active interaction is truth,
   `request_id` is a hint" over "monotonic turn sequence the agent must echo". The latter
   is cleaner long-term but requires a coordinated Zed protocol change and a compatibility
   window for older `ZED_COMMIT` sandboxes. Assumption taken: **thread-first resolver now,
   turn sequence number as a follow-up**. Confirm, or say if you want the sequence number
   done in this change.
3. **Backstop probe mechanism.** The plan reuses the existing `cancel_current_turn` →
   `turn_cancelled{status:"noop"}` round trip as a liveness probe, since Zed already
   answers it correctly. That is a *cancel* verb being used to ask a *question*, and on a
   genuinely live turn it would cancel it — so the plan gates the probe behind "interaction
   waiting AND no stream activity for N". A dedicated read-only `turn_status` query event
   would be cleaner but is a second Zed protocol addition. Which do you prefer?
4. **Backstop idle threshold.** Assumed 60s of no `message_added` activity on a `waiting`
   interaction before probing. Sized to be well clear of a long tool call producing no
   tokens; is there a known worst-case silent tool duration that should raise this?
5. **Zed UI indicator.** Luke reported Zed's own UI also showed running, while
   `thread_service` logged "no turn running" and replied `noop` — so Zed's protocol state
   was right and its UI (if read correctly) was not. This spec treats that as a **separate,
   confirm-before-fixing** item and does not schedule a fix for it. Confirm that is the
   right call for this task.
6. **Surfacing (US-6) scope.** Assumed backend-only for now: a WARN log plus the existing
   interaction-update publish carrying a flag. Should this also get an explicit frontend
   banner ("agent reports no turn running"), which would pull in frontend work?
