# Implementation Tasks

## Setup

- [ ] Create `api/pkg/revdial/revdial_test.go`
- [ ] Create `api/pkg/revdial/client_test.go`

## Unit Tests - Helper Functions

- [ ] Add `TestExtractHostAndTLS` with table-driven tests for URL parsing (http/https/ws/wss, with/without ports)
- [ ] Add `TestDialLocal` tests for TCP and Unix socket address parsing

## Unit Tests - Client Configuration

- [ ] Add `TestClientConfig_DefaultReconnectDelay` - verify 1 second default
- [ ] Add `TestClient_StartWithoutConfig` - verify graceful skip when unconfigured

## Integration Tests - Listener/Dialer

- [ ] Add `TestDialer_NewDialer` - verify dialer creation and registration
- [ ] Add `TestListener_Accept` - verify listener can accept connections
- [ ] Add `TestConnHandler` - verify HTTP handler upgrades WebSocket and matches dialer

## Integration Tests - Client Lifecycle

- [ ] Add `TestClient_StartStop` - verify clean shutdown via context cancellation
- [ ] Add `TestProxyConn` - verify bidirectional data copying between connections

## Validation

- [ ] Run `go test ./api/pkg/revdial/...` - all tests pass
- [ ] Run `go build ./api/pkg/revdial/` - no build errors