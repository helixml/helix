# Requirements: Fix Gzip-Encoded Anthropic Responses Failing LLM Call Logging

## Problem

When the Anthropic API returns a gzip-compressed response (`Content-Encoding: gzip`), the proxy fails to log the LLM call with:

```
ERR pkg/anthropic/anthropic_proxy.go:494 > failed to log LLM call
error="ERROR: invalid byte sequence for encoding \"UTF8\": 0x8b (SQLSTATE 22021)"
```

`0x8b` is the second byte of the gzip magic number (`0x1f 0x8b`). The proxy reads the raw compressed bytes and tries to store them directly as JSONB in PostgreSQL, which fails because gzip binary data is not valid UTF-8 JSON.

## User Stories

**As a platform operator**, I want LLM calls to always be logged successfully, regardless of whether the Anthropic API responds with gzip encoding, so that usage tracking and auditing works reliably.

## Acceptance Criteria

- [ ] When the Anthropic API returns a `Content-Encoding: gzip` response, the proxy decompresses it before parsing or logging
- [ ] The LLM call logging no longer produces `invalid byte sequence for encoding "UTF8"` errors for gzip responses
- [ ] The response forwarded to the original caller is still correct (properly decompressed)
- [ ] Non-gzip responses continue to work unchanged
