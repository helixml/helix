# Requirements: Fix OpenCode Turns Ending Silently After Reasoning on Qwen Models

## Background

A user working in session `ses_01m07rcre65gy6qrd9s4ykxhpa` ("Chief of Staff", on
meta.helix.ml) reported that the agent "thought, and after thinking, it just
stopped". They typed `continue`, it thought again, and stopped again.

This task is **diagnosis-first**. The investigation below is already done and its
findings are recorded in `design.md`. No code was changed (the request was
explicitly READ ONLY).

### What the session actually shows

Read via the `helix-session` MCP tools (`session_toc`, `get_turn`):

| Turn | Prompt | Stored response |
|---|---|---|
| 2 | — | "Agent switched to **opencode** at turn 1" |
| 4 | Long "personal note taker" spec | A `<thinking>` block **only**. No text, no tool call. Ends mid-sentence: `"That's fine — it's a low-risk, reversible action"` |
| 5 | `continue.` | `<thinking>` → 3 tool calls (`list_repositories`, `list_bots`, `list_topics`) → another `<thinking>` block. Ends mid-list: `"5. **File**:"` |

So the harness is **opencode** (not qwen-code), the model is a Qwen model, and in
both turns the assistant's final act was reasoning that ends mid-sentence, with
no user-visible text, no tool call, and no error.

### Why this is a Helix bug and not just a flaky model

Helix already ships a mitigation for this exact symptom class — `agentToolNudge`
in `api/pkg/server/openai_chat_handlers.go:32` — whose comment describes the
reported behaviour almost verbatim. That mitigation **was in force for this
session** (see `design.md` §2), and the stall happened anyway. Separately,
nothing anywhere in Helix inspects `finish_reason`, so a turn cut short at the
output-token cap is indistinguishable from a turn that finished normally: no log
line, no warning, no retry, nothing in the UI. That silence is the reason the
user had to guess and type `continue`.

## User Stories

### US-1 — As a user, when a turn is cut short I am told so
When the provider returns `finish_reason: "length"` (or the harness ends a turn
on a truncated response), I should see that the turn was truncated rather than a
turn that silently ends mid-thought.

**Acceptance criteria**
- [ ] A completion whose `finish_reason` is `length` is logged at WARN with
      session id, model, prompt/completion tokens, and the configured output cap.
- [ ] The truncation is recorded on the interaction so it is visible in the UI
      (banner or equivalent), not just in server logs.
- [ ] A turn ending with reasoning content but no assistant text and no tool call
      is logged distinctly, since that is the shape the user experiences as
      "it just stopped".

### US-2 — As an operator, I can tell from data which failure occurred
**Acceptance criteria**
- [ ] The `llm_calls` row for a stalled turn is sufficient to distinguish
      `finish_reason: length` (output truncation) from `finish_reason: stop`
      (clean narrate-then-stop) without reproducing the session.
- [ ] The runbook query for this is written down in `design.md` and confirmed to
      work against a real stalled session.

### US-3 — opencode is never handed a token limit Helix does not know
`api/cmd/settings-sync-daemon/opencode.go:125-129` guards with
`MaxTokens > 0 || MaxOutputTokens > 0` but writes **both** fields, and
`openCodeModelLimit.Output` has no `omitempty`. For a self-hosted model that is
absent from `model_info.json` (qwen3.8 is — 0 of 741 entries match) the context
length gets filled from the provider's advertised `/v1/models` value while
`MaxOutputTokens` stays 0, so opencode is told `"limit":{"context":N,"output":0}`.

**Acceptance criteria**
- [ ] `limit.context` is emitted only when `MaxTokens > 0`; `limit.output` only
      when `MaxOutputTokens > 0`; `limit` is omitted entirely when neither is known.
- [ ] A regression test covers the **mixed** case (`MaxTokens > 0`,
      `MaxOutputTokens == 0`). The existing `TestOpenCodeConfigOmitsUnknownLimits`
      only covers both-zero and therefore misses the qwen3.8 case.
- [ ] Behaviour matches the sibling Zed path, which already gets this right via
      `json:"max_output_tokens,omitempty"` (`settings-sync-daemon/main.go:135`).

### US-4 — A verbose-reasoning model does not spend its whole output budget thinking
`qwen3.8-27b`'s curated profile defaults to `xhigh`, the most verbose tier
(`api/pkg/model/reasoning_efforts.go:139-146`). Combined with an output cap the
model cannot see, reasoning can consume the entire budget.

**Acceptance criteria**
- [ ] The effective output cap for a self-hosted Qwen model on opencode is known
      and written down (not left to opencode's hardcoded 32k fallback).
- [ ] If truncation is confirmed as the cause, either the default effort for this
      model on the opencode runtime is lowered, or the output cap is raised so a
      full `xhigh` reasoning pass plus an answer fits.

## Non-Goals

- Changing opencode upstream. The relevant upstream defects are catalogued in
  `design.md` §4; we work around them.
- Any change to the qwen-code runtime. This session used opencode; the two are
  separate code paths.
- Retro-fixing the affected session's transcript.

## Open Questions

1. **Which failure actually occurred — truncation or a clean stop?** This is the
   one thing I could not settle from this machine. The stored response ends
   mid-sentence, which points at output truncation; but a clean
   `finish_reason: stop` after a reasoning-only turn would look nearly identical
   in the transcript. The deciding evidence is the `llm_calls` row for
   `ses_01m07rcre65gy6qrd9s4ykxhpa` on **meta.helix.ml** (production), which I
   have no access to. The query is in `design.md` §5. Please run it, or grant
   access — it determines whether US-4 or US-1 is the load-bearing fix.
2. **Exact model id.** The session shows a Qwen model and the user said
   "qwen3.8". I assumed `qwen3.8-27b`, the only Qwen family in
   `reasoning_efforts.go`. If it was a different build, the effort profile and
   default (`xhigh`) may not apply.
3. **Was a reasoning effort explicitly selected for that session?** I assumed the
   default (`xhigh`) was in force. If the user picked `low`/`medium`, US-4 is much
   weaker as an explanation.
4. **Is `<thinking>` wrapping done by Zed or opencode?** No Go code in Helix emits
   those tags, so they come from the ACP wrapper or opencode. Worth confirming,
   because a renderer that always closes the tag will mask truncation — the
   closing `</thinking>` was present in both turns even though the content inside
   ends mid-sentence.
5. **Should truncation auto-continue?** I assumed "surface it, don't auto-retry",
   since an automatic re-prompt on truncation risks the duplicate-work and
   doom-loop problems Helix already fights elsewhere. Confirm that is the wanted
   behaviour rather than a silent retry.
