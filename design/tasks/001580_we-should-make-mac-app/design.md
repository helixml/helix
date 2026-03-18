# Design: Mac App License Expiry VM Shutdown

## Architecture Overview

Add a periodic license checker goroutine in `for-mac/app.go` that runs while the VM is running. When the license expires, it starts a grace period timer and sends native notifications. If no valid license is found after the grace period, it calls `app.StopVM()`.

## Key Files

- `for-mac/app.go` — Main app lifecycle; add `startLicenseWatcher()` method here
- `for-mac/license.go` — `CanStartVM()` and `GetLicenseStatus()`; reuse for periodic checks
- `for-mac/notification_darwin.go` — Native macOS notifications; use for warnings
- `for-mac/vm.go` — `Stop()` for graceful VM shutdown (QMP + 5s grace + force kill)

## Implementation Design

### Periodic License Watcher

Add a `startLicenseWatcher(ctx context.Context)` method in `app.go`:

```
goroutine:
  ticker := time.NewTicker(15 * time.Minute)
  graceTimer := nil
  warningFired := false

  on tick:
    if VM is not running → continue (skip check)
    err := licenseValidator.CanStartVM(settings)
    if err == nil:
      // License valid — cancel any active grace timer
      if graceTimer != nil { graceTimer.Stop(); graceTimer = nil; warningFired = false }
    else if graceTimer == nil:
      // License just expired — start grace period
      sendNotification("License expired — VM will stop in 1 hour")
      graceTimer = time.AfterFunc(graceCountdown, func() { app.StopVM("license expired") })
    else if !warningFired && timeUntilGraceEnd < 10 minutes:
      sendNotification("VM stopping in 10 minutes — license expired")
      warningFired = true
```

### Grace Period Constants

Define in `license.go` or `app.go`:
```go
const (
    licenseCheckInterval = 15 * time.Minute
    licenseGracePeriod   = 1 * time.Hour
    licenseWarnBefore    = 10 * time.Minute
)
```

### Watcher Lifecycle

- Start watcher after VM transitions to `VMStateRunning` (wire into existing VM state change callback at `app.go:84-94`)
- Cancel watcher context when VM is stopped (or app shuts down)
- If a new VM start is triggered, start a fresh watcher

### StopVM Reason

Pass a reason string to `StopVM` (or emit an event before calling it) so the frontend can display "VM stopped: license expired" instead of a generic stopped state. The simplest approach is to set a field `app.lastStopReason` before calling `StopVM()`, which the frontend can read.

### Notifications

Reuse `sendNotification()` from `notification_darwin.go`. Two notifications:
1. On grace period start: "Helix license expired — VM will stop in 1 hour. Renew your license to keep it running."
2. At 10-minute warning: "Helix VM stopping in 10 minutes — enter a valid license to cancel."

## Key Decisions

- **15-minute check interval**: Frequent enough to be timely, infrequent enough to not spam validation. Matches the general UX expectation.
- **1-hour grace period**: Gives users time to renew without immediate disruption. Distinct from the existing 10-day expiration grace (which is for clock skew / network issues after a license's `valid_until` date).
- **Reuse `CanStartVM()`**: No new validation logic needed; existing function already handles all states (trial, licensed, expired).
- **Wire into VM state change**: Avoids polling VM state separately; use the existing state change callback already in place.
- **Watcher goroutine with cancellable context**: Clean shutdown; cancelled when VM stops or app exits.

## Codebase Patterns Found

- VM state changes are wired via callback at `app.go:84-94`; this is where to hook the watcher start/stop
- `notification_darwin.go` provides `sendNotification(title, body string)` — already used for VM ready events
- `app.go:185-189` shows the pattern for calling `CanStartVM()` — same call reused in watcher
- The server-side `manager.go:22` uses the same ticker pattern (`time.NewTicker`) for 1-hour periodic checks
