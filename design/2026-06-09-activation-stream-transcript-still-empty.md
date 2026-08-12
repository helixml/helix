# Worker transcripts on the activation stream (#2557 follow-up)

Date: 2026-06-09
Status: **FIXED** via a session-layer transcript mirror. Verified end-to-end
(spawner activations AND inline chat now both land on
`s-activations-<worker>`, DB + Streams UI).

## Symptoms

1. After #2557, hire/event/manual **activations** still recorded only
   lifecycle markers (`=== activation … ===`, `=== exit … ===`) on
   `s-activations-<worker>` — no `assistant:` / `tool_use` / `tool_result`.
2. Typing into the **inline "Chat with this worker"** panel (owner or any
   worker) produced a turn the chat panel displayed, but **nothing** on
   the worker's activation stream.

## Root cause: the transcript writer was bolted onto the spawner

Both surfaces already run the LLM turn through the **same** code: the
inline chat (`streaming.NewInference` → `POST /api/v1/sessions/chat`,
`streaming.tsx`) and the spawner's in-proc client (`StartChatWithStatus`
→ same `POST /api/v1/sessions/chat`, `helix_org_inproc.go`) drive the
**same per-worker `exploratory` session**.

The only thing the spawner added was a *transcript bridge* — a
subscriber to `session-updates.<owner>.<session>` that mirrors settled
entries onto `s-activations-<worker>`. Two problems:

- It was attached **only by the spawner**, **only for the duration of
  one activation** — so inline-chat turns (no spawner) were never
  mirrored.
- Even for activations it subscribed **too late**: `ensureSession`/
  `EnsureAndSend`/`StartChatWithStatus` runs the whole turn
  synchronously and publishes every frame *before returning*; the bridge
  was started only *after* `ensureSession` returned, so it subscribed
  after the turn was already over.

The owner-chat bridge that used to unify this (`api/pkg/org/server/chat`,
`HelixBridge`) was deleted in the #2516 redesign (`5a739cb42`) when the
htmx UI went away, and never rebuilt for the React/NewInference UI — the
comments referencing it stayed.

## Fix: a session-layer Mirror that *follows* the worker's session

`api/pkg/org/infrastructure/runtime/helix/mirror.go` — a `Mirror`
republishes every settled entry (and the user's prompt) to
`s-activations-<worker>` via the existing
`newBridge`/`TranscriptBody`/`PublishActivationEvent` pipeline. Because
spawner activations and inline chat share the worker's session, one
subscriber captures **every** turn from **every** surface — the single
writer the data model always wanted.

**Key:** a worker's *session is not stable.* A stale resume opens a
fresh session, and the inline chat can land on a newer session than the
spawner last persisted (observed live: persisted `ses_…38v` while the
live turn was on `ses_…3hd`/`…3n3`/`…3v5`). The **stable** identity is
the worker's *project* — every one of its sessions shares it. So the
mirror does NOT pin a session ID. It tracks the *worker* and polls its
current session, re-pointing the subscription when it changes:

- `Ensure(org, worker)` — start a per-worker tracker (idempotent). The
  tracker resolves the worker's current session, subscribes, and on each
  poll re-points if it changed (drops the old subscription, attaches to
  the new — old pump flushes on cancel). First subscribe is synchronous.
- The "current session" = the project's most-recent **exploratory**
  session (`store.GetProjectExploratorySession`) — exactly what the
  inline chat / live UI follow — wired via `MirrorConfig.ExploratorySession`.
- Poll interval: `defaultMirrorPoll` (5s); so the stream can lag a real
  session change by up to one interval, then catches up.
- `EnsureAll(org)` — tracks every worker in the org.
- `Stop(worker)` — cancels the tracker + subscription.

Lifecycle (no fixed-session pinning, no per-activation bridge):
- **Spawner** (`spawner.go`) calls `Ensure(org, worker)` — the tracker is
  long-lived and persists across activations, so it's already subscribed
  before turns happen.
- **`ensureBootstrap`** (`helix_org_middleware.go`) calls `EnsureAll`
  after its existing per-org `ReconcileAll` — once per org per process,
  so pre-existing / inline-chat-only workers are tracked after a restart.
- **`lifecycle.Fire`** calls `Stop`.

The dead `bridge.run` (per-activation subscribe loop) was removed; the
`bridge`/`EntryStream`/`TranscriptBody` rendering pipeline is reused.

### Why poll, not a broad `session-updates.>` subscription

A single wildcard subscription (resolve `session→project→worker` per
frame) would be churn-proof with no lag, but it's a firehose (all
sessions, cross-org) needing a session→worker cache + invalidation. The
poll-and-re-point approach stays per-worker, is far less code, and is
robust to churn at the cost of ≤1 poll-interval of lag. Chosen as the
proportionate fix; the broad subscription remains a future option if the
lag or per-worker poll cost ever matters. (We DO use NATS — an embedded
in-process server — so the wildcard option is technically available.)

