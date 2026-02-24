# Implementation Tasks

## CLI Command

- [ ] Create `api/pkg/cli/spectask/revdial_cmd.go` with `revdial` subcommand
- [ ] Add subcommand registration in `spectask.go` `NewCommand()`
- [ ] Implement connection status check via admin endpoint
- [ ] Implement screenshot test with latency measurement
- [ ] Format output with status indicators (✅/❌/⚠️)

## Admin API Endpoint

- [ ] Add `GET /api/v1/admin/revdial/status` handler in `api/pkg/server/admin_handlers.go`
- [ ] Expose `connman.Stats()` plus list of active connection keys
- [ ] Add swagger annotations for OpenAPI docs
- [ ] Require admin authentication

## Integration Test

- [ ] Create `integration-test/smoke/revdial_test.go`
- [ ] Test connection status endpoint returns valid response
- [ ] Test screenshot through RevDial succeeds for active session
- [ ] Add build tag `//go:build integration || revdial`

## Documentation

- [ ] Update `CLAUDE.md` with `spectask revdial` command reference
- [ ] Add example usage in CLI help text

## Verification

- [ ] Run `go build ./api/pkg/cli/spectask/` - compiles without errors
- [ ] Run `./stack update_openapi` - regenerates API client
- [ ] Manual test: `helix spectask revdial <session-id>` returns expected output
- [ ] CI passes on PR