# Fix gzip-encoded Anthropic responses causing LLM call logging failures

## Summary

When the Anthropic API returns a gzip-compressed response (`Content-Encoding: gzip`), `httputil.ReverseProxy.ModifyResponse` receives the raw compressed body. The proxy was reading these bytes without decompressing them first, causing:

1. JSON parsing failure (gzip magic bytes `0x1f 0x8b` are not valid JSON)
2. PostgreSQL JSONB storage rejection with `SQLSTATE 22021: invalid byte sequence for encoding "UTF8": 0x8b`

## Changes

- Added `compress/gzip` import to `pkg/anthropic/anthropic_proxy.go`
- In `anthropicAPIProxyModifyResponse` (non-streaming path), check for `Content-Encoding: gzip` before `io.ReadAll` and wrap the response body with `gzip.NewReader`
- Remove `Content-Encoding` and `Content-Length` headers after decompression so the forwarded response is correct
