# Design: Install Go in Helix Startup Script

## Summary

Add a function to the `./stack` script that installs Go (matching `go.mod`) if not already present.

## Approach

Use the official Go binary tarball installation method:

1. Extract required Go version from `go.mod` file
2. Check if that version is already installed via `go version`
3. If not, download the official tarball from `go.dev/dl/`
4. Extract to `$HOME/.local/go` (user-local, no sudo required)
5. Add to PATH via export

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Installation location | `$HOME/.local/go` | User-local, doesn't require root |
| Version source | Extract from `go.mod` | Single source of truth, no hardcoding |
| When to run | During `./stack start` | Ensures Go available before build |

## Implementation

Add a new function `ensure_go()` to the `stack` script:

```bash
function ensure_go() {
  local GO_INSTALL_DIR="$HOME/.local/go"
  
  # Extract Go version from go.mod (e.g., "go 1.25.0" -> "1.25.0")
  local GO_VERSION
  GO_VERSION=$(grep -E "^go [0-9]" api/go.mod | awk '{print $2}')
  
  if [[ -z "$GO_VERSION" ]]; then
    echo "❌ Could not extract Go version from api/go.mod"
    return 1
  fi
  
  # Check if correct version already installed
  if command -v go &>/dev/null && go version | grep -q "go${GO_VERSION}"; then
    return 0
  fi
  
  echo "🔄 Installing Go ${GO_VERSION}..."
  
  # Download and install
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C "$HOME/.local" -xz
  export PATH="$GO_INSTALL_DIR/bin:$PATH"
  
  echo "✅ Go ${GO_VERSION} installed"
}
```

Call `ensure_go` early in the `start()` function, before any Go build commands.

## Risks

- **Network dependency**: Download requires internet access. Mitigation: check if Go exists first.
- **Architecture assumption**: Assumes `linux-amd64`. Could detect with `uname -m` if needed.
- **go.mod parse failure**: If `go.mod` format changes. Mitigation: clear error message.