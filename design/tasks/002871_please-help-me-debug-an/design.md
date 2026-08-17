# Design: Fix OpenCode Turns Ending Silently After Reasoning on Qwen Models

## 1. The request path, established from code

```
opencode (in desktop container)
  └─ provider "helix", npm @ai-sdk/openai-compatible
     baseURL = helixURL + "/v1"                      ← zed_config_handlers.go:757
     model   = "<helix provider>/<model>"            ← zed_config_handlers.go:760
        └─ POST /v1/chat/completions                 ← server.go:1038
             └─ createChatCompletion
                  └─ injectAgentToolNudge(...)       ← openai_chat_handlers.go:114
                       └─ provider (vLLM, qwen3.8)
```

Config for opencode is rendered by the **settings-sync-daemon**, not the API:
`api/cmd/settings-sync-daemon/opencode.go` → `buildOpenCodeConfig`, delivered via
`OPENCODE_CONFIG_CONTENT`.

Two things worth knowing for anyone debugging this area again:

- `api/pkg/opencode/opencode.go` is **only** a release resolver (version pin →
  download URL + SHA256). None of the LLM plumbing lives there. The plumbing is
  split between `api/pkg/server/zed_config_handlers.go` (what the model config is)
  and `api/cmd/settings-sync-daemon/opencode.go` (how it is written out).
- The settings-sync-daemon does **not** hot reload. Per `CLAUDE.md`, changes there
  need `./stack build-ubuntu` and a **new** session.

## 2. The existing mitigation was already active, and it did not help

`openai_chat_handlers.go:32` — `agentToolNudge` — exists for precisely the
reported symptom. Its comment:

> Some models (e.g. GLM) narrate a plan and return finish_reason "stop" with no
> tool call; the Zed agent reads "no tool call" as end-of-turn and hands control
> back to the user, who then has to prod it.

It is gated on `INFERENCE_AGENT_TOOL_NUDGE_MODELS`, whose default is
`"glm,qwen"` (`config.go:494`), matched as a case-insensitive substring of
`req.Model`, and only applied when `len(req.Tools) > 0`.

All three gates passed for this session: the endpoint is
`/v1/chat/completions`, the model id (`<provider>/qwen3.8…`) contains `qwen`, and
opencode sends tools. **So the nudge was injected and the turn still stalled.**

That matters for prioritisation: a prompt-level nudge cannot rescue a turn that
was cut off at a token cap. Its failure here is weak evidence *against* the
plain narrate-then-stop explanation and *for* truncation.

Note the nudge is injected **only** in `createChatCompletion`. The sibling
proxies `/v1/responses` (`server.go:1039`) and `/v1/messages` (`server.go:1043`)
do not call `injectAgentToolNudge`. Not a factor for opencode, which uses
chat/completions — but it is a latent gap for any runtime on those endpoints.

## 3. `finish_reason` is never inspected anywhere in Helix

A repo-wide search for `finish_reason` / `FinishReasonLength` outside tests and
generated swagger returns exactly one functional hit:
`api/pkg/openai/openai_client_google.go:588`, which *maps* Gemini's
`FinishReasonMaxTokens` to `length`. Nothing ever *reads* it.

Consequences:
- A turn truncated at the output cap produces no log line, no metric, no
  interaction error, and no UI signal.
- It is indistinguishable, from Helix's side, from a turn that ended normally.
- The user's only feedback is a response that stops mid-thought — which is
  exactly what they reported.

**This is the highest-value fix regardless of which root cause is confirmed**,
because it converts an invisible failure into a diagnosable one.

## 4. Verified config defect: `limit.output: 0` is sent to opencode

`api/cmd/settings-sync-daemon/opencode.go:125-129`:

```go
if d.codeAgentConfig.MaxTokens > 0 || d.codeAgentConfig.MaxOutputTokens > 0 {
    model.Limit = &openCodeModelLimit{
        Context: d.codeAgentConfig.MaxTokens,
        Output:  d.codeAgentConfig.MaxOutputTokens,
    }
}
```

with

```go
type openCodeModelLimit struct {
    Context int `json:"context"`   // no omitempty
    Output  int `json:"output"`    // no omitempty
}
```

The guard is an **OR** but it writes **both** fields. So whenever exactly one of
the two is known, the other is emitted as a literal `0`. This defeats the guard's
own stated intent, quoted from the code immediately above it:

> Only declare limits we actually know. Writing zeros would tell opencode the
> context window is empty and it would compact after every turn.

**How a self-hosted Qwen model lands in the mixed case:**

1. `buildCodeAgentConfigFromAssistant` (`zed_config_handlers.go:799-810`) looks up
   `modelInfoProvider.GetModelInfo(...)` for `maxTokens` / `maxOutputTokens`.
