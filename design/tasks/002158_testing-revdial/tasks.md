# Implementation Tasks: End-to-End Tests for RevDial Connectivity

- [ ] Create `api/pkg/revdial/client_test.go` with unit tests for `ExtractHostAndTLS` (http/https/ws/wss schemes, with and without explicit ports)
- [ ] Add `DialLocal` tests to `client_test.go`: TCP target using a real `net.Listen` listener, and Unix socket target using a temp socket path
- [ ] Add `ProxyConn` test to `client_test.go`: two `net.Pipe()` pairs, bidirectional data written and read, verify no bytes dropped, verify goroutine exits cleanly after close
- [ ] Create `api/pkg/revdial/connectivity_test.go` with a helper `newTestRevDialServer(t)` that starts an `httptest.Server` handling both control and data WebSocket connections (routing by `revdial.dialer` query param), backed by a `connman.ConnectionManager`
- [ ] Add `TestRevDialRoundTrip` in `connectivity_test.go`: start test backend HTTP server → connect `revdial.Client` to test server → wait for `connman` to register connection → dial through tunnel → make HTTP GET → assert response body matches expected
- [ ] Add `TestRevDialReconnect` in `connectivity_test.go`: connect client → server closes control WebSocket → client reconnects (ReconnectDelay=50ms) → make HTTP GET through tunnel → assert success
- [ ] Run `go test ./api/pkg/revdial/... -v -timeout 30s` and confirm all tests pass
