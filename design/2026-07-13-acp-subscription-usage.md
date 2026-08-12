# ACP subscription usage telemetry

Claude Code and Codex subscription sessions call their providers directly, so
Helix's OpenAI-compatible proxy cannot observe those requests. Usage is instead
reported at the ACP boundary when Zed completes a turn.

Zed adds the ACP agent identifier and the prompt response's per-turn token usage
to `message_completed`. Helix records that completion through the existing
`UsageLogger` and `usage_metrics` reporting path only when the app assistant uses
subscription credentials. API-key agents continue to be measured at the Helix
proxy, avoiding duplicate metrics.

ACP implementations that do not return token usage still create a metric with
`usage_known=false`. This preserves request and activity reporting without
representing missing token counts as measured usage. Subscription metrics have
zero cost because Helix does not observe or calculate the provider subscription
charge.

WebSocket delivery can be replayed. The `(source, source_id)` unique index and
conflict handling make each session interaction idempotent.

Model attribution comes from the configured Helix assistant. ACP currently does
not expose a portable per-prompt selected-model identifier to this sync path.
