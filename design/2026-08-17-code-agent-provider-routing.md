# Code-agent provider routing

## Failure

Task `spt_01m065j93gbaxyydnaqk6qvk0v` persisted this execution config:

```json
{
  "runtime": "codex_cli",
  "credential_type": "api_key",
  "provider_ref": "pe_01kzpnf69hf73basd52k942vs3",
  "model": "qwen3.8-27b"
}
```

The provider is `ds4-flash-node06`, not OpenAI. The running container had the
Helix proxy URL in `OPENAI_BASE_URL`, but current Codex uses its user-level
`openai_base_url` configuration for the built-in OpenAI provider. It therefore
sent the session-scoped Helix key to `api.openai.com` and received a 401.

Helix also does not currently expose `/v1/responses`, so fixing Codex's base URL
alone would route the request to a missing endpoint.

## Contract

- Claude Code API-key mode accepts only the canonical `anthropic` provider.
- Codex API-key mode accepts only the canonical `openai` provider.
- Other harnesses retain the general OpenAI-compatible provider selection.
- The organization settings page controls whether the compatible provider and
  subscription sources are enabled. Models remain task-owned.
- Current Claude models are Opus 5 and Fable 5. Current Codex models are the
  GPT-5.6 Sol, Terra, and Luna variants. Older models remain selectable under a
  collapsed Legacy models section.

## Routing

- Settings sync registers a custom `helix` Codex model provider for API mode,
  using the Responses wire protocol and the Helix `/v1` base URL. WebSocket
  transport is disabled because Helix must inspect the terminal response usage
  before logging and billing it. Subscription mode removes only the managed
  Helix provider and leaves the user's other Codex configuration intact.
- `/v1/responses` authenticates the session-scoped Helix key, resolves the
  task-owned provider reference, verifies the canonical OpenAI provider and
  exact task model, checks balance, and proxies HTTP/SSE traffic with the
  configured upstream provider key. Final usage is recorded in LLM calls and
  usage metrics; billable global providers debit the owning wallet.
- `/v1/messages` resolves the task-owned provider reference before legacy
  project-App fallback, so Claude Code routes to the provider selected for the
  task rather than a removed App default.

## Verification

- Unit tests cover provider compatibility, model grouping, Codex config, and
  proxy routing/authentication.
- The inner Helix browser run created task `spt_01m06cbby92262h8bahs3h2f8a`
  with Codex, OpenAI, and `gpt-5.6-sol`. Its new desktop used image `be0fc2`
  and a `helix` model-provider stanza pointing at `http://api:8080/v1`, with
  Responses transport over HTTP/SSE and WebSockets disabled.
- The run completed through Helix without an `api.openai.com` request. Its
  `LLMCall` has no error and its usage metric records 9,996 input tokens, 10
  output tokens, and the calculated cost.
