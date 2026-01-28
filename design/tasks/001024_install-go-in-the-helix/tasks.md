# Implementation Tasks

- [ ] Add `ensure_go()` function to `helix/stack` script
  - Extract Go version from `api/go.mod` using grep/awk
  - Check if that Go version is already installed via `go version`
  - Download official Go tarball from `go.dev/dl/` if not present
  - Extract to `$HOME/.local/go`
  - Export PATH to include `$HOME/.local/go/bin` for current session
  - Add PATH to `~/.profile` for future sessions (if not already present)
  - Add hint comment to `~/.bashrc` for humans who look there

- [ ] Call `ensure_go` at start of `start()` function (before any Go commands)

- [ ] Test installation flow on clean system (no Go installed)

- [ ] Test skip behavior when correct Go version already installed

- [ ] Test error handling when `go.mod` cannot be parsed

- [ ] Test that PATH persists in new login shells after installation