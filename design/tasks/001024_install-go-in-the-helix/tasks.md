# Implementation Tasks

- [ ] Add `ensure_go()` function to `helix/stack` script
  - Check if Go 1.25.x is already installed via `go version`
  - Download official Go 1.25.0 tarball from `go.dev/dl/` if not present
  - Extract to `$HOME/.local/go`
  - Export PATH to include `$HOME/.local/go/bin`

- [ ] Call `ensure_go` at start of `start()` function (before any Go commands)

- [ ] Test installation flow on clean system (no Go installed)

- [ ] Test skip behavior when Go 1.25 already installed