2. `qwen3.8` **does not exist in `api/pkg/model/model_info.json`** — I checked all
   741 entries; 0 match `qwen3.8` (102 entries match `qwen`, none of them 3.8).
   So both values stay `0`. This matches `CLAUDE.md`'s note that self-hosted
   models have effort profiles but no catalogue entry.
3. `applyAdvertisedModelLimits` (`zed_config_handlers.go:889`) then overwrites
   `cfg.MaxTokens` from the provider's advertised `/v1/models` context length
   (vLLM reports `max_model_len`), which for a 3.8-class Qwen is large.
4. `MaxOutputTokens` is never filled by anything.

Net: `"limit":{"context":262144,"output":0}`.

The sibling Zed/qwen-code path gets this right — `AvailableModel.MaxOutputTokens`
is tagged `json:"max_output_tokens,omitempty"` (`main.go:135`) with the explicit
comment `// 0 = omitted (uses model default)` (`main.go:738`). The opencode path
is the inconsistent one.

**Test gap:** `TestOpenCodeConfigOmitsUnknownLimits`
(`opencode_test.go:149-160`) sets *both* values to 0 and asserts `limit` is
absent. The mixed case — the one that actually occurs in production for
self-hosted models — is untested.

### What opencode does with `output: 0`

From upstream issues (see Sources), `ProviderTransform.maxOutputTokens()` computes
`Math.min(model.limit.output, outputTokenMax) || outputTokenMax` with
`OUTPUT_TOKEN_MAX = 32_000`. For `output: 0` that is
`Math.min(0, 32000) === 0`, then `0 || 32000` → **32000**.

So, honestly: `output: 0` does **not** produce a tiny cap or an
`maxOutputTokens must be >= 1` error. Its real effects are narrower:

- Helix's declared output limit is silently ignored; the effective cap is
  opencode's hardcoded 32k, whatever Helix intended.
- `limit.input` is never set by Helix at all, and upstream `overflow.ts` derives
  usable context as `context - maxOutputTokens()` in that case — a sharp edge for
  models whose context and output are close.

Two upstream behaviours are directly relevant to the reported symptom:

- **Issue #18108** — "Truncated tool calls are misclassified and unrecoverable
  (`finishReason: length` + `repairToolCall` + doom loop)". Truncation is
  misclassified and no truncation signal is given to the model.
- Reasoning/thinking budget is **not** subtracted from the reserved output cap, so
  a verbose thinking pass silently eats the answer's budget.

Helix also sets `"doom_loop": "deny"` in the opencode permission map
(`opencode.go:143-146`), which interacts with that first issue.

## 5. Root-cause candidates, ranked, and the query that settles it

**A. Output truncation (most consistent with the evidence).**
`qwen3.8-27b`'s curated default reasoning effort is **`xhigh`** — the most verbose
tier it accepts (`reasoning_efforts.go:139-146`; it accepts
`none/low/medium/xhigh` and *rejects* `high`, the reverse of most models).
Reasoning tokens count against the output cap. Reasoning consumes the budget →
`finish_reason: length` → opencode ends the turn → Helix says nothing. Both stored
responses ending mid-sentence fits this and nothing else fits it as cleanly.

**B. Clean narrate-then-stop that the nudge failed to prevent.** Possible, but the
nudge was active (§2) and a natural stop would not normally cut mid-sentence.

I ruled out two alternative explanations for the mid-sentence cut:
- *Not a display artifact of my tooling.* `api/pkg/session/mcp_server.go` truncates
  only the TOC `summary` field (to 77 chars, line 368). The `response` body is
  passed through verbatim.
- *Not the streaming throttle.* There is a known truncated-snapshot hazard in
  `websocket_external_agent_sync.go:1408-1415`, but its force-flush covers the
  `tool_call` boundary and, more importantly, `message_completed` publishes a
  final corrected snapshot bypassing the throttle (line 2156-2170). A turn that
  completed — as these did, since the user could reply — has been flushed.

**The deciding query** (run on the controlplane hosting meta.helix.ml):

```sql
SELECT id, model,
       original_request->>'reasoning_effort' AS asked,
       request->>'reasoning_effort'          AS sent,
       request->>'max_tokens'                AS max_tokens,
       response->'choices'->0->>'finish_reason' AS finish_reason,
       response->'usage'->>'completion_tokens'  AS completion_tokens,
       left(error, 200) AS err
FROM llm_calls
WHERE session_id = 'ses_01m07rcre65gy6qrd9s4ykxhpa'
ORDER BY created DESC
LIMIT 20;
```

`finish_reason = 'length'` ⇒ candidate A, and US-4 is load-bearing.
`finish_reason = 'stop'` with a healthy `completion_tokens` ⇒ candidate B, and the
nudge needs strengthening instead.

## 6. Proposed changes

Deliberately small, and ordered so the observability fix lands first — it is
correct regardless of which candidate wins, and it makes the next occurrence
self-diagnosing.

### 6.1 Surface `finish_reason: length` (US-1, US-2)

