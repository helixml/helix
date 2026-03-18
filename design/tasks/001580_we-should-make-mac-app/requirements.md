# Requirements: Mac App License Expiry VM Shutdown

## Problem

The macOS Helix Desktop app only checks the license at VM startup. If a license expires while the VM is running, the VM continues indefinitely — the user is never notified and the VM is never shut down.

## User Stories

**As a user with an expiring license:**
- I want to receive a warning before my license expires so I can renew in time
- I want a grace period after expiry before the VM is forcibly stopped
- I want a clear notification telling me why the VM is shutting down

**As a user with an expired license:**
- I want the VM to be stopped automatically after expiry (plus grace period)
- I want to be able to restart once I've entered a valid license key

## Acceptance Criteria

1. **Periodic license check**: The macOS app periodically re-validates the license while the VM is running (e.g. every 15 minutes).

2. **Expiry warning**: When a license is detected as expired (or within a short window of expiry), the user receives a native macOS notification warning them.

3. **Grace period**: After expiry is detected, the VM continues running for a configurable grace period (suggested: 1 hour) before being shut down.

4. **Countdown notification**: A second native notification is sent when the grace period is running down (e.g. "VM will stop in 10 minutes").

5. **VM shutdown on expiry**: After the grace period elapses with no valid license, the VM is stopped via the normal graceful shutdown path (`Stop()`).

6. **User feedback**: The UI clearly shows why the VM was stopped (license expired), not just a generic stopped state.

7. **License renewal unblocks**: If the user enters a valid license during the grace period, the periodic check detects it and the countdown is cancelled — the VM keeps running.

## Out of Scope

- Changes to the server-side license manager (it already has 1-hour periodic checks)
- Changes to how license keys are entered or validated
- Changes to the 10-day grace period that already exists for expired-but-within-grace licenses
