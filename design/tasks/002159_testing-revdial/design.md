# Design: Integration Tests for RevDial Connectivity

## Architecture Overview

RevDial works in three layers:

```
[Local service :N]  ←  [revdial.Client (WebSocket to API)]  ←  [revdial.Listener/Dialer on API]  ←  [caller: connman.Dial()]
```

- **Control connection**: the client opens a WebSocket to `GET /api/v1/revdial?runnerid=<id>`. The server hijacks it and calls `revdial.NewDialer`.
- **Data connections**: when the server calls `Dialer.Dial()`, it writes a pickup URL over the control connection; the client opens a second WebSocket to that URL, which becomes a `net.Conn` piped to the local service.
- **connman**: sits in front of the dialer, keyed by runner ID. Implements the grace-period buffer so transient disconnects don't immediately fail in-flight dials.

## Test Design

### Package: `api/pkg/revdial/integration_test.go`

Use a real `net/http` test server (via `httptest.NewServer`) to host both:
- `GET /api/v1/revdial` — the RevDial server endpoint (HTTP hijack + `revdial.NewDialer`)
- `GET /api/v1/revdial` with `revdial.dialer` and `revdial.req` params — the data connection pickup handler (`revdial.ConnHandler`)

The test uses `revdial.Client` (the reusable embedded client from `client.go`) pointing at this test server, with a tiny `net/http` handler as the local service.

**Test 1 — `TestIntegration_BasicRoundTrip`**

1. Start `httptest.Server` with RevDial server handlers.
2. Start a local `net.Listener` that serves `GET /ping → 200 "pong"`.
3. Create and start a `revdial.Client` with `LocalAddr` pointing at the local listener.
4. Wait for the client to register (poll `connman.List()` or use a short `time.Sleep`).
5. `connman.Dial(ctx, runnerID)` → get a `net.Conn`.
6. Write an HTTP `GET /ping` request over the conn, read the response, assert body == `"pong"`.

**Test 2 — `TestIntegration_ReconnectAfterDisconnect`**

1. Same setup as Test 1.
2. Force-close the control WebSocket to simulate a container restart.
3. Wait for client's auto-reconnect loop (< 2 s with default 1 s reconnect delay).
4. Dial again and assert the request succeeds — this validates the `connman` grace-period wakeup with a real dialer.

### Key Implementation Notes

- `revdial.Client` already handles auto-reconnect; the test just needs to trigger a disconnect by closing the server-side hijacked connection.
- The `connman` used in tests must be the real `connman.New()` (not a mock) so the integration is genuine.
- Use `t.Context()` (Go 1.21+) or a manual `context.WithCancel` for clean goroutine shutdown; call `client.Stop()` in `t.Cleanup`.
- The test HTTP server must route based on query params: requests with `revdial.dialer=` go to `revdial.ConnHandler`, others go to the hijack handler.

## File Layout

```
api/pkg/revdial/
  revdial.go           (existing)
  client.go            (existing)
  revdial_test.go      (existing unit tests)
  integration_test.go  (NEW)
```

No new packages or dependencies.
