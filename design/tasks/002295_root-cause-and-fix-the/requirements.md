# Requirements: Preserve Zed Thread When Editing an Agent's Model/Provider/Credential

## Background

On meta prod (2026-07-21), a long-running spec-task session
(`spt_01kvtnrkgp5t2a7n4pwcv2cb8j`, "LinkedIn Outreach") had a healthy Claude
Code (ACP) conversation thread `bd5abc10-…` — ~869 messages, a 569 MB jsonl on
the workspace volume. A user **edited the agent's config** (Anthropic API-key →
subscription mode, and a different model — opus 4.8) and then the session's
`config.zed_thread_id` pointer was **cleared to empty**. The next message
dispatched with an empty `acp_thread_id` / `first_message=true`, so Zed **forked
brand-new empty threads** (`2c1b6724`, `1151c086`) — silent, total loss of the
agent's working context. Recovery required manually re-pointing
`config.zed_thread_id` back to `bd5abc10` in Postgres.

The Zed thread is **model-agnostic ACP state**: its context lives in
`~/.claude-state/.../<thread>.jsonl` on the persistent workspace volume, which
survives a container recreate. Changing the LLM model, provider, or credential
type has no bearing on whether that thread is still valid — so discarding it is
pure data loss.

**Important scoping (per user):** this is **not** the in-place *switch-agent*
feature (claude_code ⇄ zed-agent). The real-world trigger is editing an agent's
**configuration** (model / provider / credential type) on a running session and
then **clicking Restart**. The exact backend clear-path is not yet confirmed and
**must be identified by a live reproduction with logging** before the fix is
finalised.

This mirrors the already-shipped fix for the sibling incident,
`design/2026-07-20-restart-clears-zed-thread-context-loss.md` (PR #2860), which
gated the *restart* path so a healthy thread is preserved.

## Candidate clear-paths (all four sites that set `ZedThreadID = ""`)

| # | Site | Trigger | Current gate |
|---|------|---------|--------------|
| 1 | `session_handlers.go:2581` (`restartSessionContainer`) | Restart button | `resetThread = !lastInteractionCompletedCleanly` — preserves a healthy thread (#2860) |
| 2 | `session_switch_agent_handlers.go:237` (`switchAgentInPlaceForNextTurn`) | explicit switch-agent **and** `reconcileSessionAgentWithApp` (runs on the next message) | **unconditional clear** |
| 3 | `websocket_external_agent_sync.go:3597` (`recoverMissingThread`) | Zed reports a stale/missing thread | intentional recovery |
| 4 | `session_clear.go:90` (`Clear`) | explicit `/clear` | intentional — **leave as-is** |

Site 2 is unconditional and is reached both by the switch-agent endpoint and by
`reconcileSessionAgentWithApp`, which fires on the next chat/message send
(`session_handlers.go:563` and `:2332`) whenever `sessionUsesAgentRuntime`
returns false. Site 1 could still clear if the last interaction was left in a
non-`complete` state by the config edit. The live repro must disambiguate.

## User Stories

### US1 — Change model without losing context
As a Helix user with a running agent session, when I change the agent's **LLM
model** and continue the conversation, the agent still remembers everything from
before the change.

**Acceptance (live, connected Zed — not seeded rows):**
- Given a spec-task session with `config->>'zed_thread_id'` = a non-empty UUID
  and a `complete` last interaction,
- When I change the model (e.g. opus → sonnet) and (if applicable) restart, then
  send a message,
- Then `zed_thread_id` is **unchanged**, `last_zed_message_id` keeps climbing on
  the **same** thread, no new `thread_created` is emitted, and the agent's reply
  reflects prior context.

### US2 — Change credential type without losing context
As above, but changing the **credential type** (api_key ⇄ subscription).

**Acceptance:**
- Same as US1. If flipping to subscription requires a desktop recreate to inject
  `CLAUDE_CODE_OAUTH_TOKEN` / `ANTHROPIC_BASE_URL` (see
  `subscriptionEnvForSession`), the recreate happens but the thread pointer is
  **preserved** across it, and the reconnect `open_thread` re-attaches.

### US3 — Change provider without losing context
As above, but changing the **provider** while staying on the same
`code_agent_runtime` (e.g. claude_code → claude_code). Context preserved.

### US4 — Genuine agent-kind change MAY reset (allowed)
As a user who switches the agent **kind** (claude_code ⇄ zed-agent — different
ACP binaries / thread stores), a fresh thread is acceptable.

**Acceptance:** thread may reset; the new agent comes up cleanly and can take a
message.

### US5 — Wedged thread still resets and recovers (regression guard)
As a user whose thread is genuinely wedged (ACP agent killed mid-turn, last
interaction not in a clean terminal state), restart/next-turn still resets the
thread and recovers.

**Acceptance:** wedged thread resets, a new thread opens, the agent responds.

## Non-Goals

- Changing the explicit `/clear` behaviour (site 4) — clearing is its point.
- Changing `recoverMissingThread` (site 3) — it recovers a genuinely missing
  thread.
- The in-place switch-agent UX itself (transcript reseed / handoff) beyond
  gating its thread clear.

## Deliverables

1. **Root-cause report**: the exact clear-path that fired for
   "edit config → Restart → send", proven by live logs (distinctive log line per
   site).
2. **The gate**: preserve `ZedThreadID` on a pure model/provider/credential
   change within the same runtime; clear only on genuine agent-kind change or a
   wedged thread.
3. **Live test evidence**: `last_zed_message_id` climbing on the **same**
   `zed_thread_id` across a model/credential change (US1–US3), plus US4/US5.
4. A PR against `helixml/helix`.

## Open Questions

1. **Exact clear-path.** For a *pure* model/provider/credential change,
   `sessionUsesAgentRuntime` should stay true, so `reconcileSessionAgentWithApp`
   (site 2) should early-return, and a `complete`-last-interaction restart
   (site 1, post-#2860) should preserve. Yet the thread was cleared. The live
   repro must confirm which of: (a) restart with a non-`complete` last
   interaction, (b) reconcile firing because the edit changed the effective
   runtime/agent-name binding, or (c) a config-edit-triggered re-provision — is
   the real culprit. **Assumption:** we instrument all four sites and let the
   repro decide; the fix gates whichever fires (and hardens the others).

2. **Where is the agent config edited for a spec task?** The model/provider/
   credential live on the app assistant config
   (`types.go` `CodeAgentRuntime` / `CodeAgentCredentialType` /
   `ClaudeSubscriptionModel`). **Assumption:** the edit is a PUT to the existing
   app (edit-in-place), not pointing the session at a different app. If the UI
   actually creates/clones an app or switches assistant index, the binding could
   shift and change the analysis — to be confirmed in the repro.

3. **Does a credential/provider change require a desktop recreate?**
   `subscriptionEnvForSession` injects env at desktop-start only.
   **Assumption:** flipping to subscription needs a recreate; a plain model
   change may not. Either way the pointer must survive the recreate.

4. **PR ordering.** Fix is API-side Go only (`api/pkg/server/…`), Air
   hot-reloads, no Zed/sandbox rebuild expected. **Assumption:** no
   `sandbox-versions.txt` bump needed. Confirm during implementation.
