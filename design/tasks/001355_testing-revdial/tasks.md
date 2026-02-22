# Implementation Tasks

## Documentation

- [ ] Document existing RevDial test commands in `api/pkg/cli/spectask/README.md`
- [ ] Add troubleshooting section for common RevDial failures

## CLI Improvements

- [ ] Add `--verbose` flag to `spectask screenshot` to show connection timing
- [ ] Add `spectask revdial-status` command to show `connman.Stats()` for a session

## Observability

- [ ] Expose `connman.Stats()` via health endpoint (e.g., `/api/v1/health/revdial`)
- [ ] Add latency histogram metric for RevDial dial time

## Verification

- [ ] Test screenshot command against running session
- [ ] Test stream command with FPS measurement
- [ ] Verify error messages are clear when sandbox is not connected