# Helix as Inference Provider

**Date:** 2026-03-05

## Problem

When running Helix-in-Helix (inner dev stack inside an outer production Helix), the inner Helix needs to use the outer Helix for LLM inference. Two things must work:

1. **Model discovery** — `/v1/models` must return models from all configured providers (OpenAI, TogetherAI, Nebius, etc.), not just the default helix provider.
2. **Anthropic passthrough** — Anthropic models must go through the native `/v1/messages` endpoint to preserve prompt caching. The OpenAI-compatible `/v1/chat/completions` path doesn't support `cache_control`.

## How it works

The inner Helix configures two providers pointing at the outer Helix:

```
OPENAI_API_KEY=<outer-helix-api-key>
OPENAI_BASE_URL=http://outer-helix:8080/v1

ANTHROPIC_API_KEY=<outer-helix-api-key>
ANTHROPIC_BASE_URL=http://outer-helix:8080
```

### Zed agent (critical path)

Zed talks native Anthropic protocol. The chain is:

```
Zed → inner Helix /v1/messages → outer Helix /v1/messages → api.anthropic.com
```

Prompt caching is preserved end-to-end. No model prefixing, no protocol translation.

### Optimus PM agent

The project manager agent uses the go-openai client, which sends to `/v1/chat/completions`. The outer Helix routes unprefixed model names (e.g. `claude-haiku-4-5-20251001`) by checking all providers' cached model lists via `findProviderWithModel`.

### Model discovery

**`/v1/models` on the outer Helix** does two things depending on request headers:

- **`anthropic-version` header present** → proxies to upstream Anthropic API via the existing `anthropicProxy` reverse proxy. Returns native Anthropic model list format.
- **No Anthropic header** → aggregates models from all OpenAI-compatible providers' caches, prefixed with provider name (`openai/gpt-4o`, `togetherai/meta-llama/...`). Anthropic models are excluded (served via the Anthropic path). Helix internal models included unprefixed.

The `prefixModels` helper skips models that already contain `/` to avoid double-prefixing when an upstream Helix has already prefixed them.

## Changes

| File | What |
|------|------|
| `openai_model_handlers.go` | `listModels` detects `anthropic-version` header, proxies or aggregates. `listModelsAnthropic` forwards via anthropic proxy. `getCachedModels` / `prefixModels` helpers. |
| `openai_client_anthropic.go` | `listAnthropicModels` uses `c.baseURL` instead of hardcoded `api.anthropic.com`. Removed hardcoded 200K context length. |
| `openai_client.go` | `isAnthropic` flag on `RetryableClient` — set at config time, used to select Anthropic model listing format. |
| `provider_manager.go` | Sets `isAnthropic` on the Anthropic client. Appends `/v1` to `ANTHROPIC_BASE_URL` for the go-openai client path. |
| `anthropic_proxy.go` | Removed `r.Body == nil` guard that blocked GET requests like `/v1/models`. |
| `openai_chat_handlers.go` | `findProviderWithModel` checks all models, not just slash-containing ones. Routes unprefixed `claude-*` to anthropic provider. |

## Docker-in-Docker networking

Inner containers can't reach the outer Helix directly (different Docker network, and port 8080 is shadowed by the inner API). A `socat` proxy on port 8081 on the desktop container bridges the gap:

```
socat TCP4-LISTEN:8081,fork,reuseaddr TCP4:<outer-api-ip>:8080
```

Inner Helix uses `http://host.docker.internal:8081` as its base URL. The startup script (`helix-specs/.helix/startup.sh`) sets this up automatically.