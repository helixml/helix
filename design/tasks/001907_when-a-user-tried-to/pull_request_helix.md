# Fix non-JSON 4xx error parsing for inference providers

## Summary

When a user configured NVIDIA (or any OpenAI-compatible) provider with a bad
API key, the upstream `401 Unauthorized` response with a plain-text body was
being routed through `reasoningFieldMapper`, which tried to JSON-parse it and
crashed with `invalid character 'U' looking for beginning of value` — hiding
the real upstream message.

After this fix, the user sees the actual provider error:

```
failed to get models from '...' provider: 401 Unauthorized - Unauthorized: Invalid API Key
```

## Changes

**Backend (root-cause fix):**
- `api/pkg/openai/openai_client.go` — `openAIClientInterceptor.Do()`: only wrap response bodies in `reasoningFieldMapper` for 2xx status codes. Error bodies pass through unmodified so go-openai can surface them to the caller.
- `api/pkg/openai/openai_client.go` — `listOpenAIModels()`: include the response body text in the non-200 error message (matches the pattern already used by `listAnthropicModels()`).
- `api/pkg/openai/openai_client_test.go` — three new tests:
  - `TestInterceptor_NonJSON401BodyPassesThrough`
  - `TestInterceptor_2xxBodyIsWrapped`
  - `TestListModels_NonJSON401_IncludesUpstreamBody`

**Frontend (so end users actually see the fixed error):**
- `frontend/src/pages/Providers.tsx` — load endpoints with `with_models=true` so `status` and `error` populate; render a red Alert with the upstream error message and a red "Fix Connection" button on tiles where `status === 'error'`. Applies to both predefined providers and custom OpenAI-compatible endpoints.

## Screenshots

Broken provider tile after the fix (NVIDIA NIM with a wrong API key):
![Provider tile with error](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001907_when-a-user-tried-to/screenshots/03-error-tile-closeup.png)

## Test plan

- [x] `go test ./api/pkg/openai/` — all tests pass, including 3 new ones
- [x] End-to-end: fake provider on port 9878 returning 401 + plain-text body, configured via `/api/v1/provider-endpoints`, queried via `/v1/models?provider=...`. Got the new self-diagnosing error string instead of the JSON parse crash. Output captured in `helix-specs/design/tasks/001907_when-a-user-tried-to/screenshots/manual-test-output.txt`.
