# Implementation Tasks

- [x] In `pkg/anthropic/anthropic_proxy.go`, add `compress/gzip` to imports if not already present
- [x] In `anthropicAPIProxyModifyResponse`, before `io.ReadAll(response.Body)`, add gzip decompression: check `Content-Encoding: gzip` header, wrap body with `gzip.NewReader`, remove `Content-Encoding` and `Content-Length` headers
