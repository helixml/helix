# Implementation Tasks

## Unit Tests for revdial Package

- [ ] Create `api/pkg/revdial/revdial_test.go`
- [ ] Add `TestExtractHostAndTLS` - test URL parsing for http://, https://, ws://, wss:// schemes
- [ ] Add `TestExtractHostAndTLS_DefaultPorts` - verify :80/:443 defaults when port not specified
- [ ] Add `TestControlMessageSerialization` - verify JSON marshal/unmarshal of `controlMsg` struct
- [ ] Add `TestDialerUniqID` - verify unique ID generation for dialers

## Verify Existing Test Coverage

- [ ] Confirm `connman_test.go` covers grace period reconnection scenarios
- [ ] Confirm `connman_test.go` covers context cancellation during pending dials
- [ ] Document test coverage in this file after verification

## CLI Connectivity Testing

- [ ] Verify `helix spectask screenshot <session-id>` works with active session
- [ ] Verify clear error message when session not found
- [ ] Verify clear error message when RevDial connection timeout occurs
- [ ] Document CLI testing steps in design/testing-revdial.md (optional)

## Integration Test (Optional Enhancement)

- [ ] Add RevDial-specific test to `integration-test/smoke/` if needed
- [ ] Test should start session, wait for RevDial, call screenshot endpoint
- [ ] Test should verify 200 response and non-empty image data