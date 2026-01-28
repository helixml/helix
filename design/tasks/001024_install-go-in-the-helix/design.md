# Design: Install Go in Helix Startup Script

## Summary

Add a function to the `./stack` script that installs Go (matching `go.mod`) if not already present.

## Approach

Use the official Go binary tarball installation method:

1. Extract required Go version from `go.mod` file
2. Check if that version is already installed via `go version`
3. If not, download the official tarball from `go.dev/dl/`
4. Extract to `$HOME/.local/go` (user-local, no sudo required)
5. Add to PATH via export (for current session)
6. Add to `~/.profile` (for future sessions - sourced by login shells)
7. Add a hint comment to `~/.bashrc` for humans who look there

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Installation location | `$HOME/.local/go` | User-local, doesn't require root |
| Version source | Extract from `go.mod` | Single source of truth, no hardcoding |
| When to run | During `./stack start` | Ensures Go available before build |
| PATH persistence | Add to `~/.profile` | Sourced by login shells; works for Ghostty/terminals |
| Human hint | Add comment to `~/.bashrc` | Developers often look there first |

## Implementation

Add a new function `ensure_go()` to the `stack` script:

```bash
function ensure_go() {
  local GO_INSTALL_DIR="$HOME/.local/go"
  local PATH_LINE='export PATH="$HOME/.local/go/bin:$PATH"'
  
  # Extract Go version from go.mod (e.g., "go 1.25.0" -> "1.25.0")
  local GO_VERSION
  GO_VERSION=$(grep -E "^go [0-9]" api/go.mod | awk '{print $2}')
  
  if [[ -z "$GO_VERSION" ]]; then
    echo "❌ Could not extract Go version from api/go.mod"
    return 1
  fi
  
  # Add to PATH for current session
  export PATH="$GO_INSTALL_DIR/bin:$PATH"
  
  # Check if correct version already installed
  if command -v go &>/dev/null && go version | grep -q "go${GO_VERSION}"; then
    return 0
  fi
  
  echo "🔄 Installing Go ${GO_VERSION}..."
  
  # Download and install
  mkdir -p "$HOME/.local"
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C "$HOME/.local" -xz
  
  # Add to ~/.profile for future sessions (if not already present)
  if ! grep -q '.local/go/bin' ~/.profile 2>/dev/null; then
    echo "" >> ~/.profile
    echo "# Go (installed by helix ./stack script)" >> ~/.profile
    echo "$PATH_LINE" >> ~/.profile
    echo "✅ Added Go to ~/.profile for future sessions"
  fi
  
  # Add hint to ~/.bashrc for humans who look there
  if ! grep -q 'Go PATH is in ~/.profile' ~/.bashrc 2>/dev/null; then
    echo "" >> ~/.bashrc
    echo "# Go PATH is in ~/.profile (sourced by login shells)" >> ~/.bashrc
  fi
  
  echo "✅ Go ${GO_VERSION} installed"
}
```

Call `ensure_go` early in the `start()` function, before any Go build commands.

## Risks

- **Network dependency**: Download requires internet access. Mitigation: check if Go exists first.
- **Architecture assumption**: Assumes `linux-amd64`. Could detect with `uname -m` if needed.
- **go.mod parse failure**: If `go.mod` format changes. Mitigation: clear error message.
- **Shell compatibility**: Only updates `~/.profile`. Users of zsh/fish would need to add PATH manually.