Inspect the finish reason on the completion path used by
`/v1/chat/completions`, and:
- log WARN with session id, model, `completion_tokens`, and the request's
  `max_tokens`;
- mark the interaction as truncated so the UI can show it.

Reuse the existing interaction-error/banner mechanism rather than inventing a new
channel. Do **not** auto-retry (see Open Question 5) — an automatic re-prompt on
truncation risks duplicated side effects, which is the same class of problem the
`doom_loop` and bounded-`steps` workarounds already exist to contain.

### 6.2 Fix the limit emission (US-3)

Split the guard so each field is independent, and add `omitempty` to both:

```go
type openCodeModelLimit struct {
    Context int `json:"context,omitempty"`
    Output  int `json:"output,omitempty"`
}
...
if d.codeAgentConfig.MaxTokens > 0 || d.codeAgentConfig.MaxOutputTokens > 0 {
    model.Limit = &openCodeModelLimit{}
    if d.codeAgentConfig.MaxTokens > 0 {
        model.Limit.Context = d.codeAgentConfig.MaxTokens
    }
    if d.codeAgentConfig.MaxOutputTokens > 0 {
        model.Limit.Output = d.codeAgentConfig.MaxOutputTokens
    }
}
```

This is a correctness fix on its own merits (it restores the documented intent and
matches the Zed path). It is **not** expected to fix the stall by itself, since
`output: 0` already collapses to 32k upstream — say so in the commit message
rather than overclaiming.

### 6.3 Give self-hosted models a real output limit (US-4)

`MaxOutputTokens` is 0 for every model absent from `model_info.json`, which is all
self-hosted ones. Options, in order of preference:

1. Derive a default from the advertised context window in
   `applyAdvertisedModelLimits` — the same place that already makes `/v1/models`
   authoritative for context. This keeps the "provider is authoritative" pattern.
2. Add an explicit `MaxCompletionTokens` to the reasoning-effort/curated-model
   table, which already carries per-family self-hosted knowledge.

Whichever is chosen, the effective cap must be large enough for a full `xhigh`
Qwen reasoning pass **plus** an answer, or the effort default for this model on
opencode must come down. Reasoning tokens are not free.

## 7. Learnings for future agents

- **The harness matters more than the model name.** The user said "qwen", which
  invites a dive into `/home/retro/work/qwen-code/`. The session actually ran
  **opencode**. `session_toc` on the reported session settled it in one call —
  check that before reading any code.
- **`api/pkg/opencode/` is a release resolver, not the LLM integration.** The
  config lives in `api/cmd/settings-sync-daemon/opencode.go`; the model/limit
  decisions live in `api/pkg/server/zed_config_handlers.go`.
- **Self-hosted models are absent from `model_info.json`.** Any code path that
  reads `MaxCompletionTokens` from it silently gets `0` for them. Grep for that
  pattern before trusting a token limit.
- **Helix has a curated reasoning-effort table** (`api/pkg/model/reasoning_efforts.go`)
  because effort support is not runtime-discoverable. `qwen3.8-27b` is the trap
  entry: default `xhigh`, accepts `none/low/medium/xhigh`, **rejects `high`**, and
  silently coerces unrecognised values to `xhigh`.
- **`agentToolNudge` already exists** for narrate-then-stop stalls, defaults to
  `glm,qwen`, and is injected **only** on `/v1/chat/completions`. If a stall is
  reported on a model matching that list, the nudge was probably already applied —
  look for a different cause rather than re-adding the same mitigation.
- **A guard written as `A > 0 || B > 0` that then writes both A and B** is the bug
  shape found here. Worth grepping for elsewhere in the config builders.
- **When a transcript ends mid-sentence, rule out the plumbing before blaming the
  model:** the MCP `get_turn` path, the 50ms publish throttle, and the
  `message_completed` corrective flush. All three were checked and cleared here;
  §5 records how, so the next person need not redo it.

## Sources

- [opencode #18108 — Truncated tool calls are misclassified and unrecoverable (finishReason: length + repairToolCall + doom loop)](https://github.com/anomalyco/opencode/issues/18108)
- [opencode #22253 — Custom provider models fail with "maxOutputTokens must be >= 1" when limit is not defined](https://github.com/anomalyco/opencode/issues/22253)
- [opencode #29363 — `limit.output` is silently capped at 32k; `OPENCODE_EXPERIMENTAL_OUTPUT_TOKEN_MAX` is a poor workaround](https://github.com/anomalyco/opencode/issues/29363)
- [opencode #20078 — Custom provider (LM Studio) ignores limit.output config, hardcodes max_tokens to 32000](https://github.com/anomalyco/opencode/issues/20078)
- [opencode #2949 — OUTPUT_TOKEN_MAX hardcoded to 32k prevents using full 64k output limit](https://github.com/anomalyco/opencode/issues/2949)
