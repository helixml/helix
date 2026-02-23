# Requirements: Testing RevDial Connectivity

## User Stories

**As a developer**, I want a CLI command to test RevDial connectivity to a session, so I can verify the tunnel is working before debugging other issues.

**As a CI system**, I want a machine-readable test output format, so I can automatically detect RevDial failures in pipelines.

## Acceptance Criteria

### CLI Command
- [ ] New `helix spectask revdial <session-id>` command tests RevDial connectivity
- [ ] Tests control connection establishment (WebSocket upgrade)
- [ ] Tests data connection (round-trip through tunnel)
- [ ] Supports `--json` flag for machine-readable output
- [ ] Exit code 0 on success, non-zero on failure

### Test Coverage
- [ ] Verify RevDial control WebSocket is connected
- [ ] Verify screenshot request succeeds through RevDial tunnel
- [ ] Report round-trip latency
- [ ] Clear error messages on failure (connection refused, timeout, auth error)

### Output Format
Human output:
```
RevDial Connectivity Test: ses_01xxx
  ✅ Control connection: connected (hydra-sandbox123)
  ✅ Data tunnel: working (screenshot 45KB in 234ms)
```

JSON output:
```json
{
  "session_id": "ses_01xxx",
  "control_connected": true,
  "data_tunnel_working": true,
  "latency_ms": 234,
  "error": null
}
```

## Out of Scope
- Testing reconnection/grace period behavior (manual test only)
- Load testing the RevDial tunnel
- Testing from outside the network (assumes internal access)