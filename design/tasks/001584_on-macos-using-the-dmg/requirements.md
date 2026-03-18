# Requirements: QEMU Process Not Killed on Helix Quit (macOS)

## Problem

On macOS (DMG build), the `qemu-system-aarch64` child process continues running after
Helix quits. The user must manually `kill` it or reboot. Confirmed via `ps auxwww | grep qemu`.

## Root Causes Identified

**RC1 — `shutdown()` only stops VM when state is `VMStateRunning`** (`app.go:164`):
```go
if a.vm.GetStatus().State == VMStateRunning {
    a.vm.Stop()
}
```
If the user quits while the VM is booting (`VMStateStarting`), QEMU is already running
(spawned at `vm.go:809`) but `shutdown()` skips cleanup entirely.

**RC2 — `Stop()` refuses to act on non-`VMStateRunning` states** (`vm.go:1579`):
```go
if vm.status.State != VMStateRunning {
    return fmt.Errorf("VM is not running")
}
```
Prevents cleanup during `VMStateStarting` or `VMStateStopping`.

**RC3 — No process-group tie between Helix and QEMU**:
QEMU is started without `SysProcAttr.Setpgid = true`, so if the Helix process itself is
force-killed (crash, `kill -9`, OOM), QEMU is reparented to launchd and keeps running.
macOS has no `Pdeathsig` equivalent, so this cannot be solved at the OS level alone.

**RC4 — `killStaleQEMU()` only runs at VM start, not at app shutdown**:
Stale processes from the current session are not proactively cleaned up.

## User Stories

- As a macOS user, when I quit Helix (via tray "Quit Helix", Cmd+Q, or dock menu),
  the QEMU virtual machine process must terminate within a few seconds.
- As a macOS user, if Helix crashes or is force-killed, the next launch of Helix must
  detect and kill any leftover QEMU process before starting a new one (already partially
  implemented via `killStaleQEMU()`, but only for port conflicts).

## Acceptance Criteria

- [ ] After normal quit (tray "Quit Helix"), no `qemu-system-aarch64` process remains.
- [ ] After quit while VM is still starting (boot phase), no QEMU process remains.
- [ ] After force-kill of Helix (`kill -9`), the next launch cleans up orphaned QEMU.
- [ ] No regression: stopping the VM from the tray still works correctly.
