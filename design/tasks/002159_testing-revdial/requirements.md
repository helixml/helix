# Requirements: Integration Tests for RevDial Connectivity

## Background

RevDial is the reverse-dial mechanism that lets sandboxed containers (Hydra daemons, desktop-bridge processes) connect outward to the Helix API server through NAT, and allows the API to dial back into those containers. It is the primary communication path for screenshots, exec, MCP, and inference proxying.

Current unit tests cover the `revdial` package internals and the connection manager (`connman`) in isolation. There are no integration tests that exercise the full round-trip: client connects → API registers the runner → API dials back → HTTP request arrives at the local service.

## User Stories

**As a developer**, I want an integration test that spins up a real RevDial server and client and sends an HTTP request end-to-end, so that regressions in the WebSocket upgrade path, ping keepalive, or data-connection routing are caught before they reach production.

**As a CI engineer**, I want the test to run with `go test ./...` and complete in under 10 seconds, so it does not slow down the build pipeline.

**As an on-call engineer**, I want the test to cover reconnection (client disconnect and reconnect), so that the grace-period and reconnect-wakeup logic in `connman` is validated together with the real RevDial dialer.

## Acceptance Criteria

1. An integration test in `api/pkg/revdial/` (or a sub-package) starts an in-process HTTP server that acts as the RevDial server endpoint, a `revdial.Client` connecting to it, and a tiny local HTTP service the client proxies to.
2. The test dials through the server → client → local service and asserts the HTTP response body is correct.
3. A second test case disconnects the client, reconnects, and verifies the API can dial again after reconnection (validates `connman` grace-period wakeup with a real RevDial dialer).
4. All tests pass with `go test -race ./api/pkg/revdial/...` and complete in < 10 s.
5. No new external dependencies are introduced (use `net/http/httptest` and `net.Pipe`/`net.Listener` primitives already used in the package).
