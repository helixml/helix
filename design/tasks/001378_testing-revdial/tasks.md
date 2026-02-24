# Implementation Tasks

## Verification Tasks (No Code Changes Required)

- [ ] Verify `helix spectask screenshot <session-id>` works for an active session
- [ ] Verify `helix spectask test --session <id> --desktop` runs screenshot test
- [ ] Verify `helix spectask test --json` outputs valid JSON for CI parsing
- [ ] Verify Prometheus metric `device_connection_count` tracks connected sandboxes

## Manual Testing Checklist

- [ ] Start a new session: `helix spectask start --project $HELIX_PROJECT -n "revdial-test"`
- [ ] Wait 20 seconds for sandbox to boot and connect
- [ ] Run screenshot test: `helix spectask screenshot ses_xxx`
- [ ] Check sandbox logs: `docker compose exec -T sandbox-nvidia docker logs <container> 2>&1 | grep -i revdial`
- [ ] Check API logs: `docker compose -f docker-compose.dev.yaml logs api | grep -i revdial`

## Documentation Tasks

- [ ] Document RevDial troubleshooting steps in existing design docs
- [ ] Add RevDial test examples to CLI help text if missing