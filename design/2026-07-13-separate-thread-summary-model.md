# Separate thread-summary model for external-agent compaction (2026-07-13)

## Problem

For external (`zed_agent` framework) sessions, `GenerateZedMCPConfig`
(`api/pkg/external-agent/zed_config.go`) writes every feature-specific model -
`default_model`, `inline_assistant_model`, `commit_message_model`, and
`thread_summary_model` - to the **same** model as the main agent.

`thread_summary_model` is the model Zed uses for context compaction (both the
`/compact` command and auto-compaction). Compaction works by sending the whole
thread to the summary model to produce a summary that replaces the old turns.

Because the summary model is pinned to the main model's endpoint, when that
endpoint is overloaded, cold, or unreachable, **compaction fails the same way the
normal turn does** - it is just another large request to the same struggling
endpoint. A thread that has grown too large (or whose endpoint has gone slow) can
therefore never be compacted, and, combined with the fact that every "fresh
thread" path (Restart, Switch Agent) replays the prior transcript, has no
in-product recovery except abandoning it.

This surfaced while investigating a customer whose long-lived sandbox agent thread
started failing: there was no way to shrink the context without losing it.

## Change

Add an opt-in operator override, `HELIX_AGENT_SUMMARY_MODEL` (format
`provider/model`, e.g. `openai/gpt-4o-mini`). When set, `thread_summary_model` is
pointed at that model instead of the main agent model. Unset preserves current
behaviour exactly (summary model == agent model), so this is a safe, additive
change.

The operator is responsible for pointing it at a real, Helix-routable, healthy
model. A malformed value is ignored with a warning and falls back to the current
behaviour.

## Scope / non-goals

- Only `thread_summary_model` is redirected. `inline_assistant_model` and
  `commit_message_model` are also "fast/auxiliary" operations that could use a
  cheap model, but they are left on the main model here to keep the change focused
  on the compaction-recovery gap. Extending the override to them later is trivial.
- This is a global operator env override, not per-agent/per-org config. Per-agent
  selection can come later if needed.
- **This does not, on its own, fully restore compaction.** Helix's external-agent
  sync also does not surface ACP `available_commands`, so the `/compact` command is
  unreachable from the Helix UI, and auto-compaction is gated on the model's context
  window. A healthy summary model is necessary but not sufficient; the ACP-command
  plumbing is tracked separately.

## Testing

- `go build ./pkg/external-agent/` passes.
- Behavioural change is limited to the value written into `thread_summary_model` in
  the generated settings.json; unset env == byte-identical to today.
