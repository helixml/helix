# Implementation Tasks

## RevDial Package Tests (`api/pkg/revdial/revdial_test.go`)

- [ ] Create test file with testify/suite setup
- [ ] TestDialerListenerRoundtrip: verify bidirectional data flow through tunnel
- [ ] TestDialerClose: verify Done() channel signals on close
- [ ] TestListenerAccept: verify listener accepts dialer connections
- [ ] TestMultipleConnections: verify multiple logical connections over one tunnel
- [ ] TestControlMessages: verify keep-alive and conn-ready messages work

## Connection Manager Tests (`api/pkg/connman/connman_test.go`)

- [ ] Create test file with testify/suite setup
- [ ] TestSetAndDial: basic connection registration and dialing
- [ ] TestGracePeriodQueue: pending dials queue during disconnection
- [ ] TestGracePeriodReconnect: queued dials complete on reconnect
- [ ] TestGracePeriodExpiry: cleanup after grace period expires
- [ ] TestMaxPendingDials: verify limit is enforced
- [ ] TestList: verify List() returns active keys

## Verification

- [ ] Run `go test ./api/pkg/revdial/...` - all tests pass
- [ ] Run `go test ./api/pkg/connman/...` - all tests pass
- [ ] Run `go build ./api/pkg/revdial/ ./api/pkg/connman/` - no compile errors
- [ ] Tests complete in <30 seconds total