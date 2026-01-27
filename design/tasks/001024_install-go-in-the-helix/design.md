# Design: Install Go in Helix Startup Script

## Summary

Add a function to the `./stack` script that installs Go 1.25 (matching `go.mod`) if not already present.

## Approach

Use the official Go binary tarball installation method:

1. Check if Go 1.25.x is already installed via `go version`
2. If not, download the official tarball from `go.dev/dl/`
3. Extract to `$HOME/.local/go` (user-local, no sudo required)
4. Add to PATH via export

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Installation location | `$HOME/.local/go` | User-local, doesn't require root |
| Version source | Hardcoded `1.25.0` | Match `go.mod` exactly |
| When to run | During `./stack start` | Ensures Go available before build |

## Implementation

Add a new function `ensure_go()` to the `stack` script:

```bash
function ensure_go() {
  local GO_VERSION="1.25.0"
  local GO_INSTALL_DIR="$HOME/.local/go"
  
  # Check if correct version already installed
  if command -v go &>/dev/null && go version | grep -q "go${GO_VERSION}"; then
    return 0
  fi
  
  # Download and install
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C "$HOME/.local" -xz
  export PATH="$GO_INSTALL_DIR/bin:$PATH"
}
```

Call `ensure_go` early in the `start()` function, before any Go build commands.

## Risks

- **Network dependency**: Download requires internet access. Mitigation: check if Go exists first.
- **Architecture assumption**: Assumes `linux-amd64`. Could detect with `uname -m` if needed.