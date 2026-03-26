# Design: Fix Gzip-Encoded Anthropic Responses

## Root Cause

In `pkg/anthropic/anthropic_proxy.go`, the `anthropicAPIProxyModifyResponse` function reads the raw response body with `io.ReadAll(response.Body)` without checking for gzip compression first. When the Anthropic API returns `Content-Encoding: gzip`, the body bytes start with `0x1f 0x8b` (gzip magic number) rather than valid JSON.

These raw bytes are then:
1. Passed to `respMessage.UnmarshalJSON(resp)` — fails silently (or logs a parse error)
2. Passed to `logStore.CreateLLMCall(...)` which tries to store them as JSONB in PostgreSQL — PostgreSQL rejects the non-UTF8 bytes with SQLSTATE 22021

**Why httputil.ReverseProxy doesn't auto-decompress:** Go's standard HTTP client automatically decompresses gzip when it makes requests, but `httputil.ReverseProxy.ModifyResponse` intercepts the raw upstream response before that layer applies. The `Content-Encoding` header is still present and the body is still compressed at that point.

## Fix

In `anthropicAPIProxyModifyResponse` (non-streaming path), add gzip decompression before reading the body:

```go
// Decompress gzip-encoded responses before reading
if response.Header.Get("Content-Encoding") == "gzip" {
    gr, err := gzip.NewReader(response.Body)
    if err != nil {
        log.Error().Err(err).Msg("failed to create gzip reader for response")
        return err
    }
    defer gr.Close()
    response.Body = io.NopCloser(gr)
    response.Header.Del("Content-Encoding")
    response.Header.Del("Content-Length")
    response.ContentLength = -1
}
```

This mirrors the pattern already used in `pkg/services/git_http_server.go` (`gzipDecompressMiddleware`).

After this block, the existing `io.ReadAll(response.Body)` call reads plain JSON, which parses and stores correctly.

## Key Details

- **File to change:** `pkg/anthropic/anthropic_proxy.go`
- **Function:** `anthropicAPIProxyModifyResponse` (non-streaming path, before `io.ReadAll`)
- **Import needed:** `compress/gzip` (add to imports if not present)
- Remove `Content-Length` header too — after decompression the length changes and an incorrect value would corrupt the response to the caller
- The streaming path assembles text from SSE events so is not affected by response-level gzip
