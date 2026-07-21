# Design: Preserve Zed Thread When Editing an Agent's Model/Provider/Credential

## Principle (mirror PR #2860)

The Zed/ACP thread is **model-agnostic state on the persistent workspace
volume**. Only two things justify discarding the Helix→Zed thread pointer
(`session.Metadata.ZedThreadID`):

1. **Agent kind genuinely changed** — the old thread state is incompatible with
   the new agent (different ACP binary / thread store), e.g. `zed-agent` ⇄
   `claude_code`.
2. **Thread is wedged** — the last interaction is not in a clean terminal state
   (reuse `lastInteractionCompletedCleanly`).

Everything else — model, provider, credential type (api_key ⇄ subscription) —
within the **same** `code_agent_runtime` **preserves** the thread and lets the
reconnect `open_thread` (`websocket_external_agent_sync.go:~439`) re-attach.

## Investigation first (mandatory)

Before touching the fix, add a **distinctive log line at each of the four clear
sites** so the live repro shows exactly which one fires:

- `session_handlers.go:2581` — `restartSessionContainer`
- `session_switch_agent_handlers.go:237` — `switchAgentInPlaceForNextTurn`
- `websocket_external_agent_sync.go:3597` — `recoverMissingThread`
- `session_clear.go:90` — `Clear`

Each line logs `session_id`, `prev_thread_id`, and the caller/reason. Then run
the live repro (see Testing) and read `helix-api-1` logs to confirm the culprit.
Do **not** assume site 2 vs site 1 — let the log decide. The gate below is
written so it is correct **regardless** of which site fires (all three
non-intentional sites get gated / hardened).

## The gate

Introduce a single predicate and apply it at the clear sites that can fire on a
model/provider/credential edit.

```go
// agentKindChanged reports whether the switch crosses ACP agent kinds such that
// the old Zed thread state is incompatible with the new agent (different ACP
// binary / thread store). A pure model / provider / credential change within the
// same code_agent_runtime does NOT change the kind.
func agentKindChanged(prev, target types.CodeAgentRuntime, prevAgentName, targetAgentName string) bool {
    if prev != target {
        return true
    }
    return prevAgentName != targetAgentName
}
```

### Site 2 — `switchAgentInPlaceForNextTurn` (primary fix)

This is the single chokepoint reached by **both** the explicit switch-agent
endpoint **and** `reconcileSessionAgentWithApp` (which runs on the next
chat/message send). Replace the unconditional clear at line 237 with:

```go
kindChanged := agentKindChanged(prevRuntime, targetRuntime,
    session.Metadata.ZedAgentName, targetRuntime.ZedAgentName())
wedged := !apiServer.lastInteractionCompletedCleanly(ctx, session)
resetThread := kindChanged || wedged

if resetThread {
    session.Metadata.ZedThreadID = ""
    session.Metadata.AgentSwitchedAt = now
}
```

When `resetThread` is false (same runtime, healthy thread):
- **Keep** `ZedThreadID`.
- **Skip** the `fork_seed` transcript reseed and the synthetic handoff turn —
  they only make sense when a *new* thread needs its history injected. The same
  thread already has the full conversation; reseeding would duplicate it.
- Still call `publishAgentConfigChange` so the daemon rewrites Zed's config
  (new model/credential) and, if a desktop recreate is required for new
  subscription env, the reconnect `open_thread` re-attaches to the preserved
  thread — exactly the `restartSessionContainer(resetThread=false)` shape.
- The restart fallback keyed on `ZedThreadID` must not fire spuriously: since we
  keep the thread id, guard the fallback on `resetThread` (only arm it when we
  actually cleared).

WARN-log the surprising combination "clearing a thread whose last interaction
was `complete`" as a red flag (defence in depth, same as #2860).

### Site 1 — `restartSessionContainer` (verify, harden if needed)

Already gated by `resetThread = !lastInteractionCompletedCleanly` (#2860). If the
live repro shows the loss came through here (e.g. the config edit left a
non-`complete` last interaction such as a pending `Waiting` handoff), fix the
root cause of the non-clean state rather than loosening the gate. Add the WARN
red-flag log here too.

### Sites 3 & 4 — leave behaviour, add logging only

`recoverMissingThread` (real missing-thread recovery) and `Clear` (explicit
`/clear`) keep their behaviour. They only get the diagnostic log line so the
repro can rule them out.

## Why the reconcile path matters

Both message-send handlers call `reconcileSessionAgentWithApp` before enqueue
(`session_handlers.go:563`, `:2332`). It fires only when
`sessionUsesAgentRuntime` returns false, and that predicate keys **only** on
`CodeAgentRuntime` + `ZedAgentName`. A pure model/provider/credential change
leaves both unchanged, so reconcile *should* early-return. If the live repro
shows it firing anyway, the gate at site 2 (shared by reconcile) already covers
it — the fix is robust to that outcome. If it fires because the edit genuinely
re-binds the runtime/agent-name (Open Question 2), that would itself be a bug to
surface, but the gate still prevents thread loss for the same-kind case.

## Key facts learned (for future agents)

- Four sites set `ZedThreadID = ""`: `session_handlers.go:2581`,
  `session_switch_agent_handlers.go:237`, `websocket_external_agent_sync.go:3597`,
  `session_clear.go:90`.
- `switchAgentInPlaceForNextTurn` is shared by the switch-agent endpoint and
  `reconcileSessionAgentWithApp`; gating it once covers both.
- The switch-agent no-op guard rejects only when `sameApp && sameRuntime`, so an
  app/config edit that changes model/credential can still flow through.
- Claude Code thread context is a jsonl on the **persistent workspace volume**;
  it survives container recreate. Preserving the pointer is sufficient to keep
  context — proven in #2860 by a user manually reopening the old thread.
- `subscriptionEnvForSession` (`external_agent_handlers.go:130`) injects OAuth
  env **at desktop-start only**, so credential-type changes may require a desktop
  recreate — but the recreate must **preserve** the thread pointer.
- The restart frontend already believes it "preserves thread context"
  (`SpecTaskDetailContent.tsx:753`), which is why the loss on
  edit-config-then-restart is surprising and must be traced live.

## Scope

- API-side Go only (`api/pkg/server/…`). Air hot-reloads; no Zed/sandbox rebuild
  expected, so likely no `sandbox-versions.txt` bump.
- Unit tests may assert the gate's field value, but per CLAUDE.md they are **not**
  evidence the conversation survived — the live test is the acceptance gate.
