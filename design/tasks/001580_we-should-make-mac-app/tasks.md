# Implementation Tasks

- [ ] Add grace period constants to `for-mac/license.go` or `for-mac/app.go` (`licenseCheckInterval=15m`, `licenseGracePeriod=1h`, `licenseWarnBefore=10m`)
- [ ] Add `startLicenseWatcher(ctx context.Context)` goroutine method to `for-mac/app.go` that periodically calls `CanStartVM()` and manages a grace period timer
- [ ] Wire the watcher to VM state: start watcher when VM enters `VMStateRunning`, cancel it when VM stops (use existing state change callback at `app.go:84-94`)
- [ ] Add `lastStopReason string` field to `App` struct and set it before calling `StopVM()` from the license watcher
- [ ] Send native macOS notification at grace period start ("license expired — VM will stop in 1 hour") using existing `sendNotification()` in `notification_darwin.go`
- [ ] Send native macOS notification at 10-minute warning before forced stop
- [ ] Expose `lastStopReason` to the frontend (via Wails binding or event emit) so the UI can display "VM stopped: license expired"
- [ ] Test: VM keeps running when license is valid during periodic check
- [ ] Test: VM stops after grace period when license is expired and not renewed
- [ ] Test: Grace period is cancelled if user enters valid license key before it elapses