### Note: the churn itself

aaa's rapid session churn (`exit: error: … open fresh helix session: …
external agent … timeout`) is a **separate, pre-existing** issue — the
sandbox agent intermittently not responding — not caused by this work.
It's what exposed the mirror's fixed-session fragility. Worth a separate
look at why activations time out.

## Tests

- `mirror_test.go`:
  - `TestMirrorCapturesTurnWithoutSpawner` — a frame on the session topic
    with **no spawner** lands on the activation stream (inline-chat
    regression).
  - `TestMirrorRepointsOnSessionChurn` — when the worker's session
    changes, the mirror drops the old subscription and follows the new
    one; a turn on the new session is captured. (The core fix.)
  - `TestMirrorCapturesUserPrompt` — `user:` segment, once per
    interaction (dedup).
  - `TestMirrorEnsureIsIdempotent`, `TestMirrorStop`.
- `spawner_test.go::TestSpawnerEnsuresSessionMirror` — an activation
  registers the worker, so a *later* session turn (inline chat) is still
  captured.

## End-to-end verification

- Inline chat to `w-owner`: `assistant: MIRROR-E2E-PROBE-9931` appeared on
  `s-activations-w-owner` — captured by the mirror, no activation.
- Manual activation of `aaa` (earlier iteration): full
  `tool_use`/`tool_result`/`assistant` transcript on `s-activations-aaa`.

## User turns ARE recorded (every prompt, no filtering)

The mirror also emits a `user:` segment for each prompt. The user's
prompt isn't an entry-patch, but the same frames carry
`Interaction.PromptMessage` (`publishInteractionUpdateToFrontend` sends
the full interaction). `bridge.apply` reads `u.Interaction.PromptMessage`
and emits one `user:` line per interaction (deduped by interaction ID;
prompts come only from the single current interaction, never the
full-session history, so a restart doesn't re-emit past prompts).

Every prompt is recorded deliberately — human inline-chat turns AND the
synthetic activation prompts the spawner injects. No filtering: the
stream is a faithful, observable record of exactly what each worker was
told and what it replied. Verified live: inline chat to `w-owner`
produced `user: Reply with exactly: …` immediately followed by
`assistant: …` on `s-activations-w-owner`.

## Known limitations / follow-ups

- **Multi-part prompts** (images / non-text `PromptMessageContent`)
  produce no `user:` line — only the flat `PromptMessage` text field is
  read. Text prompts (the common case) are covered.
- **First fresh-session turn (hire).** A brand-new worker's very first
  turn streams on a session whose ID isn't known until the turn runs, so
  the mirror attaches just after and misses that one turn; every
  subsequent turn is captured. A session snapshotter (currently
  `NoopSessionPreamble`) could backfill it.
- **One goroutine/subscription per active worker.** Fine for the alpha;
  revisit if worker counts grow large.
