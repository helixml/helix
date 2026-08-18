# 2026-08-18 — a 42-second provider outage killed five spec tasks

## What happened

At **11:22:26 UTC** the Caddy reverse proxy at `100.89.187.17` lost the vLLM
backend behind `ds4-flash-node06` and began returning
`502 … upstream unavailable`. It recovered by **11:23:08** — a 42-second blip.

Five in-flight spec tasks in `prj_01kzgryhcpbh1t9y42sc0k7eaj` died permanently:

| task | session | runtime | outcome |
|---|---|---|---|
| `spt_01m0a7kr0jkm636y35ydz9wgbb` | `ses_01m0a7mgbanyz11cf0hxn2hd5m` | deepseek_harness | aborted |
| `spt_01m0a7kt0hgssgt6366zrs52ac` | `ses_01m0a7mm9882sertcsmwtpxdna` | deepseek_harness | aborted |
| `spt_01m0a7kv0fz04h24y8ykm10hz2` | `ses_01m0a8m2kqn8repnhgee8jh6bp` | deepseek_harness | aborted |
| `spt_01m0a62bcc6f9bkwqapw9kv1xj` | `ses_01m0a657v2216xkh1jvvzkcgna` | deepseek_harness | aborted |
| `spt_01m0a62dc3sx28z3vtvb2b54j6` | `ses_01m0a6djqz40s47xa1vfvkq59p` | deepseek_harness | aborted |

Every one reported the same string:

```
agent turn aborted: the ACP agent process exited mid-turn or hit max tokens
(see Zed.log 'Error in run turn' for the cause)
```

## The natural experiment

Two `opencode` sessions hit the **identical** 502s and **completed**:

```
11:22:26  ses_…mgbany  ERR 502            ← dsh
11:22:27  ses_…mgbany  ERR 502  (+944ms)
11:22:28  ses_…mgbany  ERR 502  (+1394ms) ← dsh gives up. 2.34s total.
11:22:30  ses_…594fdn  ERR 502            ← opencode
11:22:33  ses_…594fdn  ERR 502  (+3.0s)
11:22:39  ses_…594fdn  ERR 502  (+5.6s)
11:22:48  ses_…594fdn  ERR 502  (+8.8s)
11:23:08  ses_…594fdn  ok                 ← rode it out, task completed
```

The only variable is retry patience. Every dsh session with an in-flight
request died; the one dsh session that happened not to be mid-request
(`ses_01m0a7mja8g9hn0cjsqd1f2c31`) completed normally.

## Four defects

### 1. dsh's retry window was 2.3 seconds

`desktop/shared/dsh/cordis.yml` declared the `helix` provider with no
`retryPolicy`, inheriting `@deepseek-ai/dsh-llm` defaults: `maxRetries: 2`,
`initialDelayMs: 500`. The observed deltas (944ms, 1394ms = 383ms request +
500/1000ms backoff) match those defaults exactly.

A 502 *is* correctly classified retryable — `dsh-llm-pi-ai` maps it to `SERVER`
via `/\b5\d\d\b/`, which is in the default retryable set. The policy simply
expired 40 seconds too early.

**Fix:** explicit `retryPolicy` — 8 retries, 1s doubling to a 30s cap, ~90s
window. Verified by driving the real `dsh-llm-pi-ai` config schema and by
booting the full composition with the new file.

### 2. Helix's streaming path had no retry at all

`RetryableClient.CreateChatCompletionStream` called
`c.apiClient.CreateChatCompletionStream` directly, bypassing the `retry.Do`
wrapper `CreateChatCompletion` uses. Every failing call had `stream: true`.
Every coding agent streams. So the "RetryableClient" was not retryable on the
only path that matters here.

The non-streaming path named only 429 and 529 explicitly; the rest of the 5xx
range retried by accident, via a fall-through.

**Fix:** shared `classifyRequestError` used by both paths; 5xx explicitly
retryable, 4xx and TLS failures explicitly not. Stream **establishment** only —
retrying mid-stream would duplicate tokens the caller already consumed.

### 3. The error message threw away the cause

