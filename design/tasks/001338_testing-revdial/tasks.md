# Implementation Tasks

## Unit Tests for `revdial` Package

- [ ] Create `api/pkg/revdial/revdial_test.go`
  - [ ] Test `newUniqID()` generates 32-character hex strings
  - [ ] Test `newUniqID()` generates unique values on successive calls
  - [ ] Test `controlMsg` JSON marshaling/unmarshaling for all command types

- [ ] Create `api/pkg/revdial/client_test.go`
  - [ ] Test `ExtractHostAndTLS()` with http:// URLs (returns host:80, useTLS=false)
  - [ ] Test `ExtractHostAndTLS()` with https:// URLs (returns host:443, useTLS=true)
  - [ ] Test `ExtractHostAndTLS()` with ws:// and wss:// URLs
  - [ ] Test `ExtractHostAndTLS()` with explicit ports (preserves port)
  - [ ] Test `DialLocal()` detects unix:// prefix for Unix sockets
  - [ ] Test `DialLocal()` treats non-unix addresses as TCP

## Validation

- [ ] Run `go test ./api/pkg/revdial/...` - all tests pass
- [ ] Run `go build ./api/pkg/revdial/` - package builds cleanly