# Design: Fix Error-Response Parsing for Inference Providers

## Root Cause

In `api/pkg/openai/openai_client.go`, the `openAIClientInterceptor.Do()` method (line ~722) unconditionally wraps **every** HTTP response body in a `reasoningFieldMapper`:

```go
if resp != nil && resp.Body != nil {
    resp.Body = &reasoningFieldMapper{ReadCloser: resp.Body}
}
```

The `reasoningFieldMapper` scans the body line-by-line and calls `renameReasoningField()`, which attempts `json.Unmarshal` on each line. When a provider returns a non-JSON 4xx body (e.g. plain text "Unauthorized"), the unmarshal produces `invalid character 'U'` — and this garbled error replaces the real upstream message.

## Fix

### Change 1: Skip `reasoningFieldMapper` for non-2xx responses

In `openAIClientInterceptor.Do()` (~line 721), only wrap successful responses:

```go
if resp != nil && resp.Body != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
    resp.Body = &reasoningFieldMapper{ReadCloser: resp.Body}
}
```

This is the primary fix. Error responses pass through unmodified to the go-openai library, which already handles standard OpenAI JSON error bodies. For non-JSON error bodies, the raw text reaches the caller intact.

### Change 2: Improve error messages in `listOpenAIModels()`

In the same file (~line 458), the non-200 error path currently discards the response body:

```go
if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("failed to get models from '%s' provider: %s", url, resp.Status)
}
```

Read and include the body in the error message so users see what the provider actually said:

```go
if resp.StatusCode != http.StatusOK {
    body, _ := io.ReadAll(resp.Body)
    return nil, fmt.Errorf("failed to get models from '%s' provider: %s - %s", url, resp.Status, string(body))
}
```

### Change 3: Defensive JSON parsing in `renameReasoningField()`

The function already has a guard for lines not starting with `{`:

```go
if len(jsonStr) == 0 || jsonStr[0] != '{' {
    return line
}
```

This is correct — if a non-JSON line somehow reaches it, it returns unchanged. No change needed here; the fix is upstream in Change 1.

## Codebase Patterns & Notes

- **Go project** using the `github.com/sashabaranov/go-openai` library (v1.41.2) for OpenAI-compatible API calls.
- **Provider architecture**: All providers (OpenAI, NVIDIA, Anthropic, Google, TogetherAI, VLLM) go through the same `RetryableClient` with an `openAIClientInterceptor` HTTP round-tripper.
- The `reasoningFieldMapper` exists to rename provider-specific reasoning fields (e.g. `reasoning` → `reasoning_content`) for cross-provider compatibility. It must only process valid JSON streaming responses.
- `listAnthropicModels()` in `openai_client_anthropic.go` already includes the body in error messages — the OpenAI and Google variants should follow that pattern.
- The `CreateFlexibleEmbeddings()` method already handles 401s correctly (reads body, returns status + text). The model listing and streaming paths are the ones that need fixing.

## Scope

Only `api/pkg/openai/openai_client.go` needs changes. Two surgical edits:
1. Add status code check before wrapping response body (~line 722)
2. Include response body in the `listOpenAIModels` error message (~line 458)

No new dependencies, no API changes, no config changes.
