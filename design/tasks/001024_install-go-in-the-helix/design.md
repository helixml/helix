# Design: Install Go in Helix Desktop Container

## Summary

Install Go in the helix-ubuntu and helix-sway Dockerfiles so it's available for agents running inside the container.

## Approach

Add Go installation to the Dockerfiles using the official tarball method, extracting the version from `go.mod`.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Installation method | Dockerfile | Go needs to be available for agents running `bash -c` commands inside containers |
| Version source | Extract from `go.mod` at build time | Single source of truth |
| Installation location | `/usr/local/go` | Standard location, already in typical PATH |

## Implementation

Add to `Dockerfile.ubuntu-helix` and `Dockerfile.sway-helix`:

```dockerfile
# Install Go (version from go.mod)
COPY go.mod /tmp/go.mod
RUN GO_VERSION=$(grep -E "^go [0-9]" /tmp/go.mod | awk '{print $2}') && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz && \
    rm /tmp/go.mod
ENV PATH="/usr/local/go/bin:${PATH}"
```

## Why Not Stack Script?

Originally planned to install Go in `./stack start`, but:
- Agents run `bash -c "command"` which doesn't source `~/.profile` or `~/.bashrc`
- The stack script's PATH export doesn't propagate to agent subprocesses
- Dockerfile installation is simpler and more reliable for the Helix-in-Helix use case

## Implementation Notes

- Added Go installation after `_INSTALL_BASE` block in both Dockerfiles
- Go is installed to `/usr/local/go` (standard location)
- PATH is set via `ENV` directive so it's available to all processes including `bash -c`
- The `go.mod` is copied to `/tmp`, version extracted, then removed to keep image clean
- Installation is verified with `/usr/local/go/bin/go version` during build
- Files modified:
  - `Dockerfile.ubuntu-helix` (line ~237)
  - `Dockerfile.sway-helix` (line ~393)