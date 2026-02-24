# Implementation Tasks

## Manual Testing

- [ ] Start a new session: `helix spectask start --project $HELIX_PROJECT -n "revdial-test"`
- [ ] Wait 15 seconds for sandbox to establish RevDial connection
- [ ] Run screenshot test: `helix spectask screenshot <session-id>`
- [ ] Verify screenshot file is saved and contains valid JPEG data

## Automated Testing

- [ ] Run desktop test suite: `helix spectask test --session <session-id> --desktop`
- [ ] Verify all tests pass: screenshot, list_windows, read_screen
- [ ] Run with JSON output: `helix spectask test --session <session-id> --desktop --json`

## Connection Debugging (if failures occur)

- [ ] Check API logs for RevDial state: `docker compose -f docker-compose.dev.yaml logs api | grep -E "revdial|connman"`
- [ ] Check Hydra logs in sandbox: `docker compose exec -T sandbox-nvidia docker logs <container> 2>&1 | grep revdial`
- [ ] Verify runner ID matches: look for `hydra-{sandbox_id}` in both API and sandbox logs
- [ ] Check for authentication errors: `grep -i "unauthorized\|401" in logs`

## Cleanup

- [ ] Stop test session: `helix spectask stop <session-id>`
