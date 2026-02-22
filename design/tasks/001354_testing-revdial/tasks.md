# Implementation Tasks

## Testing RevDial Connectivity

- [ ] Run `helix spectask list` to find an active session with a running sandbox
- [ ] Test screenshot endpoint: `helix spectask screenshot <session-id>`
- [ ] Verify PNG data returned (non-empty response, valid image)
- [ ] Run desktop test suite: `helix spectask test --session <session-id> --desktop`
- [ ] Confirm all tests pass (screenshot, list_windows)

## If No Active Session

- [ ] Start a new session: `helix spectask start --project $HELIX_PROJECT -n "revdial-test"`
- [ ] Wait for sandbox to become ready (~15-30s)
- [ ] Run connectivity tests above
- [ ] Stop session when done: `helix spectask stop <session-id>`

## Debugging Connection Issues

- [ ] Check API logs for RevDial registration: `docker compose logs api | grep -i revdial`
- [ ] Check connman state in logs: `docker compose logs api | grep connman`
- [ ] Verify Hydra started inside sandbox: `docker compose exec sandbox-nvidia docker logs <container> 2>&1 | grep RevDial`
- [ ] Confirm runner ID format matches: `hydra-{session_id}`
