# Requirements: Fix Error-Response Parsing for Inference Providers

## User Story

As a Helix user configuring an NVIDIA (or any OpenAI-compatible) inference provider, when I enter an invalid API key, I should see a clear error message like `401 Unauthorized: Invalid API Key` — not a cryptic JSON parsing crash like `invalid character 'U' looking for beginning of value`.

## Problem

The HTTP interceptor in `openai_client.go` wraps **every** response body — including 4xx error responses — in a `reasoningFieldMapper` that attempts JSON parsing line-by-line. When a provider returns a non-JSON body (e.g. plain text "Unauthorized" or an HTML error page), the JSON parser crashes, hiding the real upstream error.

**Error chain:**
1. User configures NVIDIA provider with a bad API key
2. NVIDIA API returns `HTTP 401` with plain-text body (e.g. "Unauthorized")
3. `openAIClientInterceptor.Do()` wraps the body in `reasoningFieldMapper` (line ~722) — no status code check
4. `reasoningFieldMapper.Read()` calls `renameReasoningField("Unauthorized")`
5. `json.Unmarshal` on plain text produces: `invalid character 'U' looking for beginning of value`
6. The go-openai library receives the garbled error instead of the real 401 message

## Acceptance Criteria

- [ ] When an inference provider returns a 4xx/5xx with a non-JSON body, Helix surfaces the HTTP status code and raw body text in the error message
- [ ] The `reasoningFieldMapper` wrapper is only applied to successful (2xx) responses
- [ ] A 401 from any OpenAI-compatible provider produces an error message containing the status code (401) and the provider's response text
- [ ] Existing JSON error responses from providers (e.g. OpenAI's standard `{"error": {...}}` format) continue to work correctly
- [ ] No regressions in streaming or non-streaming chat completions for valid API keys
