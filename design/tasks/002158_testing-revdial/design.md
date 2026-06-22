# Design: End-to-End Tests for RevDial Connectivity

## Architecture overview

```
[Test HTTP backend] ←── ProxyConn ──← [revdial.Listener (Client)]
                                              │ WebSocket (control + data)
                                       [Test HTTP server (httptest)]
                                              │ connman.Set(key, conn)
                                        [connman.ConnectionManager]
                                              │ connman.Dial(ctx, key)
                                       [revdial.Dialer.Dial(ctx)]
                                              │ net.Conn
                               [Test: http.Request through the conn]
```

The full chain already exists in production. These tests wire up the same components without the Helix API server or authentication middleware.

---

## File locations

| File | What goes there |
|------|----------------|
| `api/pkg/revdial/client_test.go` | Unit tests for `ExtractHostAndTLS`, `DialLocal`, `ProxyConn` |
| `api/pkg/revdial/connectivity_test.go` | Integration tests: full round-trip, reconnection |

We stay within the `revdial` package so tests have access to unexported helpers if needed, and we avoid introducing a separate `integration` build tag for a small set of tests that run in < 5 s.

---

## Key decisions

### Test server design (US-1, US-3)
Use `httptest.NewServer` with a custom `http.Handler` that:
1. Upgrades the WebSocket for both control and data connections (differentiate by presence of `revdial.dialer` query param — same logic as `server.go:2568`).
2. For control connections: wraps the WebSocket with `wsconnadapter.New()` and calls `connman.Set(runnerID, conn)`.
3. For data connections: delegates to `revdial.ConnHandler(upgrader)`.

This mirrors `handleRevDial()` in `server.go` without the auth/session logic.

### Import of `connman`
The integration test imports `github.com/helixml/helix/api/pkg/connman` (the standalone package, not `api/pkg/server/connman` which is the older thin wrapper). The standalone `connman` package already has full grace-period support.

### No build tags
All new tests use `//go:build !race` only where timing is sensitive (US-3 reconnect test). Otherwise they are plain `_test.go` files included in normal `go test ./pkg/revdial/...`.

### Reconnect delay
`revdial.ClientConfig.ReconnectDelay` is exposed, so US-3 can set it to 50 ms without modifying production code.

### ProxyConn test (US-4)
Two `net.Pipe()` pairs suffice. No goroutine leak: `ProxyConn` exits when both sides close. Assert with a `WaitGroup` or channel.

---

## What's NOT in scope
- Testing authentication (bearer token validation) — that's the server middleware layer, not RevDial.
- Testing the Hydra container-discovery side effect triggered by `hydra-` runner IDs.
- Load or performance tests.
- Changes to production code — tests only.
