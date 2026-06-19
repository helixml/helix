# Implementation Tasks: Prevent git config Lock Contention During Container Startup

- [ ] In `syncGitIdentityToUser` (`api/pkg/services/spec_driven_task_service.go`), add a check for `~/.helix-setup-complete` via `ExecInDesktop` before running any `git config --global` commands; return `nil` (skip) if the file is absent
- [ ] Add a unit test in `spec_driven_task_service_test.go` covering the case where `ExecInDesktop` returns an error on the setup-complete check (simulating setup still in progress) — verify the function returns nil and does not call further exec commands
- [ ] Verify manually: touch `/home/retro/.helix-setup-complete` removal, trigger a phase transition, confirm no `git config` lock error in the setup terminal
