# Implementation Tasks

## Documentation & Test Scripts

- [ ] Create `docs/testing-revdial.md` with step-by-step testing guide
- [ ] Document required environment variables (`HELIX_API_KEY`, `HELIX_URL`, `HELIX_PROJECT`)
- [ ] Add troubleshooting section for common RevDial failures

## Manual Testing Checklist

- [ ] Test `helix spectask start --project $HELIX_PROJECT -n "revdial-test"` creates session
- [ ] Test `helix spectask screenshot <session-id>` returns valid JPEG
- [ ] Test screenshot fails gracefully when sandbox is disconnected
- [ ] Test RevDial reconnects after API restart

## CI Integration (Optional)

- [ ] Add RevDial health check to existing CI pipeline
- [ ] Create smoke test script that validates screenshot endpoint
- [ ] Document expected timeouts and retry logic for CI

## Log Verification

- [ ] Verify Hydra logs show `RevDial client started`
- [ ] Verify API logs show `RevDial control connection established`
- [ ] Document log locations and grep patterns for debugging