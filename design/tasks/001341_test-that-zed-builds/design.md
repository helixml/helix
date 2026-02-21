# Design: Test Zed Builds Correctly with Shared Cache

## Overview

Verify that Zed builds correctly using the Docker BuildKit cache mount architecture documented in `helix/Dockerfile.zed-build`. This is a validation task, not new feature development.

## Architecture

The existing build system uses BuildKit cache mounts:

```
./stack build-zed [dev|release]
    └── docker build --output
            └── Dockerfile.zed-build
                    ├── --mount=type=cache,target=/root/.cargo/registry
                    ├── --mount=type=cache,target=/root/.cargo/git
                    ├── --mount=type=cache,target=/root/.rustup
                    └── --mount=type=cache,target=/zed/target-ubuntu25-v2
```

## Key Decisions

1. **Use existing `./stack build-zed` command** — don't create new build infrastructure
2. **Test both dev and release modes** — release is used in CI/production, dev for local iteration
3. **Verify binary functionality** — not just that it compiles, but that it runs

## Verification Approach

1. Run `./stack build-zed release` and confirm exit code 0
2. Check `./zed-build/zed` exists and is executable
3. Run `./zed-build/zed --version` to confirm it's a working binary
4. Run build again to verify cache speeds up subsequent builds

## Constraints

- Build requires ~16GB RAM due to Rust compilation
- First build takes 15-20 minutes; cached builds take 2-5 minutes
- Must use Ubuntu 25.10 base image (matches runtime containers)
- Requires Docker with BuildKit support