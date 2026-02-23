# Implementation Tasks

## Manual Testing

- [ ] Start a spec task session with `helix spectask start --project $HELIX_PROJECT -n "revdial-test"`
- [ ] Wait 15 seconds for container and RevDial to initialize
- [ ] Run `helix spectask screenshot <session-id>` to verify RevDial connectivity
- [ ] Verify screenshot is saved as valid PNG/JPEG file

## Automated Testing

- [ ] Run `helix spectask test --session <session-id> --desktop` to execute desktop test suite
- [ ] Verify screenshot test passes (checks RevDial → Hydra → screenshot endpoint)
- [ ] Run `helix spectask test --session <session-id> --all` for full test suite

## Debugging (if tests fail)

- [ ] Check API logs: `docker compose -f docker-compose.dev.yaml logs --tail 50 api | grep -i revdial`
- [ ] Check sandbox logs: `docker compose logs --tail 100 sandbox-nvidia 2>&1 | grep -E "revdial|hydra"`
- [ ] Verify connman has registered connection: look for "Registered reverse dial connection" in logs
- [ ] Check Hydra process is running inside sandbox container

## Cleanup

- [ ] Stop test session with `helix spectask stop <session-id>`
