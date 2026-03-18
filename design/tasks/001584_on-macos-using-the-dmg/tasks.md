# Implementation Tasks

- [x] Add `ForceStop()` method to `VMManager` in `for-mac/vm.go` that cancels the context and kills the QEMU process/process-group without checking state
- [x] Set `vm.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` in `runVM()` so QEMU gets its own process group, and use `syscall.Kill(-pgid, syscall.SIGKILL)` in `ForceStop()`
- [x] Write QEMU PID to `~/.helix/qemu.pid` after `cmd.Start()` and delete it after `cmd.Wait()` returns
- [x] Update `killStaleQEMU()` to read the PID file first (direct kill), with existing lsof logic as fallback
- [x] In `app.go` `shutdown()`: replace `if state == VMStateRunning { vm.Stop() }` with unconditional `vm.ForceStop()`
- [x] Move `vm.ForceStop()` to the FIRST line of `shutdown()` — before tray.Stop() which can deadlock on main thread
- [x] Add SIGTERM/SIGINT handler in main.go as belt-and-suspenders for cases where shutdown() isn't reached
- [ ] Test: quit Helix while VM is in `VMStateRunning` — no qemu process in `ps`
- [ ] Test: quit Helix while VM is in `VMStateStarting` — no qemu process in `ps`
- [ ] Test: force-kill Helix (`kill -9`), relaunch — stale QEMU is cleaned up on relaunch
