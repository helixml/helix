# Auto-recover a lost agent thread with a full-transcript handoff

Fixes https://github.com/helixml/helix/issues/2723 for every harness.

## Incident (meta.helix.ml, 2026-10-26)

Org bot `chief-of-staff` (OpenCode, `qwen3.8-flash-next`), session
`ses_01m1txs8v79qkzfdtpg2z2jban`. Every message failed with
`Thread load failed: … OpenCode service failure: {"service": "session"}`.
The desktop had been stopped since 2026-10-12, the orphan-resource reaper
(7-day grace) deleted the session's workspace — and `opencode.db` with it —
while `config.zed_thread_id` survived in Postgres. Fixed by hand with
`config - 'zed_thread_id'`.

## Why string matching could not fix it

Probed live against `opencode acp` 1.18.18 (fresh desktop, copy of its state dir):

```
→ session/load {"sessionId":"ses_f569d9bb3ffew704MweP05X3p7"}   (id not in the store)
← {"error":{"code":-32603,"message":"Internal error: OpenCode service failure","data":{"service":"session"}}}
→ session/load {"sessionId":"not-a-session"}
← {"error":{"code":-32603,"message":"Internal error: OpenCode service failure","data":{"service":"session"}}}
```

A missing session is the generic JSON-RPC internal error with OpenCode's catch-all
wrapper — the same thing any other session-service failure produces. There is no
distinct not-found signal, so no OpenCode matcher was added to
`isAuthoritativeMissingThreadError`.

## The deterministic signal: workspace identity

Every harness keeps its session store inside the workspace bind-mounted at
`/home/retro/work` (`.opencode-state`, `.claude-state`, `.qwen-state`, `.zed-state`, …).
A recreated workspace therefore means every thread Helix has on record is gone,
for every harness.

- **Hydra** (`hydra/workspace_identity.go`): before `prepareMountSources` creates
  missing mount sources, `prepareWorkspaceIdentity` notes whether the workspace
  directory existed and reads — or writes — `<workspace>/.helix-workspace-id` (a
  UUID). `DevContainerResponse` carries `workspace_id` and `workspace_created`
  on every return path of `CreateDevContainer`. A single file, not a `.helix/`
  directory, so hydra (root) leaves no root-owned directories in the agent's home.
- **API** (`external-agent/workspace_identity.go`): `HydraExecutor.StartDesktop`
  calls `reconcileWorkspaceIdentity` right after `CreateDevContainer` returns —
  before the 90s bridge wait, since the agent can connect before `StartDesktop`
  returns. The thread is lost when the session has a `ZedThreadID` and either
  its stored `WorkspaceID` differs, or (sessions from before this change, no
  stored id) hydra created the workspace in this call. Then `ThreadReseedFrom =
  ZedThreadID`, `ZedThreadID = ""`. The id is stored on every start.
  Spec-task workspaces are shared between sessions; the stored id is what lets
  each session — not only the first to start — see the loss.
- The API-side `MkdirAll(workspaceDir)` in `StartDesktop` was removed: it ran in
  the API process, whose filesystem is not the sandbox's, so it only created
  empty directories there.

## One seed path

`SessionMetadata.ThreadReseedFrom` is the single "the next new thread must carry
the transcript" flag. Both triggers set it:

| Trigger | Where |
|---|---|
| Workspace recreated | `reconcileWorkspaceIdentity` (on `StartDesktop`) |
| Agent says the thread is missing / can never be loaded | `recoverMissingThread` (`isAuthoritativeMissingThreadError`, `isUnrestorableThreadError`) |

`maybePrependTranscript` — already called on every first message of a new thread
(direct send, prompt queue, reconnect resume, controller `RunExternalAgent`, and
now the recovery replay) — seeds when `ZedThreadID == ""` and the flag is set.
`handleThreadCreated` clears the flag when the new thread id is stored, so a
send that fails before a thread exists is seeded again on retry. `ClearSession`
also drops the flag (the history it would carry is being discarded).

`buildThreadReseedPreamble` now serializes the session with
`serializeTranscriptWithMode(…, truncateOldestFirst)` — the same serializer as an
agent switch — minus the turn being sent, under the spec-task header (name, full
original prompt, branch, design docs) and a "re-read files" framing. The 6-turn /
700-char / 6000-char summary and the `reseedContext=false` special case are gone:
a missing thread is always seeded. Budget is
`min(maxTranscriptBytes, contextTokens × 2)`, with the window taken from the
session's code-agent config (catalogue, overridden by the provider's advertised
`max_model_len`). `qwen3.8-flash-next` advertises 516,096 tokens (catalogue:
262,144), so the budget stays 400 KB.

One INFO line per seeded thread:
`thread reseed: seeding new thread with the prior transcript` with `session_id`,
`old_thread_id`, `transcript_bytes`, `truncated`.

## Two bugs found by testing live

1. `RunExternalAgent` built the command from the session snapshot taken before
   `WaitForExternalAgentReady`. That wait spans the desktop start that clears the
   thread id, so the old id was sent anyway. The session is now re-read after
   the wait (as `deliverResumedTurn` already did).
2. When the reconnect-resume path wins the dispatch claim, `RunExternalAgent`
   (the loser) still built — and logged — a second seed it never sent. Only the
   claim winner prepares the message now.

## Live verification (inner Helix, file-copy sandbox)

Reaping used hydra's own `POST /api/v1/gc/reconcile` with `grace_period_seconds: 0`
and every other session in the live set — the exact code the orphan reaper runs.

| Harness | Reaped → "what is my codeword?" | Follow-up | Log |
|---|---|---|---|
| OpenCode (`qwen3.8-flash-next`) | correct, new `zed_thread_id`, no error | same new thread, correct | 1 detection + 1 seed line |
| zed-agent (`chief-of-staff`) | correct | same new thread, correct | 1 + 1 |
| Qwen Code | correct | same new thread, correct | 1 + 1 |
| Claude Code | **not run**: the inner Helix's Anthropic provider proxies to the outer Helix, which rejects the key ("code-agent task has no API provider selected") — every Claude turn fails before any thread logic | | |

Negative: with the OpenCode workspace intact, moving `opencode.db` aside made the
load fail with the identical `OpenCode service failure` error. The error surfaced,
`zed_thread_id` and `workspace_id` were unchanged and nothing was reseeded.
After restoring the DB the next message resumed the original thread with its
native context.

Gotcha for testers: org bots without `preserve_context: true` are deliberately
cleared by the spawner on re-activation (`spawner: cleared session before
re-activation`), which also opens a new, empty thread. Use a preserve-context
bot (as `chief-of-staff` is) to test thread continuity.
