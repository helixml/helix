# Fork-and-pause: validate, harden, ship the full UX

> Started as validation of spec 002081's fork-and-pause work; grew into hardening to address every issue uncovered by manual testing. See [`pull_request_helix.md`](./pull_request_helix.md) for the full per-repo description with commit list, test coverage, and validation outcome.

## What ships

- **Confirm modal** with workspace dirty-state detection + per-repo file list + destination-branch label
- **Pre-fork commit + push safety net** (with branch recovery for the common "still on main" case)
- **Auto-handoff turn** so the new agent acknowledges full context before the user types
- **Full session-state inheritance** (15 metadata fields + interaction copies + branch checkout) so the fork is a true continuation, not a fresh shell
- **Quota hygiene**: parent desktop stopped after fork to free the `max_concurrent_desktops` slot
- **UI gating**: when the safety net has no viable target (legacy session, no spec task), block the Fork button rather than letting the user hit a confusing remote-side error
