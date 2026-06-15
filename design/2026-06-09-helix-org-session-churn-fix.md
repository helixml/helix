# helix-org session churn — root cause + fix

Date: 2026-06-09
Status: **FIXED** — helix-org now drives worker sessions through the same
canonical primitives every other autonomous flow uses; no blocking turn
wait, no stale detection, no fresh-session churn.

## Symptom

A worker (`aaa`) churned through fresh sessions every few minutes
(`ses_…38v → …3hd → …3n3 → …3v5`), each preceded by
`exit: error: ensure session: open fresh helix session: … external
agent … timeout`. Long agentic turns never completed; the activation
stream went silent (which also exposed the mirror's fixed-session
fragility — see `2026-06-09-activation-stream-transcript-still-empty.md`).

## Root cause: helix-org used the wrong send path

There are two ways to drive an external (Zed) agent in Helix:

1. **Blocking OpenAI-compat** — `POST /sessions/chat` →
   `handleExternalAgentStreaming` → `RunExternalAgent` →
   `waitForExternalAgentResponse`, which **blocks up to 180s**
   (`defaultExternalAgentWaitTimeout`) for the *whole turn*. Built for
   OpenAI API clients expecting a synchronous chat completion.
2. **Fire-and-forget** — `POST /sessions/{id}/messages` →
   `sendChatMessageToExternalAgent`: persists a Waiting interaction,
   sends the WS command, **returns immediately**. The reply arrives async
   via WS sync. No turn timeout. This is what the **human desktop** (types
   straight into Zed), **spec tasks**, and the **cron trigger** use.

helix-org's spawner used path (1) via the in-proc client's bespoke
`StartChatWithStatus`. Real helix-org turns (git pull specs, read
agent.md/role.md/identity.md, do work, commit, push) routinely exceed
180s, so they were killed mid-turn → `EnsureAndSend` misread the timeout
as a **stale session** → opened a **fresh** one → which also timed out →
churn. The "stale detection" was both unnecessary and the churn's engine.

## Fix: reuse the canonical interfaces, delete the bespoke path

`SessionClient` now exposes two methods, backed by the shared Helix
primitives the in-proc adapter routes to:

| Interface method | Backed by | Used elsewhere by |
|---|---|---|
| `StartSession` | `StartExternalAgentSession` | cron trigger (`ExternalAgentStarter`) |
| `SendMessage` | `POST /sessions/{id}/messages` (`sendSessionMessage`) | frontend, spec tasks |

Both are non-blocking, so neither is subject to the 180s response
timeout. `EnsureAndSend` collapses to:

- **No session yet** → quota pre-flight → `StartSession` (creates the
  session, starts the desktop, queues the prompt). Persist the id.
- **Has a session** → `SendMessage` (fire-and-forget).

### Why "stale" detection was deleted, not preserved

A worker now keeps **one durable session**, created once. There is no
staleness to detect because Helix already recovers a downed session
transparently: when `sendCommandToExternalAgent` finds no WS, it fires
`autoStartDevContainerForSession` (any `zed_external` session, not just
spec tasks) and `pickupWaitingInteraction` delivers the queued message on
reconnect — on the **same** session, preserving the Zed thread /
conversation. That's strictly better than the old "open a fresh session"
recovery, which lost continuity. The *capability* to recover moved to
where it already lives in Helix; helix-org just stopped *deciding* it.

## Deleted

- In-proc `StartChatWithStatus` + the `sseCapture`/`parseSSE` SSE-scraping
  machinery + `parseStartChatResponseInProc`.
- `runtimehelix.StartChatRequest` / `SessionChatMessage` / `MessageContent`
  / `NewTextMessage` (the old `/sessions/chat` request shape).
- `EnsureAndSend`'s resume-vs-fresh branching, `hadStreamErr`, cold-start
  retry, and `sendToSession`.

## Tests

- `controller_external_agent.go` is untouched (the 180s timeout stays as
  the correct cap for genuine OpenAI-compat callers).
- `spawner_test.go`: `TestSpawnerStartsFreshAndPersistsSession` (no
  session → `StartSession`), `TestSpawnerFollowUpResumesPersistedSession`
  (has session → `SendMessage`, no fresh `StartSession`),
  `TestSpawnerFollowUpSurvivesDownDesktop` (follow-up never churns),
  quota gate, semaphore, mirror wiring. Removed: cold-start-requeue and
  open-fresh-on-stale tests (behaviours that no longer exist).
- Full helix/controller/server suites green.

## Follow-up

The 180s `waitForExternalAgentResponse` timer is still a *total* cap (not
reset on activity) for the OpenAI-compat path. That's fine for bounded
chat-completion clients, but if any future caller drives long turns
through `/sessions/chat`, consider making it an idle timeout. Not needed
for helix-org now that it's off that path entirely.
