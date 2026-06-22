# Implementation Tasks: Integration Tests for RevDial Connectivity

- [ ] Create `api/pkg/revdial/integration_test.go` with a shared `startTestServer` helper that hosts both the RevDial hijack handler and `revdial.ConnHandler` on an `httptest.Server`
- [ ] Implement `TestIntegration_BasicRoundTrip`: client connects, API dials through the tunnel, assert HTTP round-trip to local service succeeds
- [ ] Implement `TestIntegration_ReconnectAfterDisconnect`: force-close the control WebSocket, wait for client auto-reconnect, assert a second dial succeeds
- [ ] Verify all tests pass with `go test -race ./api/pkg/revdial/...` and finish under 10 seconds
