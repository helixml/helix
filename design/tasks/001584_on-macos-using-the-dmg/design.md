# Design: QEMU Process Cleanup on Helix Quit (macOS)

## Architecture Context

- `for-mac/vm.go` — QEMU lifecycle: `Start()` → `runVM()` → `Stop()`
- `for-mac/app.go` — Wails app lifecycle; `shutdown()` callback is the teardown hook
- `for-mac/main.go` — Wails `OnShutdown: app.shutdown` wired here
- QEMU is started as a child process via `exec.CommandContext(ctx, qemuPath, args...)`
- VM state machine: `stopped → starting → running → stopping → stopped`

## Fix 1: Make `shutdown()` kill QEMU in all non-stopped states

**File:** `for-mac/app.go`, `shutdown()` function.

Change the guard from checking `VMStateRunning` to a general "process may be alive" check.
Add a new `vm.ForceStop()` method (or extend `Stop()`) that bypasses the state guard and
directly cancels the context + kills the process.

```go
// Current (broken):
if a.vm.GetStatus().State == VMStateRunning {
    a.vm.Stop()
}

// Fixed:
a.vm.ForceStop()  // kills QEMU regardless of state
```

`ForceStop()` in `vm.go`:
```go
func (vm *VMManager) ForceStop() {
    if vm.cancelFunc != nil {
        vm.cancelFunc()
    }
    if vm.cmd != nil && vm.cmd.Process != nil {
        vm.cmd.Process.Kill()
    }
}
```

No grace period needed here — this is called at app shutdown, speed matters.
The existing graceful `Stop()` (with QMP `system_powerdown` + 5s wait) remains for
user-initiated "Stop Environment" actions.

## Fix 2: Use a process group so QEMU dies with Helix

**File:** `for-mac/vm.go`, in `runVM()` where `vm.cmd` is configured.

Set `Setpgid: true` so QEMU gets its own process group, then kill the whole group
on shutdown instead of just the process:

```go
vm.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
```

In `ForceStop()`, kill the process group:
```go
if vm.cmd != nil && vm.cmd.Process != nil {
    pgid, err := syscall.Getpgid(vm.cmd.Process.Pid)
    if err == nil {
        syscall.Kill(-pgid, syscall.SIGKILL)
    } else {
        vm.cmd.Process.Kill()
    }
}
```

This also handles any child processes QEMU may spawn (though QEMU typically doesn't).

## Fix 3: Improve `killStaleQEMU()` to use a PID file

**File:** `for-mac/vm.go`.

Currently `killStaleQEMU()` detects orphans by probing the QMP TCP port, then uses `lsof`
to find the PID. This is fragile (port may not be bound yet, `lsof` is slow).

Better approach: write the QEMU PID to `~/.helix/qemu.pid` immediately after `cmd.Start()`,
and delete it after `cmd.Wait()` returns. On startup, `killStaleQEMU()` reads the PID file
and kills the process directly (with a fallback to the existing lsof approach).

```go
// After cmd.Start():
pidFile := vm.getPIDFilePath()
os.WriteFile(pidFile, []byte(strconv.Itoa(vm.cmd.Process.Pid)), 0644)

// After cmd.Wait():
os.Remove(pidFile)
```

In `killStaleQEMU()`:
```go
if data, err := os.ReadFile(vm.getPIDFilePath()); err == nil {
    if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
        if proc, err := os.FindProcess(pid); err == nil {
            proc.Kill()
        }
        os.Remove(vm.getPIDFilePath())
    }
}
```

## Decision: No Pdeathsig on macOS

macOS does not expose `PROC_PDEATHSIG` through Go's `syscall.SysProcAttr`. The `procctl`
system call exists on macOS 12+ but is not in Go stdlib. Using process groups (Fix 2)
is the correct portable solution — it handles the force-kill case by cleaning up at
next launch via PID file (Fix 3).

## Files Changed

| File | Change |
|------|--------|
| `for-mac/app.go` | `shutdown()`: call `vm.ForceStop()` unconditionally |
| `for-mac/vm.go` | Add `ForceStop()`, set `Setpgid`, write/delete PID file, improve `killStaleQEMU()` |