`AcpThreadEvent::Error` was a payload-free enum variant. The real error is
right there at `acp_thread.rs:3969` but only reached `log::error!`, so
`thread_service.rs` could emit nothing better than a guess ("exited mid-turn
**or** hit max tokens") pointing at a `Zed.log` inside a sandbox container that
is deleted when the task ends.

**Fix:** `AcpThreadEvent::Error(SharedString)`. Both emit sites already know
the cause — the max-tokens site and the `run_turn` Err site — so it now travels
with the event and onto the wire. This also removes the "or" from the message:
the two cases are distinguishable.

### 4. Helix knew the answer and didn't say it

Every proxied request lands in `llm_calls` with its provider error. Helix had
`502 upstream unavailable` recorded the whole time.

**Fix:** `maybeExplainProviderFailure` reads it back when the generic message
arrives — needed for sandboxes still running an older Zed. Bounded by a
3-minute lookback so a failure the agent already recovered from is never blamed
for an abort happening now.

## Related: why the org Bot filed tasks on the wrong harness

The "Software Engineer" Bot runs **opencode**, but every task it created since
2026-08-17 11:37 was **deepseek_harness**.

The Bot creates tasks by hand-writing a `curl` heredoc against
`POST /api/v1/spec-tasks/from-prompt` (visible in its session transcript,
`int_01m0a7dx4t8d8d7yejwtp8h5zp`) with `"runtime": "deepseek_harness"` as a
literal — copied forward from an earlier task and self-reinforcing. It goes to
curl because the org MCP `create_spec_task` tool has no sandbox-size or
sandbox-runtime arguments.

The underlying gap: `project.CodeAgentConfig` — what a task inherits when
created without an explicit config — was written **once** at provisioning from
the applier's `worker.*` defaults and never refreshed. The Bot's project sat on
`zed_agent`/`deepseek-v4-flash` while the Bot itself ran
`opencode`/`qwen3.8-27b`. Three different runtimes across bot, project, and
tasks.

**Fix:** `syncProjectCodeAgentConfig` converges a Worker project's task
defaults onto the Bot's own linked agent app on every activation. This is not
the "don't re-apply the provisioning spec" case the fast path warns about — the
source is the Bot's live app config, which is exactly what the settings UI
edits, so syncing propagates a user's edit rather than reverting it.

**Fix 2:** `create_spectask` now takes `sandbox_vcpus` (1|4|8) and
`sandbox_runtime`, resolved exactly as the REST path resolves them. That was
the whole reason to reach for curl. Memory is derived from the vCPU count via a
shared preset helper rather than accepted from the caller, so the pair can
never be one `ValidPreset` rejects.

There is still deliberately no code-agent argument, and the schema is
`additionalProperties: false` so one cannot be smuggled in. Instead the
returned view reports the sandbox the task got *and* the coding agent it
inherited from the project, so a Worker can see it is already configured
rather than concluding it must set one.

Still open: the REST create path refuses `ubuntu-desktop` when no
display-capable host exists (`HasDisplayCapableHost`); the org path cannot
make that check because the `SpecTasks` port has no sandbox controller. This
is not a regression — the org path already defaulted to `ubuntu-desktop`
without checking — but a Worker can now request it explicitly, so the failure
would surface at placement rather than at create.

## Also shipped

Project settings → General now has a **Coding Agent** section
(`ProjectCodeAgentDefaults`). The harness/model picker had been removed from
project settings on the grounds that harness availability is an org-level
decision — true, but it left the value that every task inherits invisible and
unchangeable from the project itself.

## Also: a recovered session kept showing the failure

An errored turn kept a red alert and a Retry button indefinitely, even after
the session went on to complete later work. Retry re-sends that interaction's
prompt — one the session has already moved past — so the button was actively
harmful, and the alert made a working session look broken.

`retrySucceeded` already covered the narrow case where the very next turn
retried the *same* prompt. The incident shape is different: the turn aborted
mid-work, the agent switched, and the session answered a different question.
`lastSuccessfulInteractionIndex` generalises it — anything errored before the
last clean completion has been overtaken, and renders as a quiet note with a
details link and no Retry. It is not erased: that turn's work really was
abandoned, and hiding it entirely would imply it finished.

## Deployment

- `cordis.yml` ships in the desktop image (`Dockerfile.ubuntu-helix:1371`) →
  needs `./stack build-ubuntu`.
- The Zed change needs `./stack build-zed release` and a `sandbox-versions.txt`
  `ZED_COMMIT` bump.
- The Go and frontend changes hot-reload.
